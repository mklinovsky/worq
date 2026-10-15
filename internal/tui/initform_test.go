package tui

import (
	"github.com/mklinovsky/worq/internal/config"
	"path/filepath"
	"testing"
)

func TestFormWithNoStepsDisablesDefaults(t *testing.T) {
	m := InitModel{}
	p := m.project()
	p.Path = "/repo"
	c := config.Config{Defaults: config.Defaults{Setup: []config.Step{{Run: "echo default"}}}, Projects: []config.Project{p}}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	back, err := config.LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Resolve("/repo", "/repo").Setup) != 0 {
		t.Fatal("empty form inherited defaults")
	}
}
