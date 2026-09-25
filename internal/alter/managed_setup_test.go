package alter

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
)

func TestManagedSetupRequiresCompatibilityFile(t *testing.T) {
	cfg := ffmpegConfig()
	selections, err := selectManagedFiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	rendered := renderSelectedManagedFiles(t, cfg)
	delete(rendered, "just/go-ffmpeg-statigo.just")
	if _, err := preflightManagedFiles(t.TempDir(), selections, rendered); err == nil || !strings.Contains(err.Error(), "setup contribution") {
		t.Fatalf("accepted missing compatibility file: %v", err)
	}
}

func TestManagedSetupEmpty(t *testing.T) {
	root := t.TempDir()
	writeManagedJustFixture(t, root, renderSelectedManagedFiles(t, &config.Config{}))
	writeManagedTestFile(t, root, "justfile", []byte("import 'just/loader.just'\n"))
	writeManagedTestFile(t, root, "just/project/setup.sh", []byte("exit 42\n"))
	// #nosec G204 -- The executable is a test tool and the fixture contains no setup commands.
	command := exec.CommandContext(t.Context(), managedLintExecutable(t), "--justfile", filepath.Join(root, "justfile"), "setup")
	output, err := command.CombinedOutput()
	if err != nil || string(output) != "No setup recipes are enabled.\n" {
		t.Fatalf("output=%q error=%v", output, err)
	}
}
