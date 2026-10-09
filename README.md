# prtool

prtool is a CLI for GitHub pull requests and GitLab merge requests. It asks `gh` or `glab` for the data, then prints aligned fields and a diff. Values can be colored and bold. Keys stay plain.

A piped `prtool diff`, with no color, bold, or view flag, is the patch itself. `patch` can apply it.

## Build

```
go build -o prtool .
```

`gh` is required for GitHub. `glab` is required for GitLab.

## Examples

```
prtool view 14629 --repo cli/cli
prtool list --repo cli/cli --limit 10
prtool diff 14629 --repo cli/cli
prtool diff 14629 --repo cli/cli --stat
prtool fields
```

`prtool pr ...` forces GitHub. `prtool mr ...` forces GitLab. An explicit `--host github` or `--host gitlab` wins over either noun.

## Color and weight

On a terminal, values are colored unless `NO_COLOR` is set or `TERM=dumb`. `--colors` forces color, including in a pipe, and overrides `NO_COLOR`. `--bold` bolds values only. It does not turn color on, and it still applies when `NO_COLOR` is set.

`--raw` prints the applicable patch even on a terminal. `--stat`, `--name-only`, `--unified`, and `--side-by-side` select a view of the diff.
