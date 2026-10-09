// Package diffview parses a unified diff and paints it.
package diffview

import (
	"regexp"
	"strings"

	"prtool/internal/style"
)

// File is one diff --git section.
type File struct {
	OldPath    string
	NewPath    string
	ChangeType string
	OldMode    string
	NewMode    string
	Binary     bool
	Additions  int
	Deletions  int
	Hunks      []Hunk
	Raw        string
}

// Hunk is one @@ region.
type Hunk struct {
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Context  string
	Lines    []Line
}

// Line is one line inside a hunk. Kind is ' ', '+', '-', or '\\'.
type Line struct {
	Kind byte
	Text string
}

// Diff is a parsed patch.
type Diff struct {
	Files []File
}

var hunkRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)$`)

// Parse reads a unified diff. ANSI and CR are stripped before a line is
// classified so a hostile sequence cannot hide a header. Binary patches are
// not counted as insertions.
func Parse(raw string) *Diff {
	_, chunks := splitChunks(raw)
	d := &Diff{}
	for _, chunk := range chunks {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		d.Files = append(d.Files, parseFile(chunk))
	}
	return d
}

func splitChunks(raw string) (preamble string, chunks []string) {
	if raw == "" {
		return "", nil
	}
	idx := 0
	if !strings.HasPrefix(raw, "diff --git ") {
		i := strings.Index(raw, "\ndiff --git ")
		if i < 0 {
			return raw, nil
		}
		preamble = raw[:i+1]
		idx = i + 1
	}
	rest := raw[idx:]
	parts := strings.Split(rest, "\ndiff --git ")
	for i, p := range parts {
		if strings.TrimSpace(p) == "" && i > 0 {
			continue
		}
		if i == 0 {
			chunks = append(chunks, p)
		} else {
			chunks = append(chunks, "diff --git "+p)
		}
	}
	return preamble, chunks
}

func parseFile(chunk string) File {
	f := File{Raw: chunk, ChangeType: "MODIFIED"}
	var (
		inHunk      bool
		oldDevNull  bool
		newDevNull  bool
		newFile     bool
		deletedFile bool
		renamed     bool
		copied      bool
		hunk        *Hunk
	)
	finish := func() {
		if hunk != nil {
			f.Hunks = append(f.Hunks, *hunk)
			hunk = nil
		}
	}
	for _, line := range strings.Split(chunk, "\n") {
		line = strings.TrimRight(line, "\r")
		line = style.StripControls(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "diff --git ") {
			if a, b, ok := parseGitHeader(line); ok {
				if a == "/dev/null" {
					oldDevNull = true
				} else {
					f.OldPath = a
				}
				if b == "/dev/null" {
					newDevNull = true
				} else {
					f.NewPath = b
				}
			}
			continue
		}
		if f.Binary {
			continue
		}
		if strings.HasPrefix(line, "GIT binary patch") || strings.HasPrefix(line, "Binary files ") {
			f.Binary = true
			finish()
			inHunk = false
			continue
		}
		if !inHunk {
			switch {
			case strings.HasPrefix(line, "new file mode "):
				f.NewMode = strings.TrimSpace(line[len("new file mode "):])
				newFile = true
				continue
			case strings.HasPrefix(line, "deleted file mode "):
				f.OldMode = strings.TrimSpace(line[len("deleted file mode "):])
				deletedFile = true
				continue
			case strings.HasPrefix(line, "old mode "):
				f.OldMode = strings.TrimSpace(line[len("old mode "):])
				continue
			case strings.HasPrefix(line, "new mode "):
				f.NewMode = strings.TrimSpace(line[len("new mode "):])
				continue
			case strings.HasPrefix(line, "rename from "):
				f.OldPath = cleanHeaderPath(line[len("rename from "):])
				renamed = true
				continue
			case strings.HasPrefix(line, "rename to "):
				f.NewPath = cleanHeaderPath(line[len("rename to "):])
				renamed = true
				continue
			case strings.HasPrefix(line, "copy from "):
				f.OldPath = cleanHeaderPath(line[len("copy from "):])
				copied = true
				continue
			case strings.HasPrefix(line, "copy to "):
				f.NewPath = cleanHeaderPath(line[len("copy to "):])
				copied = true
				continue
			case strings.HasPrefix(line, "--- "):
				p := headerPath(line[len("--- "):])
				if p == "/dev/null" {
					oldDevNull = true
				}
				continue
			case strings.HasPrefix(line, "+++ "):
				p := headerPath(line[len("+++ "):])
				if p == "/dev/null" {
					newDevNull = true
				}
				continue
			case strings.HasPrefix(line, "index ") || strings.HasPrefix(line, "similarity index ") || strings.HasPrefix(line, "dissimilarity index "):
				continue
			}
		}
		if m := hunkRe.FindStringSubmatch(line); m != nil {
			finish()
			inHunk = true
			hunk = &Hunk{
				OldStart: atoiDefault(m[1], 0),
				OldCount: atoiDefault(m[2], 1),
				NewStart: atoiDefault(m[3], 0),
				NewCount: atoiDefault(m[4], 1),
				Context:  strings.TrimPrefix(m[5], " "),
			}
			continue
		}
		if inHunk && hunk != nil {
			kind := line[0]
			if kind == '+' || kind == '-' || kind == ' ' || kind == '\\' {
				text := ""
				if len(line) > 1 {
					text = line[1:]
				}
				hunk.Lines = append(hunk.Lines, Line{Kind: kind, Text: text})
				switch kind {
				case '+':
					f.Additions++
				case '-':
					f.Deletions++
				}
				continue
			}
		}
	}
	finish()
	switch {
	case newFile || oldDevNull:
		f.ChangeType = "ADDED"
	case deletedFile || newDevNull:
		f.ChangeType = "DELETED"
	case renamed:
		f.ChangeType = "RENAMED"
	case copied:
		f.ChangeType = "COPIED"
	default:
		f.ChangeType = "MODIFIED"
	}
	return f
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func parseGitHeader(line string) (string, string, bool) {
	const p = "diff --git "
	rest := line[len(p):]
	a, rest, ok1 := readToken(rest)
	b, _, ok2 := readToken(rest)
	if !ok1 || !ok2 {
		return "", "", false
	}
	return stripAB(a), stripAB(b), true
}

func readToken(s string) (string, string, bool) {
	s = strings.TrimLeft(s, " ")
	if s == "" {
		return "", "", false
	}
	if s[0] != '"' {
		i := strings.IndexByte(s, ' ')
		if i < 0 {
			return s, "", true
		}
		return s[:i], s[i:], true
	}
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			default:
				b.WriteByte(s[i+1])
			}
			i++
			continue
		}
		if s[i] == '"' {
			return b.String(), s[i+1:], true
		}
		b.WriteByte(s[i])
	}
	return "", "", false
}

func stripAB(p string) string {
	if p == "/dev/null" {
		return p
	}
	if strings.HasPrefix(p, "a/") || strings.HasPrefix(p, "b/") {
		return p[2:]
	}
	return p
}

func headerPath(s string) string {
	if i := strings.IndexByte(s, '\t'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if s == "/dev/null" {
		return s
	}
	return stripAB(unquote(s))
}

func cleanHeaderPath(s string) string {
	return unquote(strings.TrimSpace(s))
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		tok, _, ok := readToken(s)
		if ok {
			return tok
		}
	}
	return s
}

// Path is the path to show for a file that was not renamed.
func (f File) Path() string {
	if f.NewPath != "" {
		return f.NewPath
	}
	return f.OldPath
}

// DisplayPath is old => new for renames and copies, otherwise Path.
func (f File) DisplayPath() string {
	if (f.ChangeType == "RENAMED" || f.ChangeType == "COPIED") && f.OldPath != "" && f.NewPath != "" {
		return f.OldPath + " => " + f.NewPath
	}
	return f.Path()
}

// Filter drops files whose old or new path matches any exclude glob.
// A pattern with no slash matches the base name. A pattern with a slash
// matches the full path, and ** crosses directories.
func Filter(d *Diff, globs []string) *Diff {
	if d == nil || len(globs) == 0 {
		return d
	}
	out := &Diff{}
	for _, f := range d.Files {
		if matchAny(globs, f.OldPath) || matchAny(globs, f.NewPath) {
			continue
		}
		out.Files = append(out.Files, f)
	}
	return out
}

// FilterRaw returns the original chunks that Filter would keep, so the patch
// stays applicable. A preamble before the first diff --git is preserved only
// when at least one file remains... actually preamble (diffstat junk) is
// dropped; only file chunks are returned.
func FilterRaw(raw string, globs []string) string {
	if len(globs) == 0 {
		return raw
	}
	_, chunks := splitChunks(raw)
	if len(chunks) == 0 {
		return raw
	}
	parsed := Parse(raw)
	if len(parsed.Files) != len(chunks) {
		// Fall back to filtering by parsed raw fields.
		var kept []string
		for _, f := range parsed.Files {
			if matchAny(globs, f.OldPath) || matchAny(globs, f.NewPath) {
				continue
			}
			kept = append(kept, f.Raw)
		}
		return joinChunks(kept)
	}
	var kept []string
	for i, f := range parsed.Files {
		if matchAny(globs, f.OldPath) || matchAny(globs, f.NewPath) {
			continue
		}
		kept = append(kept, chunks[i])
	}
	return joinChunks(kept)
}

func joinChunks(chunks []string) string {
	if len(chunks) == 0 {
		return ""
	}
	return strings.Join(chunks, "\n")
}

func matchAny(globs []string, path string) bool {
	if path == "" {
		return false
	}
	for _, g := range globs {
		if matchGlob(g, path) {
			return true
		}
	}
	return false
}

func matchGlob(pattern, path string) bool {
	if pattern == "" {
		return false
	}
	target := path
	if !strings.Contains(pattern, "/") {
		target = baseName(path)
	}
	re, err := regexp.Compile(globToRegexp(pattern))
	if err != nil {
		return false
	}
	return re.MatchString(target)
}

func baseName(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

func globToRegexp(pattern string) string {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '*' && i+1 < len(pattern) && pattern[i+1] == '*' {
			i++
			if i+1 < len(pattern) && pattern[i+1] == '/' {
				b.WriteString("(?:.*/)?")
				i++
			} else {
				b.WriteString(".*")
			}
			continue
		}
		switch pattern[i] {
		case '*':
			b.WriteString("[^/]*")
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
		}
	}
	b.WriteString("$")
	return b.String()
}
