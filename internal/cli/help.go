package cli

import "strings"

// Help is the usage text for cmd, or the top-level usage when cmd is empty.
func Help(cmd string) string {
	switch cmd {
	case "view":
		return viewHelp
	case "list", "ls":
		return listHelp
	case "diff":
		return diffHelp
	case "fields":
		return fieldsHelp
	default:
		return rootHelp
	}
}

const rootHelp = `prtool — pull requests and merge requests, in color

usage:
  prtool [pr|mr] <command> [flags]

commands:
  view    [number|url|branch]   show one request
  list    (alias ls)            list requests
  diff    [number|url|branch]   show the diff
  fields                        list field names

` + commonHelp

const viewHelp = `prtool view — show one pull request or merge request

usage:
  prtool view [number|url|branch] [flags]

flags:
  --fields a,b,c     columns to show (or "all")
  --repo, -R owner/repo
  --host github|gitlab
  --colors           color even when stdout is not a terminal
  --bold             bold values (keys stay plain)
  --no-pager         do not pipe long output to a pager

A leading "pr" forces GitHub and "mr" forces GitLab. An explicit --host wins.
With no number, the current branch is used.

` + commonHelp

const listHelp = `prtool list — list pull requests or merge requests

usage:
  prtool list [flags]
  prtool ls [flags]

flags:
  --fields a,b,c
  --state open|closed|merged|all
  --limit, -L N      default 30
  --author user
  --label, -l name   repeatable
  --search text
  --base branch
  --head branch
  --assignee user
  --reviewer user
  --draft, -d
  --not-draft
  --repo, -R owner/repo
  --host github|gitlab
  --colors
  --bold
  --no-pager

GitHub has no reviewer flag; --reviewer adds review-requested:USER to the
search. --not-draft adds draft:false on GitHub and --not-draft on GitLab.
GitLab list cannot show additions, deletions, or files; use prtool view.

` + commonHelp

const diffHelp = `prtool diff — a patch, or a colored diff

usage:
  prtool diff [number|url|branch] [flags]

flags:
  --exclude glob     repeatable; GitHub also passes it to gh
  --stat             summary only
  --name-only        paths only
  --unified          one column, even on a wide terminal
  --side-by-side     two columns (also the default when the terminal is at least 160 columns)
  --raw              the applicable patch, even on a terminal
  --repo, -R owner/repo
  --host github|gitlab
  --colors
  --bold
  --no-pager

Piped output with no color, bold, or view flag is the patch itself, so it can
be applied. On a terminal the diff is drawn by prtool (additions, deletions,
hunks, and a stat header). --raw conflicts with --stat, --name-only, and
--side-by-side.

` + commonHelp

const fieldsHelp = `prtool fields — field names for --fields

usage:
  prtool fields [--host github|gitlab]

The names are the same on both hosts. "reviewers" is the name to request;
GitHub's JSON field reviewRequests is filled in for you.

` + commonHelp

const commonHelp = `color:
  A terminal enables color unless NO_COLOR is set or TERM=dumb.
  --colors forces color anyway, including in a pipe. --bold is separate
  and also overrides NO_COLOR. Keys are never bold.

pager:
  Long terminal output is sent to less -FRX. PRTOOL_PAGER replaces the
  command (split on spaces). An empty PRTOOL_PAGER or --no-pager disables it.
`

func helpHas(args []string, kind string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		switch kind {
		case "help":
			if a == "-h" || a == "--help" || strings.HasPrefix(a, "--help=") {
				return true
			}
		case "version":
			if a == "--version" || strings.HasPrefix(a, "--version=") {
				return true
			}
		}
	}
	return false
}
