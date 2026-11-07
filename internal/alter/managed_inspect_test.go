package alter

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func managedMissingSetupFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for path, content := range map[string]string{
		"justfile":  "default:\n    @just --list\n",
		"flake.nix": `{ outputs = { self }: { devShells.x86_64-linux = { default = pkgs.mkShell { packages = [ pkgs.just ]; }; }; }; }`,
		".mcp.json": `{}`, ".pi/mcp.json": `{}`, "opencode.json": `{}`, ".codex/config.toml": "",
	} {
		writeManagedTestFile(t, dir, path, []byte(content))
	}
	return dir
}

func TestManagedInspectionReadsAreBoundedAndReadOnly(t *testing.T) {
	for _, tc := range []struct {
		name, path, content, cause string
		symlink, parent, directory bool
	}{
		{name: "regular", path: ".mcp.json", content: "{}"},
		{name: "missing", path: ".mcp.json", cause: "absent"},
		{name: "oversized", path: ".mcp.json", content: strings.Repeat("x", managedInspectionLimit+1), cause: "1 MiB"},
		{name: "directory", path: ".mcp.json", directory: true, cause: "not regular"},
		{name: "final symlink", path: ".mcp.json", symlink: true, cause: "symlink"},
		{name: "parent symlink", path: ".pi/mcp.json", parent: true, cause: "symlink"},
		{name: "escape", path: "../outside", cause: "local clean path"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			external := t.TempDir()
			writeManagedTestFile(t, external, "mcp.json", []byte("external secret"))
			switch {
			case tc.symlink:
				managedSymlinkOrSkip(t, filepath.Join(external, "mcp.json"), filepath.Join(dir, tc.path))
			case tc.parent:
				managedSymlinkOrSkip(t, external, filepath.Join(dir, ".pi"))
			case tc.directory:
				if err := os.Mkdir(filepath.Join(dir, tc.path), 0o755); err != nil {
					t.Fatal(err)
				}
			case tc.content != "":
				writeManagedTestFile(t, dir, tc.path, []byte(tc.content))
			}
			before, outside := snapshotManagedTree(t, dir), snapshotManagedTree(t, external)
			content, cause := readManagedInspection(dir, tc.path)
			if tc.cause == "" {
				if cause != "" || string(content) != tc.content {
					t.Fatalf("read = %q, %s", content, cause)
				}
			} else if content != nil || !strings.Contains(cause, tc.cause) {
				t.Fatalf("read = %q, %s", content, cause)
			}
			if !reflect.DeepEqual(before, snapshotManagedTree(t, dir)) || !reflect.DeepEqual(outside, snapshotManagedTree(t, external)) {
				t.Fatal("inspection changed files")
			}
		})
	}
}

func TestManagedClientInspection(t *testing.T) {
	for _, tc := range []struct {
		name, path, content string
		present, unknown    bool
	}{
		{"JSON missing", ".mcp.json", `{}`, false, false},
		{"Claude disabled", ".mcp.json", `{"mcpServers":{"playwright":{"disabled":true}}}`, true, false},
		{"Pi disabled", ".pi/mcp.json", `{"mcpServers":{"playwright":{"enabled":false}}}`, true, false},
		{"OpenCode disabled", "opencode.json", `{"mcp":{"playwright":{"enabled":false}}}`, true, false},
		{"Codex disabled", ".codex/config.toml", "[mcp_servers.\"playwright\"]\nenabled = false\n", true, false},
		{"Codex empty", ".codex/config.toml", "", false, false},
		{"invalid JSON", ".mcp.json", `{"secret":"do-not-print",`, false, true},
		{"invalid TOML", ".codex/config.toml", "[mcp_servers", false, true},
		{"null entry", ".mcp.json", `{"mcpServers":{"playwright":null}}`, true, false},
		{"wrong collection", ".mcp.json", `{"mcpServers":[]}`, false, true},
		{"wrong root", ".mcp.json", `[]`, false, true},
		{"duplicate keys", ".mcp.json", `{"mcpServers":{"playwright":{}},"mcpServers":{}}`, false, true},
		{"trailing JSON", ".mcp.json", `{} {}`, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeManagedTestFile(t, dir, tc.path, []byte(tc.content))
			cfg := &config.Config{MCP: &config.MCPSettings{Playwright: new(true)}}
			guidance := managedMCPAdoptionGuidance(cfg, tc.path, fixedManagedMCPRegistry(), dir)
			if tc.present {
				if len(guidance) != 0 {
					t.Fatalf("duplicate adoption advice: %v", guidance)
				}
			} else {
				prefix := "Add "
				if tc.unknown {
					prefix = "Verify "
				}
				if len(guidance) != 1 || !strings.HasPrefix(guidance[0], prefix) {
					t.Fatalf("guidance = %v", guidance)
				}
			}
			if strings.Contains(strings.Join(guidance, ""), "do-not-print") {
				t.Fatal("guidance exposed file content")
			}
			for _, disabled := range []*config.Config{{}, {MCP: &config.MCPSettings{Playwright: new(false)}}} {
				if got := managedMCPAdoptionGuidance(disabled, tc.path, fixedManagedMCPRegistry(), dir); len(got) != 0 {
					t.Fatalf("inactive capability advice: %v", got)
				}
			}
			assertManagedBytes(t, filepath.Join(dir, tc.path), []byte(tc.content))
		})
	}
}

