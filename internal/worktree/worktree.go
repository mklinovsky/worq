// Package worktree is the layer both the CLI and the TUI drive.
package worktree

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mklinovsky/worq/internal/config"
	"github.com/mklinovsky/worq/internal/git"
	"github.com/mklinovsky/worq/internal/naming"
	"github.com/mklinovsky/worq/internal/setup"
)

// Service holds the resolved project context for one invocation.
type Service struct {
	Cfg config.Resolved

	baseOnce sync.Once
	base     string
}

// Open resolves the repository containing dir and its configuration.
func Open(dir string) (*Service, error) {
	root, err := git.MainRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("not inside a git repository (%s)", dir)
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	return &Service{Cfg: cfg.Resolve(root, dir)}, nil
}

// Entry is a worktree plus the bits worq displays.
type Entry struct {
	git.Worktree
	Age     string // relative age of the branch tip
	Dirty   bool
	Ahead   int
	Current bool
	Loaded  bool // Dirty and Ahead have been filled in
}

// List returns the worktrees of the project, main worktree first. It runs one
// git command and does not touch the working trees, so it is fast enough to
// paint a list with; call Fill for the status of a single entry.
func (s *Service) List(cwd string) ([]Entry, error) {
	wts, err := git.List(s.Cfg.Root)
	if err != nil {
		return nil, err
	}
	ages := git.BranchAges(s.Cfg.Root)
	out := make([]Entry, 0, len(wts))
	for _, w := range wts {
		e := Entry{Worktree: w, Age: ages[w.Branch]}
		e.Current = cwd != "" && (cwd == w.Path || strings.HasPrefix(cwd, w.Path+string(filepath.Separator)))
		out = append(out, e)
	}
	return out, nil
}

// Fill adds the status of one worktree: whether tracked files were modified,
// and how many commits the branch holds that the base branch does not. Each
// call is independent, so they can run concurrently.
func (s *Service) Fill(e Entry) Entry {
	e.Dirty = git.Modified(e.Path)
	if e.Branch != "" && !e.Main {
		if n, err := git.Ahead(s.Cfg.Root, e.Branch, s.Base()); err == nil {
			e.Ahead = n
		}
	}
	e.Loaded = true
	return e
}

// ListFilled is List plus Fill for every entry, in parallel.
func (s *Service) ListFilled(cwd string) ([]Entry, error) {
	entries, err := s.List(cwd)
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i := range entries {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			entries[i] = s.Fill(entries[i])
		}(i)
	}
	wg.Wait()
	return entries, nil
}

// Base resolves the base branch once and caches it: every Ahead call needs it.
func (s *Service) Base() string {
	s.baseOnce.Do(func() {
		s.base = git.ResolveBase(s.Cfg.Root, s.Cfg.BaseBranch)
	})
	return s.base
}

// Find locates a worktree by directory name, branch or Jira key.
func (s *Service) Find(name string) (*Entry, error) {
	entries, err := s.List("")
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(name)
	var partial []Entry
	for _, e := range entries {
		if strings.EqualFold(e.Name(), name) || strings.EqualFold(e.Branch, name) {
			return &e, nil
		}
		if strings.Contains(strings.ToLower(e.Name()), needle) ||
			strings.Contains(strings.ToLower(e.Branch), needle) {
			partial = append(partial, e)
		}
	}
	switch len(partial) {
	case 0:
		return nil, fmt.Errorf("no worktree matching %q", name)
	case 1:
		return &partial[0], nil
	default:
		names := make([]string, 0, len(partial))
		for _, e := range partial {
			names = append(names, e.Name())
		}
		return nil, fmt.Errorf("%q matches %s", name, strings.Join(names, ", "))
	}
}

// CreateOptions controls New.
type CreateOptions struct {
	Args    []string // words typed on the command line
	Base    string   // override base branch
	NoSetup bool     // skip the configured setup steps
	NoFetch bool     // do not refresh the base branch from origin first
	Track   bool     // check out an existing origin branch instead of creating one
	Report  setup.Reporter
}

