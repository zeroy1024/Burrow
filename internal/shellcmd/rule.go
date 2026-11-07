package shellcmd

import (
	"regexp"
	"slices"
	"strings"

	"github.com/zeroy1024/Burrow/internal/locales"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/pattern"
	"mvdan.cc/sh/v3/syntax"
)

// Rule is a command-mode pattern written as one plain command, such as
// "rm -rf /" or "git push --force". The program matches by basename; other
// words are conditions on the invocation's arguments:
//
//   - positional words appear in the same order, with other arguments between
//     them allowed; * and ? are wildcards
//   - --long matches --long or --long=value
//   - -abc matches the same word, or every letter among the invocation's
//     single-dash options, so -rf also matches -fr and -r -f
type Rule struct {
	program                 string
	positional, long, short []string
	ignoreCase              bool
	keywords                []*regexp.Regexp
}

// Evidence explains a match. Fallback is set when the rule matched keyword
// text because an invocation could not be resolved; Count is the number of
// distinct places matched.
type Evidence struct {
	Text, Fallback string
	Via            []string
	Count          int
}

// Keyword boundaries are word separators plus shell syntax, so "rm" matches
// in "CMD=rm;" and "/bin/rm" but not in "format" or "--rm".
const boundary = "[\\s;&|()<>`$'\"/\\\\={}]"

func ParseRule(pattern string, ignoreCase bool) (Rule, error) {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(pattern), "")
	if err != nil {
		return Rule{}, locales.Errorf(locales.MessageUnableToParseCommandRule, err)
	}
	if len(file.Stmts) != 1 {
		return Rule{}, locales.NewError(locales.MessageACommandRuleCanContainOnlyOneCommandSplitCombinedCommandsIntoSeparate)
	}
	stmt := file.Stmts[0]
	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok || stmt.Negated || stmt.Background || stmt.Coprocess || stmt.Disown || len(stmt.Redirs) > 0 ||
		len(call.Assigns) > 0 || len(call.Args) == 0 {
		return Rule{}, locales.NewError(locales.MessageACommandRuleMustBeASingleSimpleCommandWithoutPipesRedirectionsVariable)
	}
	var tokens []string
	for _, w := range call.Args {
		if dynamic(w) {
			return Rule{}, locales.NewError(
				locales.MessageCommandRulesCannotContainVariablesCommandSubstitutionsOrArithmeticExpansions,
			)
		}
		fields, err := expand.Fields(nil, w)
		if err != nil {
			return Rule{}, locales.Errorf(locales.MessageUnableToParseCommandRule, err)
		}
		tokens = append(tokens, fields...)
	}
	r := Rule{program: normalize(tokens[0]), ignoreCase: ignoreCase}
	if r.program == "" {
		return Rule{}, locales.NewError(locales.MessageTheCommandRuleIsMissingACommandName)
	}
	r.keywords = append(r.keywords, keyword(r.program, true))
	optionsDone := false
	for _, token := range tokens[1:] {
		if !optionsDone && token == "--" {
			optionsDone = true
			continue
		}
		if ignoreCase {
			token = strings.ToLower(token)
		}
		switch {
		case !optionsDone && len(token) > 2 && strings.HasPrefix(token, "--"):
			r.long = append(r.long, token)
		case !optionsDone && len(token) > 1 && token[0] == '-' && token[1] != '-':
			r.short = append(r.short, token)
		default:
			r.positional = append(r.positional, token)
		}
		r.keywords = append(r.keywords, keyword(token, ignoreCase))
	}
	return r, nil
}

func keyword(token string, ignoreCase bool) *regexp.Regexp {
	quoted := regexp.QuoteMeta(token)
	quoted = strings.NewReplacer(`\*`, `\S*`, `\?`, `\S`).Replace(quoted)
	flags := ""
	if ignoreCase {
		flags = "(?i)"
	}
	return regexp.MustCompile(flags + "(?:^|" + boundary + ")" + quoted + "(?:$|" + boundary + ")")
}

