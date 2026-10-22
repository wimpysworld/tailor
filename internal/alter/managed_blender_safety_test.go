package alter

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
)

func TestManagedBlenderOwnershipAndParents(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, unsafe := range []string{"unowned", "parent"} {
			t.Run(fmt.Sprintf("%t/%s", enabled, unsafe), func(t *testing.T) {
				root := t.TempDir()
				if unsafe == "unowned" {
					writeManagedTestFile(t, root, "nix/blender.nix", []byte("user-owned\n"))
				} else {
					managedSymlinkOrSkip(t, t.TempDir(), filepath.Join(root, "nix"))
				}
				cfg := &config.Config{MCP: &config.MCPSettings{Blender: &enabled}}
				if _, err := prepareManagedExecution(cfg, root, managedTestRenderer); err == nil {
					t.Fatal("unsafe Blender destination accepted")
				}
			})
		}
	}
}

func TestManagedBlenderFinalSymlinks(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			target := filepath.Join(outside, "target")
			writeManagedTestFile(t, outside, "target", []byte("outside\n"))
			if err := os.MkdirAll(filepath.Join(root, "nix"), 0o755); err != nil {
				t.Fatal(err)
			}
			managedSymlinkOrSkip(t, target, filepath.Join(root, "nix/blender.nix"))
			managedSymlinkOrSkip(t, target, filepath.Join(root, ".mcp.json"))
			cfg := &config.Config{MCP: &config.MCPSettings{Blender: &enabled}}
			execution, err := prepareManagedExecution(cfg, root, managedTestRenderer)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := applyManagedFiles(root, execution.plan); err != nil {
				t.Fatal(err)
			}
			assertManagedFileContent(t, target, "outside\n")
			info, err := os.Lstat(filepath.Join(root, ".mcp.json"))
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatal("client symlink changed")
			}
		})
	}
}

func TestPlaywrightOnlyBytesUnchanged(t *testing.T) {
	want := map[string]string{
		".mcp.json":          "263a9dcf33e35ec484fb457b15a34b89ee2a81e8904206a9637def3df7801bd3",
		".codex/config.toml": "5364152f454ccbc45c876fef07470f271e7f7d247f6b91784e6187287e0bdd79",
		"opencode.json":      "c499decc3cc3b85272183d53948f98176e191a9baf78bc72d2ff57115f0f3817",
		".pi/mcp.json":       "ef15b37f92120183fe2259d1581d3bf372b93a634ab0084f37323c1d4bbd2c31",
	}
	rendered := renderSelectedManagedFiles(t, &config.Config{MCP: &config.MCPSettings{Playwright: new(true)}})
	for path, hash := range want {
		if got := fmt.Sprintf("%x", sha256.Sum256(rendered[path])); got != hash {
			t.Errorf("%s hash = %s, want %s", path, got, hash)
		}
	}
}
