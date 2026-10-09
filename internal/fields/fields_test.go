package fields

import (
	"strings"
	"testing"

	"prtool/internal/style"
)

func TestCatalogAndDefaults(t *testing.T) {
	if len(Catalog) != 46 {
		t.Fatalf("catalog len = %d", len(Catalog))
	}
	joined := strings.Join(Catalog, "\n")
	if !strings.Contains(joined, "\nreviewers\n") && Catalog[0] != "reviewers" && Catalog[len(Catalog)-1] != "reviewers" {
		if !strings.Contains("\n"+joined+"\n", "\nreviewers\n") {
			t.Fatal("catalog missing reviewers")
		}
	}
	if strings.Contains("\n"+joined+"\n", "\nreviewRequests\n") {
		t.Fatal("catalog must not list reviewRequests")
	}
	if got := strings.Join(Defaults(false, false), ","); got != "title,author,state,isDraft,reviewDecision,baseRefName,headRefName,additions,deletions,changedFiles,url" {
		t.Fatalf("github view defaults = %s", got)
	}
	if got := strings.Join(Defaults(false, true), ","); got != "number,title,author,state,reviewDecision,additions,deletions,changedFiles" {
		t.Fatalf("github list defaults = %s", got)
	}
	if got := strings.Join(Defaults(true, false), ","); got != "title,author,state,isDraft,baseRefName,headRefName,mergeable,labels,url" {
		t.Fatalf("gitlab view defaults = %s", got)
	}
	if got := strings.Join(Defaults(true, true), ","); got != "title,number,author,state,mergeable,labels" {
		t.Fatalf("gitlab list defaults = %s", got)
	}
}

func TestParseSpec(t *testing.T) {
	all, err := ParseSpec("all")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(all, ",") != strings.Join(Catalog, ",") {
		t.Fatal("all did not expand to the catalog")
	}
	mixed, err := ParseSpec("title, all")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(mixed, ",") != strings.Join(Catalog, ",") {
		t.Fatal("all must replace the rest of the list")
	}
	got, err := ParseSpec(" title, title, author ")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "title,author" {
		t.Fatalf("dedupe = %v", got)
	}
	for _, spec := range []string{"", "   ", ",,,", " , "} {
		if _, err := ParseSpec(spec); err == nil || !strings.Contains(err.Error(), "empty --fields") {
			t.Fatalf("ParseSpec(%q) err = %v", spec, err)
		}
	}
}

func TestUnknownFieldSuggestion(t *testing.T) {
	_, err := ParseSpec("titel")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != `unknown field "titel"; did you mean "title"?` {
		t.Fatalf("suggestion = %s", err.Error())
	}
	_, err = ParseSpec("zzzzzzzz")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "did you mean") {
		t.Fatalf("far name should not suggest: %s", err.Error())
	}
	if err.Error() != `unknown field "zzzzzzzz"` {
		t.Fatalf("far name = %s", err.Error())
	}
}

func TestGitHubJSONFields(t *testing.T) {
	got := GitHubJSONFields([]string{"title", "reviewers", "state"})
	if got != "title,reviewRequests,state" {
		t.Fatalf("json fields = %s", got)
	}
}

func TestValidateMode(t *testing.T) {
	err := ValidateMode([]string{"title", "additions"}, true, true)
	if err == nil || err.Error() != `field "additions" is not available in GitLab list; use prtool view` {
		t.Fatalf("gitlab list = %v", err)
	}
	if err := ValidateMode([]string{"additions", "deletions", "files"}, true, false); err != nil {
		t.Fatal(err)
	}
	if err := ValidateMode([]string{"additions"}, false, true); err != nil {
		t.Fatal(err)
	}
}

func TestShortHeaderAndColumns(t *testing.T) {
	if ShortHeader("number") != "#" || ShortHeader("additions") != "+" || ShortHeader("deletions") != "-" {
		t.Fatal("short headers")
	}
	if ShortHeader("changedFiles") != "files" || ShortHeader("reviewDecision") != "review" {
		t.Fatal("short headers")
	}
	if ShortHeader("title") != "title" {
		t.Fatal("unknown header should stay")
	}
	if !NumericColumn("comments") || !NumericColumn("fullDatabaseId") || NumericColumn("title") {
		t.Fatal("numeric columns")
	}
	if !FlexibleColumn("labels") || FlexibleColumn("number") {
		t.Fatal("flexible columns")
	}
}

func paint(name string, v any, list bool, color bool) string {
	return style.PaintBlock(style.Style{Color: color}, Format(name, v, list))
}

func TestFormatScalars(t *testing.T) {
	if got := paint("createdAt", "2026-10-09T07:05:56Z", false, false); got != "2026-10-09 07:05 UTC\n" {
		t.Fatalf("date = %q", got)
	}
	if got := paint("number", float64(12), false, false); got != "12\n" {
		t.Fatalf("whole float = %q", got)
	}
	if got := paint("number", float64(1.5), false, false); got != "1.5\n" {
		t.Fatalf("fraction = %q", got)
	}
	for _, v := range []any{nil, "", "  ", []any{}} {
		got := paint("title", v, false, false)
		if got != "—\n" {
			t.Fatalf("empty %v = %q", v, got)
		}
	}
	if got := paint("isDraft", false, false, false); got != "no\n" {
		t.Fatalf("false draft = %q", got)
	}
	if strings.Contains(paint("isDraft", false, false, false), "—") {
		t.Fatal("false rendered as an em dash")
	}
	if got := paint("additions", 0, false, false); got != "+0\n" {
		t.Fatalf("zero additions = %q", got)
	}
	if got := paint("additions", nil, false, true); got != "\033[90m+0\033[0m\n" {
		t.Fatalf("nil additions = %q", got)
	}
	if got := paint("deletions", float64(0), false, false); got != "-0\n" {
		t.Fatalf("zero deletions = %q", got)
	}
	if got := paint("state", "open", false, true); got != "\033[32mOPEN\033[0m\n" {
		t.Fatalf("state = %q", got)
	}
	if got := paint("state", "merged", false, true); !strings.Contains(got, "\033[35mMERGED\033[0m") {
		t.Fatalf("merged = %q", got)
	}
	if got := paint("title", "hello\033[2Jworld", false, true); strings.Contains(got, "\033[2J") || !strings.Contains(got, "helloworld") {
		t.Fatalf("hostile title = %q", got)
	}
	author := map[string]any{"login": "octocat", "name": "The Octocat"}
	if got := paint("author", author, false, false); got != "octocat (The Octocat)\n" {
		t.Fatalf("person = %q", got)
	}
	bot := map[string]any{"login": "github-actions", "is_bot": true}
	if got := paint("author", bot, false, false); got != "github-actions [bot]\n" {
		t.Fatalf("bot = %q", got)
	}
}
