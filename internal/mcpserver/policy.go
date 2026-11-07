package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"

	"github.com/zeroy1024/Burrow/internal/fileclient"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/shellcmd"
	"github.com/zeroy1024/Burrow/internal/store"
)

func ValidatePolicy(p store.MCPPolicy) error {
	if strings.TrimSpace(p.Name) == "" || len(p.Name) > 200 {
		return fail("invalid_argument", locales.MessageEnterAValidPolicyNameUpToBytes())
	}
	if !p.AllConnections && len(p.ConnectionIDs) == 0 {
		return fail("invalid_argument", locales.MessageSelectApplicableConnectionsOrUseAllConnections())
	}
	if len(p.ConnectionIDs) > 1000 {
		return fail("invalid_argument", locales.MessageAPolicyCanApplyToAtMostConnections())
	}
	if !p.AllKeys && len(p.KeyIDs) == 0 {
		return fail("invalid_argument", locales.MessageSelectApplicableKeys())
	}
	if p.Category == "ssh" {
		if len(p.CommandRules) == 0 || len(p.CommandRules) > 100 {
			return fail("invalid_argument", locales.MessageTheNumberOfCommandRulesMustBeBetweenAnd())
		}
		for _, r := range p.CommandRules {
			if r.Pattern == "" || len(r.Pattern) > 8192 {
				return fail("invalid_argument", locales.MessageTheCommandRuleIsEmptyOrTooLong())
			}
			if r.IgnoreCase && r.Mode != "command" {
				return fail("invalid_argument", locales.MessageOnlyCommandMatchingSupportsCaseInsensitiveArguments())
			}
			switch r.Mode {
			case "exact", "contains":
			case "command":
				if _, err := shellcmd.ParseRule(r.Pattern, r.IgnoreCase); err != nil {
					return fail("invalid_argument", err.Error())
				}
			case "regex":
				if _, err := regexp.Compile(r.Pattern); err != nil {
					return locales.Errorf(locales.MessageInvalidRegularExpression2, err)
				}
			default:
				return fail("invalid_argument", locales.MessageInvalidCommandMatchMode())
			}
		}
		return nil
	}
	if p.Category != "files" || len(p.FileRules) == 0 || len(p.FileRules) > 100 {
		return fail("invalid_argument", locales.MessageInvalidFilePolicyOrNumberOfRules())
	}
	for _, r := range p.FileRules {
		if r.Side != "local" && r.Side != "remote" || r.Kind != "file" && r.Kind != "dir" || r.Path == "" || len(r.Path) > 4096 || strings.ContainsRune(r.Path, 0) {
			return fail("invalid_argument", locales.MessageInvalidFilePathRule())
		}
		if r.Side == "remote" && !path.IsAbs(r.Path) {
			return fail("invalid_argument", locales.MessageRemoteRulesMustUseAbsolutePaths())
		}
		if r.Side == "local" {
			if _, err := LocalPath(r.Path); err != nil {
				return err
			}
		}
		for action := range r.DeniedActions {
			if !slices.Contains([]string{"browse", "read", "create", "write", "rename", "move", "delete"}, action) {
				return fail("invalid_argument", locales.MessageInvalidFileActionType())
			}
		}
	}
	return nil
}

func applies(p store.MCPPolicy, keyID, connectionID, category string, aliases ...string) bool {
	connectionMatches := p.AllConnections || slices.Contains(p.ConnectionIDs, connectionID)
	for _, alias := range aliases {
		connectionMatches = connectionMatches || alias != "" && slices.Contains(p.ConnectionIDs, alias)
	}
	return p.Enabled && p.Category == category && connectionMatches && (p.AllKeys || slices.Contains(p.KeyIDs, keyID))
}