func TestManagedJustInspection(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		state        managedSetupState
	}{
		{"import", "import 'just/loader.just'\n", managedSetupPresent},
		{"double quotes", "import \"./just/loader.just\" # comment\n", managedSetupPresent},
		{"missing", "default:\n    @just --list\n", managedSetupMissing},
		{"comment", "# import 'just/loader.just'\n", managedSetupMissing},
		{"string", "message := \"import 'just/loader.just'\"\n", managedSetupMissing},
		{"recipe", "show:\n    echo \"import 'just/loader.just'\"\n", managedSetupMissing},
		{"multiline string", "text := '''\nimport 'just/loader.just'\n'''\n", managedSetupUnknown},
		{"composition", "import 'other.just'\n", managedSetupUnknown},
		{"invalid", "custom just root\n", managedSetupUnknown},
		{"dynamic", "import env('JUST_IMPORT')\n", managedSetupUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := inspectManagedJust([]byte(tc.source)); got.state != tc.state {
				t.Fatalf("inspection = %#v, want %v", got, tc.state)
			}
		})
	}
}

func TestManagedNixInspection(t *testing.T) {
	for _, tc := range []struct {
		name, body      string
		packages, hooks managedSetupState
	}{
		{"missing", `packages = [ pkgs.just ];`, managedSetupMissing, managedSetupMissing},
		{"present", `packages = [ pkgs.just ] ++ import ./nix/loader.nix { inherit pkgs; }; shellHook = import ./nix/hooks.nix { inherit pkgs; };`, managedSetupPresent, managedSetupPresent},
		{"comments", `packages = []; # ++ import ./nix/loader.nix { inherit pkgs; }
# shellHook = import ./nix/hooks.nix { inherit pkgs; };`, managedSetupMissing, managedSetupMissing},
		{"block comments", `packages = []; /* shellHook = import ./nix/hooks.nix { inherit pkgs; }; */`, managedSetupMissing, managedSetupMissing},
		{"strings", `packages = [ "import ./nix/loader.nix { inherit pkgs; }" ]; note = "shellHook = import ./nix/hooks.nix { inherit pkgs; };";`, managedSetupMissing, managedSetupMissing},
		{"custom hook last", `packages = import ./nix/loader.nix { inherit pkgs; }; shellHook = (import ./nix/hooks.nix { inherit pkgs; }) + "\n" + projectHook;`, managedSetupPresent, managedSetupPresent},
		{"custom hook first", `packages = []; shellHook = projectHook + (import ./nix/hooks.nix { inherit pkgs; });`, managedSetupMissing, managedSetupUnknown},
		{"quoted interpolation", `packages = []; shellHook = "${import ./nix/hooks.nix { inherit pkgs; }}";`, managedSetupMissing, managedSetupUnknown},
		{"indented interpolation", `packages = []; shellHook = ''${import ./nix/hooks.nix { inherit pkgs; }}'';`, managedSetupMissing, managedSetupUnknown},
		{"nested interpolation", `packages = []; shellHook = "${"${import ./nix/hooks.nix { inherit pkgs; }}"}";`, managedSetupMissing, managedSetupUnknown},
		{"escaped quoted interpolation", `packages = []; shellHook = "echo \${PATH}";`, managedSetupMissing, managedSetupMissing},
		{"system name hook", `packages = []; shellHook = "x86_64-linux";`, managedSetupMissing, managedSetupMissing},
		{"escaped indented interpolation", `packages = []; shellHook = ''echo ''${PATH}'';`, managedSetupMissing, managedSetupMissing},
		{"overridden args", `packages = import ./nix/loader.nix { pkgs = pkgs // { go = pkgs.go_1_26; }; }; shellHook = import ./nix/hooks.nix { inherit pkgs; };`, managedSetupUnknown, managedSetupPresent},
		{"dynamic packages", `packages = projectPackages; shellHook = projectHook;`, managedSetupUnknown, managedSetupUnknown},
		{"inheritance", `inherit packages shellHook;`, managedSetupUnknown, managedSetupUnknown},
		{"conditional loader", `packages = if enabled then [] ++ import ./nix/loader.nix { inherit pkgs; } else [];`, managedSetupUnknown, managedSetupMissing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := `{ outputs = { self }: { devShells.x86_64-linux = { default = pkgs.mkShell { ` + tc.body + "\n }; }; }; }"
			got := inspectManagedNix([]byte(source))
			if got.packages.state != tc.packages || got.hooks.state != tc.hooks {
				t.Fatalf("inspection = %#v", got)
			}
		})
	}
	for _, source := range []string{
		`import ./shells.nix`,
		`{ outputs = { self }: { devShells = forAllSystems (system: if enabled then { default = pkgs.mkShell { packages = []; }; } else {}); }; }`,
		`{ outputs = { self }: { devShells.x86_64-linux = projectShells // { default = pkgs.mkShell { packages = []; }; }; }; }`,
		`{ notes = "default = pkgs.mkShell { packages = []; };"; }`,
		`{ devShells.x86_64-linux = { default = pkgs.mkShell { packages = []; }; }`,
	} {
		if got := inspectManagedNix([]byte(source)); got.packages.state != managedSetupUnknown || got.hooks.state != managedSetupUnknown {
			t.Fatalf("unsupported composition accepted: %s: %#v", source, got)
		}
	}
}

