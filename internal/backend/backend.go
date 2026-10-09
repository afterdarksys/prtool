// Package backend shells out to gh and glab. Arguments are a list, never a shell command.
package backend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"sync"
)

// Host is the forge a command talks to.
type Host int

const (
	HostGitHub Host = iota
	HostGitLab
)

func (h Host) String() string {
	if h == HostGitLab {
		return "gitlab"
	}
	return "github"
}

// ExitError is a non-zero exit from gh or glab. Code is that process's status.
type ExitError struct {
	Code int
	Msg  string
}

func (e *ExitError) Error() string { return e.Msg }

// Runner runs a tool by argv. LookPath reports whether name is installed.
type Runner interface {
	LookPath(name string) error
	Run(ctx context.Context, name string, args ...string) (stdout, stderr string, err error)
}

// Exec is the real process runner.
type Exec struct{}

func (Exec) LookPath(name string) error {
	_, err := exec.LookPath(name)
	return err
}

func (Exec) Run(ctx context.Context, name string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			msg := strings.TrimSpace(errb.String())
			if msg == "" {
				msg = strings.TrimSpace(out.String())
			}
			if msg == "" {
				msg = err.Error()
			}
			return out.String(), errb.String(), &ExitError{Code: ee.ExitCode(), Msg: msg}
		}
		return out.String(), errb.String(), err
	}
	return out.String(), errb.String(), nil
}

// Result is one scripted command response.
type Result struct {
	Stdout string
	Stderr string
	Err    error
	Code   int
}

// Call is one recorded invocation. Args is a copy.
type Call struct {
	Name string
	Args []string
}

// Script is an order-based runner for tests. An extra call returns an error.
type Script struct {
	mu        sync.Mutex
	Responses []Result
	Calls     []Call
	i         int
	Missing   map[string]bool
}

func (s *Script) LookPath(name string) error {
	if s.Missing[name] {
		return fmt.Errorf("%s not found", name)
	}
	return nil
}

func (s *Script) Run(ctx context.Context, name string, args ...string) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := append([]string{}, args...)
	s.Calls = append(s.Calls, Call{Name: name, Args: cp})
	if s.i >= len(s.Responses) {
		return "", "", fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
	}
	r := s.Responses[s.i]
	s.i++
	if r.Code != 0 {
		msg := strings.TrimSpace(r.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(r.Stdout)
		}
		return r.Stdout, r.Stderr, &ExitError{Code: r.Code, Msg: msg}
	}
	return r.Stdout, r.Stderr, r.Err
}

// Request is a view, list, or diff query.
type Request struct {
	Ref      string
	Repo     string
	Fields   []string
	State    string
	Limit    int
	Author   string
	Labels   []string
	Search   string
	Base     string
	Head     string
	Assignee string
	Reviewer string
	Draft    string // "", "yes", "no"
	Exclude  []string
}

// Input is what the CLI knows before a host is chosen.
type Input struct {
	ExplicitHost string
	Ref          string
	Repo         string
	Noun         string // "pr", "mr", or ""
	Remote       string
	RemoteErr    error
}

// Target is a resolved forge, ref, and repo.
type Target struct {
	Host Host
	Ref  string
	Repo string
}

// NeedsRemote reports whether origin must be read to choose a host.
// A URL ref, an explicit host, a pr/mr noun, or a repo that already names a
// host is enough; owner/repo alone is not.
func NeedsRemote(explicit, ref, repo, noun string) bool {
	if explicit != "" || noun == "pr" || noun == "mr" {
		return false
	}
	if isURL(ref) {
		return false
	}
	if repoLooksHosted(repo) {
		return false
	}
	return true
}

// Resolve picks a host and normalizes a GitLab merge-request URL to an iid.
func Resolve(in Input) (Target, error) {
	var (
		host    Host
		hostSet bool
		ref     = in.Ref
		repo    = in.Repo
	)
	if in.ExplicitHost != "" {
		h, err := parseHostFlag(in.ExplicitHost)
		if err != nil {
			return Target{}, err
		}
		host, hostSet = h, true
	} else if in.Noun == "pr" {
		host, hostSet = HostGitHub, true
	} else if in.Noun == "mr" {
		host, hostSet = HostGitLab, true
	}

	if h, proj, iid, ok := parseRefURL(ref); ok {
		if !hostSet {
			host, hostSet = h, true
		}
		if h == HostGitLab {
			ref = iid
			if repo == "" {
				repo = proj
			}
		}
	} else if isURL(ref) {
		u, err := url.Parse(ref)
		if err == nil && u.Host != "" {
			if _, known := classifyHost(u.Hostname()); !known && !hostSet {
				return Target{}, unknownHost(u.Hostname())
			}
		}
	}

	if !hostSet && repo != "" {
		h, known, bad := hostFromRepo(repo)
		if bad != nil {
			return Target{}, bad
		}
		if known {
			host, hostSet = h, true
		}
	}

	if !hostSet {
		if in.RemoteErr == nil && strings.TrimSpace(in.Remote) != "" {
			h, _, bad := hostFromRemote(in.Remote)
			if bad != nil {
				return Target{}, bad
			}
			host, hostSet = h, true
		}
	}
	if !hostSet {
		host = HostGitHub
	}
	return Target{Host: host, Ref: ref, Repo: repo}, nil
}