// Created describes a new worktree.
type Created struct {
	Path    string
	Branch  string
	Results []setup.Result
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

func emit(r setup.Reporter, e setup.Event) {
	if r != nil {
		r(e)
	}
}

// New creates the worktree, then runs the configured setup steps.
func (s *Service) New(opts CreateOptions) (*Created, error) {
	var (
		branch, dir string
		track       bool
	)
	if opts.Track {
		var err error
		if branch, err = s.remoteBranch(opts); err != nil {
			return nil, err
		}
		dir, track = naming.DirName(branch), true
	} else {
		n := naming.Build(opts.Args, s.Cfg.JiraKey, s.Cfg.BranchPrefix)
		if n.Branch == "" {
			return nil, errors.New("give me something to name the branch")
		}
		branch, dir = n.Branch, n.Dir
	}

	path := filepath.Join(s.Cfg.WorktreeBase, dir)
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("%s already exists", path)
	}
	if err := os.MkdirAll(s.Cfg.WorktreeBase, 0o755); err != nil {
		return nil, err
	}

	switch {
	case track && !git.BranchExists(s.Cfg.Root, branch):
		emit(opts.Report, setup.Event{Label: "tracking origin/" + branch, Done: true})
		if err := git.AddTrackingWorktree(s.Cfg.Root, path, branch, "origin"); err != nil {
			return nil, err
		}
	default:
		base := s.Cfg.BaseBranch
		if opts.Base != "" {
			base = opts.Base
		}
		// Branch from what the remote has, not from a local checkout that may
		// be days behind: fetch the base branch, then start from origin/<base>.
		if !track && !opts.NoFetch && git.HasRemote(s.Cfg.Root, "origin") && !strings.HasPrefix(base, "origin/") {
			s.fetch(opts.Report, base)
		}
		if err := git.AddWorktree(s.Cfg.Root, path, branch, git.StartPoint(s.Cfg.Root, base)); err != nil {
			return nil, err
		}
	}

	c := &Created{Path: path, Branch: branch}
	if !opts.NoSetup {
		c.Results = setup.Run(s.Cfg.Setup, s.Cfg.Root, path, opts.Report)
	}
	return c, setup.Failed(c.Results)
}

// remoteBranch resolves the branch named for --track, fetching it first so a
// branch pushed a minute ago is found.
func (s *Service) remoteBranch(opts CreateOptions) (string, error) {
	if len(opts.Args) != 1 {
		return "", errors.New("--track takes exactly one branch name")
	}
	branch := strings.TrimPrefix(strings.TrimSpace(opts.Args[0]), "origin/")
	if branch == "" {
		return "", errors.New("--track needs a branch name")
	}
	if !git.HasRemote(s.Cfg.Root, "origin") {
		return "", errors.New("this repository has no origin remote")
	}
	if !opts.NoFetch {
		s.fetch(opts.Report, branch)
	}
	if !git.BranchExists(s.Cfg.Root, branch) && !git.RemoteRefExists(s.Cfg.Root, "origin/"+branch) {
		return "", fmt.Errorf("no branch %q on origin", branch)
	}
	return branch, nil
}

func (s *Service) fetch(report setup.Reporter, branch string) {
	label := "fetch origin/" + branch
	emit(report, setup.Event{Label: label})
	err := git.Fetch(s.Cfg.Root, "origin", branch)
	if err != nil {
		// git is chatty on failure; one line is enough to say we are offline.
		err = errors.New(firstLine(err.Error()))
	}
	emit(report, setup.Event{Label: label, Done: true, Err: err})
	if err != nil {
		emit(report, setup.Event{Label: label, Line: "using what is already fetched"})
	}
}

// Setup re-runs the configured steps against an existing worktree.
func (s *Service) Setup(path string, report setup.Reporter) []setup.Result {
	return setup.Run(s.Cfg.Setup, s.Cfg.Root, path, report)
}

// Remove deletes a worktree and optionally its branch.
func (s *Service) Remove(e *Entry, force, deleteBranch bool) error {
	if e.Main {
		return errors.New("refusing to remove the main worktree")
	}
	if !force && git.Dirty(e.Path) {
		return fmt.Errorf("%s has uncommitted changes (use --force)", e.Name())
	}
	if err := git.RemoveWorktree(s.Cfg.Root, e.Path, force); err != nil {
		return err
	}
	if deleteBranch && e.Branch != "" {
		if err := git.DeleteBranch(s.Cfg.Root, e.Branch, force); err != nil {
			return fmt.Errorf("worktree removed, branch kept: %w", err)
		}
	}
	return nil
}

// Stale returns worktrees whose branch holds no commits missing from base:
// merged or never used, so safe to clean up. Dirty ones are never stale.
func (s *Service) Stale() ([]Entry, error) {
	entries, err := s.ListFilled("")
	if err != nil {
		return nil, err
	}
	var stale []Entry
	for _, e := range entries {
		if e.Main || e.Locked || e.Dirty || e.Branch == "" || e.Ahead != 0 {
			continue
		}
		if git.Dirty(e.Path) { // thorough check before proposing a deletion
			continue
		}
		stale = append(stale, e)
	}
	return stale, nil
}
