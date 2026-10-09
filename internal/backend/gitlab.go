package backend

import (
	"context"
	"strconv"
	"strings"

	"prtool/internal/diffview"
	"prtool/internal/fields"
)

// GitLab reads merge requests through glab.
type GitLab struct {
	Runner Runner
}

func (g GitLab) tool() error {
	return requireTool(g.Runner, "glab", "glab is not installed (needed for GitLab merge requests)")
}

// View returns one merge request in the shared field vocabulary.
// Additions, deletions, and files come from the diff. When the request is only
// those stat fields, the merge request JSON is not fetched.
func (g GitLab) View(ctx context.Context, req Request) (map[string]any, error) {
	if err := g.tool(); err != nil {
		return nil, err
	}
	needDiff, onlyDiff := diffNeed(req.Fields)
	var obj map[string]any
	if !onlyDiff {
		args := []string{"mr", "view"}
		if req.Ref != "" {
			args = append(args, req.Ref)
		}
		args = append(args, "--output", "json")
		if req.Repo != "" {
			args = append(args, "--repo", req.Repo)
		}
		out, _, err := g.Runner.Run(ctx, "glab", args...)
		if err != nil {
			return nil, err
		}
		raw, err := decodeObject(out)
		if err != nil {
			return nil, err
		}
		obj = mapMR(raw)
	} else {
		obj = map[string]any{}
	}
	if needDiff {
		raw, err := g.Diff(ctx, req)
		if err != nil {
			return nil, err
		}
		applyDiff(obj, raw)
	}
	return obj, nil
}

// List returns merge requests. It does not fetch a diff per row.
func (g GitLab) List(ctx context.Context, req Request) ([]map[string]any, error) {
	if err := fields.ValidateMode(req.Fields, true, true); err != nil {
		return nil, err
	}
	if err := g.tool(); err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 30
	}
	args := []string{"mr", "list", "--output", "json", "--per-page", strconv.Itoa(limit)}
	switch req.State {
	case "merged":
		args = append(args, "--merged")
	case "closed":
		args = append(args, "--closed")
	case "all":
		args = append(args, "--all")
	}
	if req.Author != "" {
		args = append(args, "--author", req.Author)
	}
	if req.Assignee != "" {
		args = append(args, "--assignee", req.Assignee)
	}
	if req.Reviewer != "" {
		args = append(args, "--reviewer", req.Reviewer)
	}
	if req.Search != "" {
		args = append(args, "--search", req.Search)
	}
	if req.Head != "" {
		args = append(args, "--source-branch", req.Head)
	}
	if req.Base != "" {
		args = append(args, "--target-branch", req.Base)
	}
	if req.Repo != "" {
		args = append(args, "--repo", req.Repo)
	}
	for _, label := range req.Labels {
		args = append(args, "--label", label)
	}
	switch req.Draft {
	case "yes":
		args = append(args, "--draft")
	case "no":
		args = append(args, "--not-draft")
	}
	out, _, err := g.Runner.Run(ctx, "glab", args...)
	if err != nil {
		return nil, err
	}
	rows, err := decodeList(out)
	if err != nil {
		return nil, err
	}
	mapped := make([]map[string]any, len(rows))
	for i, row := range rows {
		mapped[i] = mapMR(row)
	}
	return mapped, nil
}

// Diff returns a colorless raw unified diff. Exclude globs are applied locally.
func (g GitLab) Diff(ctx context.Context, req Request) (string, error) {
	if err := g.tool(); err != nil {
		return "", err
	}
	args := []string{"mr", "diff"}
	if req.Ref != "" {
		args = append(args, req.Ref)
	}
	args = append(args, "--color=never", "--raw")
	if req.Repo != "" {
		args = append(args, "--repo", req.Repo)
	}
	out, _, err := g.Runner.Run(ctx, "glab", args...)
	if err != nil {
		return "", err
	}
	if len(req.Exclude) > 0 {
		out = diffview.FilterRaw(out, req.Exclude)
	}
	return out, nil
}

func diffNeed(names []string) (need, only bool) {
	if len(names) == 0 {
		return false, false
	}
	only = true
	for _, name := range names {
		switch name {
		case "additions", "deletions", "files", "changedFiles":
			need = true
		default:
			only = false
		}
	}
	if !need {
		only = false
	}
	return need, only
}

func applyDiff(obj map[string]any, raw string) {
	d := diffview.Parse(raw)
	var files []any
	add, del := 0, 0
	for _, f := range d.Files {
		add += f.Additions
		del += f.Deletions
		path := f.Path()
		if f.ChangeType == "RENAMED" || f.ChangeType == "COPIED" {
			path = f.DisplayPath()
		}
		files = append(files, map[string]any{
			"path":       path,
			"additions":  f.Additions,
			"deletions":  f.Deletions,
			"changeType": f.ChangeType,
		})
	}
	if files == nil {
		files = []any{}
	}
	obj["additions"] = add
	obj["deletions"] = del
	obj["changedFiles"] = len(d.Files)
	obj["files"] = files
}