func TestManagedRootGuidanceForMissingPackagesAndCustomHooks(t *testing.T) {
	for _, tc := range []struct {
		name, body     string
		want, excluded []string
	}{
		{
			name:     "no packages attribute",
			body:     `shellHook = import ./nix/hooks.nix { inherit pkgs; };`,
			want:     []string{"Add `packages = import ./nix/loader.nix { inherit pkgs; };`", "same overridden `pkgs` for both loaders"},
			excluded: []string{"Append", "shellHook ="},
		},
		{
			name:     "quoted custom hook",
			body:     `packages = import ./nix/loader.nix { inherit pkgs; }; shellHook = "echo custom";`,
			want:     []string{"Prepend `(import ./nix/hooks.nix { inherit pkgs; }) + \"\\n\" +` to the existing `shellHook` expression", "same overridden `pkgs` for both loaders", "keep custom hooks last"},
			excluded: []string{"shellHook =", "Add `", "echo custom"},
		},
		{
			name:     "indented custom hook",
			body:     `packages = import ./nix/loader.nix { inherit pkgs; }; shellHook = ''echo custom'';`,
			want:     []string{"Prepend `(import ./nix/hooks.nix { inherit pkgs; }) + \"\\n\" +` to the existing `shellHook` expression", "same overridden `pkgs` for both loaders", "keep custom hooks last"},
			excluded: []string{"shellHook =", "Add `", "echo custom"},
		},
		{
			name:     "quoted interpolated hook",
			body:     `packages = import ./nix/loader.nix { inherit pkgs; }; shellHook = "${import ./nix/hooks.nix { inherit pkgs; }}";`,
			want:     []string{"Verify `flake.nix`", "shellHook composition"},
			excluded: []string{"Prepend", "Append", "Add `"},
		},
		{
			name:     "indented interpolated hook",
			body:     `packages = import ./nix/loader.nix { inherit pkgs; }; shellHook = ''${import ./nix/hooks.nix { inherit pkgs; }}'';`,
			want:     []string{"Verify `flake.nix`", "shellHook composition"},
			excluded: []string{"Prepend", "Append", "Add `"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			source := []byte(`{ outputs = { self }: { devShells.x86_64-linux = { default = pkgs.mkShell { ` + tc.body + ` }; }; }; }`)
			writeManagedTestFile(t, dir, "flake.nix", source)
			guidance := managedRootGuidance(&config.Config{}, dir, "flake.nix")
			if len(guidance) != 1 {
				t.Fatalf("guidance = %v, want one action", guidance)
			}
			for _, text := range tc.want {
				if !strings.Contains(guidance[0], text) {
					t.Errorf("guidance lacks %q: %s", text, guidance[0])
				}
			}
			for _, text := range tc.excluded {
				if strings.Contains(guidance[0], text) {
					t.Errorf("guidance includes %q: %s", text, guidance[0])
				}
			}
			assertManagedBytes(t, filepath.Join(dir, "flake.nix"), source)
		})
	}
}

