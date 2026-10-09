package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/term"

	"prtool/internal/backend"
	"prtool/internal/diffview"
	"prtool/internal/fields"
	"prtool/internal/render"
	"prtool/internal/style"
)

// Version is the string printed by --version.
const Version = "1.0.0"

// IO is the process boundary. Tests replace every field. Nil Env is empty,
// so a test cannot accidentally inherit NO_COLOR from the machine.
type IO struct {
	Out    io.Writer
	Err    io.Writer
	Env    []string
	Runner backend.Runner
	TTY    bool
	Width  int
	Height int
	Page   func(string) error
	Remote func() (string, error)
}

// Main runs prtool with the real terminal, environment, and gh/glab.
func Main() int {
	return Run(os.Args[1:], realIO())
}

func realIO() IO {
	opts := IO{
		Out:    os.Stdout,
		Err:    os.Stderr,
		Env:    os.Environ(),
		Runner: backend.Exec{},
		Remote: gitOrigin,
		Page:   runPager,
	}
	fd := int(os.Stdout.Fd())
	opts.TTY = term.IsTerminal(fd)
	if opts.TTY {
		w, h, err := term.GetSize(fd)
		if err == nil {
			opts.Width = w
			opts.Height = h
		}
	}
	return opts
}

func gitOrigin() (string, error) {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// runPager sends content to less -FRX, or to PRTOOL_PAGER split on spaces.
// A missing pager (*exec.Error) is the caller's signal to print the text.
func runPager(content string) error {
	argv := []string{"less", "-FRX"}
	if v, ok := style.LookupEnv(os.Environ(), "PRTOOL_PAGER"); ok && v != "" {
		argv = strings.Fields(v)
	}
	if len(argv) == 0 {
		return nil
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = strings.NewReader(content)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Run executes one invocation. args is the command line after the program name.
func Run(args []string, opts IO) int {
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	if opts.Err == nil {
		opts.Err = io.Discard
	}
	if opts.Runner == nil {
		opts.Runner = backend.Exec{}
	}
	if len(args) == 0 {
		writeHelp(opts.Out, "")
		return 0
	}
	help, version, cmd := peekMeta(args)
	if help {
		writeHelp(opts.Out, cmd)
		return 0
	}
	if version {
		fmt.Fprintf(opts.Out, "prtool %s\n", Version)
		return 0
	}
	act, err := parseArgs(args)
	if err != nil {
		return usage(opts, err)
	}
	switch act.cmd {
	case "view":
		return cmdView(act, opts)
	case "list", "ls":
		return cmdList(act, opts)
	case "diff":
		return cmdDiff(act, opts)
	case "fields":
		return cmdFields(act, opts)
	default:
		return usage(opts, fmt.Errorf("unknown command %q", act.cmd))
	}
}

func writeHelp(w io.Writer, cmd string) {
	s := Help(cmd)
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	fmt.Fprint(w, s)
}

func usage(opts IO, err error) int {
	fmt.Fprintf(opts.Err, "prtool: %s\n", err)
	return 1
}

func fail(opts IO, err error) int {
	var ee *backend.ExitError
	if errors.As(err, &ee) {
		if ee.Msg != "" {
			fmt.Fprintf(opts.Err, "%s\n", ee.Msg)
		}
		if ee.Code <= 0 {
			return 1
		}
		return ee.Code
	}
	fmt.Fprintf(opts.Err, "prtool: %s\n", err)
	return 1
}

const (
	kindBool = iota + 1
	kindValue
)

// flagKind is every flag prtool accepts. Value flags consume the next token,
// so "--fields --help" is a field list, not a help request.
var flagKind = map[string]int{
	"fields":       kindValue,
	"repo":         kindValue,
	"host":         kindValue,
	"state":        kindValue,
	"limit":        kindValue,
	"author":       kindValue,
	"search":       kindValue,
	"base":         kindValue,
	"head":         kindValue,
	"assignee":     kindValue,
	"reviewer":     kindValue,
	"exclude":      kindValue,
	"label":        kindValue,
	"colors":       kindBool,
	"bold":         kindBool,
	"no-pager":     kindBool,
	"stat":         kindBool,
	"unified":      kindBool,
	"side-by-side": kindBool,
	"name-only":    kindBool,
	"raw":          kindBool,
	"draft":        kindBool,
	"not-draft":    kindBool,
	"help":         kindBool,
	"version":      kindBool,
}

func shortName(b byte) (string, bool) {
	switch b {
	case 'R':
		return "repo", true
	case 'L':
		return "limit", true
	case 'l':
		return "label", true
	case 'd':
		return "draft", true
	case 'h':
		return "help", true
	default:
		return "", false
	}
}

func isCommand(s string) bool {
	switch s {
	case "view", "list", "ls", "diff", "fields":
		return true
	default:
		return false
	}
}

// softFlag identifies a flag token without rejecting unknown names.
// hasValue is true when the value is attached (--flag=value or -Rowner).
func softFlag(tok string) (name, value string, hasValue bool) {
	if strings.HasPrefix(tok, "--") {
		body := tok[2:]
		if i := strings.IndexByte(body, '='); i >= 0 {
			return body[:i], body[i+1:], true
		}
		return body, "", false
	}
	if len(tok) >= 2 {
		if n, ok := shortName(tok[1]); ok {
			if len(tok) > 2 && flagKind[n] == kindValue {
				return n, tok[2:], true
			}
			if len(tok) == 2 {
				return n, "", false
			}
		}
	}
	return "", "", false
}

// peekMeta finds help, version, and the command before strict errors.
// A flag after "--" is a positional, so it is not help.
func peekMeta(args []string) (help, version bool, cmd string) {
	end := false
	need := false
	for _, a := range args {
		if need {
			need = false
			continue
		}
		if !end && a == "--" {
			end = true
			continue
		}
		if !end && len(a) > 1 && strings.HasPrefix(a, "-") {
			name, _, hasValue := softFlag(a)
			if name == "help" {
				help = true
			}
			if name == "version" {
				version = true
			}
			if !hasValue && flagKind[name] == kindValue {
				need = true
			}
			continue
		}
		if cmd == "" && isCommand(a) {
			cmd = a
		}
	}
	return help, version, cmd
}

type action struct {
	cmd       string
	noun      string
	ref       string
	fields    string
	fieldsSet bool
	repo      string
	host      string
	state     string
	stateSet  bool
	limit     int
	limitSet  bool
	limitStr  string
	author    string
	search    string
	base      string
	head      string
	assignee  string
	reviewer  string
	labels    []string
	exclude   []string
	draft     string
	draftYes  bool
	draftNo   bool
	colors    bool
	bold      bool
	noPager   bool
	stat      bool
	unified   bool
	side      bool
	nameOnly  bool
	raw       bool
	seen      []string
}

func parseFlagToken(tok string) (name, value string, hasValue bool, err error) {
	if strings.HasPrefix(tok, "--") {
		body := tok[2:]
		if body == "" {
			return "", "", false, fmt.Errorf("unknown flag %s", tok)
		}
		name = body
		if i := strings.IndexByte(body, '='); i >= 0 {
			name = body[:i]
			value = body[i+1:]
			hasValue = true
		}
		if _, ok := flagKind[name]; !ok {
			return "", "", false, fmt.Errorf("unknown flag --%s", name)
		}
		return name, value, hasValue, nil
	}
	if len(tok) < 2 || tok[0] != '-' {
		return "", "", false, fmt.Errorf("unknown flag %s", tok)
	}
	n, ok := shortName(tok[1])
	if !ok || (len(tok) > 2 && flagKind[n] != kindValue) {
		return "", "", false, fmt.Errorf("unknown flag %s", tok)
	}
	if len(tok) > 2 {
		return n, tok[2:], true, nil
	}
	return n, "", false, nil
}

func parseArgs(args []string) (action, error) {
	var act action
	var pos []string
	end := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !end && a == "--" {
			end = true
			continue
		}
		if !end && len(a) > 1 && strings.HasPrefix(a, "-") {
			name, value, hasValue, err := parseFlagToken(a)
			if err != nil {
				return action{}, err
			}
			kind := flagKind[name]
			if kind == kindBool && hasValue {
				return action{}, fmt.Errorf("--%s does not take a value", name)
			}
			if kind == kindValue && !hasValue {
				if i+1 >= len(args) || args[i+1] == "--" {
					return action{}, fmt.Errorf("--%s requires a value", name)
				}
				i++
				value = args[i]
			}
			if name == "help" || name == "version" {
				continue
			}
			if err := act.apply(name, value); err != nil {
				return action{}, err
			}
			act.seen = append(act.seen, name)
			continue
		}
		if act.cmd == "" {
			switch a {
			case "pr", "mr":
				if act.noun != "" && act.noun != a {
					return action{}, fmt.Errorf("cannot combine pr and mr")
				}
				act.noun = a
			case "view", "list", "ls", "diff", "fields":
				act.cmd = a
			default:
				return action{}, fmt.Errorf("unknown command %q", a)
			}
			continue
		}
		pos = append(pos, a)
	}
	if act.cmd == "" {
		return action{}, fmt.Errorf("missing command")
	}
	allowed := allowedFlags(act.cmd)
	for _, name := range act.seen {
		if !allowed[name] {
			return action{}, fmt.Errorf("flag --%s is not valid for %s", name, act.cmd)
		}
	}
	if act.draftYes && act.draftNo {
		return action{}, fmt.Errorf("--draft and --not-draft cannot be used together")
	}
	if act.draftYes {
		act.draft = "yes"
	}
	if act.draftNo {
		act.draft = "no"
	}
	if act.side && act.unified {
		return action{}, fmt.Errorf("--side-by-side and --unified cannot be used together")
	}
	if act.stat && act.nameOnly {
		return action{}, fmt.Errorf("--stat and --name-only cannot be used together")
	}
	if act.raw && act.stat {
		return action{}, fmt.Errorf("--raw cannot be used with --stat")
	}
	if act.raw && act.nameOnly {
		return action{}, fmt.Errorf("--raw cannot be used with --name-only")
	}
	if act.raw && act.side {
		return action{}, fmt.Errorf("--raw cannot be used with --side-by-side")
	}
	if act.stateSet {
		switch strings.ToLower(act.state) {
		case "open", "closed", "merged", "all":
			act.state = strings.ToLower(act.state)
		default:
			return action{}, fmt.Errorf("invalid --state %q (use open, closed, merged, or all)", act.state)
		}
	}
	if act.limitSet {
		n, err := strconv.Atoi(act.limitStr)
		if err != nil || n <= 0 {
			return action{}, fmt.Errorf("invalid --limit %q (want a number greater than 0)", act.limitStr)
		}
		act.limit = n
	}
	switch act.cmd {
	case "view", "diff":
		if len(pos) > 1 {
			return action{}, fmt.Errorf("too many arguments")
		}
		if len(pos) == 1 {
			act.ref = pos[0]
		}
	case "list", "ls", "fields":
		if len(pos) > 0 {
			return action{}, fmt.Errorf("unexpected argument %q", pos[0])
		}
	}
	return act, nil
}

func (act *action) apply(name, value string) error {
	switch name {
	case "fields":
		act.fields = value
		act.fieldsSet = true
	case "repo":
		act.repo = value
	case "host":
		act.host = value
	case "state":
		act.state = value
		act.stateSet = true
	case "limit":
		act.limitStr = value
		act.limitSet = true
	case "author":
		act.author = value
	case "search":
		act.search = value
	case "base":
		act.base = value
	case "head":
		act.head = value
	case "assignee":
		act.assignee = value
	case "reviewer":
		act.reviewer = value
	case "exclude":
		act.exclude = append(act.exclude, value)
	case "label":
		act.labels = append(act.labels, value)
	case "colors":
		act.colors = true
	case "bold":
		act.bold = true
	case "no-pager":
		act.noPager = true
	case "stat":
		act.stat = true
	case "unified":
		act.unified = true
	case "side-by-side":
		act.side = true
	case "name-only":
		act.nameOnly = true
	case "raw":
		act.raw = true
	case "draft":
		act.draftYes = true
	case "not-draft":
		act.draftNo = true
	default:
		return fmt.Errorf("unknown flag --%s", name)
	}
	return nil
}

func allowedFlags(cmd string) map[string]bool {
	ok := map[string]bool{}
	add := func(names ...string) {
		for _, n := range names {
			ok[n] = true
		}
	}
	switch cmd {
	case "view":
		add("fields", "repo", "host", "colors", "bold", "no-pager")
	case "list", "ls":
		add("fields", "repo", "host", "colors", "bold", "no-pager",
			"state", "limit", "author", "search", "base", "head",
			"assignee", "reviewer", "label", "draft", "not-draft")
	case "diff":
		add("repo", "host", "colors", "bold", "no-pager",
			"exclude", "stat", "unified", "side-by-side", "name-only", "raw")
	case "fields":
		add("host")
	}
	return ok
}

func (act action) request(tgt backend.Target, names []string) backend.Request {
	limit := act.limit
	if (act.cmd == "list" || act.cmd == "ls") && !act.limitSet {
		limit = 30
	}
	return backend.Request{
		Ref:      tgt.Ref,
		Repo:     tgt.Repo,
		Fields:   names,
		State:    act.state,
		Limit:    limit,
		Author:   act.author,
		Labels:   act.labels,
		Search:   act.search,
		Base:     act.base,
		Head:     act.head,
		Assignee: act.assignee,
		Reviewer: act.reviewer,
		Draft:    act.draft,
		Exclude:  act.exclude,
	}
}

func resolveTarget(act action, opts IO) (backend.Target, error) {
	in := backend.Input{
		ExplicitHost: act.host,
		Ref:          act.ref,
		Repo:         act.repo,
		Noun:         act.noun,
	}
	if backend.NeedsRemote(act.host, act.ref, act.repo, act.noun) && opts.Remote != nil {
		remote, err := opts.Remote()
		in.Remote = remote
		in.RemoteErr = err
	}
	return backend.Resolve(in)
}

// loadFields parses --fields before any git remote or gh/glab call.
func loadFields(act action, list bool, opts IO) (backend.Target, []string, error) {
	var names []string
	if act.fieldsSet {
		var err error
		names, err = fields.ParseSpec(act.fields)
		if err != nil {
			return backend.Target{}, nil, err
		}
	}
	tgt, err := resolveTarget(act, opts)
	if err != nil {
		return backend.Target{}, nil, err
	}
	if !act.fieldsSet {
		names = fields.Defaults(tgt.Host == backend.HostGitLab, list)
	}
	if err := fields.ValidateMode(names, tgt.Host == backend.HostGitLab, list); err != nil {
		return backend.Target{}, nil, err
	}
	return tgt, names, nil
}

func presentation(opts IO, act action) (style.Style, int) {
	st := style.Resolve(act.colors, act.bold, opts.Env, opts.TTY)
	width := opts.Width
	if !opts.TTY {
		width = 0
	}
	return st, width
}

func emit(text string, opts IO, noPager bool) int {
	if shouldPage(text, opts, noPager) {
		err := opts.Page(text)
		if err == nil {
			return 0
		}
		var startErr *exec.Error
		if errors.As(err, &startErr) {
			fmt.Fprint(opts.Out, text)
			return 0
		}
		return fail(opts, err)
	}
	fmt.Fprint(opts.Out, text)
	return 0
}

func shouldPage(text string, opts IO, noPager bool) bool {
	if noPager || !opts.TTY || opts.Page == nil || opts.Height <= 0 {
		return false
	}
	if v, ok := style.LookupEnv(opts.Env, "PRTOOL_PAGER"); ok && v == "" {
		return false
	}
	return strings.Count(text, "\n") > opts.Height
}

func cmdView(act action, opts IO) int {
	tgt, names, err := loadFields(act, false, opts)
	if err != nil {
		return fail(opts, err)
	}
	req := act.request(tgt, names)
	ctx := context.Background()
	var obj map[string]any
	if tgt.Host == backend.HostGitLab {
		obj, err = (backend.GitLab{Runner: opts.Runner}).View(ctx, req)
	} else {
		obj, err = (backend.GitHub{Runner: opts.Runner}).View(ctx, req)
	}
	if err != nil {
		return fail(opts, err)
	}
	st, width := presentation(opts, act)
	return emit(render.KeyValue(names, obj, st, width), opts, act.noPager)
}

func cmdList(act action, opts IO) int {
	tgt, names, err := loadFields(act, true, opts)
	if err != nil {
		return fail(opts, err)
	}
	req := act.request(tgt, names)
	ctx := context.Background()
	var rows []map[string]any
	if tgt.Host == backend.HostGitLab {
		rows, err = (backend.GitLab{Runner: opts.Runner}).List(ctx, req)
	} else {
		rows, err = (backend.GitHub{Runner: opts.Runner}).List(ctx, req)
	}
	if err != nil {
		return fail(opts, err)
	}
	if len(rows) == 0 {
		msg := "No pull requests.\n"
		if tgt.Host == backend.HostGitLab {
			msg = "No merge requests.\n"
		}
		return emit(msg, opts, act.noPager)
	}
	st, width := presentation(opts, act)
	return emit(render.Table(names, rows, st, width), opts, act.noPager)
}

func cmdDiff(act action, opts IO) int {
	tgt, err := resolveTarget(act, opts)
	if err != nil {
		return fail(opts, err)
	}
	req := act.request(tgt, nil)
	ctx := context.Background()
	var raw string
	if tgt.Host == backend.HostGitLab {
		raw, err = (backend.GitLab{Runner: opts.Runner}).Diff(ctx, req)
	} else {
		raw, err = (backend.GitHub{Runner: opts.Runner}).Diff(ctx, req)
	}
	if err != nil {
		return fail(opts, err)
	}
	st, width := presentation(opts, act)
	viewFlag := act.stat || act.unified || act.side || act.nameOnly
	wantRaw := act.raw || (!opts.TTY && !st.Color && !st.Bold && !viewFlag)
	if wantRaw {
		fmt.Fprint(opts.Out, raw)
		return 0
	}
	opt := diffview.Options{
		Style:     st,
		Width:     width,
		StatOnly:  act.stat,
		NameOnly:  act.nameOnly,
		Highlight: st.Color || st.Bold,
	}
	if act.side || (!act.unified && width >= 160) {
		opt.SideBySide = true
	}
	return emit(diffview.Render(diffview.Parse(raw), opt), opts, act.noPager)
}

func cmdFields(act action, opts IO) int {
	if _, err := resolveTarget(act, opts); err != nil {
		return fail(opts, err)
	}
	var b strings.Builder
	for _, name := range fields.Catalog {
		b.WriteString(name)
		b.WriteByte('\n')
	}
	return emit(b.String(), opts, false)
}