func mapMR(raw map[string]any) map[string]any {
	out := map[string]any{}
	put := func(k string, v any) {
		if v == nil {
			return
		}
		if s, ok := v.(string); ok && s == "" {
			return
		}
		out[k] = v
	}
	put("title", raw["title"])
	put("number", raw["iid"])
	put("body", raw["description"])
	state := mapState(strOf(raw["state"]))
	put("state", state)
	draft := truthy(raw["draft"]) || truthy(raw["work_in_progress"])
	out["isDraft"] = draft
	out["closed"] = state == "CLOSED" || state == "MERGED"
	put("url", raw["web_url"])
	put("headRefName", raw["source_branch"])
	put("baseRefName", raw["target_branch"])
	put("headRefOid", raw["sha"])
	if a := mapUser(raw["author"]); a != nil {
		out["author"] = a
	}
	if a := mapUser(raw["merged_by"]); a != nil {
		out["mergedBy"] = a
	}
	out["assignees"] = mapUsers(raw["assignees"])
	out["reviewers"] = mapUsers(raw["reviewers"])
	out["labels"] = mapLabels(raw["labels"])
	if ms := mapMilestone(raw["milestone"]); ms != nil {
		out["milestone"] = ms
	}
	out["mergeable"] = mapMergeable(raw)
	if st := mapMergeStatus(raw); st != "" {
		out["mergeStateStatus"] = st
	}
	if raw["user_notes_count"] != nil {
		out["comments"] = raw["user_notes_count"]
	}
	if raw["changes_count"] != nil {
		out["changedFiles"] = normalizeCount(raw["changes_count"])
	}
	put("createdAt", raw["created_at"])
	put("updatedAt", raw["updated_at"])
	put("mergedAt", raw["merged_at"])
	put("closedAt", raw["closed_at"])
	if sha := strOf(raw["merge_commit_sha"]); sha != "" {
		out["mergeCommit"] = map[string]any{"oid": sha}
	}
	if id := raw["id"]; id != nil {
		out["fullDatabaseId"] = id
		out["id"] = id
	}
	return out
}

func mapState(s string) string {
	switch strings.ToLower(s) {
	case "opened", "open":
		return "OPEN"
	case "merged":
		return "MERGED"
	case "closed":
		return "CLOSED"
	case "":
		return ""
	default:
		return strings.ToUpper(s)
	}
}

func mapMergeable(raw map[string]any) string {
	if truthy(raw["has_conflicts"]) || strings.EqualFold(strOf(raw["merge_status"]), "cannot_be_merged") {
		return "CONFLICTING"
	}
	if strings.EqualFold(strOf(raw["merge_status"]), "can_be_merged") {
		return "MERGEABLE"
	}
	return "UNKNOWN"
}

func mapMergeStatus(raw map[string]any) string {
	if s := strOf(raw["detailed_merge_status"]); s != "" {
		return strings.ToUpper(s)
	}
	if s := strOf(raw["merge_status"]); s != "" {
		return strings.ToUpper(s)
	}
	return ""
}

func mapUser(v any) map[string]any {
	m, ok := v.(map[string]any)
	if !ok || m == nil {
		return nil
	}
	login := strOf(m["username"])
	if login == "" {
		login = strOf(m["login"])
	}
	name := strOf(m["name"])
	if login == "" && name == "" {
		return nil
	}
	out := map[string]any{}
	if login != "" {
		out["login"] = login
	}
	if name != "" {
		out["name"] = name
	}
	if truthy(m["bot"]) || truthy(m["is_bot"]) {
		out["is_bot"] = true
	}
	return out
}

func mapUsers(v any) []any {
	arr, ok := v.([]any)
	if !ok {
		return []any{}
	}
	out := make([]any, 0, len(arr))
	for _, item := range arr {
		if u := mapUser(item); u != nil {
			out = append(out, u)
		}
	}
	return out
}

func mapLabels(v any) []any {
	switch t := v.(type) {
	case nil:
		return []any{}
	case []any:
		out := make([]any, 0, len(t))
		for _, item := range t {
			switch x := item.(type) {
			case string:
				out = append(out, map[string]any{"name": x})
			case map[string]any:
				name := strOf(x["name"])
				if name == "" {
					name = strOf(x["title"])
				}
				if name == "" {
					continue
				}
				lab := map[string]any{"name": name}
				if c := strings.TrimPrefix(strOf(x["color"]), "#"); c != "" {
					lab["color"] = c
				}
				out = append(out, lab)
			}
		}
		return out
	default:
		return []any{}
	}
}

func mapMilestone(v any) map[string]any {
	m, ok := v.(map[string]any)
	if !ok || m == nil {
		return nil
	}
	title := strOf(m["title"])
	if title == "" {
		title = strOf(m["name"])
	}
	if title == "" {
		return nil
	}
	return map[string]any{"title": title}
}

func normalizeCount(v any) any {
	s := strOf(v)
	if s == "" {
		return v
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return s
}

func strOf(v any) string {
	s, _ := v.(string)
	return s
}

func truthy(v any) bool {
	b, ok := v.(bool)
	return ok && b
}
