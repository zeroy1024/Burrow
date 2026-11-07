// Package shellcmd finds the programs a shell command line would run, so MCP
// command rules can match invocations instead of raw text. It guards against
// mistakes, not adversaries: anything that cannot be resolved statically is
// reported as uncertain, and callers fall back to conservative keyword matching.
package shellcmd

import (
	"slices"
	"strings"

	"sshdesk/internal/locales"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

// Budgets bound the work for one command line. Hitting one makes the affected
// text uncertain instead of silently skipping it.
const (
	maxDepth   = 8
	maxCalls   = 4096
	maxScripts = 256
)

// Arg is one argument after quote removal and brace expansion.
type Arg struct {
	Value string
	// Dynamic marks parameter, command or arithmetic expansion: the value is
	// only known on the remote host.
	Dynamic bool
	raw     string
	// glob keeps shell quoting and escapes, independently of the literal value.
	glob string
	// tail is the literal basename after the last '/' of a dynamic word, as in
	// "$HOME/bin/tool", which still names the program.
	tail string
}

// Call is one program invocation.
type Call struct {
	Args  []Arg
	names []string
	// Text is the simple command as written in its script; Via lists the
	// launchers it ran through, outermost first.
	Text string
	Via  []string
}

// Uncertain is text whose invocations could not be determined.
type Uncertain struct {
	Text, Reason string
	Via          []string
}

type Analysis struct {
	Calls     []Call
	Uncertain []Uncertain
}

func Analyze(command string) Analysis {
	a := analyzer{parsed: map[string]bool{}}
	a.script(command, nil, 0)
	return a.out
}

type analyzer struct {
	out     Analysis
	parsed  map[string]bool
	scripts int
}

// input is text a command reads on stdin. Shells run it as a script; when it
// carries unexpanded parts it is only usable for keyword matching.
type input struct {
	text  string
	parse bool
}

func (a *analyzer) uncertain(text, reason string, via []string) {
	for _, u := range a.out.Uncertain {
		if u.Text == text && u.Reason == reason {
			return
		}
	}
	a.out.Uncertain = append(a.out.Uncertain, Uncertain{Text: text, Reason: reason, Via: slices.Clone(via)})
}

func (a *analyzer) script(src string, via []string, depth int) {
	if strings.TrimSpace(src) == "" || a.parsed[src] {
		return
	}
	if depth > maxDepth || a.scripts >= maxScripts {
		a.uncertain(src, locales.MessageNestingDepthOrParsingVolumeExceedsTheLimit(), via)
		return
	}
	a.parsed[src] = true
	a.scripts++
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(src), "")
	if err != nil {
		a.uncertain(src, locales.MessageUnableToFullyParseTheCommand()+err.Error(), via)
		return
	}
	// Collect before expanding: expansion may rewrite brace words into nodes
	// that syntax.Walk does not traverse.
	type pipe struct{ from, to *syntax.Stmt }
	var stmts []*syntax.Stmt
	var pipes []pipe
	syntax.Walk(file, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.Stmt:
			if _, ok := n.Cmd.(*syntax.CallExpr); ok {
				stmts = append(stmts, n)
			}
		case *syntax.BinaryCmd:
			if n.Op == syntax.Pipe || n.Op == syntax.PipeAll {
				pipes = append(pipes, pipe{n.X, n.Y})
			}
		}
		return true
	})
	feeds := map[*syntax.Stmt][]input{}
	for _, p := range pipes {
		feeds[p.to] = append(feeds[p.to], a.output(src, p.from)...)
	}
	for _, stmt := range stmts {
		call := stmt.Cmd.(*syntax.CallExpr)
		if len(call.Args) == 0 {
			continue
		}
		stdin := append(redirectInput(src, stmt), feeds[stmt]...)
		a.invoke(words(src, call.Args), frame{text: span(src, call), script: src, stdin: stdin, via: via, depth: depth})
	}
}

// frame is where an invocation sits: its simple command, the enclosing script
// (the keyword-fallback text), stdin, the launcher chain and nesting depth.
type frame struct {
	text, script string
	stdin        []input
	via          []string
	depth        int
}

func (f frame) through(label string) frame {
	f.via = append(slices.Clip(f.via), label)
	return f
}

func (a *analyzer) invoke(args []Arg, f frame) {
	if len(args) == 0 {
		return
	}
	program := args[0]
	if program.Dynamic && program.tail == "" {
		a.uncertain(f.script, locales.MessageTheCommandNameCannotBeDeterminedStatically(), f.via)
		return
	}
	if len(a.out.Calls) >= maxCalls {
		a.uncertain(f.script, locales.MessageTheNumberOfCommandsExceedsTheLimit(), f.via)
		return
	}
	names := programNames(program)
	a.out.Calls = append(a.out.Calls, Call{Args: args, names: names, Text: f.text, Via: slices.Clone(f.via)})
	a.launch(names[0], args, f)
}

// nested runs a command string given as an argument, such as bash -c's. The
// string is unknown when it carries expansions, so the whole enclosing script
// becomes keyword-fallback text.
func (a *analyzer) nested(arg Arg, reason string, f frame) {
	if arg.Dynamic {
		a.uncertain(f.script, reason, f.via)
		return
	}
	a.script(arg.Value, f.via, f.depth+1)
}

func (a *analyzer) stdinScripts(f frame) {
	f = f.through(locales.MessageStandardInput())
	for _, in := range f.stdin {
		if in.parse {
			a.script(in.text, f.via, f.depth+1)
		} else {
			a.uncertain(f.script, locales.MessageStandardInputContainsDynamicContent(), f.via)
		}
	}
}

