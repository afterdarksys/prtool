package cli_test

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"prtool/internal/backend"
	"prtool/internal/cli"
)

const viewJSON = "title,author,state,isDraft,reviewDecision,baseRefName,headRefName,additions,deletions,changedFiles,url"
const listJSON = "number,title,author,state,reviewDecision,additions,deletions,changedFiles"

const tinyDiff = "diff --git a/a.txt b/a.txt\n" +
	"index 1..2 100644\n" +
	"--- a/a.txt\n" +
	"+++ b/a.txt\n" +
	"@@ -1,2 +1,3 @@\n" +
	" line\n" +
	"-old\n" +
	"+new\n" +
	"+extra\n"

func run(t *testing.T, args []string, s *backend.Script, tweak func(*cli.IO)) (int, string, string) {
	t.Helper()
	if s == nil {
		s = &backend.Script{}
	}
	var out, errb bytes.Buffer
	opts := cli.IO{
		Out:    &out,
		Err:    &errb,
		Env:    []string{"TERM=xterm"},
		Runner: s,
		Remote: func() (string, error) {
			t.Fatal("git remote was called")
			return "", errors.New("remote")
		},
	}
	if tweak != nil {
		tweak(&opts)
	}
	code := cli.Run(args, opts)
	return code, out.String(), errb.String()
}

func TestHelpAndVersion(t *testing.T) {
	code, out, errb := run(t, nil, nil, nil)
	if code != 0 || errb != "" || !strings.Contains(out, "prtool") {
		t.Fatalf("no args code=%d out=%q err=%q", code, out, errb)
	}
	code, out, errb = run(t, []string{"diff", "--help"}, nil, nil)
	if code != 0 || errb != "" || !strings.Contains(out, "--raw") {
		t.Fatalf("diff help code=%d out=%q err=%q", code, out, errb)
	}
	code, out, errb = run(t, []string{"--help", "--version"}, nil, nil)
	if code != 0 || strings.Contains(out, "prtool 1.0.0") || !strings.Contains(out, "commands:") {
		t.Fatalf("help should win: %q", out)
	}
	s := &backend.Script{}
	code, out, errb = run(t, []string{"--version"}, s, nil)
	if code != 0 || out != "prtool 1.0.0\n" || errb != "" || len(s.Calls) != 0 {
		t.Fatalf("version code=%d out=%q err=%q calls=%d", code, out, errb, len(s.Calls))
	}
}

func TestUsageErrors(t *testing.T) {
	cases := []struct {
		args []string
		sub  string
	}{
		{[]string{"--colors"}, "missing command"},
		{[]string{"pr"}, "missing command"},
		{[]string{"nope"}, `unknown command "nope"`},
		{[]string{"view", "--colors=1"}, "--colors does not take a value"},
		{[]string{"list", "--state", "foo", "--host", "github"}, `invalid --state "foo"`},
		{[]string{"list", "--limit", "0", "--host", "github"}, `invalid --limit "0"`},
		{[]string{"list", "--draft", "--not-draft", "--host", "github"}, "--draft and --not-draft cannot be used together"},
		{[]string{"diff", "--side-by-side", "--unified"}, "--side-by-side and --unified cannot be used together"},
		{[]string{"diff", "--stat", "--name-only"}, "--stat and --name-only cannot be used together"},
		{[]string{"diff", "--raw", "--stat"}, "--raw cannot be used with --stat"},
		{[]string{"diff", "--fields", "title"}, "flag --fields is not valid for diff"},
		{[]string{"view", "-wip"}, "unknown flag -wip"},
		{[]string{"view", "--fields", "--help", "--host", "github"}, "unknown field"},
		{[]string{"list", "extra", "--host", "github"}, `unexpected argument "extra"`},
		{[]string{"view", "1", "2", "--host", "github"}, "too many arguments"},
	}
	for _, tc := range cases {
		s := &backend.Script{}
		code, out, errb := run(t, tc.args, s, nil)
		if code != 1 || out != "" || !strings.Contains(errb, tc.sub) || len(s.Calls) != 0 {
			t.Fatalf("%v code=%d out=%q err=%q calls=%d", tc.args, code, out, errb, len(s.Calls))
		}
		if !strings.HasPrefix(errb, "prtool: ") {
			t.Fatalf("usage prefix: %q", errb)
		}
	}
}

