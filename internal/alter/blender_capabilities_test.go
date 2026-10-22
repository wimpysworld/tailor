package alter_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestBlenderLifecycleAcrossModes(t *testing.T) {
	for _, mode := range []struct {
		name  string
		value alter.ApplyMode
	}{
		{name: "dry-run", value: alter.DryRun},
		{name: "apply", value: alter.Apply},
		{name: "recut", value: alter.Recut},
	} {
		for _, state := range []string{"", "false", "true"} {
			name := state
			if name == "" {
				name = "absent"
			}
			t.Run(mode.name+"/"+name, func(t *testing.T) {
				declaration := ""
				if state != "" {
					declaration = "mcp: {blender: " + state + "}\n"
				}
				tc := setupAlterTest(t, "license: none\n"+declaration+"swatches: []\n")
				fragment, err := swatch.Content("nix/blender.nix")
				if err != nil {
					t.Fatal(err)
				}
				writeOnDisk(t, tc.Dir, "nix/blender.nix", fragment)
				for _, path := range playwrightPaths[1:] {
					writeOnDisk(t, tc.Dir, path, []byte("disabled user settings\n"))
				}
				output := captureAlterRun(t, loadTestConfig(t, tc.Dir), tc.Dir, mode.value, tc.Client)
				for _, path := range playwrightPaths[1:] {
					assertCapabilityBytes(t, tc.Dir, path, []byte("disabled user settings\n"))
				}
				if state == "false" && mode.value != alter.DryRun {
					assertCapabilityMissing(t, tc.Dir, "nix/blender.nix")
				} else {
					assertCapabilityBytes(t, tc.Dir, "nix/blender.nix", fragment)
				}
				if state == "false" && !strings.Contains(output, "missing `blender-bridge` executable") {
					t.Fatal("missing removal warning")
				}
				if state == "" && strings.Contains(output, "blender") {
					t.Fatal("absent Blender was reported")
				}
				if _, err := os.Stat(filepath.Join(tc.Dir, "just/blender.just")); !os.IsNotExist(err) {
					t.Fatal("unexpected Blender Just fragment")
				}
			})
		}
	}
}
