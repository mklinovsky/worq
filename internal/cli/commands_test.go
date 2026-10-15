package cli

import (
	"github.com/mklinovsky/worq/internal/config"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupFromSubdirectory(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	run := func(dir string, args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		if b, e := c.CombinedOutput(); e != nil {
			t.Fatalf("%v: %s %v", args, b, e)
		}
	}
	run(root, "init", "-b", "main")
	run(root, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "initial")
	wt := filepath.Join(t.TempDir(), "feature")
	run(root, "worktree", "add", "-b", "feature", wt)
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TEST=1"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Defaults: config.Defaults{Setup: []config.Step{{Copy: []string{".env"}}, {Run: "pwd > setup-location"}}}}
	t.Setenv("HOME", t.TempDir())
	if err := cfg.Save(config.Path()); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{root, wt} {
		sub := filepath.Join(target, "src")
		if err := os.Mkdir(sub, 0700); err != nil {
			t.Fatal(err)
		}
		t.Chdir(sub)
		if err := cmdSetup([]string{"--quiet"}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(target, ".env")); err != nil {
			t.Fatal(err)
		}
		location, err := os.ReadFile(filepath.Join(target, "setup-location"))
		if err != nil {
			t.Fatal(err)
		}
		want, err := filepath.EvalSymlinks(target)
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(location)) != want {
			t.Fatalf("setup ran in %q, want %q", location, want)
		}
		if _, err := os.Stat(filepath.Join(sub, ".env")); !os.IsNotExist(err) {
			t.Fatal("copied into subdirectory")
		}
	}
}