func TestUnknownFieldDoesNotRun(t *testing.T) {
	s := &backend.Script{}
	remote := false
	code, _, errb := run(t, []string{"view", "--fields", "titel"}, s, func(opts *cli.IO) {
		opts.Remote = func() (string, error) {
			remote = true
			return "", errors.New("no")
		}
	})
	if code != 1 || remote || len(s.Calls) != 0 || !strings.Contains(errb, `did you mean "title"`) {
		t.Fatalf("code=%d remote=%v calls=%d err=%q", code, remote, len(s.Calls), errb)
	}
}

func TestViewListDiffDispatch(t *testing.T) {
	s := &backend.Script{Responses: []backend.Result{{Stdout: `{"title":"Fix timeout","state":"OPEN","author":{"login":"octocat"}}`}}}
	code, out, errb := run(t, []string{"--repo", "o/r", "--host", "github", "view", "pr"}, s, nil)
	if code != 0 || errb != "" {
		t.Fatalf("view pr code=%d err=%q", code, errb)
	}
	if !strings.Contains(out, "Fix timeout") {
		t.Fatalf("out = %q", out)
	}
	want := []string{"pr", "view", "pr", "--json", viewJSON, "--repo", "o/r"}
	if s.Calls[0].Name != "gh" || !same(s.Calls[0].Args, want) {
		t.Fatalf("view argv %#v", s.Calls[0])
	}

	s = &backend.Script{Responses: []backend.Result{{Stdout: "[]"}}}
	code, out, errb = run(t, []string{"list", "--repo", "o/r", "--host", "github"}, s, nil)
	if code != 0 || out != "No pull requests.\n" || strings.Contains(out, "title") {
		t.Fatalf("empty list code=%d out=%q err=%q", code, out, errb)
	}
	if !containsSeq(s.Calls[0].Args, "--limit", "30") || !containsSeq(s.Calls[0].Args, "--json", listJSON) {
		t.Fatalf("default list %#v", s.Calls[0].Args)
	}

	s = &backend.Script{Responses: []backend.Result{{Stdout: "[]"}}}
	code, out, _ = run(t, []string{"ls", "--host", "gitlab", "--repo", "g/p"}, s, nil)
	if code != 0 || out != "No merge requests.\n" || s.Calls[0].Name != "glab" {
		t.Fatalf("gitlab empty code=%d out=%q name=%s", code, out, s.Calls[0].Name)
	}

	s = &backend.Script{Responses: []backend.Result{{Stdout: `[{"number":1,"title":"Fix","author":{"login":"ada"},"state":"OPEN"}]`}}}
	code, out, errb = run(t, []string{"list", "--repo", "cli/cli", "--limit", "3", "--fields", "number,title,author,state", "--host", "github", "--no-pager"}, s, nil)
	if code != 0 || errb != "" || !strings.Contains(out, "Fix") || !strings.Contains(out, "ada") {
		t.Fatalf("list code=%d out=%q err=%q", code, out, errb)
	}
}

func TestListSearchAndShorts(t *testing.T) {
	s := &backend.Script{Responses: []backend.Result{{Stdout: "[]"}}}
	args := []string{"list", "--search", "timeout", "--reviewer", "ada", "--not-draft", "--repo", "o/r", "--host", "github"}
	code, _, errb := run(t, args, s, nil)
	if code != 0 || errb != "" {
		t.Fatal(errb)
	}
	if !containsSeq(s.Calls[0].Args, "--search", "timeout review-requested:ada draft:false") {
		t.Fatalf("search argv %#v", s.Calls[0].Args)
	}
	search := ""
	for i, a := range s.Calls[0].Args {
		if a == "--search" {
			search = s.Calls[0].Args[i+1]
		}
	}
	if search != "timeout review-requested:ada draft:false" {
		t.Fatalf("search element = %q", search)
	}

	s = &backend.Script{Responses: []backend.Result{{Stdout: "[]"}}}
	code, _, errb = run(t, []string{"list", "-Rowner/repo", "-L10", "-lbug", "-d", "--host", "github"}, s, nil)
	if code != 0 || errb != "" {
		t.Fatal(errb)
	}
	got := s.Calls[0].Args
	wantTail := []string{"--limit", "10", "--repo", "owner/repo", "--label", "bug", "--draft"}
	if !hasSuffix(got, wantTail) && !containsAll(got, wantTail) {
		t.Fatalf("shorts %#v", got)
	}
	if !containsSeq(got, "--repo", "owner/repo") || !containsSeq(got, "--limit", "10") || !containsSeq(got, "--label", "bug") {
		t.Fatalf("shorts missing pieces %#v", got)
	}
	if got[len(got)-1] != "--draft" {
		t.Fatalf("draft position %#v", got)
	}
}

