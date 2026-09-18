package config

import (
	"os"
	"path/filepath"
)

// Template is written by `worq init`.
const Template = `# worq configuration
#
# Precedence: CLI flags > matched projects entry > defaults > built-in.
# Paths accept ~, $VARS and the placeholders {root}, {parent} and {name}.

defaults:
  # Where worktrees are created when a project does not override it.
  worktree_base: "{parent}/{name}-worktrees"
  base_branch: main

  # Steps run after every worktree is created, in order.
  # Each step does exactly one of: copy, run.
  #   copy     paths copied from the main worktree (missing paths are skipped)
  #   run      shell command executed in the new worktree
  #   dir      run the command in this subdirectory of the worktree
  #   optional a failure is reported but does not abort the rest
  setup:
    - name: env files
      copy: [".env", ".env.local"]

# projects:
#   - name: frontend
#     path: ~/Projects/bloomreach/frontend
#     worktree_base: ~/Projects/bloomreach/frontend-worktrees
#     base_branch: master
#     jira_key: EFRON          # worq new 123 fix login -> efron-123-fix-login
#     branch_prefix: ""        # used when the name has no Jira key
#     setup:
#       - name: env files
#         copy: [".env", ".env.local", "src/environments/local.ts"]
#       - name: install
#         run: pnpm install --prefer-offline
#         optional: true
`

// WriteTemplate creates the config file. It refuses to clobber an existing one
// unless force is set.
func WriteTemplate(path string, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return os.ErrExist
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(Template), 0o644)
}