func TestManagedNixActiveOutputsScope(t *testing.T) {
	unused := `{ devShells.x86_64-linux = { default = pkgs.mkShell { packages = []; }; }; packages.x86_64-linux = {}; }`
	for _, tc := range []struct {
		name, source string
	}{
		{"top-level let", `let pkgs = {}; unused = ` + unused + `; in import ./actual-flake.nix`},
		{"root without outputs", unused},
		{"nested output declaration", `{ unused = { outputs = { self }: ` + unused + `; }; }`},
		{"imported outputs", `{ unused = ` + unused + `; outputs = { self }: import ./outputs.nix; }`},
		{"outputs let with import", `{ outputs = { self }: let unused = ` + unused + `; in import ./outputs.nix; }`},
		{"nested unused shell", `{ outputs = { self }: { unused = ` + unused + `; }; }`},
		{"merged outputs", `{ outputs = { self }: ` + unused + ` // import ./outputs.nix; }`},
		{"conditional outputs", `{ outputs = { self }: if enabled then ` + unused + ` else {}; }`},
		{"merged flake", `{ outputs = { self }: ` + unused + `; } // import ./actual-flake.nix`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := inspectManagedNix([]byte(tc.source))
			for _, result := range []managedSetupInspection{got.packages, got.hooks, got.android, got.blender} {
				if result.state != managedSetupUnknown {
					t.Fatalf("unsupported scope accepted: %#v", got)
				}
			}
			dir := t.TempDir()
			writeManagedTestFile(t, dir, "flake.nix", []byte(tc.source))
			cfg := ebitengineConfig()
			cfg.MCP = &config.MCPSettings{Blender: new(true)}
			guidance := managedRootGuidance(cfg, dir, "flake.nix")
			if len(guidance) != 1 || !strings.HasPrefix(guidance[0], "Verify ") {
				t.Fatalf("unsafe guidance: %v", guidance)
			}
			assertManagedBytes(t, filepath.Join(dir, "flake.nix"), []byte(tc.source))
		})
	}
	source := `{ outputs = { self }: let unused = ` + unused + `; in {
		unused = ` + unused + `;
		devShells.x86_64-linux = { default = pkgs.mkShell {
			packages = import ./nix/loader.nix { inherit pkgs; };
			shellHook = import ./nix/hooks.nix { inherit pkgs; };
		}; };
	}; }`
	got := inspectManagedNix([]byte(source))
	if got.packages.state != managedSetupPresent || got.hooks.state != managedSetupPresent {
		t.Fatalf("active scope not selected: %#v", got)
	}
}

func TestManagedNixOptionalOutputs(t *testing.T) {
	content, err := swatch.Content("flake.nix")
	if err != nil {
		t.Fatal(err)
	}
	androidGuard := `system == "x86_64-linux" && builtins.pathExists ./nix/shells/go-ebitengine-android.nix`
	blenderGuard := `builtins.pathExists ./nix/lib/blender.nix`
	for _, tc := range []struct {
		name, androidGuard, blenderGuard string
		android, blender                 managedSetupState
	}{
		{"true", "true", "true", managedSetupPresent, managedSetupPresent},
		{"false", "false", "false", managedSetupMissing, managedSetupMissing},
		{"unknown", "enabled", "enabled", managedSetupUnknown, managedSetupUnknown},
		{"generated guards", androidGuard, blenderGuard, managedSetupPresent, managedSetupPresent},
		{"wrong system", strings.ReplaceAll(androidGuard, "x86_64-linux", "aarch64-linux"), blenderGuard, managedSetupUnknown, managedSetupPresent},
		{"wrong Android path", strings.ReplaceAll(androidGuard, "go-ebitengine-android.nix", "other.nix"), blenderGuard, managedSetupUnknown, managedSetupPresent},
		{"wrong Blender path", androidGuard, "builtins.pathExists ./other.nix", managedSetupPresent, managedSetupUnknown},
		{"swapped guards", blenderGuard, androidGuard, managedSetupUnknown, managedSetupUnknown},
		{"disabled composition", androidGuard + " && false", blenderGuard + " && false", managedSetupUnknown, managedSetupUnknown},
		{"empty predicate", "", "", managedSetupUnknown, managedSetupUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := strings.NewReplacer(androidGuard, tc.androidGuard, blenderGuard, tc.blenderGuard).Replace(string(content))
			got := inspectManagedNix([]byte(source))
			if got.android.state != tc.android || got.blender.state != tc.blender {
				t.Fatalf("inspection = %#v, want Android %v, Blender %v", got, tc.android, tc.blender)
			}
			if tc.android != managedSetupUnknown && (got.packages.state != managedSetupPresent || got.hooks.state != managedSetupPresent) {
				t.Fatalf("unconditional shell setup lost: %#v", got)
			}
			dir := t.TempDir()
			writeManagedTestFile(t, dir, "flake.nix", []byte(source))
			cfg := ebitengineConfig()
			cfg.MCP = &config.MCPSettings{Blender: new(true)}
			guidance := strings.Join(managedRootGuidance(cfg, dir, "flake.nix"), "\n")
			for _, output := range []struct {
				name  string
				state managedSetupState
			}{
				{"android", tc.android},
				{"blender-agent-bridge-extension", tc.blender},
			} {
				switch output.state {
				case managedSetupPresent:
					if strings.Contains(guidance, output.name) {
						t.Errorf("satisfied output has guidance: %s", guidance)
					}
				case managedSetupMissing:
					if !strings.Contains(guidance, "Add `"+output.name+" =") {
						t.Errorf("disabled output lacks adoption guidance: %s", guidance)
					}
				case managedSetupUnknown:
					if !strings.Contains(guidance, "Verify `flake.nix`") || !strings.Contains(guidance, output.name+" output:") || strings.Contains(guidance, "Add `"+output.name+" =") {
						t.Errorf("unknown output lacks safe verification guidance: %s", guidance)
					}
				}
			}
			assertManagedBytes(t, filepath.Join(dir, "flake.nix"), []byte(source))
		})
	}
}