func TestRawAndStyledDiff(t *testing.T) {
	patch := "diff --git a/a.txt b/a.txt\n+x"
	s := &backend.Script{Responses: []backend.Result{{Stdout: patch}}}
	paged := false
	code, out, errb := run(t, []string{"diff", "1", "--repo", "o/r", "--host", "github"}, s, func(opts *cli.IO) {
		opts.Page = func(string) error {
			paged = true
			return nil
		}
		opts.TTY = false
	})
	if code != 0 || out != patch || errb != "" || paged || strings.Contains(out, "\033") {
		t.Fatalf("raw pipe code=%d out=%q err=%q paged=%v", code, out, errb, paged)
	}

	s = &backend.Script{Responses: []backend.Result{{Stdout: patch}}}
	paged = false
	code, out, errb = run(t, []string{"diff", "1", "--raw", "--repo", "o/r", "--host", "github"}, s, func(opts *cli.IO) {
		opts.TTY = true
		opts.Height = 1
		opts.Width = 80
		opts.Page = func(string) error {
			paged = true
			return errors.New("paged raw")
		}
	})
	if code != 0 || out != patch || paged {
		t.Fatalf("raw tty code=%d out=%q err=%q paged=%v", code, out, errb, paged)
	}

	s = &backend.Script{Responses: []backend.Result{{Stdout: tinyDiff}}}
	code, out, errb = run(t, []string{"diff", "1", "--colors", "--repo", "o/r", "--host", "github"}, s, func(opts *cli.IO) {
		opts.TTY = false
		opts.Env = []string{"NO_COLOR=1"}
	})
	// Bold and color share one SGR, so a path is "\033[1;36m" rather than "\033[1m".
	bold := strings.Contains(out, "\033[1m") || strings.Contains(out, "\033[1;")
	if code != 0 || errb != "" || !bold || !strings.Contains(out, "\033[32m") {
		t.Fatalf("colors off tty code=%d err=%q out=%q", code, errb, out)
	}

	s = &backend.Script{Responses: []backend.Result{{Stdout: tinyDiff}}}
	code, out, errb = run(t, []string{"diff", "1", "--bold", "--repo", "o/r", "--host", "github"}, s, func(opts *cli.IO) {
		opts.TTY = false
	})
	if code != 0 || !strings.Contains(out, "\033[1m") || strings.Contains(out, "32m") {
		t.Fatalf("bold only code=%d out=%q err=%q", code, out, errb)
	}

	s = &backend.Script{Responses: []backend.Result{{Stdout: tinyDiff}}}
	code, out, errb = run(t, []string{"diff", "1", "--stat", "--no-pager", "--repo", "o/r", "--host", "github"}, s, func(opts *cli.IO) {
		opts.TTY = true
		opts.Width = 80
		opts.Height = 40
	})
	if code != 0 || strings.Contains(out, "@@") || !strings.Contains(out, "1 file changed") {
		t.Fatalf("stat code=%d out=%q err=%q", code, out, errb)
	}
}

func TestColorOnTTY(t *testing.T) {
	s := &backend.Script{Responses: []backend.Result{{Stdout: `{"title":"T","state":"OPEN"}`}}}
	code, out, errb := run(t, []string{"view", "1", "--fields", "state", "--repo", "o/r", "--host", "github", "--no-pager"}, s, func(opts *cli.IO) {
		opts.TTY = true
	})
	if code != 0 || !strings.Contains(out, "\033[32mOPEN\033[0m") {
		t.Fatalf("tty color code=%d out=%q err=%q", code, out, errb)
	}

	s = &backend.Script{Responses: []backend.Result{{Stdout: `{"state":"OPEN"}`}}}
	code, out, errb = run(t, []string{"view", "1", "--fields", "state", "--repo", "o/r", "--host", "github"}, s, func(opts *cli.IO) {
		opts.TTY = true
		opts.Env = []string{"NO_COLOR=1", "TERM=xterm"}
	})
	if code != 0 || strings.Contains(out, "\033[32m") {
		t.Fatalf("NO_COLOR out=%q err=%q", out, errb)
	}
}

