package fields

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"prtool/internal/style"
)

const emdash = "—"

// Format renders one field value. listMode flattens bodies, files, reviews,
// and checks. The returned spans are not bold; the renderer sets that from --bold.
func Format(name string, value any, listMode bool) style.Block {
	if isEmpty(value) && name != "additions" && name != "deletions" {
		return dashBlock()
	}
	switch name {
	case "state":
		return one(formatState(scalar(value)))
	case "reviewDecision":
		return one(formatReviewDecision(scalar(value)))
	case "mergeable":
		return one(formatMergeable(scalar(value)))
	case "mergeStateStatus":
		return one(formatMergeStatus(scalar(value)))
	case "isDraft":
		return one(formatYesNo(value, style.ToneWarn, style.ToneMuted))
	case "closed":
		return one(formatYesNo(value, style.ToneBad, style.ToneMuted))
	case "additions":
		return one(formatSignedCount(value, "+", style.ToneGood))
	case "deletions":
		return one(formatSignedCount(value, "-", style.ToneBad))
	case "author", "mergedBy":
		return one(style.Span{Text: formatPerson(value)})
	case "assignees":
		return one(style.Span{Text: formatPeopleList(value)})
	case "reviewers":
		return one(style.Span{Text: formatReviewers(value)})
	case "labels":
		return formatLabels(value)
	case "files":
		return formatFiles(value, listMode)
	case "comments":
		return formatComments(value, listMode)
	case "reviews", "latestReviews":
		return formatReviews(value, listMode)
	case "statusCheckRollup":
		return formatChecks(value, listMode)
	case "commits":
		return formatCommits(value, listMode)
	case "body":
		return formatBody(value, listMode)
	case "closingIssuesReferences":
		return formatIssues(value, listMode)
	case "reactionGroups":
		return formatReactions(value, listMode)
	case "autoMergeRequest":
		return one(style.Span{Text: formatAutoMerge(value)})
	case "mergeCommit", "potentialMergeCommit":
		return one(style.Span{Text: formatOID(value), Tone: style.ToneInfo})
	case "headRepository":
		return one(style.Span{Text: formatRepo(value)})
	case "headRepositoryOwner":
		return one(style.Span{Text: formatPerson(value)})
	case "milestone":
		return one(style.Span{Text: formatMilestone(value)})
	case "url":
		return one(style.Span{Text: scalar(value), Tone: style.ToneInfo})
	case "title":
		return one(style.Span{Text: scalar(value)})
	default:
		return formatGenericBlock(value, listMode)
	}
}

func one(sp style.Span) style.Block {
	if strings.TrimSpace(sp.Text) == "" {
		return dashBlock()
	}
	return style.Block{Lines: []style.Line{{Spans: []style.Span{sp}}}}
}

func dashBlock() style.Block {
	return style.Block{Lines: []style.Line{{Spans: []style.Span{{Text: emdash, Tone: style.ToneMuted}}}}}
}

func plainLines(text string) style.Block {
	if text == "" {
		return dashBlock()
	}
	parts := strings.Split(text, "\n")
	var lines []style.Line
	for _, p := range parts {
		lines = append(lines, style.Line{Spans: []style.Span{{Text: p}}})
	}
	return style.Block{Lines: lines}
}

func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) == ""
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	default:
		return false
	}
}

func scalar(v any) string {
	s, ok := scalarString(v)
	if !ok {
		return formatGeneric(v, 0, true)
	}
	return s
}

func scalarString(v any) (string, bool) {
	switch t := v.(type) {
	case nil:
		return "", true
	case string:
		if ts, ok := formatTime(t); ok {
			return ts, true
		}
		return t, true
	case bool:
		if t {
			return "yes", true
		}
		return "no", true
	case float64:
		return formatNumber(t), true
	case int:
		return strconv.Itoa(t), true
	case int64:
		return strconv.FormatInt(t, 10), true
	case jsonNumber:
		return string(t), true
	default:
		return "", false
	}
}

// jsonNumber is a tiny alias so tests can pass encoding/json.Number-like values
// without importing encoding/json here. Real JSON numbers arrive as float64.
type jsonNumber string

