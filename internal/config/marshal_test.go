package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMarshalOmitsEmptyFields(t *testing.T) {
	c := &Config{
		Defaults: Defaults{BaseBranch: "main"},
		Projects: []Project{{
			Name:    "frontend",
			Path:    "~/Projects/frontend",
			JiraKey: "EFRON",
			Setup: []Step{
				{Name: "env", Copy: []string{".env", ".env.local"}},
				{Name: "install", Run: "pnpm i", Dir: "app", Optional: true},
			},
		}},
	}
	out := c.Marshal()
	for _, want := range []string{
		"base_branch: main",
		"projects:",
		"jira_key: EFRON",
		"- .env.local",
		"run: pnpm i",
		"dir: app",
		"optional: true",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "branch_prefix") || strings.Contains(out, `worktree_base`) {
		t.Errorf("empty fields were written:\n%s", out)
	}
}

func TestMarshalRoundTrips(t *testing.T) {
	c := &Config{
		Defaults: Defaults{WorktreeBase: "{parent}/{name}-worktrees", BaseBranch: "main"},
		Projects: []Project{
			{Name: "a", Path: "/repos/a", BaseBranch: "master", Setup: []Step{{Name: "env", Copy: []string{".env"}}}},
			{Name: "b", Path: "/repos/b", BranchPrefix: "feat/"},
		},
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := c.Save(p); err != nil {
		t.Fatal(err)
	}
	back, err := LoadFrom(p)
	if err != nil {
		t.Fatalf("re-reading what we wrote failed: %v", err)
	}
	if len(back.Projects) != 2 || back.Projects[0].BaseBranch != "master" {
		t.Fatalf("round trip lost data: %+v", back.Projects)
	}
	if back.Defaults.WorktreeBase != "{parent}/{name}-worktrees" {
		t.Errorf("defaults lost: %+v", back.Defaults)
	}
	if len(back.Projects[0].Setup) != 1 || back.Projects[0].Setup[0].Copy[0] != ".env" {
		t.Errorf("steps lost: %+v", back.Projects[0].Setup)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
}

func TestUpsertReplacesByPath(t *testing.T) {
	c := &Config{Projects: []Project{{Name: "old", Path: "/repos/a"}}}
	if replaced := c.Upsert(Project{Name: "new", Path: "/repos/a"}); !replaced {
		t.Fatal("expected a replacement")
	}
	if len(c.Projects) != 1 || c.Projects[0].Name != "new" {
		t.Fatalf("got %+v", c.Projects)
	}
	if replaced := c.Upsert(Project{Name: "other", Path: "/repos/b"}); replaced {
		t.Fatal("expected an append")
	}
	if len(c.Projects) != 2 {
		t.Fatalf("got %+v", c.Projects)
	}
}

func TestSpecialCharactersRoundTrip(t *testing.T) {
	c := &Config{Projects: []Project{{Path: `/a"b\c`}}}
	dir := t.TempDir()
	p := filepath.Join(dir, "c.yaml")
	if err := c.Save(p); err != nil {
		t.Fatal(err)
	}
	back, err := LoadFrom(p)
	if err != nil {
		t.Fatalf("escaped value did not parse back: %v", err)
	}
	if back.Projects[0].Path != `/a"b\c` {
		t.Fatalf("got %q", back.Projects[0].Path)
	}
}

func TestMarshalProjectSkipsHeaderComment(t *testing.T) {
	out := MarshalProject(Project{Name: "a", Path: "/repos/a"})
	if !strings.HasPrefix(out, "- name: a") {
		t.Fatalf("preview should start at the entry, got:\n%s", out)
	}
	if strings.Contains(out, "#") {
		t.Fatalf("preview leaked the header comment:\n%s", out)
	}
}
