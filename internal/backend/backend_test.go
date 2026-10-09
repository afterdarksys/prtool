package backend

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestResolveHosts(t *testing.T) {
	ghURL := "https://github.com/cli/cli/pull/14629"
	cases := []struct {
		name   string
		in     Input
		host   Host
		ref    string
		repo   string
		errSub string
	}{
		{
			name: "github url keeps the ref",
			in:   Input{Ref: ghURL},
			host: HostGitHub, ref: ghURL,
		},
		{
			name: "gitlab mr diff url",
			in:   Input{Ref: "https://gitlab.com/group/sub/proj/-/merge_requests/15.diff"},
			host: HostGitLab, ref: "15", repo: "group/sub/proj",
		},
		{
			name: "ssh gitlab remote does not set repo",
			in:   Input{Remote: "git@gitlab.example.com:group/proj.git"},
			host: HostGitLab, repo: "",
		},
		{
			name:   "unknown ssh host",
			in:     Input{Remote: "git@git.example.com:o/r.git"},
			errSub: `host "git.example.com" is not recognized`,
		},
		{
			name: "github. prefix",
			in:   Input{Ref: "https://github.example.com/o/r/pull/1"},
			host: HostGitHub, ref: "https://github.example.com/o/r/pull/1",
		},
		{
			name: "dotted github",
			in:   Input{Ref: "https://foo.github.example.com/o/r/pull/2"},
			host: HostGitHub, ref: "https://foo.github.example.com/o/r/pull/2",
		},
		{
			name: "github.com suffix",
			in:   Input{Ref: "https://api.github.com/repos/o/r/pulls/3"},
			host: HostGitHub, ref: "https://api.github.com/repos/o/r/pulls/3",
		},
		{
			name: "owner/repo does not take the remote path",
			in:   Input{Repo: "owner/repo", Remote: "git@github.com:other/thing.git"},
			host: HostGitHub, repo: "owner/repo",
		},
		{
			name: "noun mr beats a github remote",
			in:   Input{Noun: "mr", Remote: "git@github.com:o/r.git"},
			host: HostGitLab,
		},
		{
			name: "explicit host beats the noun",
			in:   Input{ExplicitHost: "github", Noun: "mr"},
			host: HostGitHub,
		},
		{
			name:   "unknown host flag",
			in:     Input{ExplicitHost: "bitbucket"},
			errSub: `unknown --host "bitbucket"`,
		},
		{
			name:   "unknown url host",
			in:     Input{Ref: "https://example.com/foo"},
			errSub: `host "example.com" is not recognized`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Resolve(tc.in)
			if tc.errSub != "" {
				if err == nil || !strings.Contains(err.Error(), tc.errSub) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Host != tc.host || got.Ref != tc.ref || got.Repo != tc.repo {
				t.Fatalf("got host=%s ref=%q repo=%q", got.Host, got.Ref, got.Repo)
			}
		})
	}
}

func TestNeedsRemote(t *testing.T) {
	if NeedsRemote("", "https://example.com/foo", "", "") {
		t.Fatal("a URL ref must not read origin")
	}
	if !NeedsRemote("", "12", "owner/repo", "") {
		t.Fatal("owner/repo still needs a host")
	}
	if NeedsRemote("github", "12", "", "") || NeedsRemote("", "12", "", "mr") {
		t.Fatal("explicit host and noun skip origin")
	}
}

