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

	"sshdesk/internal/fileclient"
	"sshdesk/internal/shellcmd"
	"sshdesk/internal/store"
)

func ValidatePolicy(p store.MCPPolicy) error {
	if strings.TrimSpace(p.Name) == "" || len(p.Name) > 200 {
		return fail("invalid_argument", "请填写有效的策略名称（最多 200 字节）")
	}
	if !p.AllConnections && len(p.ConnectionIDs) == 0 {
		return fail("invalid_argument", "请选择适用连接或使用全部连接")
	}
	if len(p.ConnectionIDs) > 1000 {
		return fail("invalid_argument", "适用连接最多 1000 个")
	}
	if !p.AllKeys && len(p.KeyIDs) == 0 {
		return fail("invalid_argument", "请选择适用密钥")
	}
	if p.Category == "ssh" {
		if len(p.CommandRules) == 0 || len(p.CommandRules) > 100 {
			return fail("invalid_argument", "命令规则数量必须为 1 至 100")
		}
		for _, r := range p.CommandRules {
			if r.Pattern == "" || len(r.Pattern) > 8192 {
				return fail("invalid_argument", "命令规则为空或超长")
			}
			if r.IgnoreCase && r.Mode != "command" {
				return fail("invalid_argument", "仅命令匹配支持忽略参数大小写")
			}
			switch r.Mode {
			case "exact", "contains":
			case "command":
				if _, err := shellcmd.ParseRule(r.Pattern, r.IgnoreCase); err != nil {
					return fail("invalid_argument", err.Error())
				}
			case "regex":
				if _, err := regexp.Compile(r.Pattern); err != nil {
					return fmt.Errorf("正则表达式无效：%w", err)
				}
			default:
				return fail("invalid_argument", "无效的命令匹配类型")
			}
		}
		return nil
	}
	if p.Category != "files" || len(p.FileRules) == 0 || len(p.FileRules) > 100 {
		return fail("invalid_argument", "无效的文件策略或规则数量")
	}
	for _, r := range p.FileRules {
		if r.Side != "local" && r.Side != "remote" || r.Kind != "file" && r.Kind != "dir" || r.Path == "" || len(r.Path) > 4096 || strings.ContainsRune(r.Path, 0) {
			return fail("invalid_argument", "无效的文件路径规则")
		}
		if r.Side == "remote" && !path.IsAbs(r.Path) {
			return fail("invalid_argument", "远端规则必须使用绝对路径")
		}
		if r.Side == "local" {
			if _, err := LocalPath(r.Path); err != nil {
				return err
			}
		}
		for action := range r.DeniedActions {
			if !slices.Contains([]string{"browse", "read", "create", "write", "rename", "move", "delete"}, action) {
				return fail("invalid_argument", "无效的文件操作类型")
			}
		}
	}
	return nil
}

func applies(p store.MCPPolicy, keyID, connectionID, category string) bool {
	return p.Enabled && p.Category == category && (p.AllConnections || slices.Contains(p.ConnectionIDs, connectionID)) &&
		(p.AllKeys || slices.Contains(p.KeyIDs, keyID))
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
				rule += "（参数忽略大小写）"
			}
			hit := store.MCPHit{PolicyID: p.ID, Name: p.Name, Version: p.Version,
				Category: p.Category, Rule: rule, Action: "exec"}
			reason := fmt.Sprintf("策略「%s」的规则 %s", p.Name, rule)
			if r.Mode == "command" {
				matcher, err := shellcmd.ParseRule(r.Pattern, r.IgnoreCase)
				if err != nil {
					return fail("invalid_policy", "已保存的策略无效，拒绝执行")
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
					reason += fmt.Sprintf(" 按关键词从严命中（%s）：%q", ev.Fallback, clip(ev.Text))
				} else {
					reason += fmt.Sprintf(" 命中命令 %q", clip(ev.Text))
					if match.Via != "" {
						reason += "（经由 " + match.Via + "）"
					}
					if ev.Count > 1 {
						reason += fmt.Sprintf("，共 %d 处", ev.Count)
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
					return fail("invalid_policy", "已保存的策略无效，拒绝执行")
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
				reason += " 与整条命令完全一致"
			case match.Text == "":
				reason += fmt.Sprintf(" 在第 %d 字节处匹配到空文本", start)
			case match.Context != match.Text:
				reason += fmt.Sprintf(" 命中 %q（位于 %q）", clip(match.Text), clip(match.Context))
			default:
				reason += fmt.Sprintf(" 命中 %q", clip(match.Text))
			}
			hits, matches, reasons = append(hits, hit), append(matches, match), append(reasons, reason)
		}
	}
	if len(hits) > 0 {
		return &Failure{Code: "intercepted", Message: "命令被拦截策略拒绝：" + strings.Join(reasons, "；"), Hits: hits, Matches: matches}
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

func (s *Service) CheckFile(ctx context.Context, keyID, connectionID, side, name, action string, client fileclient.Client) error {
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
		if !applies(p, keyID, connectionID, "files") {
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
				return fail("path_unverifiable", "无法核实被保护路径的实际位置")
			}
			protected, err := canonical(r.Path)
			if err != nil {
				return fail("path_unverifiable", "无法核实拦截规则的实际路径")
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
				reasons = append(reasons, fmt.Sprintf("策略「%s」的规则 %s 禁止 %s，实际路径 %q 受保护", p.Name, hit.Rule, action, actual))
			}
		}
	}
	if len(hits) > 0 {
		return &Failure{Code: "intercepted", Message: "文件操作被拦截策略拒绝：" + strings.Join(reasons, "；"), Hits: hits, Matches: matches}
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
				return "", fail("path_unverifiable", "本地传输路径不能经过符号链接")
			}
			if _, err := os.Stat(dir); err != nil {
				return "", err
			}
			return name, nil
		}
	}
	return "", fail("permission_denied", "本地路径不在该密钥授权目录内")
}
