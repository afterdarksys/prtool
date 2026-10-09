// Package render paints pull-request fields as a key/value view or a table.
// Keys stay plain. --bold is applied only to value spans.
package render

import (
	"strings"

	"prtool/internal/fields"
	"prtool/internal/style"
)

// KeyValue renders one object. Keys are padded to the same width, then two
// spaces, then the value. Later lines of a value are indented by that same
// amount. width 0 does not truncate.
func KeyValue(names []string, obj map[string]any, st style.Style, width int) string {
	if obj == nil {
		obj = map[string]any{}
	}
	keyWidth := 0
	for _, name := range names {
		if w := style.Width(name); w > keyWidth {
			keyWidth = w
		}
	}
	tone := style.ToneNone
	if st.Color {
		tone = style.ToneMuted
	}
	var lines []style.Line
	indent := strings.Repeat(" ", keyWidth+2)
	for _, name := range names {
		block := emphasize(st, fields.Format(name, obj[name], false))
		if len(block.Lines) == 0 {
			block = emphasize(st, fields.Format(name, nil, false))
		}
		for i, ln := range block.Lines {
			spans := ln.Spans
			if width > 0 {
				avail := width - keyWidth - 2
				if avail > 0 {
					spans = style.TruncateSpans(spans, avail)
				}
			}
			if i == 0 {
				lines = append(lines, style.Line{Spans: []style.Span{
					{Text: style.PadRight(name, keyWidth), Tone: tone},
					{Text: "  "},
				}})
				lines[len(lines)-1].Spans = append(lines[len(lines)-1].Spans, spans...)
				continue
			}
			lines = append(lines, style.Line{Spans: append([]style.Span{{Text: indent}}, spans...)})
		}
	}
	return style.PaintBlock(st, style.Block{Lines: lines})
}

// Table renders a list. Numeric columns are right-aligned. Flexible columns
// shrink only when width is positive. Columns are separated by two spaces.
func Table(names []string, rows []map[string]any, st style.Style, width int) string {
	if len(names) == 0 || len(rows) == 0 {
		return ""
	}
	body := make([][]cell, len(rows))
	colW := make([]int, len(names))
	for i, name := range names {
		colW[i] = style.Width(fields.ShortHeader(name))
	}
	for r, row := range rows {
		body[r] = make([]cell, len(names))
		for c, name := range names {
			body[r][c] = makeCell(name, row[name], st)
			if body[r][c].width > colW[c] {
				colW[c] = body[r][c].width
			}
		}
	}
	if width > 0 {
		shrink(names, colW, width)
	}
	lines := []style.Line{style.Line{Spans: joinCols(headerCols(names, colW, st.Color))}}
	for r := range rows {
		var cols [][]style.Span
		for c, name := range names {
			cols = append(cols, fitCell(body[r][c], colW[c], fields.NumericColumn(name)))
		}
		lines = append(lines, style.Line{Spans: joinCols(cols)})
	}
	return style.PaintBlock(st, style.Block{Lines: lines})
}

type cell struct {
	spans []style.Span
	width int
}

func makeCell(name string, value any, st style.Style) cell {
	block := emphasize(st, fields.Format(name, value, true))
	var spans []style.Span
	for i, ln := range block.Lines {
		if i > 0 {
			spans = append(spans, style.Span{Text: " "})
		}
		spans = append(spans, ln.Spans...)
	}
	return cell{spans: spans, width: style.Width(style.Plain(spans))}
}

func emphasize(st style.Style, b style.Block) style.Block {
	if !st.Bold {
		return b
	}
	for i := range b.Lines {
		for j := range b.Lines[i].Spans {
			b.Lines[i].Spans[j].Bold = true
		}
	}
	return b
}

func shrink(names []string, colW []int, width int) {
	gap := 0
	if len(colW) > 1 {
		gap = 2 * (len(colW) - 1)
	}
	total := gap
	for _, w := range colW {
		total += w
	}
	overflow := total - width
	for overflow > 0 {
		best, bestW := -1, 1
		for i, name := range names {
			if fields.FlexibleColumn(name) && colW[i] > bestW {
				best, bestW = i, colW[i]
			}
		}
		if best < 0 {
			return
		}
		colW[best]--
		overflow--
	}
}

func headerCols(names []string, colW []int, color bool) [][]style.Span {
	tone := style.ToneNone
	if color {
		tone = style.ToneMuted
	}
	cols := make([][]style.Span, len(names))
	for i, name := range names {
		text := fields.ShortHeader(name)
		w := style.Width(text)
		if w > colW[i] {
			text = style.Truncate(text, colW[i])
			w = style.Width(text)
		}
		pad := strings.Repeat(" ", max(0, colW[i]-w))
		if fields.NumericColumn(name) {
			cols[i] = []style.Span{{Text: pad + text, Tone: tone}}
		} else {
			cols[i] = []style.Span{{Text: text + pad, Tone: tone}}
		}
	}
	return cols
}

func fitCell(c cell, colW int, right bool) []style.Span {
	spans := c.spans
	w := c.width
	if w > colW {
		spans = style.TruncateSpans(spans, colW)
		w = style.Width(style.Plain(spans))
	}
	pad := strings.Repeat(" ", max(0, colW-w))
	if pad == "" {
		return spans
	}
	sp := style.Span{Text: pad}
	if right {
		return append([]style.Span{sp}, spans...)
	}
	return append(spans, sp)
}

func joinCols(cols [][]style.Span) []style.Span {
	var spans []style.Span
	for i, col := range cols {
		if i > 0 {
			spans = append(spans, style.Span{Text: "  "})
		}
		if i == len(cols)-1 {
			col = trimEnd(col)
		}
		spans = append(spans, col...)
	}
	return spans
}

// trimEnd removes trailing spaces on the last column. Right-align padding
// sits in front of the value, so it stays.
func trimEnd(col []style.Span) []style.Span {
	if len(col) == 0 {
		return col
	}
	col = append([]style.Span{}, col...)
	last := &col[len(col)-1]
	last.Text = strings.TrimRight(last.Text, " ")
	if last.Text == "" {
		col = col[:len(col)-1]
	}
	return col
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