func TestManagedNixOptionalAttrsComposition(t *testing.T) {
	for _, tc := range []struct {
		name, expression string
		state            managedSetupState
	}{
		{"inactive duplicate", `{ android = ` + managedNixAndroidOutput + `; } // pkgs.lib.optionalAttrs (false) { android = other; }`, managedSetupPresent},
		{"active duplicate", `{ android = ` + managedNixAndroidOutput + `; } // pkgs.lib.optionalAttrs (true) { android = other; }`, managedSetupUnknown},
		{"unknown override", `{ android = ` + managedNixAndroidOutput + `; } // pkgs.lib.optionalAttrs (enabled) { android = other; }`, managedSetupUnknown},
		{"unrelated guarded output", `pkgs.lib.optionalAttrs (builtins.pathExists ./nix/lib/blender.nix) { android = ` + managedNixAndroidOutput + `; }`, managedSetupUnknown},
		{"extra guarded output", `pkgs.lib.optionalAttrs (builtins.pathExists ./nix/lib/blender.nix) { blender-agent-bridge-extension = ` + managedNixBlenderOutput + `; android = ` + managedNixAndroidOutput + `; }`, managedSetupUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := `{ outputs = { self }: { devShells.x86_64-linux = ` + tc.expression + `; }; }`
			if got := inspectManagedNix([]byte(source)); got.android.state != tc.state {
				t.Fatalf("inspection = %#v, want Android %v", got, tc.state)
			}
		})
	}
}

func TestManagedNixStarterSuppressesSatisfiedGuidance(t *testing.T) {
	dir := t.TempDir()
	content, err := swatch.Content("flake.nix")
	if err != nil {
		t.Fatal(err)
	}
	writeManagedTestFile(t, dir, "flake.nix", content)
	cfg := ebitengineConfig()
	cfg.MCP = &config.MCPSettings{Blender: new(true)}
	if guidance := managedRootGuidance(cfg, dir, "flake.nix"); len(guidance) != 0 {
		t.Fatalf("starter advice = %v", guidance)
	}
}

func TestManagedGuidanceIsSeparateAndDeduplicated(t *testing.T) {
	dir := managedMissingSetupFixture(t)
	results := []SwatchResult{{Path: "justfile", Category: Skipped, Reason: SkipManagedRootExists}, {Path: ".mcp.json", Category: Skipped, Reason: SkipManagedSharedExists}}
	cfg := &config.Config{MCP: &config.MCPSettings{Playwright: new(true)}}
	for _, mode := range []ApplyMode{DryRun, Apply} {
		report := buildReport("baste", "", nil, nil, nil, results, mode)
		original := report.Plain
		appendManagedReporting(&report, cfg, results, dir)
		first := report.Plain
		appendManagedReporting(&report, cfg, results, dir)
		if report.Plain != first || !strings.HasPrefix(first, original) {
			t.Fatal("report repeated guidance or changed result rows")
		}
		for _, guidance := range report.Document.Guidance {
			if strings.Contains(guidance.Text, "\n") && (!strings.HasPrefix(guidance.Text, "Add Tailor's") || !strings.Contains(guidance.Text, "\nLocation:") || !strings.Contains(guidance.Text, "\nCheck:")) {
				t.Fatalf("combined actions without a location and check: %#v", guidance)
			}
		}
	}
}
