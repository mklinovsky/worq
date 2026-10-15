package worktree_test

import (
	"github.com/mklinovsky/worq/internal/config"
	"github.com/mklinovsky/worq/internal/worktree"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestReviewUnknownBaseMustNotBeStale(t *testing.T) {
	root := t.TempDir()
	git := func(dir string, args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		if b, e := c.CombinedOutput(); e != nil {
			t.Fatalf("%v: %s %v", args, b, e)
		}
	}
	git(root, "init", "-b", "develop")
	git(root, "-c", "user.name=Review", "-c", "user.email=review@example.com", "commit", "--allow-empty", "-m", "initial")
	wt := filepath.Join(t.TempDir(), "feature")
	git(root, "worktree", "add", "-b", "feature", wt)
	git(wt, "-c", "user.name=Review", "-c", "user.email=review@example.com", "commit", "--allow-empty", "-m", "unmerged")
	svc := worktree.Service{Cfg: config.Resolved{Root: root, BaseBranch: "main"}}
	stale, err := svc.Stale()
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) > 0 {
		t.Fatalf("unmerged feature classified as stale: %+v", stale)
	}
	svc = worktree.Service{Cfg: config.Resolved{Root: root, BaseBranch: "develop"}}
	stale, err = svc.Stale()
	if err != nil || len(stale) != 0 {
		t.Fatalf("unmerged: %v %v", stale, err)
	}
	git(root, "merge", "--ff-only", "feature")
	stale, err = svc.Stale()
	if err != nil || len(stale) != 1 {
		t.Fatalf("merged: %v %v", stale, err)
	}
}