func (s *Service) CheckCommand(ctx context.Context, keyID, connectionID, command string) error {
	policies, err := s.Store.MCPPolicies(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return s.storageFault(err)
	}
	var hits []store.MCPHit
	var matches []Match
	var reasons []string
	var analysis *shellcmd.Analysis
	for _, p := range policies {
		if !applies(p, keyID, connectionID, "ssh") {
			continue
		}
		for _, r := range p.CommandRules {
			rule := r.Mode + ": " + r.Pattern
			if r.IgnoreCase {
				rule += locales.MessageCaseInsensitiveArguments()
			}
			hit := store.MCPHit{PolicyID: p.ID, Name: p.Name, Version: p.Version,
				Category: p.Category, Rule: rule, Action: "exec"}
			reason := fmt.Sprintf(locales.MessagePolicyRule(), p.Name, rule)
			if r.Mode == "command" {
				matcher, err := shellcmd.ParseRule(r.Pattern, r.IgnoreCase)
				if err != nil {
					return fail("invalid_policy", locales.MessageTheSavedPolicyIsInvalidExecutionDenied())
				}
				// One parse serves every command rule of every policy.
				if analysis == nil {
					result := shellcmd.Analyze(command)
					analysis = &result
				}
				ev, ok := matcher.Match(*analysis)
				if !ok {
					continue
				}
				match := Match{Text: ev.Text, Via: strings.Join(ev.Via, " ▸ "), Fallback: ev.Fallback}
				if ev.Fallback != "" {
					reason += fmt.Sprintf(locales.MessageConservativelyMatchedByKeyword(), ev.Fallback, clip(ev.Text))
				} else {
					reason += fmt.Sprintf(locales.MessageMatchedCommand(), clip(ev.Text))
					if match.Via != "" {
						reason += locales.PolicyVia(match.Via)
					}
					if ev.Count > 1 {
						reason += locales.PolicyMatchCount(ev.Count)
					}
				}
				hits, matches, reasons = append(hits, hit), append(matches, match), append(reasons, reason)
				continue
			}
			start, end := -1, -1
			switch r.Mode {
			case "exact":
				if command == r.Pattern {
					start, end = 0, len(command)
				}
			case "contains":
				if start = strings.Index(command, r.Pattern); start >= 0 {
					end = start + len(r.Pattern)
				}
			case "regex":
				re, err := regexp.Compile(r.Pattern)
				if err != nil {
					return fail("invalid_policy", locales.MessageTheSavedPolicyIsInvalidExecutionDenied())
				}
				if span := re.FindStringIndex(command); span != nil {
					start, end = span[0], span[1]
				}
			}
			if start < 0 {
				continue
			}
			match := Match{Text: command[start:end], Context: commandWord(command, start, end)}
			switch {
			case r.Mode == "exact":
				reason += locales.MessageExactlyMatchesTheFullCommand()
			case match.Text == "":
				reason += fmt.Sprintf(locales.MessageMatchedEmptyTextAtByte(), start)
			case match.Context != match.Text:
				reason += fmt.Sprintf(locales.MessageMatchedIn(), clip(match.Text), clip(match.Context))
			default:
				reason += fmt.Sprintf(locales.MessageMatched(), clip(match.Text))
			}
			hits, matches, reasons = append(hits, hit), append(matches, match), append(reasons, reason)
		}
	}
	if len(hits) > 0 {
		return &Failure{Code: "intercepted", Message: locales.MessageCommandDeniedByInterceptionPolicy() + strings.Join(reasons, "；"), Hits: hits, Matches: matches}
	}
	return nil
}

// commandWord widens a match to the surrounding whitespace-delimited word so
// the agent can see why, e.g., "rm" matched inside "format". A word longer
// than the clip limit is not useful context and is omitted.
func commandWord(command string, start, end int) string {
	from := strings.LastIndexAny(command[:start], " \t\r\n") + 1
	to := len(command)
	if i := strings.IndexAny(command[end:], " \t\r\n"); i >= 0 {
		to = end + i
	}
	if to-from > clipBytes {
		return ""
	}
	return command[from:to]
}

const clipBytes = 120

// clip shortens text echoed in messages without splitting a UTF-8 sequence.
func clip(text string) string {
	if len(text) <= clipBytes {
		return text
	}
	cut := clipBytes
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "…"
}

