// Package config loads and resolves worq configuration.
//
// Model mirrors gira: a single central file with [defaults] and a list of
// [[projects]] matched against the current working directory. Longest match
// wins. Precedence: CLI flags > project > defaults > built-in.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Step is a single post-create setup action. Exactly one of Copy/Run should be
// set; steps run in the order they appear in the config.
type Step struct {
	Name     string   `toml:"name"`
	Copy     []string `toml:"copy"`
	Run      string   `toml:"run"`
	Dir      string   `toml:"dir"`
	Optional bool     `toml:"optional"`
}

func (s Step) Label() string {
	if s.Name != "" {
		return s.Name
	}
	switch {
	case len(s.Copy) > 0:
		return "copy " + strings.Join(s.Copy, ", ")
	case s.Run != "":
		return s.Run
	}
	return "noop"
}

// Defaults applies to every project unless overridden.
type Defaults struct {
	WorktreeBase string `toml:"worktree_base"`
	BaseBranch   string `toml:"base_branch"`
	BranchPrefix string `toml:"branch_prefix"`
	Setup        []Step `toml:"setup"`
}

// Project is one repository entry.
type Project struct {
	Name         string `toml:"name"`
	Path         string `toml:"path"`
	WorktreeBase string `toml:"worktree_base"`
	BaseBranch   string `toml:"base_branch"`
	BranchPrefix string `toml:"branch_prefix"`
	JiraKey      string `toml:"jira_key"`
	Setup        []Step `toml:"setup"`
}

type Config struct {
	Defaults Defaults  `toml:"defaults"`
	Projects []Project `toml:"projects"`
}

// Resolved is the effective config for one repository.
type Resolved struct {
	Name         string
	Root         string
	WorktreeBase string
	BaseBranch   string
	BranchPrefix string
	JiraKey      string
	Setup        []Step
	Matched      bool // true when a [[projects]] entry matched
}

// Path returns the config file location: $WORQ_CONFIG, else
// $XDG_CONFIG_HOME/worq/config.toml, else ~/.config/worq/config.toml.
func Path() string {
	if p := os.Getenv("WORQ_CONFIG"); p != "" {
		return Expand(p)
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "worq", "config.toml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "worq", "config.toml")
}

// Expand resolves a leading ~ and makes the path absolute where possible.
func Expand(p string) string {
	if p == "" {
		return ""
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			p = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
		}
	}
	return filepath.Clean(os.ExpandEnv(p))
}

// Load reads the config file. A missing file is not an error: worq works with
// built-in defaults so it is useful before `worq init` is ever run.
func Load() (*Config, error) {
	return LoadFrom(Path())
}

func LoadFrom(path string) (*Config, error) {
	cfg := &Config{}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	if err := toml.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// Resolve merges defaults with the project entry matching root (or cwd when
// running inside a linked worktree kept outside the repo).
func (c *Config) Resolve(root, cwd string) Resolved {
	r := Resolved{
		Root:         root,
		Name:         filepath.Base(root),
		BaseBranch:   c.Defaults.BaseBranch,
		BranchPrefix: c.Defaults.BranchPrefix,
		WorktreeBase: c.Defaults.WorktreeBase,
		Setup:        c.Defaults.Setup,
	}

	if p := c.match(root, cwd); p != nil {
		r.Matched = true
		if p.Name != "" {
			r.Name = p.Name
		}
		if p.WorktreeBase != "" {
			r.WorktreeBase = p.WorktreeBase
		}
		if p.BaseBranch != "" {
			r.BaseBranch = p.BaseBranch
		}
		if p.BranchPrefix != "" {
			r.BranchPrefix = p.BranchPrefix
		}
		if p.JiraKey != "" {
			r.JiraKey = p.JiraKey
		}
		if len(p.Setup) > 0 {
			r.Setup = p.Setup
		}
	}

	if r.BaseBranch == "" {
		r.BaseBranch = "main"
	}
	if r.WorktreeBase == "" {
		r.WorktreeBase = filepath.Join(filepath.Dir(root), r.Name+"-worktrees")
	}
	r.WorktreeBase = Expand(expandVars(r.WorktreeBase, r.Root, r.Name))
	return r
}

// match picks the project entry with the longest path that contains root or
// cwd. Checking cwd too means commands still resolve the right project when
// run from a worktree kept outside the repository.
func (c *Config) match(root, cwd string) *Project {
	var best *Project
	bestLen := -1
	consider := func(p *Project, candidate string) {
		if candidate == "" || len(candidate) <= bestLen {
			return
		}
		best, bestLen = p, len(candidate)
	}
	for i := range c.Projects {
		p := &c.Projects[i]
		path := Expand(p.Path)
		if path == "" {
			continue
		}
		if under(root, path) || under(cwd, path) {
			consider(p, path)
		}
		if base := Expand(expandVars(p.WorktreeBase, path, projName(p, path))); base != "" {
			if under(root, base) || under(cwd, base) {
				consider(p, base)
			}
		}
	}
	return best
}

func projName(p *Project, path string) string {
	if p.Name != "" {
		return p.Name
	}
	return filepath.Base(path)
}

func expandVars(s, root, name string) string {
	s = strings.ReplaceAll(s, "{root}", root)
	s = strings.ReplaceAll(s, "{name}", name)
	s = strings.ReplaceAll(s, "{parent}", filepath.Dir(root))
	return s
}

// under reports whether path is dir or lives inside it.
func under(path, dir string) bool {
	if path == "" || dir == "" {
		return false
	}
	path, dir = filepath.Clean(path), filepath.Clean(dir)
	if path == dir {
		return true
	}
	return strings.HasPrefix(path, dir+string(filepath.Separator))
}
