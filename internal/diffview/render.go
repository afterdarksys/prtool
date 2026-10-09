package diffview

import (
	"fmt"
	"strings"

	"prtool/internal/style"
)

// Options controls a beautified diff. Width 0 means the terminal width is
// unknown: no line numbers, and no side-by-side unless SideBySide is set.
type Options struct {
	Style      style.Style
	Width      int
	SideBySide bool
	StatOnly   bool
	NameOnly   bool
	Highlight  bool
}

// Render paints a diff. The result always ends with a newline when it is
// non-empty.
func Render(d *Diff, opt Options) string {
	if d == nil || len(d.Files) == 0 {
		return "No changes.\n"
	}
	if opt.NameOnly {
		return paint(opt.Style, nameBlock(d))
	}
	stat := statBlock(d, opt)
	if opt.StatOnly {
		return paint(opt.Style, stat)
	}
	body := fileBlock(d, opt)
	block := style.Block{Lines: append(append(stat.Lines, style.Line{}), body.Lines...)}
	return paint(opt.Style, block)
}

func paint(st style.Style, b style.Block) string {
	return style.PaintBlock(st, b)
}

func nameBlock(d *Diff) style.Block {
	var lines []style.Line
	for _, f := range d.Files {
		lines = append(lines, style.Line{Spans: []style.Span{{
			Text: f.DisplayPath(), Tone: style.ToneInfo, Bold: true,
		}}})
	}
	return style.Block{Lines: lines}
}

func statBlock(d *Diff, opt Options) style.Block {
	barW := 20
	if opt.Width > 40 {
		barW = opt.Width / 4
		if barW < 20 {
			barW = 20
		}
		if barW > 40 {
			barW = 40
		}
	}
	var lines []style.Line
	add, del := 0, 0
	for _, f := range d.Files {
		add += f.Additions
		del += f.Deletions
		lines = append(lines, statLine(f, barW))
	}
	lines = append(lines, style.Line{Spans: []style.Span{{Text: summary(len(d.Files), add, del)}}})
	return style.Block{Lines: lines}
}

func statLine(f File, barW int) style.Line {
	name := f.DisplayPath()
	spans := []style.Span{{Text: name, Tone: style.ToneInfo, Bold: true}, {Text: " | "}}
	if f.Binary {
		spans = append(spans, style.Span{Text: "Bin", Tone: style.ToneMuted})
		return style.Line{Spans: spans}
	}
	total := f.Additions + f.Deletions
	spans = append(spans, style.Span{Text: fmt.Sprintf("%d ", total)})
	plus, minus := barParts(f.Additions, f.Deletions, barW)
	if plus != "" {
		spans = append(spans, style.Span{Text: plus, Tone: style.ToneGood})
	}
	if minus != "" {
		spans = append(spans, style.Span{Text: minus, Tone: style.ToneBad})
	}
	return style.Line{Spans: spans}
}

func barParts(ins, del, width int) (string, string) {
	if ins+del == 0 || width <= 0 {
		return "", ""
	}
	if ins+del <= width {
		return strings.Repeat("+", ins), strings.Repeat("-", del)
	}
	plus := int(float64(ins)/float64(ins+del)*float64(width) + 0.5)
	minus := width - plus
	if ins > 0 && del > 0 && width >= 2 {
		if plus == 0 {
			plus = 1
			minus = width - 1
		}
		if minus == 0 {
			minus = 1
			plus = width - 1
		}
	}
	if plus < 0 {
		plus = 0
	}
	if minus < 0 {
		minus = 0
	}
	return strings.Repeat("+", plus), strings.Repeat("-", minus)
}

func summary(files, ins, del int) string {
	fs := "files"
	if files == 1 {
		fs = "file"
	}
	parts := []string{fmt.Sprintf("%d %s changed", files, fs)}
	if ins > 0 {
		w := "insertions"
		if ins == 1 {
			w = "insertion"
		}
		parts = append(parts, fmt.Sprintf("%d %s(+)", ins, w))
	}
	if del > 0 {
		w := "deletions"
		if del == 1 {
			w = "deletion"
		}
		parts = append(parts, fmt.Sprintf("%d %s(-)", del, w))
	}
	return strings.Join(parts, ", ")
}