func (s *Service) CheckFile(ctx context.Context, keyID, connectionID, side, name, action string, client fileclient.Client, aliases ...string) error {
	policies, err := s.Store.MCPPolicies(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return s.storageFault(err)
	}
	var hits []store.MCPHit
	var matches []Match
	var reasons []string
	for _, p := range policies {
		if !applies(p, keyID, connectionID, "files", aliases...) {
			continue
		}
		for _, r := range p.FileRules {
			if r.Side != side || !r.DeniedActions[action] {
				continue
			}
			canonical := canonicalLocal
			if side == "remote" {
				canonical = func(name string) (string, error) { return canonicalRemote(ctx, client, name) }
			}
			target, err := canonical(name)
			if err != nil {
				return fail("path_unverifiable", locales.MessageUnableToVerifyTheActualLocationOfTheProtectedPath())
			}
			protected, err := canonical(r.Path)
			if err != nil {
				return fail("path_unverifiable", locales.MessageUnableToVerifyTheActualPathOfTheInterceptionRule())
			}
			actual := target
			// Match conservatively on native platforms where case and Unicode
			// aliases are common. This only broadens a denial, never a grant.
			if side == "local" && (runtime.GOOS == "darwin" || runtime.GOOS == "windows") {
				target, protected = cases.Fold().String(norm.NFC.String(target)), cases.Fold().String(norm.NFC.String(protected))
			}
			matched := target == protected
			if r.Kind == "dir" && r.IncludeSubdirs {
				if side == "local" {
					matched = localWithin(protected, target)
				} else {
					matched = target == protected || strings.HasPrefix(target, strings.TrimSuffix(protected, "/")+"/")
				}
			}
			if matched {
				hit := store.MCPHit{PolicyID: p.ID, Name: p.Name, Version: p.Version,
					Category: p.Category, Rule: side + ": " + r.Path, Action: action}
				// Report the canonical path: it may differ from the requested
				// name when a symlink or alias led into the protected location.
				hits, matches = append(hits, hit), append(matches, Match{Text: actual})
				reasons = append(reasons, fmt.Sprintf(
					locales.MessagePolicyRuleBlocksActualPathIsProtected(),
					p.Name,
					hit.Rule,
					action,
					actual,
				))
			}
		}
	}
	if len(hits) > 0 {
		return &Failure{Code: "intercepted", Message: locales.MessageFileOperationDeniedByInterceptionPolicy() + strings.Join(reasons, "；"), Hits: hits, Matches: matches}
	}
	return nil
}

// Canonicalize the existing ancestor; a create target may not exist yet. A
// protocol unable to resolve directories cannot enforce an alias-aware rule.
func canonicalRemote(ctx context.Context, client fileclient.Client, name string) (string, error) {
	resolver, ok := client.(fileclient.CanonicalDirectoryResolver)
	if !ok {
		return "", errors.New("protocol cannot resolve paths")
	}
	name = path.Clean(name)
	entry, statErr := client.Stat(ctx, name)
	if statErr == nil {
		if entry.Symlink {
			return "", errors.New("symbolic links are not traversed")
		}
		if entry.Directory {
			return resolver.ResolveCanonicalDirectory(ctx, name)
		}
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return "", statErr
	}
	parent, suffix := path.Dir(name), path.Base(name)
	if name == "/" {
		parent, suffix = "/", ""
	}
	for {
		resolved, err := resolver.ResolveCanonicalDirectory(ctx, parent)
		if err == nil {
			if !path.IsAbs(resolved) {
				return "", errors.New("nonabsolute canonical path")
			}
			return path.Join(resolved, suffix), nil
		}
		if !errors.Is(err, fs.ErrNotExist) || parent == "/" {
			return "", err
		}
		suffix, parent = path.Join(path.Base(parent), suffix), path.Dir(parent)
	}
}

func canonicalLocal(name string) (string, error) {
	name, err := LocalPath(name)
	if err != nil {
		return "", err
	}
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(name)
		if err == nil {
			slices.Reverse(suffix)
			return filepath.Join(append([]string{resolved}, suffix...)...), nil
		}
		if !errors.Is(err, fs.ErrNotExist) || filepath.Dir(name) == name {
			return "", err
		}
		suffix = append(suffix, filepath.Base(name))
		name = filepath.Dir(name)
	}
}

func localWithin(root, name string) bool {
	rel, err := filepath.Rel(root, name)
	return err == nil && filepath.IsLocal(rel)
}

func AuthorizeLocal(key store.MCPKey, name string, download bool) (string, error) {
	name, err := LocalPath(name)
	if err != nil {
		return "", err
	}
	actual, err := canonicalLocal(name)
	if err != nil {
		return "", err
	}
	dirs := key.UploadDirs
	if download {
		dirs = key.DownloadDirs
	}
	for _, dir := range dirs {
		root, err := canonicalLocal(dir)
		if err != nil {
			return "", err
		}
		if localWithin(dir, name) && localWithin(root, actual) {
			// Refuse alias traversal: the existing transfer engine anchors its
			// selected source/destination with os.Root before authentication.
			if filepath.Clean(name) != actual {
				return "", fail("path_unverifiable", locales.MessageLocalTransferPathsCannotTraverseSymbolicLinks())
			}
			if _, err := os.Stat(dir); err != nil {
				return "", err
			}
			return name, nil
		}
	}
	return "", fail("permission_denied", locales.MessageTheLocalPathIsOutsideThisKeySAuthorizedDirectories())
}