// output returns what a pipeline producer writes when it is fixed text:
// echo/printf arguments, or the here-document cat copies.
func (a *analyzer) output(src string, stmt *syntax.Stmt) []input {
	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Args) == 0 {
		return nil
	}
	args := words(src, call.Args)
	if args[0].Dynamic {
		return nil
	}
	rest, separator := args[1:], " "
	switch programNames(args[0])[0] {
	case "echo":
		for len(rest) > 0 && !rest[0].Dynamic && isEchoOption(rest[0].Value) {
			rest = rest[1:]
		}
	case "printf":
		separator = "\n"
	case "cat":
		if len(rest) == 0 {
			return redirectInput(src, stmt)
		}
		return nil
	default:
		return nil
	}
	values := make([]string, len(rest))
	for i, arg := range rest {
		if arg.Dynamic {
			return []input{{text: span(src, call)}}
		}
		values[i] = arg.Value
	}
	return []input{{text: strings.Join(values, separator), parse: true}}
}

func isEchoOption(v string) bool {
	return len(v) > 1 && v[0] == '-' && strings.Trim(v[1:], "neE") == ""
}

func redirectInput(src string, stmt *syntax.Stmt) []input {
	var out []input
	for _, r := range stmt.Redirs {
		switch r.Op {
		case syntax.Hdoc, syntax.DashHdoc:
			if r.Hdoc == nil {
				continue
			}
			if !dynamic(r.Hdoc) {
				if text, err := expand.Literal(nil, r.Hdoc); err == nil {
					out = append(out, input{text: text, parse: true})
					continue
				}
			}
			// Expansions in the body still run as written; drop the
			// delimiter line that ends the span.
			body := span(src, r.Hdoc)
			if i := strings.LastIndexByte(body, '\n'); i >= 0 {
				out = append(out, input{text: body[:i], parse: true})
			}
		case syntax.WordHdoc:
			if dynamic(r.Word) {
				out = append(out, input{text: span(src, r.Word)})
				continue
			}
			if text, err := expand.Literal(nil, r.Word); err == nil {
				out = append(out, input{text: text, parse: true})
			}
		}
	}
	return out
}

func words(src string, list []*syntax.Word) []Arg {
	var args []Arg
	for _, w := range list {
		raw := span(src, w)
		if dynamic(w) {
			tail, glob := literalTail(w)
			args = append(args, Arg{Dynamic: true, raw: raw, tail: tail, glob: glob})
			continue
		}
		// Expand braces before removing quotes so each resulting argument keeps
		// its own glob pattern, including mixed quoted and unquoted characters.
		word := *w
		syntax.SplitBraces(&word)
		start := len(args)
		for field, err := range expand.BracesSeq(nil, &word) {
			if err != nil {
				args = append(args[:start], Arg{Dynamic: true, raw: raw})
				break
			}
			arg, err := staticArg(field, raw)
			if err != nil {
				args = append(args[:start], Arg{Dynamic: true, raw: raw})
				break
			}
			args = append(args, arg)
		}
	}
	return args
}

func staticArg(w *syntax.Word, raw string) (Arg, error) {
	value, err := expand.Literal(nil, w)
	if err != nil {
		return Arg{}, err
	}
	glob, err := expand.Pattern(nil, w)
	if err != nil {
		return Arg{}, err
	}
	return Arg{Value: value, raw: raw, glob: glob}, nil
}

func dynamic(w *syntax.Word) bool {
	found := false
	syntax.Walk(w, func(node syntax.Node) bool {
		switch node.(type) {
		case *syntax.ParamExp, *syntax.CmdSubst, *syntax.ArithmExp, *syntax.ProcSubst:
			found = true
		}
		return !found
	})
	return found
}

func literalTail(w *syntax.Word) (string, string) {
	parts := w.Parts
	if len(parts) == 0 {
		return "", ""
	}
	quoted, ok := parts[len(parts)-1].(*syntax.DblQuoted)
	if ok {
		parts = quoted.Parts
	}
	if len(parts) < 2 {
		return "", ""
	}
	lit, ok := parts[len(parts)-1].(*syntax.Lit)
	if !ok {
		return "", ""
	}
	i := strings.LastIndexAny(lit.Value, `/\`)
	if i < 0 || i == len(lit.Value)-1 {
		return "", ""
	}
	tail := *lit
	tail.Value = lit.Value[i+1:]
	word := &syntax.Word{Parts: []syntax.WordPart{&tail}}
	if quoted != nil {
		word.Parts = []syntax.WordPart{&syntax.DblQuoted{Parts: word.Parts}}
	}
	arg, err := staticArg(word, "")
	if err != nil {
		return "", ""
	}
	return arg.Value, arg.glob
}

func span(src string, node syntax.Node) string {
	return src[node.Pos().Offset():node.End().Offset()]
}

// programNames normalizes a command word: basename, lower case (case-folding
// file systems run RM as rm) and no .exe. The raw spelling is kept as a second
// candidate because shell quoting removes Windows path backslashes.
func programNames(arg Arg) []string {
	value := arg.Value
	if arg.Dynamic {
		value = arg.tail
	}
	names := []string{normalize(value)}
	if raw := strings.Trim(arg.raw, `"'`); !arg.Dynamic && strings.ContainsRune(raw, '\\') {
		if name := normalize(raw); name != names[0] {
			names = append(names, name)
		}
	}
	return names
}

func normalize(name string) string {
	name = strings.ToLower(name)
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	return strings.TrimSuffix(name, ".exe")
}
