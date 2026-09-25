package alter

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
)

func TestManagedSetupEmpty(t *testing.T) {
	root := t.TempDir()
	writeManagedJustFixture(t, root, renderSelectedManagedFiles(t, &config.Config{}))
	writeManagedTestFile(t, root, "justfile", []byte("import 'just/loader.just'\n"))
	// #nosec G204 -- The executable is a test tool and the fixture contains no setup commands.
	command := exec.CommandContext(t.Context(), managedLintExecutable(t), "--justfile", filepath.Join(root, "justfile"), "setup")
	output, err := command.CombinedOutput()
	if err != nil || string(output) != "No setup recipes are enabled.\n" {
		t.Fatalf("output=%q error=%v", output, err)
	}
}
