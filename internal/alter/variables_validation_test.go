package alter_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/model"
)

func TestAlterRunRejectsInvalidVariablesBeforeSideEffects(t *testing.T) {
	for _, tt := range []struct {
		name    string
		entries []model.VariableEntry
	}{
		{"missing value", []model.VariableEntry{{Name: "APP"}}},
		{"reserved name", []model.VariableEntry{{Name: "github_app", Value: new("")}}},
		{"duplicate name", []model.VariableEntry{{Name: "APP", Value: new("one")}, {Name: "app", Value: new("two")}}},
		{"oversized value", []model.VariableEntry{{Name: "APP", Value: new(strings.Repeat("a", 48*1024+1))}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			const configYAML = "license: none\nswatches:\n  - path: .github/workflows/tailor.yml\n    alteration: never\n  - path: .gitignore\n    alteration: first-fit\n"
			tc := setupAlterTest(t, configYAML)
			writeOnDisk(t, tc.Dir, ".github/workflows/tailor.yml", []byte("retired"))
			cfg := loadTestConfig(t, tc.Dir)
			cfg.Variables = tt.entries
			if err := alter.Run(cfg, tc.Dir, alter.Apply, tc.Client, nil, nil); err == nil || !strings.Contains(err.Error(), "variable") {
				t.Fatalf("Run() = %v, want variable validation error", err)
			}
			if calls := tc.Calls(); len(calls) != 0 {
				t.Fatalf("unexpected API calls: %v", calls)
			}
			for path, want := range map[string]string{".tailor.yml": configYAML, ".github/workflows/tailor.yml": "retired"} {
				data, err := os.ReadFile(filepath.Join(tc.Dir, path))
				if err != nil || string(data) != want {
					t.Fatalf("%s changed, error = %v", path, err)
				}
			}
			if _, err := os.Stat(filepath.Join(tc.Dir, ".gitignore")); !os.IsNotExist(err) {
				t.Fatalf("unexpected .gitignore, error = %v", err)
			}
		})
	}
}