func fileBlock(d *Diff, opt Options) style.Block {
	var lines []style.Line
	for i, f := range d.Files {
		if i > 0 {
			lines = append(lines, style.Line{})
		}
		lines = append(lines, style.Line{Spans: []style.Span{{
			Text: f.DisplayPath(), Tone: style.ToneInfo, Bold: true,
		}}})
		if f.OldMode != "" && f.NewMode != "" && f.OldMode != f.NewMode {
			lines = append(lines, style.Line{Spans: []style.Span{{
				Text: fmt.Sprintf("mode %s → %s", f.OldMode, f.NewMode),
				Tone: style.ToneMuted,
			}}})
		}
		if f.Binary {
			lines = append(lines, style.Line{Spans: []style.Span{{Text: "Binary file", Tone: style.ToneMuted}}})
			continue
		}
		if opt.SideBySide {
			lines = append(lines, renderSideBySide(f, opt)...)
			continue
		}
		lines = append(lines, renderUnified(f, opt)...)
	}
	return style.Block{Lines: lines}
}

func renderUnified(f File, opt Options) []style.Line {
	numW, showNum := numberWidth(f, opt.Width)
	var lines []style.Line
	for _, h := range f.Hunks {
		lines = append(lines, hunkHeader(h))
		oldN := h.OldStart
		newN := h.NewStart
		src := h.Lines
		for i := 0; i < len(src); {
			ln := src[i]
			if ln.Kind == '\\' {
				lines = append(lines, style.Line{Spans: []style.Span{{
					Text: "\\" + ln.Text, Tone: style.ToneMuted,
				}}})
				i++
				continue
			}
			if ln.Kind == ' ' {
				lines = append(lines, unifiedRow(' ', ln.Text, "", oldN, true, newN, true, showNum, numW, opt))
				oldN++
				newN++
				i++
				continue
			}
			var dels, adds []Line
			for i < len(src) && src[i].Kind == '-' {
				dels = append(dels, src[i])
				i++
			}
			for i < len(src) && src[i].Kind == '+' {
				adds = append(adds, src[i])
				i++
			}
			for k, d := range dels {
				peer := ""
				if k < len(adds) {
					peer = adds[k].Text
				}
				lines = append(lines, unifiedRow('-', d.Text, peer, oldN, true, 0, false, showNum, numW, opt))
				oldN++
			}
			for k, a := range adds {
				peer := ""
				if k < len(dels) {
					peer = dels[k].Text
				}
				lines = append(lines, unifiedRow('+', a.Text, peer, 0, false, newN, true, showNum, numW, opt))
				newN++
			}
		}
	}
	return lines
}

func unifiedRow(kind byte, text, peer string, oldN int, oldOK bool, newN int, newOK bool, showNum bool, numW int, opt Options) style.Line {
	textSpans := lineSpans(kind, text, peer, opt)
	var spans []style.Span
	if showNum {
		spans = append(spans,
			style.Span{Text: fmtNum(oldN, oldOK, numW), Tone: style.ToneMuted},
			style.Span{Text: " "},
			style.Span{Text: fmtNum(newN, newOK, numW), Tone: style.ToneMuted},
			style.Span{Text: " "},
		)
	}
	spans = append(spans, textSpans...)
	return style.Line{Spans: spans}
}

func hunkHeader(h Hunk) style.Line {
	head := fmt.Sprintf("@@ -%s +%s @@", hunkRange(h.OldStart, h.OldCount), hunkRange(h.NewStart, h.NewCount))
	spans := []style.Span{{Text: head, Tone: style.ToneInfo}}
	if h.Context != "" {
		spans = append(spans, style.Span{Text: " " + h.Context, Tone: style.ToneInfo, Bold: true})
	}
	return style.Line{Spans: spans}
}

func hunkRange(start, count int) string {
	if count == 1 {
		return fmt.Sprintf("%d,1", start)
	}
	return fmt.Sprintf("%d,%d", start, count)
}

func fmtNum(n int, present bool, w int) string {
	if !present {
		return strings.Repeat(" ", w)
	}
	return fmt.Sprintf("%*d", w, n)
}

func numberWidth(f File, width int) (int, bool) {
	if width > 0 && width < 60 {
		return 0, false
	}
	if width == 0 {
		return 0, false
	}
	maxN := 1
	for _, h := range f.Hunks {
		if end := h.OldStart + h.OldCount; end > maxN {
			maxN = end
		}
		if end := h.NewStart + h.NewCount; end > maxN {
			maxN = end
		}
	}
	w := len(fmt.Sprintf("%d", maxN))
	if w < 1 {
		w = 1
	}
	return w, true
}

