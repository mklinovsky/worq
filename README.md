# worq

Git worktrees and per-project setup, in one tool. Built to replace the pile of
shell aliases: create a worktree from a Jira key, copy the env files across,
run whatever the project needs, and hand you the path.

Config lives in one central file, gira-style: `[defaults]` plus a list of
`[[projects]]` matched against the current directory, longest match wins.

## Install

```bash
cd ~/Projects/worq
go mod tidy          # first run only, fetches deps
make install         # builds and installs to ~/.local/bin/worq
```

worq never moves you anywhere — it prints the worktree path on stdout and
leaves sessions to you:

```bash
tmux new-session -s fix -c "$(worq new EFRON-1234 fix login redirect)"
tmux new-window -c "$(worq path 1234)"
tmux new-window -c "$(worq ls)"           # the browser used as a picker
```

Progress and errors go to stderr, so the path is the only thing on stdout.

## Use

```bash
cd ~/Projects/some-repo && worq init      # register this repo (form)
worq config                               # what worq resolved for this repo

worq new EFRON-1234 fix login redirect    # create + setup, prints the path
worq new 1234 fix login                   # same, when jira_key is set
worq new feat/mobile/login                # explicit branch, taken as typed
worq ls                                   # the browser
worq path 1234                            # by key, branch or directory name

worq ls --plain                           # table; --json / --paths for scripts
worq setup                                # re-run setup steps here
worq rm 1234                              # remove worktree + branch
worq clean --dry-run                      # what is merged and could go
```

`worq new EFRON-1234 fix login redirect` creates
`<worktree_base>/efron-1234-fix-login-redirect` on branch
`efron-1234-fix-login-redirect`.

It fetches the base branch first and starts from `origin/<base_branch>`, so a
new worktree always begins at what the remote has — never at a local checkout
that has not been pulled in a fortnight. Your local `master` is not touched.
When the fetch fails (no network, no remote) worq says so and falls back to the
local branch; `--no-fetch` skips it, and `--base` overrides which branch to
start from.

### The browser

`worq ls` opens the Bubble Tea browser (`worq` on its own just prints help):

```
enter  print the selected path and exit   n  new worktree
space  mark / unmark for removal          s  re-run setup steps
d      remove marked (or the cursor)      a  mark all
/      filter                             A  clear marks
r      refresh                            q  quit
```

The repository checkout is not listed: it is named in the header instead, since
it is not something you create, switch between or remove. `worq ls --plain
--all` shows it if you want it in the table.

Rows appear as soon as `git worktree list` returns, and each row's status fills
in behind it — the dirty check runs per worktree and a large repository takes a
moment. The badge reflects tracked changes only, so the `.env` files worq copied
in do not make everything look dirty; the thorough check still runs before
anything is removed.

Marked rows carry a ✓. `d` asks before removing anything and lists what is
about to go; worktrees with uncommitted changes are called out, and `y` removes
only the clean ones while `f` removes them all. Branches go with their
worktrees.

Each row shows the branch, whether it is dirty, whether you are standing in it,
and how many commits it holds that the base branch does not — `merged` means
nothing would be lost by removing it.

## worq init

Run it inside a repository. It reads the repo and fills the form in for you:
the default branch from `origin/HEAD`, the Jira key from the issue keys in your
branch history, an existing sibling `*-worktrees` directory if you have one, and
and a step to copy any `.env` files it finds.

Nothing else is suggested on purpose: a worktree exists to have its own
dependencies, so `node_modules` and friends are never shared or copied between
them. Add a `run` step if you want the install to happen automatically.

```
tab/↑↓   move between fields          enter or ctrl+s   save
```

Tab past the last field to reach the setup steps, where `a` adds, `e` edits,
`d` deletes, `space` toggles optional, `J`/`K` reorder and `r` restores what was
detected. A live preview of the TOML sits under the form.

Running it again in a repo that is already registered edits that entry rather
than adding a second one. Saving re-serialises the whole config file, so
comments you added by hand do not survive — `worq init --template` writes the
commented example file if that is what you want.

## Config

`~/.config/worq/config.toml` (override with `$WORQ_CONFIG`).

```toml
[defaults]
worktree_base = "{parent}/{name}-worktrees"
base_branch = "main"

[[defaults.setup]]
name = "env files"
copy = [".env", ".env.local"]

[[projects]]
name = "frontend"
path = "~/Projects/bloomreach/frontend"
worktree_base = "~/Projects/bloomreach/frontend-worktrees"
base_branch = "master"
jira_key = "EFRON"

[[projects.setup]]
name = "env files"
copy = [".env", ".env.local"]

[[projects.setup]]
name = "install"
run = "pnpm install --prefer-offline"
optional = true
```

Precedence: CLI flags > matched `[[projects]]` entry > `[defaults]` > built-in.
Paths accept `~`, `$VARS` and `{root}`, `{parent}`, `{name}`.

A project's `setup` list replaces the default list rather than appending, so a
project either inherits the defaults or spells out its own steps.

### Setup steps

Each step does exactly one thing, and they run in order:

| key | effect |
| --- | --- |
| `copy` | copy paths from the main worktree; missing paths are skipped |
| `run` | run a shell command in the new worktree |
| `dir` | with `run`: the subdirectory to run it in |
| `optional` | a failure is reported but the rest still runs |

A step with neither `copy` nor `run` does nothing, and worq reports it as an
error rather than a silent success — usually it means the config still has a
key that no longer exists. `worq config` flags them too.

`worq setup` re-runs them against an existing worktree.

### Matching a project

`path` is matched against the repository root; `worktree_base` is matched too,
so commands still resolve the right project from inside a linked worktree kept
outside the repo. The longest matching path wins, which means a monorepo
package can override its parent.

Used as a picker, `worq ls` prints the path you selected on stdout — the
interface itself is drawn on stderr, so `tmux new-window -c "$(worq tui)"`
works.

## Relationship to gira

[gira](https://github.com/mklinovsky/gira) stays in charge of Jira and GitLab:
creating issues, merge requests, transitions. worq owns the local side —
worktrees and getting them ready to work in. They share the branch naming
convention (`efron-1234-slug`), so `gira mr` still derives the issue from a
branch worq created. The Jira/GitLab commands may move in here later.

## Development

```bash
make test     # go test ./...
make check    # gofmt -l, go vet, go test
make build    # ./bin/worq
```
