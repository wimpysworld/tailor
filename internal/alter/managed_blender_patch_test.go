package alter

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestManagedBlenderPatchOffline(t *testing.T) {
	source := os.Getenv("TAILOR_BLENDER_TEST_SOURCE")
	if source == "" {
		t.Skip("set TAILOR_BLENDER_TEST_SOURCE to an unpacked, unpatched pinned source")
	}
	patch := requireManagedExecutable(t, "patch")
	python := requireManagedExecutable(t, "python3")
	// The import policy rejects hidden ancestors, including a hidden test cache.
	root, err := os.MkdirTemp("/tmp", "tailor-blender-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})
	if err := os.CopyFS(root, os.DirFS(source)); err != nil {
		t.Fatal(err)
	}
	content, err := swatch.Content("nix/patches/blender-agent-bridge-local-glb.patch")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), patch, "--batch", "--fuzz=0", "-p1")
	command.Dir = root
	command.Stdin = strings.NewReader(string(content))
	output, err := command.CombinedOutput()
	if err != nil || strings.Contains(string(output), "offset") {
		t.Fatalf("strict patch application: %v\n%s", err, output)
	}
	command = exec.CommandContext(t.Context(), python, "-m", "unittest", "tests.unit.test_package_metadata", "tests.unit.test_mcp_stdio", "tests.unit.test_local_glb", "tests.unit.test_matched_views", "tests.unit.test_tool_registry", "-v")
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("patched source tests: %v\n%s", err, output)
	} else {
		t.Log(string(output))
	}
	command = exec.CommandContext(t.Context(), python, "scripts/build_extension_zip.py", "--dist-dir", filepath.Join(root, "dist"))
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("extension build: %v\n%s", err, output)
	}
	command = exec.CommandContext(t.Context(), python, "-c", `import pathlib, zipfile
archive, = pathlib.Path("dist").glob("*.zip")
with zipfile.ZipFile(archive) as z:
    names = set(z.namelist())
    assert {"__init__.py", "blender_manifest.toml", "LICENSE", "local_glb.py", "matched_views.py"} <= names
    assert not any(name.startswith("claude_blender/") for name in names)
`)
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("extension layout: %v\n%s", err, output)
	}
}
