package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// CreateFile creates a file at filepath.Join(dir, name) with dummy content.
// Parent directories are created as needed.
func CreateFile(t *testing.T, dir, name string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// WriteConfig writes a .tailor/config.yml file in dir with the given content.
func WriteConfig(t *testing.T, dir, content string) {
	t.Helper()
	tailorDir := filepath.Join(dir, ".tailor")
	if err := os.MkdirAll(tailorDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tailorDir, "config.yml"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// AssertBoolPtr checks a *bool field. When wantNil is true, it expects got to
// be nil. Otherwise it expects got to be non-nil with value wantVal.
func AssertBoolPtr(t *testing.T, got *bool, wantNil bool, wantVal bool, field string) {
	t.Helper()
	if wantNil {
		if got != nil {
			t.Errorf("%s = %v, want nil", field, *got)
		}
		return
	}
	if got == nil {
		t.Errorf("%s is nil, want %v", field, wantVal)
		return
	}
	if *got != wantVal {
		t.Errorf("%s = %v, want %v", field, *got, wantVal)
	}
}

// AssertStringPtr checks a *string field. When wantNil is true, it expects got
// to be nil. Otherwise it expects got to be non-nil with value wantVal.
func AssertStringPtr(t *testing.T, got *string, wantNil bool, wantVal string, field string) {
	t.Helper()
	if wantNil {
		if got != nil {
			t.Errorf("%s = %q, want nil", field, *got)
		}
		return
	}
	if got == nil {
		t.Errorf("%s is nil, want %q", field, wantVal)
		return
	}
	if *got != wantVal {
		t.Errorf("%s = %q, want %q", field, *got, wantVal)
	}
}
