// Package git wraps the git commands worq needs.
package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(out.String()), nil
}

// Run executes git in dir and returns trimmed stdout.
func Run(dir string, args ...string) (string, error) { return run(dir, args...) }

// MainRoot returns the main worktree of the repository containing dir, so the
// same project resolves whether you are in the repo or in a linked worktree.
func MainRoot(dir string) (string, error) {
	out, err := run(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	root := strings.TrimSuffix(strings.TrimSpace(out), "/.git")
	if root == out { // bare repo or unusual layout
		root = strings.TrimSuffix(out, "/.git/")
	}
	return root, nil
}

// Root returns the top level of the worktree containing dir.
func Root(dir string) (string, error) {
	return run(dir, "rev-parse", "--show-toplevel")
}

// CurrentBranch returns the checked out branch, or "" when detached.
func CurrentBranch(dir string) string {
	out, err := run(dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return out
}

// Worktree is one entry of `git worktree list`.
type Worktree struct {
	Path     string
	Head     string
	Branch   string
	Detached bool
	Locked   bool
	Prunable bool
	Main     bool
}

// Name is the directory name, which is how worq addresses a worktree.
func (w Worktree) Name() string {
	i := strings.LastIndex(w.Path, "/")
	if i < 0 {
		return w.Path
	}
	return w.Path[i+1:]
}

// List parses `git worktree list --porcelain`.
func List(dir string) ([]Worktree, error) {
	out, err := run(dir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var (
		res []Worktree
		cur *Worktree
	)
	flush := func() {
		if cur != nil {
			res = append(res, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur = &Worktree{Path: strings.TrimPrefix(line, "worktree "), Main: len(res) == 0}
		case cur == nil:
			continue
		case strings.HasPrefix(line, "HEAD "):
			cur.Head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		case line == "detached":
			cur.Detached = true
		case line == "locked" || strings.HasPrefix(line, "locked "):
			cur.Locked = true
		case line == "prunable" || strings.HasPrefix(line, "prunable "):
			cur.Prunable = true
		}
	}
	flush()
	return res, nil
}

// BranchExists reports whether a local branch of that name exists.
func BranchExists(dir, branch string) bool {
	_, err := run(dir, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// RemoteBranchExists reports whether origin has the branch.
func RemoteBranchExists(dir, branch string) bool {
	out, err := run(dir, "ls-remote", "--heads", "origin", branch)
	return err == nil && out != ""
}

// HasRemote reports whether the named remote is configured.
func HasRemote(dir, remote string) bool {
	out, err := run(dir, "remote")
	if err != nil {
		return false
	}
	for _, r := range strings.Split(out, "\n") {
		if strings.TrimSpace(r) == remote {
			return true
		}
	}
	return false
}

// Fetch updates one branch from the remote.
func Fetch(dir, remote, branch string) error {
	_, err := run(dir, "fetch", "--quiet", remote, branch)
	return err
}

// StartPoint returns the ref a new branch should start from: the remote
// tracking branch when there is one, so worktrees begin at what the remote
// actually has rather than at a local checkout that may be weeks old.
func StartPoint(dir, base string) string {
	if strings.HasPrefix(base, "origin/") {
		return base
	}
	if _, err := run(dir, "rev-parse", "--verify", "--quiet", "origin/"+base+"^{commit}"); err == nil {
		return "origin/" + base
	}
	return ResolveBase(dir, base)
}

// AddWorktree creates a worktree at path. When the branch already exists it is
// checked out; otherwise it is created from base.
func AddWorktree(dir, path, branch, base string) error {
	args := []string{"worktree", "add"}
	if BranchExists(dir, branch) {
		args = append(args, path, branch)
	} else {
		args = append(args, "-b", branch, path, base)
	}
	_, err := run(dir, args...)
	return err
}

// RemoteRefExists reports whether a remote-tracking ref such as origin/main is
// present locally (after a fetch).
func RemoteRefExists(dir, ref string) bool {
	_, err := run(dir, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return err == nil
}

// AddTrackingWorktree checks out a branch that exists on the remote, with the
// local branch set to track it.
func AddTrackingWorktree(dir, path, branch, remote string) error {
	_, err := run(dir, "worktree", "add", "--track", "-b", branch, path, remote+"/"+branch)
	return err
}

// RemoveWorktree removes a worktree, optionally discarding local changes.
func RemoveWorktree(dir, path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	_, err := run(dir, append(args, path)...)
	return err
}

// DeleteBranch deletes a local branch.
func DeleteBranch(dir, branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := run(dir, "branch", flag, branch)
	return err
}

func Prune(dir string) error {
	_, err := run(dir, "worktree", "prune")
	return err
}

// Ahead returns the number of commits on branch that are not in base. Zero
// means the branch is fully contained in base, i.e. safe to clean up.
func Ahead(dir, branch, base string) (int, error) {
	out, err := run(dir, "rev-list", "--count", base+".."+branch)
	if err != nil {
		return 0, err
	}
	n := 0
	if _, err := fmt.Sscanf(out, "%d", &n); err != nil {
		return 0, err
	}
	return n, nil
}

// Status returns porcelain status lines for a worktree.
func Status(dir string) ([]string, error) {
	out, err := run(dir, "status", "--porcelain")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// Dirty reports whether the worktree has uncommitted changes, untracked files
// included. It walks the working tree, so it is the slow one: use it before
// destroying something, not to paint a list.
func Dirty(dir string) bool {
	lines, err := Status(dir)
	return err == nil && len(lines) > 0
}

// Modified reports whether tracked files differ from HEAD. Roughly ten times
// cheaper than Dirty on a large repository because it never scans for
// untracked files — which also keeps copied .env files from reading as dirty.
func Modified(dir string) bool {
	_, err := run(dir, "diff", "--quiet", "HEAD")
	return err != nil
}

// BranchAges returns a relative commit age per local branch, in one call.
func BranchAges(dir string) map[string]string {
	out, err := run(dir, "for-each-ref", "--format=%(refname:short)\t%(committerdate:relative)", "refs/heads")
	if err != nil {
		return nil
	}
	ages := make(map[string]string)
	for _, line := range strings.Split(out, "\n") {
		if name, age, ok := strings.Cut(line, "\t"); ok {
			ages[name] = age
		}
	}
	return ages
}

// ResolveBase returns base if it exists locally, otherwise a sensible
// fallback among origin/<base>, main and master.
func ResolveBase(dir, base string) string {
	for _, c := range []string{base, "origin/" + base, "main", "master"} {
		if _, err := run(dir, "rev-parse", "--verify", "--quiet", c+"^{commit}"); err == nil {
			return c
		}
	}
	return base
}
