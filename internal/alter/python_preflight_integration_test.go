package alter_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
)

func TestPythonPreflightStopsBeforeWritesAndRemoteCalls(t *testing.T) {
	for _, policy := range []string{"malformed", "symlink"} {
		t.Run(policy, func(t *testing.T) {
			tc := setupAlterTest(t, "license: none\nrepository:\n  has_wiki: true\nlanguages:\n  python: true\nswatches:\n  - path: .tailor.yml\n    alteration: always\n  - path: ruff.toml\n    alteration: always\n")
			writeOnDisk(t, tc.Dir, "ruff.toml", []byte("# existing Ruff policy\n"))
			writeOnDisk(t, tc.Dir, ".github/workflows/tailor.yml", []byte("retired workflow\n"))
			outside := t.TempDir()
			writeOnDisk(t, outside, "policy.toml", []byte("[tool.ruff]\n"))
			wantError := "reading Python policy"
			if policy == "malformed" {
				writeOnDisk(t, tc.Dir, "pyproject.toml", []byte("[tool.ruff\n"))
			} else {
				if err := os.Symlink(filepath.Join(outside, "policy.toml"), filepath.Join(tc.Dir, "pyproject.toml")); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
				wantError = "must be a regular file, not a symlink"
			}
			before := pagesAcceptanceSnapshot(t, tc.Dir)
			outsideBefore := pagesAcceptanceSnapshot(t, outside)
			_, err := alter.Execute(loadTestConfig(t, tc.Dir), tc.Dir, alter.Apply, tc.Client, nil, alter.Options{})
			if err == nil || !strings.Contains(err.Error(), wantError) {
				t.Fatalf("error = %v, want %q", err, wantError)
			}
			if calls := tc.Calls(); len(calls) != 0 {
				t.Fatalf("Python preflight allowed API calls: %v", calls)
			}
			if !reflect.DeepEqual(before, pagesAcceptanceSnapshot(t, tc.Dir)) {
				t.Fatal("Python preflight changed project files")
			}
			if !reflect.DeepEqual(outsideBefore, pagesAcceptanceSnapshot(t, outside)) {
				t.Fatal("Python preflight changed the symlink target")
			}
		})
	}
}