func TestPager(t *testing.T) {
	body := `{"title":"Fix timeout","author":{"login":"octocat"}}`
	s := &backend.Script{Responses: []backend.Result{{Stdout: body}}}
	var paged string
	code, out, errb := run(t, []string{"view", "1", "--fields", "title,author", "--repo", "o/r", "--host", "github"}, s, func(opts *cli.IO) {
		opts.TTY = true
		opts.Height = 1
		opts.Page = func(text string) error {
			paged = text
			return nil
		}
	})
	if code != 0 || out != "" || paged == "" || !strings.Contains(paged, "Fix timeout") {
		t.Fatalf("page code=%d out=%q paged=%q err=%q", code, out, paged, errb)
	}

	s = &backend.Script{Responses: []backend.Result{{Stdout: body}}}
	code, out, errb = run(t, []string{"view", "1", "--fields", "title", "--repo", "o/r", "--host", "github"}, s, func(opts *cli.IO) {
		opts.TTY = true
		opts.Height = 1
		opts.Page = func(string) error {
			t.Fatal("one line was paged")
			return nil
		}
	})
	if code != 0 || !strings.Contains(out, "Fix timeout") {
		t.Fatalf("one line code=%d out=%q err=%q", code, out, errb)
	}

	s = &backend.Script{Responses: []backend.Result{{Stdout: body}}}
	code, out, errb = run(t, []string{"view", "1", "--fields", "title,author", "--repo", "o/r", "--host", "github"}, s, func(opts *cli.IO) {
		opts.TTY = true
		opts.Height = 1
		opts.Page = func(string) error {
			return &exec.Error{Name: "less", Err: exec.ErrNotFound}
		}
	})
	if code != 0 || errb != "" || !strings.Contains(out, "Fix timeout") || !strings.Contains(out, "octocat") {
		t.Fatalf("fallback code=%d out=%q err=%q", code, out, errb)
	}

	s = &backend.Script{Responses: []backend.Result{{Stdout: body}}}
	code, out, errb = run(t, []string{"view", "1", "--fields", "title,author", "--repo", "o/r", "--host", "github"}, s, func(opts *cli.IO) {
		opts.TTY = true
		opts.Height = 1
		opts.Page = func(string) error { return errors.New("pager failed") }
	})
	if code != 1 || out != "" || !strings.Contains(errb, "pager failed") || strings.Contains(out, "Fix timeout") {
		t.Fatalf("pager err code=%d out=%q err=%q", code, out, errb)
	}

	cmd := exec.Command("false")
	exitErr := cmd.Run()
	s = &backend.Script{Responses: []backend.Result{{Stdout: body}}}
	code, out, errb = run(t, []string{"view", "1", "--fields", "title,author", "--repo", "o/r", "--host", "github"}, s, func(opts *cli.IO) {
		opts.TTY = true
		opts.Height = 1
		opts.Page = func(string) error { return exitErr }
	})
	if code != 1 || strings.Contains(out, "Fix timeout") || errb == "" {
		t.Fatalf("exit pager code=%d out=%q err=%q", code, out, errb)
	}

	s = &backend.Script{Responses: []backend.Result{{Stdout: body}}}
	code, out, errb = run(t, []string{"view", "1", "--fields", "title,author", "--repo", "o/r", "--host", "github"}, s, func(opts *cli.IO) {
		opts.TTY = true
		opts.Height = 1
		opts.Env = []string{"PRTOOL_PAGER="}
		opts.Page = func(string) error {
			t.Fatal("empty pager still paged")
			return nil
		}
	})
	if code != 0 || !strings.Contains(out, "Fix timeout") {
		t.Fatalf("empty pager code=%d out=%q err=%q", code, out, errb)
	}

	s = &backend.Script{Responses: []backend.Result{{Stdout: body}}}
	code, out, _ = run(t, []string{"view", "1", "--fields", "title,author", "--no-pager", "--repo", "o/r", "--host", "github"}, s, func(opts *cli.IO) {
		opts.TTY = true
		opts.Height = 1
		opts.Page = func(string) error {
			t.Fatal("--no-pager still paged")
			return nil
		}
	})
	if code != 0 || !strings.Contains(out, "octocat") {
		t.Fatalf("no-pager code=%d out=%q", code, out)
	}
}

