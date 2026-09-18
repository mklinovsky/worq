package config

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const header = `# worq configuration
#
# Precedence: CLI flags > matched projects entry > defaults > built-in.
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

// Marshal renders the config as YAML, emitting only the fields that are set.
// Hand-written comments in an existing file are not preserved: the file is
// re-serialised from the parsed config.
func (c *Config) Marshal() string {
	return header + "\n" + encode(c)
}

// MarshalProject renders a single projects entry, for previews.
func MarshalProject(p Project) string {
	return encode([]Project{p})
}

// Save writes the config, creating the directory if needed.
func (c *Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(c.Marshal()), 0o644)
}

func encode(v any) string {
	var b strings.Builder
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	// Every value here comes from our own structs, which always encode.
	if err := enc.Encode(v); err != nil {
		return ""
	}
	enc.Close()
	return b.String()
}
