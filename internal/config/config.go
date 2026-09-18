// Package config loads and resolves worq configuration.
//
// A single central file holds `defaults` and a list of `projects` matched
// against the current working directory. Longest match wins.
// Precedence: CLI flags > project > defaults > built-in.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Step is a single post-create setup action. Exactly one of Copy/Run should be
// set; steps run in the order they appear in the config.
type Step struct {
	Name     string   `yaml:"name,omitempty"`
	Copy     []string `yaml:"copy,omitempty"`
	Run      string   `yaml:"run,omitempty"`
	Dir      string   `yaml:"dir,omitempty"`
	Optional bool     `yaml:"optional,omitempty"`
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
	WorktreeBase string `yaml:"worktree_base,omitempty"`
	BaseBranch   string `yaml:"base_branch,omitempty"`
	BranchPrefix string `yaml:"branch_prefix,omitempty"`
	Setup        []Step `yaml:"setup,omitempty"`
}

// Project is one repository entry.
type Project struct {
	Name         string `yaml:"name,omitempty"`
	Path         string `yaml:"path,omitempty"`
	WorktreeBase string `yaml:"worktree_base,omitempty"`
	BaseBranch   string `yaml:"base_branch,omitempty"`
	BranchPrefix string `yaml:"branch_prefix,omitempty"`
	JiraKey      string `yaml:"jira_key,omitempty"`
	Setup        []Step `yaml:"setup,omitempty"`
}

type Config struct {
	Defaults Defaults  `yaml:"defaults,omitempty"`
	Projects []Project `yaml:"projects,omitempty"`
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
	Matched      bool // true when a projects entry matched
}

func Path() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".worq", "config.yaml")
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
	if err := yaml.Unmarshal(b, cfg); err != nil {
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

// match picks the projects entry with the longest path that contains root or
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
