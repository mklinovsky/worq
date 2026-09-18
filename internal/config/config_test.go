package config

import (
	"os"
	"path/filepath"
	"testing"
)

const sample = `
[defaults]
worktree_base = "{parent}/{name}-worktrees"
base_branch = "main"
branch_prefix = "feat/"

[[defaults.setup]]
name = "env"
copy = [".env"]

[[projects]]
path = "/repos/app"
base_branch = "master"
jira_key = "APP"

[[projects.setup]]
name = "install"
run = "pnpm i"

[[projects]]
path = "/repos/app/packages/ui"
worktree_base = "/wt/ui"

[[projects]]
path = "/repos/other"
`

func load(t *testing.T) *Config {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(p, []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestResolveDefaultsWhenNoProjectMatches(t *testing.T) {
	c := load(t)
	r := c.Resolve("/repos/unknown", "/repos/unknown")
	if r.Matched {
		t.Fatal("expected no match")
	}
	if r.BaseBranch != "main" {
		t.Errorf("base branch = %q, want main", r.BaseBranch)
	}
	if want := "/repos/unknown-worktrees"; r.WorktreeBase != want {
		t.Errorf("worktree base = %q, want %q", r.WorktreeBase, want)
	}
	if len(r.Setup) != 1 || r.Setup[0].Name != "env" {
		t.Errorf("setup = %+v, want the default step", r.Setup)
	}
}

func TestResolveProjectOverrides(t *testing.T) {
	c := load(t)
	r := c.Resolve("/repos/app", "/repos/app/src")
	if !r.Matched || r.BaseBranch != "master" || r.JiraKey != "APP" {
		t.Fatalf("got %+v", r)
	}
	if len(r.Setup) != 1 || r.Setup[0].Run != "pnpm i" {
		t.Errorf("project setup should replace defaults, got %+v", r.Setup)
	}
	if r.BranchPrefix != "feat/" {
		t.Errorf("branch prefix should fall through to defaults, got %q", r.BranchPrefix)
	}
}

func TestLongestPathWins(t *testing.T) {
	c := load(t)
	r := c.Resolve("/repos/app/packages/ui", "/repos/app/packages/ui")
	if r.WorktreeBase != "/wt/ui" {
		t.Fatalf("worktree base = %q, want /wt/ui", r.WorktreeBase)
	}
	if r.BaseBranch != "main" {
		t.Errorf("nested entry should not inherit the parent project, got %q", r.BaseBranch)
	}
}

func TestMatchFromInsideWorktree(t *testing.T) {
	c := load(t)
	// A linked worktree kept outside the repo still resolves to its project,
	// because worktree_base is matched as well as path.
	r := c.Resolve("/wt/ui/feature-x", "/wt/ui/feature-x")
	if !r.Matched || r.WorktreeBase != "/wt/ui" {
		t.Fatalf("expected the ui entry to match via worktree_base, got %+v", r)
	}
}

func TestMostSpecificProjectWinsOverAncestor(t *testing.T) {
	c := load(t)
	r := c.Resolve("/repos/app/packages/ui", "/repos/app/packages/ui/src")
	if r.WorktreeBase != "/wt/ui" {
		t.Fatalf("nested project should win over /repos/app, got %+v", r)
	}
}

func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	if got := Expand("~/x"); got != filepath.Join(home, "x") {
		t.Fatalf("Expand(~/x) = %q", got)
	}
}
