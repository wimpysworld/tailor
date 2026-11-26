package alter

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedGoVendorHashCheck(t *testing.T) {
	const (
		declared = "sha256-Qn3XxM9KybGXE92RT26ldj+x8HIReXDGPBkAqv1NnNQ="
		other    = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	)
	literal := "{ pkgs, ... }:\npkgs.buildGoModule {\n  pname = \"demo\";\n  vendorHash = \"" + declared + "\";\n}\n"
	for _, tc := range []struct {
		name, mode, packageNix, computed string
		noNix, wantErr, vendors          bool
		wantOutput, wantPackage          []string
	}{
		{name: "match", mode: "check", packageNix: literal, computed: declared, vendors: true, wantOutput: []string{"vendorHash matches"}},
		{name: "mismatch", mode: "check", packageNix: literal, computed: other, vendors: true, wantErr: true, wantOutput: []string{"nix/package.nix", declared, other}},
		{name: "correct updates", mode: "correct", packageNix: literal, computed: other, vendors: true, wantOutput: []string{"Updated vendorHash"}, wantPackage: []string{`vendorHash = "` + other + `";`}},
		{name: "absent", mode: "check", computed: other, wantOutput: []string{"No nix/package.nix found"}},
		{name: "no hash", mode: "check", packageNix: "{ pkgs, ... }:\npkgs.buildGoModule { pname = \"demo\"; }\n", computed: other, wantOutput: []string{"has no vendorHash"}},
		{name: "null", mode: "check", packageNix: "{\n  vendorHash = null;\n}\n", computed: other, wantOutput: []string{"vendorHash = null"}},
		{name: "proxy vendor", mode: "check", packageNix: "{\n  proxyVendor = true;\n  vendorHash = \"" + declared + "\";\n}\n", computed: other, wantOutput: []string{"proxyVendor = true"}},
		{name: "not literal", mode: "check", packageNix: "{\n  vendorHash = pkgs.lib.fakeHash;\n}\n", computed: other, wantOutput: []string{"no single literal vendorHash"}},
		{name: "nix unavailable", mode: "check", packageNix: literal, computed: other, noNix: true, wantOutput: []string{"nix is unavailable"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(testTempDir(t), "project with spaces")
			writeManagedLintFixture(t, root, renderSelectedManagedFiles(t, managedRenderConfig(true, nil)))
			if tc.packageNix != "" {
				writeManagedTestFile(t, root, "nix/package.nix", []byte(tc.packageNix))
			}
			bin, log, scratch := filepath.Join(root, "stubs"), filepath.Join(root, "calls"), filepath.Join(root, "scratch")
			if err := os.MkdirAll(scratch, 0o755); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"golangci-lint", "govulncheck", "gocyclo", "actionlint"} {
				writeManagedExecutable(t, filepath.Join(bin, name), "#!/bin/sh\nprintf '%s\\n' \""+name+" $*\" >> \"$CALLS\"\n")
			}
			writeManagedExecutable(t, filepath.Join(bin, "go"), "#!/bin/sh\nprintf '%s\\n' \"go $*\" >> \"$CALLS\"\nif [ \"$1 $2 $3\" = 'mod vendor -o' ]; then mkdir -p \"$4\" && : > \"$4/modules.txt\"; fi\n")
			if !tc.noNix {
				writeManagedExecutable(t, filepath.Join(bin, "nix"), "#!/bin/sh\nprintf '%s\\n' \"nix $*\" >> \"$CALLS\"\nprintf '%s\\n' \"$NIX_HASH\"\n")
			}
			for _, name := range []string{"bash", "mkdir", "mktemp", "rm"} {
				path, err := exec.LookPath(name)
				if err != nil {
					t.Skipf("%s is required: %v", name, err)
				}
				if err := os.Symlink(path, filepath.Join(bin, name)); err != nil {
					t.Fatal(err)
				}
			}
			// #nosec G204 -- The executable, recipe arguments, and temporary fixture are controlled by this test.
			command := exec.CommandContext(t.Context(), managedLintExecutable(t), "--justfile", filepath.Join(root, "justfile"), "lint", tc.mode)
			command.Dir = root
			command.Env = append(managedEnvironmentWithout("PATH", "BASH_ENV", "ENV", "CALLS", "TMPDIR", "NIX_HASH"), "PATH="+bin, "CALLS="+log, "TMPDIR="+scratch, "NIX_HASH="+tc.computed)
			output, err := command.CombinedOutput()
			if (err != nil) != tc.wantErr {
				t.Fatalf("lint error = %v, want error %v\n%s", err, tc.wantErr, output)
			}
			for _, want := range tc.wantOutput {
				if !strings.Contains(string(output), want) {
					t.Errorf("output lacks %q\n%s", want, output)
				}
			}
			calls, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			if vendored := strings.Contains(string(calls), "go mod vendor -o "+scratch+"/"); vendored != tc.vendors {
				t.Errorf("go mod vendor into TMPDIR = %v, want %v\n%s", vendored, tc.vendors, calls)
			}
			if entries, err := os.ReadDir(scratch); err != nil || len(entries) != 0 {
				t.Errorf("temporary vendor directory remains: %v %v", entries, err)
			}
			if tc.packageNix == "" {
				return
			}
			got, err := os.ReadFile(filepath.Join(root, "nix", "package.nix"))
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantPackage == nil && string(got) != tc.packageNix {
				t.Errorf("package.nix changed:\n%s", got)
			}
			for _, want := range tc.wantPackage {
				if !strings.Contains(string(got), want) || strings.Contains(string(got), declared) {
					t.Errorf("package.nix lacks %q or keeps the old hash:\n%s", want, got)
				}
			}
		})
	}
}
