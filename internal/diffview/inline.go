package diffview

import "prtool/internal/style"

// token is a whitespace run or a non-space run. Offsets are bytes.
// Only ASCII space and tab count as whitespace.
type token struct {
	text  string
	space bool
}

const (
	inlineMaxTokens = 120
	inlineMaxCells  = 20000
)

// ChangedSpans marks the tokens that differ between two lines.
// ok is false when the lines are too long for the LCS and the caller should
// skip intra-line highlight. Identical lines return ok with no inverse spans.
func ChangedSpans(oldLine, newLine string) (oldSpans, newSpans []style.Span, ok bool) {
	if oldLine == newLine {
		return []style.Span{{Text: oldLine}}, []style.Span{{Text: newLine}}, true
	}
	a := tokenize(oldLine)
	b := tokenize(newLine)
	if len(a) > inlineMaxTokens || len(b) > inlineMaxTokens || len(a)*len(b) > inlineMaxCells {
		return nil, nil, false
	}
	keepA, keepB := lcsKeep(a, b)
	return spansFrom(a, keepA), spansFrom(b, keepB), true
}

func tokenize(s string) []token {
	var out []token
	i := 0
	for i < len(s) {
		space := s[i] == ' ' || s[i] == '\t'
		j := i + 1
		for j < len(s) && (s[j] == ' ' || s[j] == '\t') == space {
			j++
		}
		out = append(out, token{text: s[i:j], space: space})
		i = j
	}
	return out
}

func lcsKeep(a, b []token) (keepA, keepB []bool) {
	n, m := len(a), len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			if a[i-1].text == b[j-1].text {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}
	keepA = make([]bool, n)
	keepB = make([]bool, m)
	i, j := n, m
	for i > 0 && j > 0 {
		if a[i-1].text == b[j-1].text && dp[i][j] == dp[i-1][j-1]+1 {
			keepA[i-1] = true
			keepB[j-1] = true
			i--
			j--
			continue
		}
		if dp[i-1][j] >= dp[i][j-1] {
			i--
		} else {
			j--
		}
	}
	return keepA, keepB
}

func spansFrom(tokens []token, keep []bool) []style.Span {
	var spans []style.Span
	for i, t := range tokens {
		text := t.text
		inv := !keep[i]
		if inv && t.space {
			text = visualizeSpace(t.text)
		}
		if len(spans) > 0 && spans[len(spans)-1].Inverse == inv {
			spans[len(spans)-1].Text += text
			continue
		}
		spans = append(spans, style.Span{Text: text, Inverse: inv})
	}
	if len(spans) == 0 {
		return []style.Span{{Text: ""}}
	}
	return spans
}

func visualizeSpace(s string) string {
	var b []rune
	for i := 0; i < len(s); i++ {
		if s[i] == '\t' {
			b = append(b, '→')
		} else {
			b = append(b, '·')
		}
	}
	return string(b)
}