func TestRefsFlagsAndExitCode(t *testing.T) {
	s := &backend.Script{Responses: []backend.Result{{Stdout: `{"title":"T"}`}}}
	code, _, errb := run(t, []string{"view", "--host", "github", "--repo", "o/r", "--fields", "title", "--", "-wip"}, s, nil)
	if code != 0 || errb != "" || !containsSeq(s.Calls[0].Args, "view", "-wip") {
		t.Fatalf("dash ref code=%d err=%q argv=%#v", code, errb, s.Calls[0].Args)
	}

	s = &backend.Script{Responses: []backend.Result{{Code: 2, Stderr: "no such pull request\n"}}}
	code, out, errb := run(t, []string{"view", "9", "--host", "github", "--repo", "o/r"}, s, nil)
	if code != 2 || out != "" || errb != "no such pull request\n" {
		t.Fatalf("propagate code=%d out=%q err=%q", code, out, errb)
	}

	s = &backend.Script{Responses: []backend.Result{{Stdout: `{"title":"T"}`}}}
	code, _, errb = run(t, []string{"view", "1", "--repo", "owner/repo;touch /tmp/pwned", "--host", "github", "--fields", "title"}, s, nil)
	if code != 0 || errb != "" || s.Calls[0].Name != "gh" {
		t.Fatalf("inject code=%d err=%q", code, errb)
	}
	found := false
	for _, a := range s.Calls[0].Args {
		if a == "touch" || a == "sh" || a == "/tmp/pwned" {
			t.Fatalf("argv split: %#v", s.Calls[0].Args)
		}
		if a == "owner/repo;touch /tmp/pwned" {
			found = true
		}
	}
	if !found {
		t.Fatalf("repo arg missing: %#v", s.Calls[0].Args)
	}

	s = &backend.Script{}
	code, out, errb = run(t, []string{"fields", "--host", "github"}, s, nil)
	if code != 0 || errb != "" || len(s.Calls) != 0 {
		t.Fatalf("fields code=%d err=%q calls=%d", code, errb, len(s.Calls))
	}
	if !strings.Contains(out, "reviewers\n") || strings.Contains(out, "reviewRequests") {
		t.Fatalf("catalog = %s", out)
	}

	s = &backend.Script{Responses: []backend.Result{{Stdout: `{"title":"T"}`}}}
	code, _, errb = run(t, []string{"--host", "github", "--repo", "o/r", "--fields", "title", "--", "view", "1"}, s, nil)
	if code != 0 || errb != "" || len(s.Calls) != 1 {
		t.Fatalf("command after -- code=%d err=%q", code, errb)
	}
}

func TestHostSelectionFromCLI(t *testing.T) {
	s := &backend.Script{Responses: []backend.Result{{Stdout: `{"title":"T","state":"opened","iid":4}`}}}
	code, _, errb := run(t, []string{"view", "4"}, s, func(opts *cli.IO) {
		opts.Remote = func() (string, error) {
			return "git@gitlab.example.com:group/proj.git", nil
		}
	})
	if code != 0 || errb != "" || s.Calls[0].Name != "glab" {
		t.Fatalf("remote host code=%d err=%q call=%+v", code, errb, s.Calls)
	}
	for _, a := range s.Calls[0].Args {
		if a == "--repo" || strings.Contains(a, "group/proj") {
			t.Fatalf("remote repo leaked: %#v", s.Calls[0].Args)
		}
	}

	s = &backend.Script{}
	code, _, errb = run(t, []string{"view", "1"}, s, func(opts *cli.IO) {
		opts.Remote = func() (string, error) {
			return "git@git.example.com:o/r.git", nil
		}
	})
	if code != 1 || len(s.Calls) != 0 || !strings.Contains(errb, `host "git.example.com" is not recognized`) {
		t.Fatalf("unknown remote code=%d err=%q", code, errb)
	}

	s = &backend.Script{}
	code, _, errb = run(t, []string{"view", "https://example.com/foo"}, s, nil)
	if code != 1 || len(s.Calls) != 0 || !strings.Contains(errb, `host "example.com" is not recognized`) {
		t.Fatalf("unknown url code=%d err=%q", code, errb)
	}

	s = &backend.Script{Responses: []backend.Result{{Stdout: `{"title":"T","state":"opened"}`}}}
	code, _, errb = run(t, []string{"mr", "view", "3", "--repo", "g/p", "--fields", "title"}, s, nil)
	if code != 0 || s.Calls[0].Name != "glab" {
		t.Fatalf("mr noun %+v err=%s", s.Calls, errb)
	}

	s = &backend.Script{Responses: []backend.Result{{Stdout: `{"title":"T"}`}}}
	code, _, errb = run(t, []string{"--host", "github", "mr", "view", "3", "--repo", "o/r", "--fields", "title"}, s, nil)
	if code != 0 || s.Calls[0].Name != "gh" {
		t.Fatalf("explicit host %+v err=%s", s.Calls, errb)
	}
}

func same(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func containsSeq(args []string, a, b string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == a && args[i+1] == b {
			return true
		}
	}
	return false
}

func containsAll(got, want []string) bool {
	for _, w := range want {
		ok := false
		for _, g := range got {
			if g == w {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func hasSuffix(got, suf []string) bool {
	if len(got) < len(suf) {
		return false
	}
	return same(got[len(got)-len(suf):], suf)
}
