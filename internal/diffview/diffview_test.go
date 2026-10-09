package diffview

import (
	"strings"
	"testing"

	"prtool/internal/style"
)

const tinyDiff = "diff --git a/a.txt b/a.txt\n" +
	"index 1..2 100644\n" +
	"--- a/a.txt\n" +
	"+++ b/a.txt\n" +
	"@@ -1,2 +1,3 @@\n" +
	" line\n" +
	"-old\n" +
	"+new\n" +
	"+extra\n"

const plainBody = "a.txt | 3 ++-\n" +
	"1 file changed, 2 insertions(+), 1 deletion(-)\n" +
	"\n" +
	"a.txt\n" +
	"@@ -1,2 +1,3 @@\n" +
	"1 1  line\n" +
	"2   -old\n" +
	"  2 +new\n" +
	"  3 +extra\n"

func TestTinyDiffPlain(t *testing.T) {
	d := Parse(tinyDiff)
	if len(d.Files) != 1 {
		t.Fatalf("files = %d", len(d.Files))
	}
	f := d.Files[0]
	if f.Additions != 2 || f.Deletions != 1 {
		t.Fatalf("counts +%d -%d", f.Additions, f.Deletions)
	}
	if f.Path() != "a.txt" || f.ChangeType != "MODIFIED" {
		t.Fatalf("path %s type %s", f.Path(), f.ChangeType)
	}
	if len(f.Hunks) != 1 || f.Hunks[0].OldStart != 1 || f.Hunks[0].OldCount != 2 || f.Hunks[0].NewCount != 3 {
		t.Fatalf("hunk %+v", f.Hunks)
	}
	got := Render(d, Options{Width: 80})
	if !strings.Contains(got, "\033[1ma.txt\033[0m") {
		t.Fatalf("path should stay bold with color off: %q", got)
	}
	if style.StripControls(got) != plainBody {
		t.Fatalf("body:\n%s\nwant:\n%s", style.StripControls(got), plainBody)
	}
}

func TestStatAndNameOnly(t *testing.T) {
	d := Parse(tinyDiff)
	stat := Render(d, Options{Width: 80, StatOnly: true})
	plain := style.StripControls(stat)
	if strings.Contains(plain, "@@") {
		t.Fatalf("stat included a hunk: %s", plain)
	}
	if !strings.Contains(plain, "a.txt | 3 ++-") || !strings.Contains(plain, "1 file changed, 2 insertions(+), 1 deletion(-)") {
		t.Fatalf("stat = %s", plain)
	}
	colored := Render(d, Options{Style: style.Style{Color: true}, Width: 80, StatOnly: true})
	if !strings.Contains(colored, "\033[32m++") || !strings.Contains(colored, "\033[31m-") {
		t.Fatalf("stat colors = %q", colored)
	}
	names := Render(d, Options{NameOnly: true})
	if names != "\033[1ma.txt\033[0m\n" {
		t.Fatalf("name-only = %q", names)
	}
	if style.StripControls(names) != "a.txt\n" {
		t.Fatalf("stripped names = %q", style.StripControls(names))
	}
}

func TestEmptyAndSideBySide(t *testing.T) {
	if got := Render(Parse(""), Options{}); got != "No changes.\n" {
		t.Fatalf("empty = %q", got)
	}
	if got := Render(nil, Options{}); got != "No changes.\n" {
		t.Fatalf("nil = %q", got)
	}
	got := Render(Parse(tinyDiff), Options{SideBySide: true, Width: 0})
	if !strings.Contains(got, " │ ") {
		t.Fatalf("missing gutter: %s", style.StripControls(got))
	}
}

func TestIntraLine(t *testing.T) {
	raw := "diff --git a/a.txt b/a.txt\n" +
		"--- a/a.txt\n" +
		"+++ b/a.txt\n" +
		"@@ -1 +1 @@\n" +
		"-the quick brown fox\n" +
		"+the slow brown fox\n"
	got := Render(Parse(raw), Options{
		Style:     style.Style{Color: true},
		Width:     80,
		Highlight: true,
	})
	if !strings.Contains(got, "\033[7;31mquick") || !strings.Contains(got, "\033[7;32mslow") {
		t.Fatalf("intra-line = %q", got)
	}
	if strings.Contains(got, "\033[7;31mthe") || strings.Contains(got, "\033[7;32mthe") || strings.Contains(got, "\033[7mthe") {
		t.Fatalf("inverse landed on an unchanged word: %q", got)
	}
	bold := Render(Parse(raw), Options{
		Style:     style.Style{Color: true, Bold: true},
		Width:     80,
		Highlight: true,
	})
	if !strings.Contains(bold, "\033[1;7;31mquick") {
		t.Fatalf("bold inverse = %q", bold)
	}
}