func lineSpans(kind byte, text, peer string, opt Options) []style.Span {
	tone := style.ToneNone
	marker := " "
	switch kind {
	case '+':
		tone = style.ToneGood
		marker = "+"
	case '-':
		tone = style.ToneBad
		marker = "-"
	}
	prefix := style.Span{Text: marker, Tone: tone}
	if opt.Highlight && peer != "" && (kind == '+' || kind == '-') {
		old, newS, ok := ChangedSpans(peerText(kind, text, peer), selfText(kind, text, peer))
		if ok {
			var word []style.Span
			if kind == '-' {
				word = old
			} else {
				word = newS
			}
			for i := range word {
				word[i].Tone = tone
				if word[i].Inverse && opt.Style.Bold {
					word[i].Bold = true
				}
			}
			return append([]style.Span{prefix}, word...)
		}
	}
	return []style.Span{prefix, {Text: text, Tone: tone}}
}

func peerText(kind byte, text, peer string) string {
	if kind == '-' {
		return text
	}
	return peer
}

func selfText(kind byte, text, peer string) string {
	if kind == '+' {
		return text
	}
	return peer
}

func renderSideBySide(f File, opt Options) []style.Line {
	width := opt.Width
	if width <= 0 {
		width = 160
	}
	numW, _ := numberWidth(f, 80) // always number on a wide layout
	if numW < 1 {
		numW = 1
	}
	gutter := " │ "
	contentW := (width - style.Width(gutter) - 2*(numW+1)) / 2
	if contentW < 8 {
		contentW = 8
	}
	var lines []style.Line
	for _, h := range f.Hunks {
		lines = append(lines, hunkHeader(h))
		for _, row := range pairHunk(h) {
			left := sideSpans(row.leftKind, row.leftText, row.leftNum, row.leftOK, row.rightText, numW, contentW, opt)
			right := sideSpans(row.rightKind, row.rightText, row.rightNum, row.rightOK, row.leftText, numW, contentW, opt)
			spans := append(left, style.Span{Text: gutter, Tone: style.ToneMuted})
			spans = append(spans, right...)
			lines = append(lines, style.Line{Spans: spans})
		}
	}
	return lines
}

type paired struct {
	leftKind, rightKind byte
	leftText, rightText string
	leftNum, rightNum   int
	leftOK, rightOK     bool
}

func pairHunk(h Hunk) []paired {
	var rows []paired
	oldN, newN := h.OldStart, h.NewStart
	lines := h.Lines
	for i := 0; i < len(lines); {
		ln := lines[i]
		if ln.Kind == '\\' {
			i++
			continue
		}
		if ln.Kind == ' ' {
			rows = append(rows, paired{
				leftKind: ' ', rightKind: ' ',
				leftText: ln.Text, rightText: ln.Text,
				leftNum: oldN, rightNum: newN, leftOK: true, rightOK: true,
			})
			oldN++
			newN++
			i++
			continue
		}
		var dels, adds []Line
		for i < len(lines) && lines[i].Kind == '-' {
			dels = append(dels, lines[i])
			i++
		}
		for i < len(lines) && lines[i].Kind == '+' {
			adds = append(adds, lines[i])
			i++
		}
		n := len(dels)
		if len(adds) > n {
			n = len(adds)
		}
		for k := 0; k < n; k++ {
			var row paired
			if k < len(dels) {
				row.leftKind = '-'
				row.leftText = dels[k].Text
				row.leftNum = oldN
				row.leftOK = true
				oldN++
			}
			if k < len(adds) {
				row.rightKind = '+'
				row.rightText = adds[k].Text
				row.rightNum = newN
				row.rightOK = true
				newN++
			}
			rows = append(rows, row)
		}
	}
	return rows
}

func sideSpans(kind byte, text string, num int, ok bool, peer string, numW, contentW int, opt Options) []style.Span {
	numText := fmtNum(num, ok, numW)
	if !ok {
		kind = ' '
		text = ""
		peer = ""
	}
	body := lineSpans(kind, text, peer, opt)
	body = style.TruncateSpans(body, contentW)
	return append([]style.Span{{Text: numText + " ", Tone: style.ToneMuted}}, body...)
}
