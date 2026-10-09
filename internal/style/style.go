// Package style paints terminal text. Callers decide what is bold or colored;
// Paint only emits SGR for the spans it is given.
package style

import (
	"fmt"
	"math"
	"strings"
)

// Tone is a semantic color. ToneNone emits no color.
type Tone int

const (
	ToneNone Tone = iota
	ToneGood
	ToneBad
	ToneWarn
	ToneInfo
	ToneAccent
	ToneMuted
)

// RGB is a 24-bit color, used for label chips.
type RGB struct {
	R, G, B uint8
}

// Span is one run of text with a single style.
// Bold and Inverse are properties of the span. Color is applied only when
// Style.Color is set, so a piped plain render stays plain.
type Span struct {
	Text    string
	Tone    Tone
	Bold    bool
	Inverse bool
	RGB     *RGB
}

// Line is a row of spans. It does not contain a trailing newline.
type Line struct {
	Spans []Span
}

// Block is a sequence of lines.
type Block struct {
	Lines []Line
}

// Style is the resolved user preference. Bold here is the --bold flag and is
// applied by callers to value spans; Paint does not read it.
type Style struct {
	Color bool
	Bold  bool
}

// Resolve decides color and bold.
//
// Neither flag and a TTY enables color, unless NO_COLOR is set or TERM=dumb.
// An explicit color flag forces color even then. Bold follows the bold flag
// and does not imply color. env is a list of KEY=VALUE entries; nil is empty.
func Resolve(explicitColor, explicitBold bool, env []string, isTTY bool) Style {
	color := false
	if explicitColor {
		color = true
	} else if isTTY && !envPresent(env, "NO_COLOR") && lookupEnv(env, "TERM") != "dumb" {
		color = true
	}
	return Style{Color: color, Bold: explicitBold}
}

func lookupEnv(env []string, key string) string {
	v, _ := LookupEnv(env, key)
	return v
}

func envPresent(env []string, key string) bool {
	_, ok := LookupEnv(env, key)
	return ok
}

// LookupEnv returns the last value of key. The boolean is false when the key
// is absent. An empty value is present.
func LookupEnv(env []string, key string) (string, bool) {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return env[i][len(prefix):], true
		}
	}
	return "", false
}

// Threats: pull request titles, bodies, and diffs are attacker-controlled.
// CSI and OSC sequences in that text can clear the screen, rewrite the
// scrollback, retitle the terminal, or hide later characters. StripControls
// removes them before anything is painted. The viewer emits only its own SGR.
// Raw patch output is the exception and is never passed through Paint.
func StripControls(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		c := s[i]
		if c == 0x1b {
			if i+1 >= len(s) {
				break
			}
			switch s[i+1] {
			case '[':
				j := i + 2
				for j < len(s) && !csiFinal(s[j]) {
					j++
				}
				if j < len(s) {
					j++
				}
				i = j
			case ']':
				j := i + 2
				for j < len(s) {
					if s[j] == 0x07 {
						j++
						break
					}
					if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
						j += 2
						break
					}
					j++
				}
				i = j
			default:
				// nF sequences: ESC, zero or more intermediates (0x20-0x2F),
				// then a final (0x30-0x7E). ESC ( B is the common G0 designation.
				j := i + 1
				for j < len(s) && s[j] >= 0x20 && s[j] <= 0x2f {
					j++
				}
				if j < len(s) && s[j] >= 0x30 && s[j] <= 0x7e {
					j++
				} else if j == i+1 {
					j = i + 2
				}
				if j > len(s) {
					j = len(s)
				}
				i = j
			}
			continue
		}
		if c == 0x9b {
			j := i + 1
			for j < len(s) && !csiFinal(s[j]) {
				j++
			}
			if j < len(s) {
				j++
			}
			i = j
			continue
		}
		if c < 0x20 && c != '\t' && c != '\n' && c != '\r' {
			i++
			continue
		}
		if c == 0x7f {
			i++
			continue
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

func csiFinal(c byte) bool {
	return c >= 0x40 && c <= 0x7e
}

// Paint renders one span. Control sequences in the source text are removed
// before any SGR of ours is added. Attribute order is bold, inverse, color.
func Paint(st Style, sp Span) string {
	text := StripControls(sp.Text)
	if text == "" {
		return ""
	}
	var attrs []string
	if sp.Bold {
		attrs = append(attrs, "1")
	}
	if sp.Inverse {
		attrs = append(attrs, "7")
	}
	if st.Color {
		if sp.RGB != nil {
			attrs = append(attrs, fmt.Sprintf("38;2;%d;%d;%d", sp.RGB.R, sp.RGB.G, sp.RGB.B))
		} else if code := toneCode(sp.Tone); code != "" {
			attrs = append(attrs, code)
		}
	}
	if len(attrs) == 0 {
		return text
	}
	return "\033[" + strings.Join(attrs, ";") + "m" + text + "\033[0m"
}

func toneCode(t Tone) string {
	switch t {
	case ToneGood:
		return "32"
	case ToneBad:
		return "31"
	case ToneWarn:
		return "33"
	case ToneInfo:
		return "36"
	case ToneAccent:
		return "35"
	case ToneMuted:
		return "90"
	default:
		return ""
	}
}

// PaintLine concatenates the spans of a line.
func PaintLine(st Style, line Line) string {
	var b strings.Builder
	for _, sp := range line.Spans {
		b.WriteString(Paint(st, sp))
	}
	return b.String()
}

// PaintBlock joins lines with newlines and does not add a trailing newline
// when the block is empty. A non-empty block ends with a newline.
func PaintBlock(st Style, block Block) string {
	if len(block.Lines) == 0 {
		return ""
	}
	var b strings.Builder
	for i, line := range block.Lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(PaintLine(st, line))
	}
	b.WriteByte('\n')
	return b.String()
}

