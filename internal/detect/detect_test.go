package detect

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func repo(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "frontend")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "init", "-q", "-b", "master", ".")
	run(t, dir, "config", "user.email", "t@t")
	run(t, dir, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-qm", "init")
	return dir
}

func TestInspectBasics(t *testing.T) {
	dir := repo(t)
	r, err := Inspect(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "frontend" {
		t.Errorf("name = %q", r.Name)
	}
	if r.BaseBranch != "master" {
		t.Errorf("base branch = %q, want master", r.BaseBranch)
	}
	if want := filepath.Join(filepath.Dir(dir), "frontend-worktrees"); r.WorktreeBase != want {
		t.Errorf("worktree base = %q, want %q", r.WorktreeBase, want)
	}
	if r.JiraKey != "" {
		t.Errorf("jira key = %q, want none", r.JiraKey)
	}
}

func TestInspectPicksExistingWorktreeDir(t *testing.T) {
	dir := repo(t)
	existing := filepath.Join(filepath.Dir(dir), "worktrees")
	if err := os.Mkdir(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	r, _ := Inspect(dir, nil)
	if r.WorktreeBase != existing {
		t.Fatalf("worktree base = %q, want the existing %q", r.WorktreeBase, existing)
	}
}

func TestInspectFindsDominantJiraKey(t *testing.T) {
	dir := repo(t)
	for _, b := range []string{"efron-1-a", "efron-2-b", "abc-9-c"} {
		run(t, dir, "branch", b)
	}
	r, _ := Inspect(dir, nil)
	if r.JiraKey != "EFRON" {
		t.Fatalf("jira key = %q, want EFRON", r.JiraKey)
	}
}

func TestSuggestionsCopyEnvFilesOnly(t *testing.T) {
	dir := repo(t)
	os.WriteFile(filepath.Join(dir, ".env"), []byte("A=1"), 0o644)
	os.WriteFile(filepath.Join(dir, ".env.local"), []byte("B=2"), 0o644)
	os.WriteFile(filepath.Join(dir, "pnpm-lock.yaml"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644)
	os.Mkdir(filepath.Join(dir, "node_modules"), 0o755)

	r, _ := Inspect(dir, nil)
	if len(r.Suggestions) != 1 {
		t.Fatalf("expected only the env step, got %+v", r.Suggestions)
	}
	s := r.Suggestions[0]
	if len(s.Copy) != 2 || s.Copy[0] != ".env" || s.Copy[1] != ".env.local" {
		t.Errorf("env step = %+v", s)
	}
	// A worktree exists to have its own dependencies: nothing may share them.
	for _, step := range r.Suggestions {
		for _, path := range step.Copy {
			if path == "node_modules" {
				t.Fatal("node_modules must never be suggested")
			}
		}
	}
}

func TestNoSuggestionsWithoutEnvFiles(t *testing.T) {
	dir := repo(t)
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644)
	r, _ := Inspect(dir, nil)
	if len(r.Suggestions) != 0 {
		t.Fatalf("expected no steps, got %+v", r.Suggestions)
	}
}
