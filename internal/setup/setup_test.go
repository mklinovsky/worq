package setup

import (
	"github.com/mklinovsky/worq/internal/config"
	"os"
	"path/filepath"
	"testing"
)

func TestInvalidStepHonorsOptional(t *testing.T) {
	for _, optional := range []bool{false, true} {
		t.Run(map[bool]string{false: "required", true: "optional"}[optional], func(t *testing.T) {
			src, dst := t.TempDir(), t.TempDir()
			if err := os.WriteFile(filepath.Join(src, "marker"), []byte("ok"), 0600); err != nil {
				t.Fatal(err)
			}
			results := Run([]config.Step{{Name: "invalid", Optional: optional}, {Copy: []string{"marker"}}}, src, dst, nil)
			if (Failed(results) == nil) != optional {
				t.Fatalf("unexpected failure: %+v", results)
			}
			_, err := os.Stat(filepath.Join(dst, "marker"))
			if optional && err != nil {
				t.Fatal("optional failure stopped later steps", err)
			}
			if !optional && !os.IsNotExist(err) {
				t.Fatal("required failure did not stop later steps")
			}
		})
	}
}