func parseHostFlag(s string) (Host, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "github":
		return HostGitHub, nil
	case "gitlab":
		return HostGitLab, nil
	default:
		return 0, fmt.Errorf("unknown --host %q (use github or gitlab)", s)
	}
}

func unknownHost(host string) error {
	return fmt.Errorf("host %q is not recognized; pass --host github or --host gitlab", host)
}

func classifyHost(host string) (Host, bool) {
	h := strings.ToLower(host)
	if i := strings.IndexByte(h, ':'); i >= 0 {
		h = h[:i]
	}
	switch {
	case h == "github.com" || strings.HasSuffix(h, ".github.com") || strings.HasPrefix(h, "github.") || strings.Contains(h, ".github."):
		return HostGitHub, true
	case strings.Contains(h, "gitlab"):
		return HostGitLab, true
	default:
		return 0, false
	}
}

func isURL(s string) bool {
	return strings.Contains(s, "://")
}

// parseRefURL extracts a forge, project, and GitLab iid. GitHub refs stay intact
// at the call site; iid is empty for GitHub. ok is false when s is not a URL.
func parseRefURL(ref string) (Host, string, string, bool) {
	if !isURL(ref) {
		return 0, "", "", false
	}
	u, err := url.Parse(ref)
	if err != nil || u.Host == "" {
		return 0, "", "", false
	}
	hostName := u.Hostname()
	h, known := classifyHost(hostName)
	if !known {
		if strings.Contains(hostName, ".") {
			// Signal an unknown dotted host by returning ok with a panic-free
			// path: callers that need the error use isURL + classify separately.
			// Returning ok=false would fall through to remote detection.
			// Resolve handles unknown URL hosts before this returns false...
			// We return ok true only for known hosts. Unknown is handled by Resolve
			// via isURL. So return false here.
			return 0, "", "", false
		}
		return 0, "", "", false
	}
	path := strings.Trim(u.Path, "/")
	path = strings.TrimSuffix(path, ".diff")
	path = strings.TrimSuffix(path, ".patch")
	if h == HostGitLab {
		const marker = "/-/merge_requests/"
		i := strings.Index(path, marker)
		if i < 0 {
			return h, "", "", true
		}
		proj := path[:i]
		rest := path[i+len(marker):]
		iid := rest
		if slash := strings.IndexByte(rest, '/'); slash >= 0 {
			iid = rest[:slash]
		}
		return h, proj, iid, true
	}
	return h, "", "", true
}

func repoLooksHosted(repo string) bool {
	if repo == "" {
		return false
	}
	if isURL(repo) {
		return true
	}
	if strings.Contains(repo, ":") && strings.HasPrefix(repo, "git@") {
		return true
	}
	parts := strings.Split(repo, "/")
	return len(parts) >= 2 && strings.Contains(parts[0], ".")
}

func hostFromRepo(repo string) (Host, bool, error) {
	if isURL(repo) {
		u, err := url.Parse(repo)
		if err != nil || u.Host == "" {
			return 0, false, nil
		}
		h, ok := classifyHost(u.Hostname())
		if !ok {
			return 0, false, unknownHost(u.Hostname())
		}
		return h, true, nil
	}
	if strings.HasPrefix(repo, "git@") {
		h, _, err := hostFromRemote(repo)
		if err != nil {
			return 0, false, err
		}
		host, _ := splitRemote(repo)
		_, ok := classifyHost(host)
		return h, ok, nil
	}
	parts := strings.Split(repo, "/")
	if len(parts) >= 2 && strings.Contains(parts[0], ".") {
		h, ok := classifyHost(parts[0])
		if !ok {
			return 0, false, unknownHost(parts[0])
		}
		return h, true, nil
	}
	return 0, false, nil
}

func hostFromRemote(remote string) (Host, string, error) {
	host, _ := splitRemote(remote)
	if host == "" {
		return 0, "", nil
	}
	h, ok := classifyHost(host)
	if !ok && strings.Contains(host, ".") {
		return 0, "", unknownHost(host)
	}
	if !ok {
		return 0, "", nil
	}
	return h, "", nil
}

func splitRemote(raw string) (host, project string) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimSuffix(raw, ".git")
	if strings.HasPrefix(raw, "git@") {
		rest := strings.TrimPrefix(raw, "git@")
		h, path, ok := strings.Cut(rest, ":")
		if !ok {
			return "", ""
		}
		return h, strings.Trim(path, "/")
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", ""
		}
		return u.Hostname(), strings.Trim(u.Path, "/")
	}
	parts := strings.Split(raw, "/")
	if len(parts) >= 3 && strings.Contains(parts[0], ".") {
		return parts[0], strings.Join(parts[1:], "/")
	}
	return "", raw
}

func requireTool(r Runner, name, missing string) error {
	if err := r.LookPath(name); err != nil {
		return fmt.Errorf("%s", missing)
	}
	return nil
}