// Match reports the first invocation the rule matches. Without one, uncertain
// text matches when every rule word appears in it as a keyword.
func (r Rule) Match(a Analysis) (Evidence, bool) {
	var ev Evidence
	var seen []string
	for _, call := range a.Calls {
		if !r.matchCall(call) {
			continue
		}
		if ev.Count == 0 {
			ev.Text, ev.Via = call.Text, call.Via
		}
		if !slices.Contains(seen, call.Text) {
			seen = append(seen, call.Text)
			ev.Count++
		}
	}
	if ev.Count > 0 {
		return ev, true
	}
	for _, u := range a.Uncertain {
		if r.matchKeywords(u.Text) {
			return Evidence{Text: u.Text, Fallback: u.Reason, Via: u.Via, Count: 1}, true
		}
	}
	return Evidence{}, false
}

func (r Rule) matchKeywords(text string) bool {
	for _, k := range r.keywords {
		if !k.MatchString(text) {
			return false
		}
	}
	return true
}

func (r Rule) matchCall(call Call) bool {
	if !r.matchProgram(call) {
		return false
	}
	var values []string
	var options []string
	letters := map[rune]bool{}
	optionsDone := false
	for _, arg := range call.Args[1:] {
		// An expansion's value is unknown here; it does not satisfy a condition.
		if arg.Dynamic {
			continue
		}
		v := arg.Value
		if !optionsDone && v == "--" {
			optionsDone = true
			continue
		}
		if r.ignoreCase {
			v = strings.ToLower(v)
		}
		values = append(values, v)
		if optionsDone {
			continue
		}
		options = append(options, v)
		if shortCluster(v) {
			for _, letter := range v[1:] {
				letters[letter] = true
			}
		}
	}
	next := 0
	for _, v := range values {
		if next < len(r.positional) && wildcard(r.positional[next], v) {
			next++
		}
	}
	if next < len(r.positional) {
		return false
	}
	for _, long := range r.long {
		if !slices.ContainsFunc(options, func(v string) bool { return v == long || strings.HasPrefix(v, long+"=") }) {
			return false
		}
	}
	for _, short := range r.short {
		if slices.Contains(options, short) {
			continue
		}
		if !shortCluster(short) {
			return false
		}
		for _, letter := range short[1:] {
			if !letters[letter] {
				return false
			}
		}
	}
	return true
}

// matchProgram compares a normalized program name. A command name that is
// itself a glob (/bin/r?) matches when it could expand to the rule's program.
func (r Rule) matchProgram(call Call) bool {
	if slices.ContainsFunc(call.names, func(name string) bool { return wildcard(r.program, name) }) {
		return true
	}
	// Only the request's unquoted metacharacters can expand on the remote
	// shell. Rule wildcards are handled separately by the literal comparison.
	glob := strings.ToLower(call.Args[0].glob)
	if i := strings.LastIndexByte(glob, '/'); i >= 0 {
		glob = glob[i+1:]
	}
	glob = strings.TrimSuffix(glob, ".exe")
	if !pattern.HasMeta(glob, 0) {
		return false
	}
	expr, err := pattern.Regexp(glob, pattern.EntireString|pattern.NoGlobStar)
	if err != nil {
		// An invalid glob remains literal, as with the shell's '[' command.
		return false
	}
	return regexp.MustCompile(expr).MatchString(r.program)
}

func shortCluster(v string) bool {
	if len(v) < 2 || v[0] != '-' || v[1] == '-' {
		return false
	}
	for _, c := range v[1:] {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// wildcard matches * (any run of characters, including /) and ? (one
// character); everything else is literal.
func wildcard(pattern, s string) bool {
	p, t := []rune(pattern), []rune(s)
	i, j, star, mark := 0, 0, -1, 0
	for j < len(t) {
		switch {
		case i < len(p) && (p[i] == '?' || p[i] == t[j]):
			i, j = i+1, j+1
		case i < len(p) && p[i] == '*':
			star, mark = i, j
			i++
		case star >= 0:
			mark++
			i, j = star+1, mark
		default:
			return false
		}
	}
	for i < len(p) && p[i] == '*' {
		i++
	}
	return i == len(p)
}