// Width is the display width of s after control sequences are removed.
// Tabs advance to the next multiple of 4. Wide characters (CJK, emoji) count as 2.
func Width(s string) int {
	s = StripControls(s)
	w := 0
	for _, r := range s {
		w = advance(w, r)
	}
	return w
}

// advance moves a column cursor past r. A tab stops on the next column
// that is a multiple of 4, so "a\tb" is five columns.
func advance(w int, r rune) int {
	if r == '\t' {
		return w + 4 - (w % 4)
	}
	return w + runeWidth(r)
}

func runeWidth(r rune) int {
	switch r {
	case '\n', '\r', '\t':
		return 0
	}
	if r < 0x20 || r == 0x7f {
		return 0
	}
	if isWide(r) {
		return 2
	}
	return 1
}

func isWide(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115f:
		return true
	case r >= 0x2e80 && r <= 0xa4cf:
		return true
	case r >= 0xac00 && r <= 0xd7a3:
		return true
	case r >= 0xf900 && r <= 0xfaff:
		return true
	case r >= 0xfe10 && r <= 0xfe19:
		return true
	case r >= 0xfe30 && r <= 0xfe6f:
		return true
	case r >= 0xff00 && r <= 0xff60:
		return true
	case r >= 0xffe0 && r <= 0xffe6:
		return true
	case r >= 0x1f300 && r <= 0x1f64f:
		return true
	case r >= 0x1f900 && r <= 0x1f9ff:
		return true
	case r >= 0x1fa70 && r <= 0x1faff:
		return true
	case r >= 0x20000 && r <= 0x3fffd:
		return true
	default:
		return false
	}
}

// Truncate shortens s to max display columns, ending with an ellipsis when it
// cuts. max <= 0 yields an empty string.
func Truncate(s string, max int) string {
	s = StripControls(s)
	if Width(s) <= max {
		return s
	}
	if max <= 0 {
		return ""
	}
	if max == 1 {
		return "…"
	}
	var b strings.Builder
	w := 0
	for _, r := range s {
		nw := advance(w, r)
		if nw > max-1 {
			break
		}
		b.WriteRune(r)
		w = nw
	}
	b.WriteString("…")
	return b.String()
}

// TruncateSpans keeps a prefix of spans that fits in max columns.
func TruncateSpans(spans []Span, max int) []Span {
	if max <= 0 {
		return nil
	}
	var out []Span
	used := 0
	for _, sp := range spans {
		w := Width(sp.Text)
		if used+w <= max {
			out = append(out, sp)
			used += w
			continue
		}
		remain := max - used
		if remain <= 0 {
			break
		}
		sp.Text = Truncate(sp.Text, remain)
		out = append(out, sp)
		break
	}
	return out
}

// Lift brightens a label color whose relative luminance is below 80 so it
// stays readable on a dark terminal. Black becomes a neutral gray.
func Lift(c RGB) RGB {
	l := luminance(c)
	if l >= 80 {
		return c
	}
	if l == 0 {
		return RGB{120, 120, 120}
	}
	scale := 120 / l
	return RGB{
		R: scaleChannel(c.R, scale),
		G: scaleChannel(c.G, scale),
		B: scaleChannel(c.B, scale),
	}
}

func luminance(c RGB) float64 {
	return 0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)
}

func scaleChannel(v uint8, scale float64) uint8 {
	n := math.Round(float64(v) * scale)
	if n > 255 {
		return 255
	}
	if n < 0 {
		return 0
	}
	return uint8(n)
}

// ParseHex reads RRGGBB or RGB, with or without a leading '#'.
func ParseHex(s string) (RGB, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return RGB{}, false
	}
	var n uint32
	for i := 0; i < 6; i++ {
		var d byte
		switch c := s[i]; {
		case c >= '0' && c <= '9':
			d = c - '0'
		case c >= 'a' && c <= 'f':
			d = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			d = c - 'A' + 10
		default:
			return RGB{}, false
		}
		n = n<<4 | uint32(d)
	}
	return RGB{uint8(n >> 16), uint8(n >> 8), uint8(n)}, true
}

// PadRight pads s with spaces to width columns. s wider than width is unchanged.
func PadRight(s string, width int) string {
	w := Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// PadLeft pads s with spaces on the left.
func PadLeft(s string, width int) string {
	w := Width(s)
	if w >= width {
		return s
	}
	return strings.Repeat(" ", width-w) + s
}

// Plain joins span text. It is used by tests and width measurement.
func Plain(spans []Span) string {
	var b strings.Builder
	for _, sp := range spans {
		b.WriteString(StripControls(sp.Text))
	}
	return b.String()
}
