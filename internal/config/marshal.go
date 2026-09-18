package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const header = `# worq configuration
#
# Precedence: CLI flags > matched [[projects]] entry > [defaults] > built-in.
# Paths accept ~, $VARS and the placeholders {root}, {parent} and {name}.
`

// Upsert adds the project, replacing any existing entry with the same path.
// Returns true when an existing entry was replaced.
func (c *Config) Upsert(p Project) bool {
	target := Expand(p.Path)
	for i := range c.Projects {
		if Expand(c.Projects[i].Path) == target {
			c.Projects[i] = p
			return true
		}
	}
	c.Projects = append(c.Projects, p)
	sort.SliceStable(c.Projects, func(i, j int) bool {
		return Expand(c.Projects[i].Path) < Expand(c.Projects[j].Path)
	})
	return false
}

// Remove drops the entry for path, if present.
func (c *Config) Remove(path string) bool {
	target := Expand(path)
	for i := range c.Projects {
		if Expand(c.Projects[i].Path) == target {
			c.Projects = append(c.Projects[:i], c.Projects[i+1:]...)
			return true
		}
	}
	return false
}

// Find returns the entry registered for path, or nil.
func (c *Config) Find(path string) *Project {
	target := Expand(path)
	for i := range c.Projects {
		if Expand(c.Projects[i].Path) == target {
			return &c.Projects[i]
		}
	}
	return nil
}

// Marshal renders the config as TOML, emitting only the fields that are set.
// Hand-written comments in an existing file are not preserved: the file is
// re-serialised from the parsed config.
func (c *Config) Marshal() string {
	var b strings.Builder
	b.WriteString(header)

	b.WriteString("\n[defaults]\n")
	d := c.Defaults
	writeStr(&b, "worktree_base", d.WorktreeBase)
	writeStr(&b, "base_branch", d.BaseBranch)
	writeStr(&b, "branch_prefix", d.BranchPrefix)
	writeSteps(&b, "defaults", d.Setup)

	for _, p := range c.Projects {
		b.WriteString("\n[[projects]]\n")
		writeStr(&b, "name", p.Name)
		writeStr(&b, "path", p.Path)
		writeStr(&b, "worktree_base", p.WorktreeBase)
		writeStr(&b, "base_branch", p.BaseBranch)
		writeStr(&b, "jira_key", p.JiraKey)
		writeStr(&b, "branch_prefix", p.BranchPrefix)
		writeSteps(&b, "projects", p.Setup)
	}
	return b.String()
}

// MarshalProject renders a single [[projects]] entry, for previews.
func MarshalProject(p Project) string {
	c := &Config{Projects: []Project{p}}
	out := c.Marshal()
	// The header comment mentions [[projects]] too, so match the real table.
	i := strings.Index(out, "\n[[projects]]\n")
	if i < 0 {
		return out
	}
	return strings.TrimLeft(out[i:], "\n")
}

// Save writes the config, creating the directory if needed.
func (c *Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(c.Marshal()), 0o644)
}

func writeSteps(b *strings.Builder, owner string, steps []Step) {
	for _, s := range steps {
		fmt.Fprintf(b, "\n[[%s.setup]]\n", owner)
		writeStr(b, "name", s.Name)
		writeList(b, "copy", s.Copy)
		writeStr(b, "run", s.Run)
		writeStr(b, "dir", s.Dir)
		if s.Optional {
			b.WriteString("optional = true\n")
		}
	}
}

func writeStr(b *strings.Builder, key, val string) {
	if val == "" {
		return
	}
	fmt.Fprintf(b, "%s = %s\n", key, quote(val))
}

func writeList(b *strings.Builder, key string, vals []string) {
	if len(vals) == 0 {
		return
	}
	parts := make([]string, 0, len(vals))
	for _, v := range vals {
		parts = append(parts, quote(v))
	}
	fmt.Fprintf(b, "%s = [%s]\n", key, strings.Join(parts, ", "))
}

var quoter = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`, "\r", `\r`)

func quote(s string) string { return `"` + quoter.Replace(s) + `"` }
