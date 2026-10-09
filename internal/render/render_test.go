package render

import (
	"strings"
	"testing"

	"prtool/internal/style"
)

func TestKeyValueExact(t *testing.T) {
	obj := map[string]any{
		"title":  "Fix timeout",
		"author": "octocat",
	}
	got := KeyValue([]string{"title", "author"}, obj, style.Style{Color: true, Bold: true}, 0)
	want := "\033[90mtitle \033[0m  \033[1mFix timeout\033[0m\n\033[90mauthor\033[0m  \033[1moctocat\033[0m\n"
	if got != want {
		t.Fatalf("kv = %q\nwant %q", got, want)
	}
	bold := KeyValue([]string{"title"}, map[string]any{"title": "Fix timeout"}, style.Style{Bold: true}, 0)
	if bold != "title  \033[1mFix timeout\033[0m\n" {
		t.Fatalf("bold only = %q", bold)
	}
	if strings.Contains(bold, "\033[32m") || strings.Contains(bold, "\033[90m") {
		t.Fatalf("bold only picked up color: %q", bold)
	}
	state := KeyValue([]string{"state"}, map[string]any{"state": "OPEN"}, style.Style{Color: true, Bold: true}, 0)
	if state != "\033[90mstate\033[0m  \033[1;32mOPEN\033[0m\n" {
		t.Fatalf("state = %q", state)
	}
}

func TestKeyValueHostileAndEmpty(t *testing.T) {
	obj := map[string]any{"title": "hello\033[2Jworld"}
	for _, st := range []style.Style{{}, {Color: true}} {
		got := KeyValue([]string{"title"}, obj, st, 0)
		if strings.Contains(got, "\033[2J") || !strings.Contains(got, "helloworld") {
			t.Fatalf("hostile survived %+v: %q", st, got)
		}
	}
	empty := KeyValue([]string{"title"}, map[string]any{}, style.Style{}, 0)
	if empty != "title  —\n" {
		t.Fatalf("empty = %q", empty)
	}
	dated := KeyValue([]string{"createdAt"}, map[string]any{"createdAt": "2026-10-09T07:05:56Z"}, style.Style{}, 0)
	if dated != "createdAt  2026-10-09 07:05 UTC\n" {
		t.Fatalf("date = %q", dated)
	}
	draft := KeyValue([]string{"isDraft"}, map[string]any{"isDraft": false}, style.Style{}, 0)
	if !strings.Contains(draft, "no") || strings.Contains(draft, "—") {
		t.Fatalf("draft = %q", draft)
	}
}

func TestTableExact(t *testing.T) {
	rows := []map[string]any{
		{"number": float64(1), "title": "Fix"},
		{"number": float64(12), "title": "Other"},
	}
	got := Table([]string{"number", "title"}, rows, style.Style{}, 0)
	want := " #  title\n 1  Fix\n12  Other\n"
	if got != want {
		t.Fatalf("table = %q\nwant %q", got, want)
	}
	if Table([]string{"title"}, nil, style.Style{}, 0) != "" {
		t.Fatal("empty rows should be blank")
	}
	if Table(nil, rows, style.Style{}, 0) != "" {
		t.Fatal("empty names should be blank")
	}
}
