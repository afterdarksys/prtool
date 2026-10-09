// Package fields names the columns prtool can show and checks a --fields list.
package fields

import (
	"fmt"
	"strings"
)

// Catalog is every user-facing field, in a stable order. reviewers is the
// cross-host name; GitHub's JSON field is reviewRequests.
var Catalog = []string{
	"additions",
	"assignees",
	"author",
	"autoMergeRequest",
	"baseRefName",
	"baseRefOid",
	"body",
	"changedFiles",
	"closed",
	"closedAt",
	"closingIssuesReferences",
	"comments",
	"commits",
	"createdAt",
	"deletions",
	"files",
	"fullDatabaseId",
	"headRefName",
	"headRefOid",
	"headRepository",
	"headRepositoryOwner",
	"id",
	"isCrossRepository",
	"isDraft",
	"labels",
	"latestReviews",
	"maintainerCanModify",
	"mergeCommit",
	"mergeStateStatus",
	"mergeable",
	"mergedAt",
	"mergedBy",
	"milestone",
	"number",
	"potentialMergeCommit",
	"projectCards",
	"projectItems",
	"reactionGroups",
	"reviewDecision",
	"reviewers",
	"reviews",
	"state",
	"statusCheckRollup",
	"title",
	"updatedAt",
	"url",
}

var githubViewDefault = []string{
	"title", "author", "state", "isDraft", "reviewDecision",
	"baseRefName", "headRefName", "additions", "deletions", "changedFiles", "url",
}

var githubListDefault = []string{
	"number", "title", "author", "state", "reviewDecision",
	"additions", "deletions", "changedFiles",
}

var gitlabViewDefault = []string{
	"title", "author", "state", "isDraft",
	"baseRefName", "headRefName", "mergeable", "labels", "url",
}

var gitlabListDefault = []string{
	"title", "number", "author", "state", "mergeable", "labels",
}

// known is the catalog plus the GitHub wire name, so a suggestion can mention it.
var knownSet = func() map[string]bool {
	m := make(map[string]bool, len(Catalog)+1)
	for _, n := range Catalog {
		m[n] = true
	}
	return m
}()

// Defaults returns the columns used when --fields is omitted.
func Defaults(gitlab, list bool) []string {
	switch {
	case gitlab && list:
		return append([]string{}, gitlabListDefault...)
	case gitlab:
		return append([]string{}, gitlabViewDefault...)
	case list:
		return append([]string{}, githubListDefault...)
	default:
		return append([]string{}, githubViewDefault...)
	}
}

// ParseSpec splits a comma-separated --fields value. "all" expands to Catalog.
// Names are trimmed and de-duplicated. An empty spec is an error.
func ParseSpec(spec string) ([]string, error) {
	if strings.TrimSpace(spec) == "" {
		return nil, fmt.Errorf("empty --fields")
	}
	raw := strings.Split(spec, ",")
	var names []string
	all := false
	seen := map[string]bool{}
	for _, part := range raw {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		if name == "all" {
			all = true
			continue
		}
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	if all {
		names = append([]string{}, Catalog...)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("empty --fields")
	}
	if err := validateKnown(names); err != nil {
		return nil, err
	}
	return names, nil
}

// ValidateMode rejects fields a host cannot fill for this command.
func ValidateMode(names []string, gitlab, list bool) error {
	if err := validateKnown(names); err != nil {
		return err
	}
	if gitlab && list {
		for _, name := range names {
			switch name {
			case "additions", "deletions", "files":
				return fmt.Errorf("field %q is not available in GitLab list; use prtool view", name)
			}
		}
	}
	return nil
}

func validateKnown(names []string) error {
	for _, name := range names {
		if knownSet[name] {
			continue
		}
		msg := fmt.Sprintf("unknown field %q", name)
		if sug := suggest(name); sug != "" {
			msg += fmt.Sprintf("; did you mean %q?", sug)
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func suggest(name string) string {
	best := ""
	bestD := 4 // threshold is distance <= 3
	for _, cand := range Catalog {
		d := levenshtein(strings.ToLower(name), strings.ToLower(cand))
		if d < bestD {
			bestD = d
			best = cand
		}
	}
	return best
}

func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	if abs(len(a)-len(b)) > 3 {
		return 99
	}
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func min(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

// GitHubJSONFields maps user-facing names onto the gh --json list.
func GitHubJSONFields(names []string) string {
	out := make([]string, len(names))
	for i, name := range names {
		if name == "reviewers" {
			name = "reviewRequests"
		}
		out[i] = name
	}
	return strings.Join(out, ",")
}

// ShortHeader is the list-table heading for a field.
func ShortHeader(name string) string {
	switch name {
	case "number":
		return "#"
	case "additions":
		return "+"
	case "deletions":
		return "-"
	case "changedFiles":
		return "files"
	case "reviewDecision":
		return "review"
	case "baseRefName":
		return "base"
	case "headRefName":
		return "head"
	case "mergeable":
		return "merge"
	case "mergeStateStatus":
		return "status"
	case "isDraft":
		return "draft"
	default:
		return name
	}
}

// NumericColumn reports whether a list column is right-aligned.
func NumericColumn(name string) bool {
	switch name {
	case "number", "additions", "deletions", "changedFiles", "comments", "fullDatabaseId":
		return true
	default:
		return false
	}
}

// FlexibleColumn is truncated first when the table is wider than the terminal.
func FlexibleColumn(name string) bool {
	switch name {
	case "title", "body", "url", "labels":
		return true
	default:
		return false
	}
}