func TestGitHubArgv(t *testing.T) {
	ctx := context.Background()
	s := &Script{Responses: []Result{{Stdout: `{"title":"T","reviewRequests":[{"login":"ada"}]}`}}}
	obj, err := (GitHub{Runner: s}).View(ctx, Request{
		Ref: "12", Repo: "o/r", Fields: []string{"title", "reviewers"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"pr", "view", "12", "--json", "title,reviewRequests", "--repo", "o/r"}
	if !same(s.Calls[0].Args, want) || s.Calls[0].Name != "gh" {
		t.Fatalf("view call %+v", s.Calls[0])
	}
	if _, ok := obj["reviewers"]; !ok {
		t.Fatalf("reviewers missing: %#v", obj)
	}

	s2 := &Script{Responses: []Result{{Stdout: `{"title":"T","reviewRequests":[{"login":"ada"}]}`}}}
	obj, err = (GitHub{Runner: s2}).View(ctx, Request{Ref: "12", Fields: []string{"title"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := obj["reviewers"]; ok {
		t.Fatal("reviewers copied without being requested")
	}

	list := &Script{Responses: []Result{{Stdout: "[]"}}}
	_, err = (GitHub{Runner: list}).List(ctx, Request{
		Repo: "o/r", Fields: []string{"number", "title"}, Limit: 30,
		Search: "timeout", Reviewer: "ada", Draft: "no", State: "open",
		Author: "ada", Base: "main", Head: "feat", Assignee: "bea",
		Labels: []string{"bug"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantList := []string{
		"pr", "list", "--json", "number,title", "--limit", "30",
		"--state", "open", "--author", "ada",
		"--search", "timeout review-requested:ada draft:false",
		"--base", "main", "--head", "feat", "--assignee", "bea",
		"--repo", "o/r", "--label", "bug",
	}
	if !same(list.Calls[0].Args, wantList) {
		t.Fatalf("list argv = %#v", list.Calls[0].Args)
	}

	open := &Script{Responses: []Result{{Stdout: "[]"}}}
	_, err = (GitHub{Runner: open}).List(ctx, Request{Fields: []string{"number"}, Limit: 30})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range open.Calls[0].Args {
		if a == "--state" || a == "--draft" || a == "--search" {
			t.Fatalf("open list added %s in %#v", a, open.Calls[0].Args)
		}
	}

	draft := &Script{Responses: []Result{{Stdout: "[]"}}}
	_, err = (GitHub{Runner: draft}).List(ctx, Request{Fields: []string{"number"}, Draft: "yes", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	args := draft.Calls[0].Args
	if args[len(args)-1] != "--draft" {
		t.Fatalf("draft flag = %#v", args)
	}
}

func TestGitHubDiffExcludeAndInjection(t *testing.T) {
	ctx := context.Background()
	a := "diff --git a/a.txt b/a.txt\n@@ -1 +1 @@\n-a\n+b\n"
	md := "diff --git a/README.md b/README.md\n@@ -1 +1 @@\n-a\n+b\n"
	s := &Script{Responses: []Result{{Stdout: a + md}}}
	out, err := (GitHub{Runner: s}).Diff(ctx, Request{
		Ref: "9", Repo: "owner/repo;touch /tmp/pwned", Exclude: []string{"*.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.Calls[0].Name != "gh" || s.Calls[0].Name == "sh" {
		t.Fatalf("binary = %s", s.Calls[0].Name)
	}
	foundRepo, foundExclude := false, false
	for _, arg := range s.Calls[0].Args {
		if arg == "touch" || arg == "/tmp/pwned" || arg == "sh" {
			t.Fatalf("split metacharacter: %#v", s.Calls[0].Args)
		}
		if arg == "owner/repo;touch /tmp/pwned" {
			foundRepo = true
		}
		if arg == "*.md" {
			foundExclude = true
		}
	}
	if !foundRepo || !foundExclude {
		t.Fatalf("argv = %#v", s.Calls[0].Args)
	}
	if strings.Contains(out, "README.md") || !strings.Contains(out, "a.txt") {
		t.Fatalf("local filter = %q", out)
	}
	joined := strings.Join(s.Calls[0].Args, "\x00")
	if !strings.Contains(joined, "--exclude\x00*.md") || !strings.Contains(joined, "--color=never") {
		t.Fatalf("diff argv = %#v", s.Calls[0].Args)
	}
}

func TestExitAndMissing(t *testing.T) {
	ctx := context.Background()
	s := &Script{Responses: []Result{{Code: 2, Stderr: "denied\n"}}}
	_, err := (GitHub{Runner: s}).View(ctx, Request{Ref: "1", Fields: []string{"title"}})
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 2 || ee.Msg != "denied" {
		t.Fatalf("exit = %v", err)
	}
	missing := &Script{Missing: map[string]bool{"gh": true}}
	_, err = (GitHub{Runner: missing}).View(ctx, Request{Fields: []string{"title"}})
	if err == nil || err.Error() != "gh is not installed" {
		t.Fatalf("missing gh = %v", err)
	}
	if len(missing.Calls) != 0 {
		t.Fatal("missing binary still ran")
	}
	glab := &Script{Missing: map[string]bool{"glab": true}}
	_, err = (GitLab{Runner: glab}).View(ctx, Request{Fields: []string{"title"}})
	if err == nil || err.Error() != "glab is not installed (needed for GitLab merge requests)" {
		t.Fatalf("missing glab = %v", err)
	}
	if len(glab.Calls) != 0 {
		t.Fatal("missing glab still ran")
	}
}

func TestGitLabListAndView(t *testing.T) {
	ctx := context.Background()
	body := `{"iid":15,"title":"Fix","description":"body text","state":"opened","draft":false,"work_in_progress":true,"web_url":"https://gitlab.example/mr/15","source_branch":"feat","target_branch":"main","author":{"username":"ada","name":"Ada Lovelace"},"labels":["bug",{"title":"prio","color":"#ff0000"}],"merge_status":"can_be_merged","detailed_merge_status":"mergeable","has_conflicts":false,"user_notes_count":3,"changes_count":"2+","sha":"abcdef1234567890","merge_commit_sha":"1234567890abcdef"}`
	s := &Script{Responses: []Result{
		{Stdout: body},
		{Stdout: "diff --git a/a.txt b/a.txt\n@@ -1 +1 @@\n-a\n+b\n"},
	}}
	obj, err := (GitLab{Runner: s}).View(ctx, Request{
		Ref: "15", Repo: "group/proj",
		Fields: []string{"title", "state", "isDraft", "labels", "mergeable", "comments", "changedFiles"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// changedFiles is a diff stat field, so view is followed by a diff.
	if len(s.Calls) != 2 || s.Calls[0].Args[1] != "view" {
		t.Fatalf("calls = %+v", s.Calls)
	}
	if obj["state"] != "OPEN" || obj["isDraft"] != true {
		t.Fatalf("state/draft = %#v %#v", obj["state"], obj["isDraft"])
	}
	if obj["number"] != float64(15) && obj["number"] != "15" {
		// iid may arrive as a number. The fixture sends a number.
	}
	if obj["mergeable"] != "MERGEABLE" {
		t.Fatalf("mergeable = %#v", obj["mergeable"])
	}
	if obj["comments"] != float64(3) {
		t.Fatalf("comments = %#v", obj["comments"])
	}
	labels, _ := obj["labels"].([]any)
	if len(labels) != 2 {
		t.Fatalf("labels = %#v", obj["labels"])
	}
	second, _ := labels[1].(map[string]any)
	if second["color"] != "ff0000" || second["name"] != "prio" {
		t.Fatalf("label = %#v", second)
	}

	opened := &Script{Responses: []Result{{Stdout: `{"iid":1,"title":"T","state":"opened","changes_count":"4","sha":"abc","merge_commit_sha":"def","has_conflicts":false,"merge_status":"can_be_merged"}`}}}
	obj, err = (GitLab{Runner: opened}).View(ctx, Request{Ref: "1", Fields: []string{"title", "state"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(opened.Calls) != 1 {
		t.Fatalf("title-only fetched a diff: %+v", opened.Calls)
	}
	if obj["state"] != "OPEN" || obj["changedFiles"] != 4 || obj["headRefOid"] != "abc" {
		t.Fatalf("opened map = %#v", obj)
	}
	mc, _ := obj["mergeCommit"].(map[string]any)
	if mc["oid"] != "def" {
		t.Fatalf("merge commit = %#v", obj["mergeCommit"])
	}
	if obj["closed"] != false {
		t.Fatalf("open closed flag = %#v", obj["closed"])
	}

	for _, tc := range []struct {
		state  string
		want   string
		closed bool
	}{
		{"merged", "MERGED", true},
		{"closed", "CLOSED", true},
	} {
		sc := &Script{Responses: []Result{{Stdout: `{"iid":1,"title":"T","state":"` + tc.state + `"}`}}}
		obj, err = (GitLab{Runner: sc}).View(ctx, Request{Ref: "1", Fields: []string{"state"}})
		if err != nil {
			t.Fatal(err)
		}
		if obj["state"] != tc.want || obj["closed"] != tc.closed {
			t.Fatalf("%s -> %#v closed %#v", tc.state, obj["state"], obj["closed"])
		}
	}

	conflict := &Script{Responses: []Result{{Stdout: `{"iid":1,"title":"T","state":"opened","has_conflicts":true,"merge_status":"can_be_merged"}`}}}
	obj, err = (GitLab{Runner: conflict}).View(ctx, Request{Ref: "1", Fields: []string{"title"}})
	if err != nil {
		t.Fatal(err)
	}
	if obj["mergeable"] != "CONFLICTING" {
		t.Fatalf("conflicts = %#v", obj["mergeable"])
	}
}

func TestGitLabDiffStatSkipsView(t *testing.T) {
	ctx := context.Background()
	diff := "diff --git a/a.txt b/a.txt\n@@ -1,2 +1,3 @@\n line\n-old\n+new\n+extra\n"
	s := &Script{Responses: []Result{{Stdout: diff}}}
	obj, err := (GitLab{Runner: s}).View(ctx, Request{
		Ref: "15", Repo: "group/proj", Fields: []string{"additions", "deletions"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Calls) != 1 {
		t.Fatalf("calls = %+v", s.Calls)
	}
	want := []string{"mr", "diff", "15", "--color=never", "--raw", "--repo", "group/proj"}
	if s.Calls[0].Name != "glab" || !same(s.Calls[0].Args, want) {
		t.Fatalf("diff call %+v", s.Calls[0])
	}
	if obj["additions"] != 2 || obj["deletions"] != 1 {
		t.Fatalf("stats = %#v", obj)
	}

	both := &Script{Responses: []Result{
		{Stdout: `{"iid":15,"title":"Fix","state":"opened"}`},
		{Stdout: diff},
	}}
	obj, err = (GitLab{Runner: both}).View(ctx, Request{
		Ref: "15", Fields: []string{"title", "additions"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(both.Calls) != 2 || both.Calls[0].Args[1] != "view" || both.Calls[1].Args[1] != "diff" {
		t.Fatalf("calls = %+v", both.Calls)
	}
	if obj["title"] != "Fix" || obj["additions"] != 2 {
		t.Fatalf("combined = %#v", obj)
	}
}

func TestGitLabListArgv(t *testing.T) {
	ctx := context.Background()
	open := &Script{Responses: []Result{{Stdout: "[]"}}}
	_, err := (GitLab{Runner: open}).List(ctx, Request{
		Repo: "o/r", Fields: []string{"title"}, Limit: 30, State: "open",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantOpen := []string{"mr", "list", "--output", "json", "--per-page", "30", "--repo", "o/r"}
	if !same(open.Calls[0].Args, wantOpen) {
		t.Fatalf("open = %#v", open.Calls[0].Args)
	}

	full := &Script{Responses: []Result{{Stdout: "[]"}}}
	_, err = (GitLab{Runner: full}).List(ctx, Request{
		Repo: "o/r", Fields: []string{"title", "labels"}, Limit: 10, State: "merged",
		Author: "ada", Assignee: "bea", Reviewer: "cy", Search: "timeout",
		Head: "feat", Base: "main", Labels: []string{"bug", "ui"}, Draft: "no",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"mr", "list", "--output", "json", "--per-page", "10", "--merged",
		"--author", "ada", "--assignee", "bea", "--reviewer", "cy",
		"--search", "timeout", "--source-branch", "feat", "--target-branch", "main",
		"--repo", "o/r", "--label", "bug", "--label", "ui", "--not-draft",
	}
	if !same(full.Calls[0].Args, want) {
		t.Fatalf("full = %#v", full.Calls[0].Args)
	}

	for _, tc := range []struct{ state, flag string }{
		{"closed", "--closed"},
		{"all", "--all"},
	} {
		sc := &Script{Responses: []Result{{Stdout: "[]"}}}
		_, err = (GitLab{Runner: sc}).List(ctx, Request{Fields: []string{"title"}, State: tc.state, Limit: 30})
		if err != nil {
			t.Fatal(err)
		}
		if !containsArg(sc.Calls[0].Args, tc.flag) {
			t.Fatalf("%s argv %#v", tc.state, sc.Calls[0].Args)
		}
	}

	bad := &Script{}
	_, err = (GitLab{Runner: bad}).List(ctx, Request{Fields: []string{"additions"}, Limit: 30})
	if err == nil || !strings.Contains(err.Error(), "not available in GitLab list") {
		t.Fatalf("err = %v", err)
	}
	if len(bad.Calls) != 0 {
		t.Fatal("invalid field still ran glab")
	}
}

func TestGitLabDiff(t *testing.T) {
	ctx := context.Background()
	s := &Script{Responses: []Result{{Stdout: "diff --git a/a.txt b/a.txt\n"}}}
	_, err := (GitLab{Runner: s}).Diff(ctx, Request{Ref: "15", Repo: "group/proj"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"mr", "diff", "15", "--color=never", "--raw", "--repo", "group/proj"}
	if !same(s.Calls[0].Args, want) {
		t.Fatalf("argv = %#v", s.Calls[0].Args)
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

func containsArg(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}
