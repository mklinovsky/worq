# worq

Git worktrees with per-project setup. `worq new` creates a worktree from a Jira
key, copies your env files, runs your setup commands and prints the path.

## Install

```bash
make install   # builds to ~/.local/bin/worq
```

## Usage

```bash
worq init                            # register the current repo
worq new PROJ-1234 fix login         # create proj-1234-fix-login, print its path
worq new 1234 fix login              # same, when jira_key is set
worq new --track PROJ-7926-dark-mode # check out an existing origin branch
worq ls                              # interactive browser
worq ls --plain                      # table; --paths prints paths only
worq path 1234                       # path by key, branch or directory name
worq setup                           # re-run setup steps in this worktree
worq rm 1234                         # remove worktree and branch
worq clean --dry-run                 # list merged worktrees
worq config                          # show the resolved config
```

New worktrees start from `origin/<base_branch>` after a fetch. `--no-fetch`
skips the fetch, `--base` picks another branch.

The path goes to stdout and everything else to stderr, so worq composes with
your own tools:

```bash
tmux new-window -c "$(worq new PROJ-1234 fix login)"
tmux new-window -c "$(worq ls)"     # browser as a picker
```

## Browser

```
enter  print path and exit     n  new worktree
space  mark for removal        s  re-run setup
d      remove marked           a  mark all
/      filter                  A  clear marks
r      refresh                 q  quit
```

Each row shows the branch, dirty state and commits ahead of the base branch.
`merged` means nothing is lost by removing it. If the comparison fails, the
status is unknown and `clean` skips the worktree.

## Config

`~/.worq/config.yaml`, written by `worq init` (`worq init --template` writes a
commented example):

```yaml
defaults:
  worktree_base: "{parent}/{name}-worktrees"
  base_branch: main
  setup:
    - copy: [".env", ".env.local"]

projects:
  - name: foo
    path: ~/Projects/foo
    base_branch: master
    jira_key: PROJ
    setup:
      - copy: [".env"]
      - run: pnpm install --prefer-offline
        optional: true
```

Precedence: CLI flags > matching project > `defaults`. The longest matching
`path` or `worktree_base` wins. Paths accept `~`, `$VARS`, `{root}`, `{parent}`
and `{name}`.

A project's `setup` replaces the default steps; `setup: []` disables them.

| step key   | effect                                              |
| ---------- | --------------------------------------------------- |
| `copy`     | copy paths from the main worktree, skipping missing |
| `run`      | run a shell command in the new worktree             |
| `dir`      | with `run`: subdirectory to run it in               |
| `optional` | a failure is reported but the rest still runs       |
| `name`     | label shown in progress output                      |

## Development

```bash
make test    # go test ./...
make check   # vet, test, gofmt
make build   # ./bin/worq
```