func formatNumber(f float64) string {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	if f == math.Trunc(f) && f <= math.MaxInt64 && f >= math.MinInt64 {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func formatTime(s string) (string, bool) {
	if len(s) < 20 || s[4] != '-' || s[10] != 'T' {
		return "", false
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, err = time.Parse(time.RFC3339, s)
		if err != nil {
			return "", false
		}
	}
	return t.UTC().Format("2006-01-02 15:04 UTC"), true
}

func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func asSlice(v any) ([]any, bool) {
	s, ok := v.([]any)
	return s, ok
}

func asInt(v any) (int, bool) {
	switch t := v.(type) {
	case float64:
		return int(t), true
	case int:
		return t, true
	case int64:
		return int(t), true
	case string:
		n, err := strconv.Atoi(t)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func mapStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := str(m[k]); s != "" {
			return s
		}
	}
	return ""
}

func formatState(s string) style.Span {
	up := strings.ToUpper(s)
	tone := style.ToneNone
	switch up {
	case "OPEN":
		tone = style.ToneGood
	case "MERGED":
		tone = style.ToneAccent
	case "CLOSED":
		tone = style.ToneBad
	case "DRAFT":
		tone = style.ToneWarn
	}
	return style.Span{Text: up, Tone: tone}
}

func formatReviewDecision(s string) style.Span {
	up := strings.ToUpper(s)
	tone := style.ToneNone
	switch up {
	case "APPROVED":
		tone = style.ToneAccent
	case "CHANGES_REQUESTED":
		tone = style.ToneWarn
	case "REVIEW_REQUIRED":
		tone = style.ToneMuted
	case "COMMENTED":
		tone = style.ToneInfo
	case "DISMISSED":
		tone = style.ToneMuted
	}
	return style.Span{Text: up, Tone: tone}
}

func formatMergeable(s string) style.Span {
	up := strings.ToUpper(s)
	tone := style.ToneMuted
	switch up {
	case "MERGEABLE":
		tone = style.ToneGood
	case "CONFLICTING":
		tone = style.ToneBad
	}
	return style.Span{Text: up, Tone: tone}
}

func formatMergeStatus(s string) style.Span {
	up := strings.ToUpper(s)
	tone := style.ToneNone
	switch up {
	case "CLEAN", "MERGEABLE", "CAN_BE_MERGED", "CI_STILL_PASSING":
		tone = style.ToneGood
	case "DIRTY", "CONFLICTING", "CANNOT_BE_MERGED":
		tone = style.ToneBad
	case "BLOCKED", "BEHIND", "UNSTABLE", "DRAFT", "UNCHECKED", "HAS_HOOKS":
		tone = style.ToneWarn
	case "UNKNOWN", "":
		tone = style.ToneMuted
	}
	if up == "" {
		return style.Span{Text: emdash, Tone: style.ToneMuted}
	}
	return style.Span{Text: up, Tone: tone}
}

func formatYesNo(v any, yes, no style.Tone) style.Span {
	b, ok := v.(bool)
	if !ok {
		s := strings.ToLower(scalar(v))
		b = s == "yes" || s == "true"
		if s != "yes" && s != "no" && s != "true" && s != "false" {
			return style.Span{Text: scalar(v)}
		}
	}
	if b {
		return style.Span{Text: "yes", Tone: yes}
	}
	return style.Span{Text: "no", Tone: no}
}

func formatSignedCount(v any, sign string, tone style.Tone) style.Span {
	if isEmpty(v) {
		return style.Span{Text: sign + "0", Tone: style.ToneMuted}
	}
	n, ok := asInt(v)
	if !ok {
		return style.Span{Text: sign + scalar(v), Tone: tone}
	}
	if n == 0 {
		tone = style.ToneMuted
	}
	return style.Span{Text: fmt.Sprintf("%s%d", sign, n), Tone: tone}
}

func formatPerson(v any) string {
	m, ok := asMap(v)
	if !ok {
		s := scalar(v)
		if s == "" {
			return emdash
		}
		return s
	}
	login := mapStr(m, "login", "username")
	name := str(m["name"])
	s := login
	if s == "" {
		s = name
	} else if name != "" && name != login {
		s = login + " (" + name + ")"
	}
	if b, ok := m["is_bot"].(bool); ok && b {
		s += " [bot]"
	}
	if s == "" {
		return emdash
	}
	return s
}

func formatPeopleList(v any) string {
	items, ok := asSlice(v)
	if !ok {
		return formatPerson(v)
	}
	var parts []string
	for _, item := range items {
		p := formatPerson(item)
		if p != "" && p != emdash {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return emdash
	}
	return strings.Join(parts, ", ")
}

func formatReviewers(v any) string {
	items, ok := asSlice(v)
	if !ok {
		return formatPerson(v)
	}
	var parts []string
	for _, item := range items {
		m, ok := asMap(item)
		if !ok {
			parts = append(parts, scalar(item))
			continue
		}
		if login := mapStr(m, "login", "username"); login != "" {
			parts = append(parts, login)
			continue
		}
		if name := str(m["name"]); name != "" {
			parts = append(parts, name)
			continue
		}
		// Requested reviewer wrappers: {login} user or {name} team, sometimes nested.
		if u, ok := asMap(m["user"]); ok {
			parts = append(parts, formatPerson(u))
			continue
		}
		if tm, ok := asMap(m["team"]); ok {
			if name := str(tm["name"]); name != "" {
				parts = append(parts, name)
			}
			continue
		}
		parts = append(parts, formatGeneric(item, 1, true))
	}
	if len(parts) == 0 {
		return emdash
	}
	return strings.Join(parts, ", ")
}

func formatLabels(v any) style.Block {
	items, ok := asSlice(v)
	if !ok {
		return one(style.Span{Text: scalar(v)})
	}
	var spans []style.Span
	for _, item := range items {
		name := ""
		var rgb *style.RGB
		switch t := item.(type) {
		case string:
			name = t
		case map[string]any:
			name = mapStr(t, "name", "title")
			if c := mapStr(t, "color"); c != "" {
				if parsed, ok := style.ParseHex(c); ok {
					lifted := style.Lift(parsed)
					rgb = &lifted
				}
			}
		}
		if name == "" {
			continue
		}
		if len(spans) > 0 {
			spans = append(spans, style.Span{Text: " "})
		}
		spans = append(spans, style.Span{Text: name, RGB: rgb})
	}
	if len(spans) == 0 {
		return dashBlock()
	}
	return style.Block{Lines: []style.Line{{Spans: spans}}}
}

func formatFiles(v any, listMode bool) style.Block {
	items, ok := asSlice(v)
	if !ok {
		return plainLines(scalar(v))
	}
	if listMode {
		if len(items) == 1 {
			return one(style.Span{Text: filePath(items[0])})
		}
		return one(style.Span{Text: fmt.Sprintf("%d files", len(items))})
	}
	var lines []style.Line
	for _, item := range items {
		lines = append(lines, fileLine(item))
	}
	if len(lines) == 0 {
		return dashBlock()
	}
	return style.Block{Lines: lines}
}

func filePath(v any) string {
	m, ok := asMap(v)
	if !ok {
		return scalar(v)
	}
	if p := mapStr(m, "path", "name"); p != "" {
		return p
	}
	return formatGeneric(v, 1, true)
}

func fileLine(v any) style.Line {
	m, ok := asMap(v)
	if !ok {
		return style.Line{Spans: []style.Span{{Text: scalar(v)}}}
	}
	path := filePath(m)
	add, _ := asInt(m["additions"])
	del, _ := asInt(m["deletions"])
	kind := strings.ToUpper(mapStr(m, "changeType"))
	spans := []style.Span{{Text: path, Tone: style.ToneInfo, Bold: true}}
	spans = append(spans, style.Span{Text: "  "})
	addTone := style.ToneGood
	if add == 0 {
		addTone = style.ToneMuted
	}
	delTone := style.ToneBad
	if del == 0 {
		delTone = style.ToneMuted
	}
	spans = append(spans,
		style.Span{Text: fmt.Sprintf("+%d", add), Tone: addTone},
		style.Span{Text: "  "},
		style.Span{Text: fmt.Sprintf("-%d", del), Tone: delTone},
	)
	if kind != "" {
		spans = append(spans, style.Span{Text: "  "}, style.Span{Text: kind, Tone: changeTone(kind)})
	}
	return style.Line{Spans: spans}
}

func changeTone(kind string) style.Tone {
	switch kind {
	case "ADDED":
		return style.ToneGood
	case "DELETED":
		return style.ToneBad
	case "RENAMED", "COPIED":
		return style.ToneAccent
	default:
		return style.ToneNone
	}
}

func formatComments(v any, listMode bool) style.Block {
	if n, ok := asInt(v); ok {
		return one(style.Span{Text: strconv.Itoa(n)})
	}
	items, ok := asSlice(v)
	if !ok {
		return plainLines(scalar(v))
	}
	if listMode {
		return one(style.Span{Text: strconv.Itoa(len(items))})
	}
	var lines []style.Line
	for _, item := range items {
		m, ok := asMap(item)
		if !ok {
			lines = append(lines, style.Line{Spans: []style.Span{{Text: scalar(item)}}})
			continue
		}
		who := formatPerson(m["author"])
		when := ""
		if ts, ok := formatTime(str(m["createdAt"])); ok {
			when = ts
		} else if ts, ok := formatTime(str(m["created_at"])); ok {
			when = ts
		}
		head := who
		if when != "" {
			head += "  " + when
		}
		lines = append(lines, style.Line{Spans: []style.Span{{Text: head, Tone: style.ToneMuted}}})
		body := str(m["body"])
		if body == "" {
			body = str(m["bodyText"])
		}
		if body != "" {
			for _, part := range strings.Split(body, "\n") {
				lines = append(lines, style.Line{Spans: []style.Span{{Text: part}}})
			}
		}
	}
	if len(lines) == 0 {
		return dashBlock()
	}
	return style.Block{Lines: lines}
}

func formatReviews(v any, listMode bool) style.Block {
	items, ok := asSlice(v)
	if !ok {
		return plainLines(scalar(v))
	}
	if listMode {
		return one(style.Span{Text: reviewSummary(items)})
	}
	var lines []style.Line
	for _, item := range items {
		m, ok := asMap(item)
		if !ok {
			continue
		}
		who := formatPerson(m["author"])
		state := strings.ToUpper(mapStr(m, "state"))
		when := ""
		if ts, ok := formatTime(mapStr(m, "submittedAt", "submitted_at", "createdAt")); ok {
			when = ts
		}
		spans := []style.Span{{Text: who}}
		if state != "" {
			spans = append(spans, style.Span{Text: "  "}, formatReviewDecision(state))
		}
		if when != "" {
			spans = append(spans, style.Span{Text: "  " + when, Tone: style.ToneMuted})
		}
		lines = append(lines, style.Line{Spans: spans})
		if body := str(m["body"]); body != "" {
			for _, part := range strings.Split(body, "\n") {
				lines = append(lines, style.Line{Spans: []style.Span{{Text: part}}})
			}
		}
	}
	if len(lines) == 0 {
		return dashBlock()
	}
	return style.Block{Lines: lines}
}

func reviewSummary(items []any) string {
	counts := map[string]int{}
	order := []string{"APPROVED", "CHANGES_REQUESTED", "COMMENTED", "DISMISSED"}
	for _, item := range items {
		m, ok := asMap(item)
		if !ok {
			continue
		}
		counts[strings.ToUpper(mapStr(m, "state"))]++
	}
	var parts []string
	for _, state := range order {
		n := counts[state]
		if n == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%d %s", n, reviewWord(state, n)))
	}
	if len(parts) == 0 {
		return emdash
	}
	return strings.Join(parts, ", ")
}

func reviewWord(state string, n int) string {
	switch state {
	case "APPROVED":
		if n == 1 {
			return "approved"
		}
		return "approved"
	case "CHANGES_REQUESTED":
		if n == 1 {
			return "changes requested"
		}
		return "changes requested"
	case "COMMENTED":
		if n == 1 {
			return "commented"
		}
		return "commented"
	case "DISMISSED":
		if n == 1 {
			return "dismissed"
		}
		return "dismissed"
	default:
		return strings.ToLower(state)
	}
}

func formatChecks(v any, listMode bool) style.Block {
	items, ok := asSlice(v)
	if !ok {
		return plainLines(scalar(v))
	}
	if listMode {
		return one(style.Span{Text: checkSummary(items)})
	}
	var lines []style.Line
	for _, item := range items {
		m, ok := asMap(item)
		if !ok {
			continue
		}
		name := mapStr(m, "name", "context")
		result := strings.ToUpper(mapStr(m, "conclusion", "state"))
		if result == "" {
			result = strings.ToUpper(str(m["status"]))
		}
		spans := []style.Span{{Text: name}}
		if result != "" {
			spans = append(spans, style.Span{Text: "  "}, style.Span{Text: result, Tone: checkTone(result)})
		}
		lines = append(lines, style.Line{Spans: spans})
	}
	if len(lines) == 0 {
		return dashBlock()
	}
	return style.Block{Lines: lines}
}

func checkSummary(items []any) string {
	var passed, failed, pending, skipped int
	for _, item := range items {
		m, ok := asMap(item)
		if !ok {
			continue
		}
		result := strings.ToUpper(mapStr(m, "conclusion", "state"))
		switch result {
		case "SUCCESS", "NEUTRAL":
			passed++
		case "FAILURE", "ERROR", "TIMED_OUT", "CANCELLED", "STARTUP_FAILURE", "ACTION_REQUIRED":
			failed++
		case "SKIPPED":
			skipped++
		case "PENDING", "EXPECTED", "QUEUED", "IN_PROGRESS", "":
			if result == "" && strings.ToUpper(str(m["status"])) == "COMPLETED" {
				passed++
			} else {
				pending++
			}
		default:
			pending++
		}
	}
	var parts []string
	if passed > 0 {
		parts = append(parts, fmt.Sprintf("%d passed", passed))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	if pending > 0 {
		parts = append(parts, fmt.Sprintf("%d pending", pending))
	}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skipped))
	}
	if len(parts) == 0 {
		return emdash
	}
	return strings.Join(parts, ", ")
}

func checkTone(result string) style.Tone {
	switch result {
	case "SUCCESS", "NEUTRAL", "COMPLETED":
		return style.ToneGood
	case "FAILURE", "ERROR", "TIMED_OUT", "CANCELLED", "STARTUP_FAILURE", "ACTION_REQUIRED":
		return style.ToneBad
	case "PENDING", "EXPECTED", "QUEUED", "IN_PROGRESS", "SKIPPED":
		return style.ToneWarn
	default:
		return style.ToneMuted
	}
}

func formatCommits(v any, listMode bool) style.Block {
	items, ok := asSlice(v)
	if !ok {
		return plainLines(scalar(v))
	}
	if listMode {
		word := "commits"
		if len(items) == 1 {
			word = "commit"
		}
		return one(style.Span{Text: fmt.Sprintf("%d %s", len(items), word)})
	}
	var lines []style.Line
	for _, item := range items {
		m, ok := asMap(item)
		if !ok {
			continue
		}
		oid := mapStr(m, "oid", "sha", "id")
		if len(oid) > 7 {
			oid = oid[:7]
		}
		msg := mapStr(m, "messageHeadline", "message", "title")
		if i := strings.IndexByte(msg, '\n'); i >= 0 {
			msg = msg[:i]
		}
		spans := []style.Span{{Text: oid, Tone: style.ToneInfo}}
		if msg != "" {
			spans = append(spans, style.Span{Text: "  " + msg})
		}
		lines = append(lines, style.Line{Spans: spans})
	}
	if len(lines) == 0 {
		return dashBlock()
	}
	return style.Block{Lines: lines}
}

func formatBody(v any, listMode bool) style.Block {
	text := str(v)
	if text == "" {
		text = scalar(v)
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if listMode {
		line := text
		if i := strings.IndexByte(line, '\n'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return dashBlock()
		}
		return one(style.Span{Text: line})
	}
	return plainLines(strings.TrimRight(text, "\n"))
}

func formatIssues(v any, listMode bool) style.Block {
	items, ok := asSlice(v)
	if !ok {
		return plainLines(scalar(v))
	}
	var parts []string
	for _, item := range items {
		m, ok := asMap(item)
		if !ok {
			parts = append(parts, scalar(item))
			continue
		}
		n, _ := asInt(m["number"])
		title := str(m["title"])
		s := fmt.Sprintf("#%d", n)
		if title != "" {
			s += " " + title
		}
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		return dashBlock()
	}
	if listMode {
		return one(style.Span{Text: strings.Join(parts, ", ")})
	}
	return plainLines(strings.Join(parts, "\n"))
}

func formatReactions(v any, listMode bool) style.Block {
	items, ok := asSlice(v)
	if !ok {
		return plainLines(scalar(v))
	}
	var parts []string
	for _, item := range items {
		m, ok := asMap(item)
		if !ok {
			continue
		}
		content := mapStr(m, "content")
		total := 0
		if users, ok := asMap(m["users"]); ok {
			total, _ = asInt(users["totalCount"])
		} else {
			total, _ = asInt(m["totalCount"])
		}
		if content == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %d", content, total))
	}
	if len(parts) == 0 {
		return dashBlock()
	}
	if listMode {
		return one(style.Span{Text: strings.Join(parts, ", ")})
	}
	return plainLines(strings.Join(parts, "\n"))
}

func formatAutoMerge(v any) string {
	m, ok := asMap(v)
	if !ok {
		return emdash
	}
	method := mapStr(m, "mergeMethod", "merge_method")
	who := ""
	if by, ok := asMap(m["enabledBy"]); ok {
		who = formatPerson(by)
	} else if by, ok := asMap(m["enabled_by"]); ok {
		who = formatPerson(by)
	}
	if method == "" && (who == "" || who == emdash) {
		return emdash
	}
	method = strings.ToUpper(method)
	if who != "" && who != emdash {
		return method + " by " + who
	}
	return method
}

func formatOID(v any) string {
	if m, ok := asMap(v); ok {
		oid := mapStr(m, "oid", "sha")
		if len(oid) > 7 {
			return oid[:7]
		}
		if oid == "" {
			return emdash
		}
		return oid
	}
	s := scalar(v)
	if len(s) > 7 && !strings.Contains(s, " ") {
		return s[:7]
	}
	if s == "" {
		return emdash
	}
	return s
}

func formatRepo(v any) string {
	m, ok := asMap(v)
	if !ok {
		return scalar(v)
	}
	if s := mapStr(m, "nameWithOwner", "full_path", "path_with_namespace"); s != "" {
		return s
	}
	return mapStr(m, "name")
}

func formatMilestone(v any) string {
	m, ok := asMap(v)
	if !ok {
		return scalar(v)
	}
	if s := mapStr(m, "title", "name"); s != "" {
		return s
	}
	return formatGeneric(v, 1, true)
}

func formatGenericBlock(v any, listMode bool) style.Block {
	text := formatGeneric(v, 0, listMode)
	if text == "" {
		return dashBlock()
	}
	return plainLines(text)
}

func formatGeneric(v any, depth int, listMode bool) string {
	if depth >= 4 {
		return "{…}"
	}
	if s, ok := scalarString(v); ok {
		return s
	}
	if m, ok := asMap(v); ok {
		if _, has := m["login"]; has {
			return formatPerson(m)
		}
		if _, has := m["username"]; has {
			return formatPerson(m)
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			if strings.HasPrefix(k, "__") {
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) == 0 {
			return emdash
		}
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, k+"="+formatGeneric(m[k], depth+1, listMode))
		}
		return strings.Join(parts, " ")
	}
	if items, ok := asSlice(v); ok {
		parts := make([]string, 0, len(items))
		for _, item := range items {
			parts = append(parts, formatGeneric(item, depth+1, listMode))
		}
		sep := ", "
		if !listMode {
			sep = "\n"
		}
		return strings.Join(parts, sep)
	}
	return fmt.Sprint(v)
}
