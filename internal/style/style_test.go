package style

import (
	"strings"
	"testing"
)

func TestStripControls(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"sgr", "\033[31mred\033[0m", "red"},
		{"osc bel", "\033]0;title\a", ""},
		{"osc st", "\033]8;;http://x\033\\link\033]8;;\033\\", "link"},
		{"clear", "hello\033[2Jworld", "helloworld"},
		{"c1", "\x9b31mred\x9b0m", "red"},
		{"two byte", "\033(Bplain", "plain"},
		{"dangling", "tail\033", "tail"},
		{"keep tab", "a\tb", "a\tb"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := StripControls(tc.in); got != tc.want {
				t.Fatalf("StripControls(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestPaintStripsHostileBeforeAddingSGR(t *testing.T) {
	in := "hello\033[2Jworld"
	got := Paint(Style{Color: true}, Span{Text: in, Tone: ToneBad})
	if strings.Contains(got, "\033[2J") {
		t.Fatalf("hostile clear survived paint: %q", got)
	}
	if !strings.Contains(got, "helloworld") {
		t.Fatalf("text missing: %q", got)
	}
	if !strings.Contains(got, "\033[31m") {
		t.Fatalf("expected red SGR, got %q", got)
	}
	plain := Paint(Style{}, Span{Text: in, Tone: ToneBad})
	if strings.Contains(plain, "\033") || plain != "helloworld" {
		t.Fatalf("plain paint = %q", plain)
	}
}

func TestPaintAttributeOrder(t *testing.T) {
	rgb := RGB{1, 2, 3}
	got := Paint(Style{Color: true}, Span{Text: "x", Tone: ToneGood, Bold: true, Inverse: true, RGB: &rgb})
	if got != "\033[1;7;38;2;1;2;3mx\033[0m" {
		t.Fatalf("got %q", got)
	}
	boldOnly := Paint(Style{}, Span{Text: "x", Bold: true})
	if boldOnly != "\033[1mx\033[0m" {
		t.Fatalf("bold only = %q", boldOnly)
	}
	noColorIgnoresTone := Paint(Style{}, Span{Text: "x", Tone: ToneGood})
	if noColorIgnoresTone != "x" {
		t.Fatalf("tone without color = %q", noColorIgnoresTone)
	}
}

func TestResolve(t *testing.T) {
	if got := Resolve(false, false, nil, true); !got.Color || got.Bold {
		t.Fatalf("tty auto = %+v", got)
	}
	if got := Resolve(false, false, nil, false); got.Color || got.Bold {
		t.Fatalf("pipe = %+v", got)
	}
	if got := Resolve(false, false, []string{"NO_COLOR=1"}, true); got.Color {
		t.Fatal("NO_COLOR should disable automatic color")
	}
	if got := Resolve(false, false, []string{"NO_COLOR="}, true); got.Color {
		t.Fatal("empty NO_COLOR still disables automatic color")
	}
	if got := Resolve(false, false, []string{"TERM=dumb"}, true); got.Color {
		t.Fatal("TERM=dumb should disable automatic color")
	}
	if got := Resolve(true, false, []string{"NO_COLOR=1", "TERM=dumb"}, false); !got.Color {
		t.Fatal("explicit color overrides NO_COLOR and TERM")
	}
	if got := Resolve(false, true, []string{"NO_COLOR=1"}, false); got.Color || !got.Bold {
		t.Fatalf("bold is independent of color: %+v", got)
	}
}

func TestWidthAndTruncate(t *testing.T) {
	if Width("\033[31mab\033[0m") != 2 {
		t.Fatal("ansi counted toward width")
	}
	if Width("a\tb") != 5 {
		t.Fatalf("tab width = %d", Width("a\tb"))
	}
	if Width("你") != 2 {
		t.Fatalf("cjk width = %d", Width("你"))
	}
	if got := Truncate("hello", 4); got != "hel…" {
		t.Fatalf("truncate = %q", got)
	}
	if got := Truncate("hi", 4); got != "hi" {
		t.Fatalf("short truncate = %q", got)
	}
}

func TestLift(t *testing.T) {
	in := RGB{39, 0, 175}
	got := Lift(in)
	// Green is zero, so even (255,0,255) stays under 80. Lift must still
	// brighten the channels the scale can move.
	if luminance(got) <= luminance(in) {
		t.Fatalf("lifted lum = %v (%+v)", luminance(got), got)
	}
	if got.B != 255 || got.R <= 39 {
		t.Fatalf("expected a brighter purple, got %+v", got)
	}
	bright := RGB{200, 200, 200}
	if Lift(bright) != bright {
		t.Fatal("bright color should be unchanged")
	}
	if Lift(RGB{}) != (RGB{120, 120, 120}) {
		t.Fatal("black should lift to gray")
	}
}
