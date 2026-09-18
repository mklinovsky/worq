// Package detect inspects a repository so `worq init` can pre-fill its form
// with what is actually there instead of asking you to type it.
package detect

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mklinovsky/worq/internal/config"
	"github.com/mklinovsky/worq/internal/git"
)

// issuePrefix matches the project key at the head of a branch name, e.g. the
// EFRON in efron-1234-fix-login. It is the only place worq reads an issue key
// out of a branch, and it exists so `worq init` can pre-fill jira_key.
var issuePrefix = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_]+)-\d+`)

// Result is everything worq could work out about the repo.
type Result struct {
	Root         string
	Name         string
	Path         string // root, with $HOME folded back to ~
	WorktreeBase string
	BaseBranch   string
	JiraKey      string
	Suggestions  []config.Step
	Existing     *config.Project // already registered, if it was
}

// Inspect examines the repository containing dir.
func Inspect(dir string, cfg *config.Config) (Result, error) {
	root, err := git.MainRoot(dir)
	if err != nil {
		return Result{}, err
	}
	r := Result{
		Root:       root,
		Name:       filepath.Base(root),
		Path:       tilde(root),
		BaseBranch: baseBranch(root),
		JiraKey:    jiraKey(root),
	}
	r.WorktreeBase = worktreeBase(root, r.Name)
	r.Suggestions = suggest(root)
	if cfg != nil {
		r.Existing = cfg.Find(root)
	}
	return r, nil
}

// baseBranch prefers what origin says its default is, then the usual names,
// then whatever is checked out.
func baseBranch(root string) string {
	if out, err := git.Run(root, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil {
		if b := strings.TrimPrefix(out, "origin/"); b != "" {
			return b
		}
	}
	for _, b := range []string{"main", "master", "develop"} {
		if git.BranchExists(root, b) {
			return b
		}
	}
	if b := git.CurrentBranch(root); b != "" {
		return b
	}
	return "main"
}

// jiraKey returns the project key used by most branches in the repo, so a repo
// whose history is full of EFRON-123 branches pre-fills EFRON.
func jiraKey(root string) string {
	out, err := git.Run(root, "for-each-ref", "--format=%(refname:short)", "--count=300",
		"--sort=-committerdate", "refs/heads", "refs/remotes")
	if err != nil {
		return ""
	}
	counts := map[string]int{}
	for _, ref := range strings.Split(out, "\n") {
		name := strings.TrimPrefix(ref, "origin/")
		if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		if m := issuePrefix.FindStringSubmatch(name); m != nil {
			counts[strings.ToUpper(m[1])]++
		}
	}
	type kv struct {
		key string
		n   int
	}
	var all []kv
	for k, n := range counts {
		all = append(all, kv{k, n})
	}
	if len(all) == 0 {
		return ""
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].n != all[j].n {
			return all[i].n > all[j].n
		}
		return all[i].key < all[j].key
	})
	return all[0].key
}

// worktreeBase prefers a sibling directory that already exists, so an
// established layout is picked up rather than replaced.
func worktreeBase(root, name string) string {
	parent := filepath.Dir(root)
	for _, candidate := range []string{name + "-worktrees", name + ".worktrees", "worktrees"} {
		if isDir(filepath.Join(parent, candidate)) {
			return tilde(filepath.Join(parent, candidate))
		}
	}
	if isDir(filepath.Join(root, ".worktrees")) {
		return tilde(filepath.Join(root, ".worktrees"))
	}
	return tilde(filepath.Join(parent, name+"-worktrees"))
}

// suggest proposes setup steps based on what is in the repo. Only the files a
// new worktree cannot produce for itself are worth copying; anything a build
// tool regenerates (node_modules above all) belongs to the worktree alone.
func suggest(root string) []config.Step {
	var envs []string
	for _, f := range []string{".env", ".env.local", ".env.development"} {
		if exists(filepath.Join(root, f)) {
			envs = append(envs, f)
		}
	}
	if len(envs) == 0 {
		return nil
	}
	return []config.Step{{Name: "env files", Copy: envs}}
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// tilde folds $HOME back to ~ so written config stays portable and readable.
func tilde(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}
