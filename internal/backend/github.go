package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"prtool/internal/diffview"
	"prtool/internal/fields"
)

// GitHub reads pull requests through gh.
type GitHub struct {
	Runner Runner
}

func (g GitHub) tool() error {
	return requireTool(g.Runner, "gh", "gh is not installed")
}

// View returns one pull request object.
func (g GitHub) View(ctx context.Context, req Request) (map[string]any, error) {
	if err := g.tool(); err != nil {
		return nil, err
	}
	args := []string{"pr", "view"}
	if req.Ref != "" {
		args = append(args, req.Ref)
	}
	args = append(args, "--json", fields.GitHubJSONFields(req.Fields))
	if req.Repo != "" {
		args = append(args, "--repo", req.Repo)
	}
	out, _, err := g.Runner.Run(ctx, "gh", args...)
	if err != nil {
		return nil, err
	}
	obj, err := decodeObject(out)
	if err != nil {
		return nil, err
	}
	normalizeGitHub(obj, req.Fields)
	return obj, nil
}

// List returns pull request objects.
func (g GitHub) List(ctx context.Context, req Request) ([]map[string]any, error) {
	if err := g.tool(); err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 30
	}
	args := []string{"pr", "list", "--json", fields.GitHubJSONFields(req.Fields), "--limit", strconv.Itoa(limit)}
	if req.State != "" {
		args = append(args, "--state", req.State)
	}
	if req.Author != "" {
		args = append(args, "--author", req.Author)
	}
	if search := githubSearch(req); search != "" {
		args = append(args, "--search", search)
	}
	if req.Base != "" {
		args = append(args, "--base", req.Base)
	}
	if req.Head != "" {
		args = append(args, "--head", req.Head)
	}
	if req.Assignee != "" {
		args = append(args, "--assignee", req.Assignee)
	}
	if req.Repo != "" {
		args = append(args, "--repo", req.Repo)
	}
	for _, label := range req.Labels {
		args = append(args, "--label", label)
	}
	if req.Draft == "yes" {
		args = append(args, "--draft")
	}
	out, _, err := g.Runner.Run(ctx, "gh", args...)
	if err != nil {
		return nil, err
	}
	rows, err := decodeList(out)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		normalizeGitHub(row, req.Fields)
	}
	return rows, nil
}

// Diff returns a colorless unified diff.
func (g GitHub) Diff(ctx context.Context, req Request) (string, error) {
	if err := g.tool(); err != nil {
		return "", err
	}
	args := []string{"pr", "diff"}
	if req.Ref != "" {
		args = append(args, req.Ref)
	}
	args = append(args, "--color=never")
	if req.Repo != "" {
		args = append(args, "--repo", req.Repo)
	}
	for _, ex := range req.Exclude {
		args = append(args, "--exclude", ex)
	}
	out, _, err := g.Runner.Run(ctx, "gh", args...)
	if err != nil {
		return "", err
	}
	// gh also receives --exclude. Filter the returned patch locally so a glob
	// gh does not understand still cannot leak a file into the view.
	if len(req.Exclude) > 0 {
		out = diffview.FilterRaw(out, req.Exclude)
	}
	return out, nil
}

func githubSearch(req Request) string {
	var parts []string
	if req.Search != "" {
		parts = append(parts, req.Search)
	}
	if req.Reviewer != "" {
		parts = append(parts, "review-requested:"+req.Reviewer)
	}
	if req.Draft == "no" {
		parts = append(parts, "draft:false")
	}
	return strings.Join(parts, " ")
}

func normalizeGitHub(obj map[string]any, requested []string) {
	wantReviewers := false
	for _, name := range requested {
		if name == "reviewers" {
			wantReviewers = true
			break
		}
	}
	if wantReviewers {
		if v, ok := obj["reviewRequests"]; ok {
			obj["reviewers"] = v
		}
	}
}

func decodeObject(s string) (map[string]any, error) {
	v, err := decodeJSON(s)
	if err != nil {
		return nil, err
	}
	switch t := v.(type) {
	case map[string]any:
		return t, nil
	case []any:
		if len(t) == 0 {
			return map[string]any{}, nil
		}
		if m, ok := t[0].(map[string]any); ok && len(t) == 1 {
			return m, nil
		}
	}
	return nil, fmt.Errorf("expected a JSON object from gh")
}

func decodeList(s string) ([]map[string]any, error) {
	v, err := decodeJSON(s)
	if err != nil {
		return nil, err
	}
	switch t := v.(type) {
	case nil:
		return nil, nil
	case []any:
		return mapsOf(t), nil
	case map[string]any:
		for _, key := range []string{"items", "pull_requests", "merge_requests"} {
			if arr, ok := t[key].([]any); ok {
				return mapsOf(arr), nil
			}
		}
	}
	return nil, fmt.Errorf("expected a JSON array from gh")
}

func mapsOf(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, m)
	}
	return out
}

func decodeJSON(s string) (any, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty response")
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	return v, nil
}