func TestParseEdges(t *testing.T) {
	renamed := Parse("diff --git a/old.txt b/new.txt\nrename from old.txt\nrename to new.txt\n")
	if renamed.Files[0].ChangeType != "RENAMED" || renamed.Files[0].DisplayPath() != "old.txt => new.txt" {
		t.Fatalf("rename %+v", renamed.Files[0])
	}
	added := Parse("diff --git a/n.txt b/n.txt\nnew file mode 100644\n--- /dev/null\n+++ b/n.txt\n@@ -0,0 +1 @@\n+hi\n")
	if added.Files[0].ChangeType != "ADDED" || added.Files[0].Additions != 1 {
		t.Fatalf("add %+v", added.Files[0])
	}
	bin := Parse("diff --git a/a.bin b/a.bin\nGIT binary patch\nliteral 4\n+abcd\n")
	if !bin.Files[0].Binary || bin.Files[0].Additions != 0 || bin.Files[0].Deletions != 0 {
		t.Fatalf("binary %+v", bin.Files[0])
	}
	quoted := Parse("diff --git \"a/foo bar.txt\" \"b/foo bar.txt\"\n")
	if quoted.Files[0].Path() != "foo bar.txt" {
		t.Fatalf("quoted path = %q", quoted.Files[0].Path())
	}
	none := Parse("diff --git a/a.txt b/a.txt\n@@ -1 +1 @@\n-old\n\\ No newline at end of file\n+new\n")
	if none.Files[0].Additions != 1 || none.Files[0].Deletions != 1 {
		t.Fatalf("no-newline counts +%d -%d", none.Files[0].Additions, none.Files[0].Deletions)
	}
	if none.Files[0].Hunks[0].Lines[1].Kind != '\\' {
		t.Fatalf("kind = %q", none.Files[0].Hunks[0].Lines[1].Kind)
	}
	cr := Parse("diff --git a/a.txt b/a.txt\n@@ -1 +1 @@\n+new\r\n")
	if cr.Files[0].Hunks[0].Lines[0].Text != "new" || cr.Files[0].Additions != 1 {
		t.Fatalf("cr %+v", cr.Files[0].Hunks[0].Lines)
	}
	hostile := Parse("diff --git a/a.txt b/a.txt\n@@ -0,0 +1 @@\n+hello\x1b[2Jworld\n")
	if hostile.Files[0].Hunks[0].Lines[0].Text != "helloworld" || hostile.Files[0].Additions != 1 {
		t.Fatalf("ansi text = %+v", hostile.Files[0])
	}
	mode := Render(Parse("diff --git a/a.txt b/a.txt\nold mode 100644\nnew mode 100755\n@@ -1 +1 @@\n-a\n+b\n"), Options{Width: 80})
	if !strings.Contains(style.StripControls(mode), "mode 100644 → 100755") {
		t.Fatalf("mode line missing: %s", style.StripControls(mode))
	}
}

func TestExcludeAndFilterRaw(t *testing.T) {
	a := "diff --git a/a.txt b/a.txt\n@@ -1 +1 @@\n-a\n+b"
	md := "diff --git a/README.md b/README.md\n@@ -1 +1 @@\n-a\n+b"
	sub := "diff --git a/sub/a.txt b/sub/a.txt\n@@ -1 +1 @@\n-a\n+b"
	nested := "diff --git a/sub/nested/b.txt b/sub/nested/b.txt\n@@ -1 +1 @@\n-a\n+b"
	raw := strings.Join([]string{a, md, sub, nested}, "\n")

	kept := Filter(Parse(raw), []string{"*.md"})
	if len(kept.Files) != 3 {
		t.Fatalf("*.md kept %d", len(kept.Files))
	}
	for _, f := range kept.Files {
		if strings.HasSuffix(f.Path(), ".md") {
			t.Fatalf("md survived: %s", f.Path())
		}
	}
	subKept := Filter(Parse(raw), []string{"sub/*"})
	got := map[string]bool{}
	for _, f := range subKept.Files {
		got[f.Path()] = true
	}
	if got["sub/a.txt"] || !got["sub/nested/b.txt"] || !got["a.txt"] || !got["README.md"] {
		t.Fatalf("sub/* paths = %v", got)
	}

	if FilterRaw(raw, nil) != raw {
		t.Fatal("no globs must return the original bytes")
	}
	both := strings.Join([]string{a, nested}, "\n")
	if FilterRaw(both, []string{"*.md"}) != both {
		t.Fatal("excluding nothing that matches must keep the original bytes")
	}
	onlyA := FilterRaw(raw, []string{"*.md", "sub/*", "sub/nested/*"})
	if onlyA != a {
		t.Fatalf("single kept chunk = %q", onlyA)
	}
	if strings.Contains(onlyA, "README.md") {
		t.Fatal("excluded file leaked into the patch")
	}
	mdGone := FilterRaw(strings.Join([]string{a, md}, "\n"), []string{"*.md"})
	if mdGone != a || strings.Contains(mdGone, "README.md") {
		t.Fatalf("md filter = %q", mdGone)
	}
}
