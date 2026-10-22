package alter

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedBuildWritesDiscoveredExecutables(t *testing.T) {
	just := managedLintExecutable(t)
	tests := []struct {
		name      string
		module    string
		files     map[string]string
		env       []string
		want      []string
		wantError string
	}{
		{
			name:   "root semantic module",
			module: "example.com/tool/v2",
			files:  map[string]string{"main.go": "package main\nfunc main() {}\n"},
			want:   []string{"tool"},
		},
		{
			name:   "command and library",
			module: "example.com/project",
			files: map[string]string{
				"library.go":      "package project\nconst Value = 1\n",
				"cmd/app/main.go": "package main\nfunc main() {}\n",
			},
			want: []string{"app"},
		},
		{
			name:   "multiple commands",
			module: "example.com/project",
			files: map[string]string{
				"cmd/alpha/main.go": "package main\nfunc main() {}\n",
				"cmd/beta/main.go":  "package main\nfunc main() {}\n",
			},
			want: []string{"alpha", "beta"},
		},
		{
			name:   "library only",
			module: "example.com/library",
			files:  map[string]string{"library.go": "package library\nconst Value = 1\n"},
		},
		{
			name:   "output collision",
			module: "example.com/project",
			files: map[string]string{
				"cmd/tool/main.go":   "package main\nfunc main() {}\n",
				"tools/tool/main.go": "package main\nfunc main() {}\n",
			},
			wantError: "both produce bin/tool",
		},
		{
			name:   "Windows suffix",
			module: "example.com/project",
			files:  map[string]string{"cmd/app/main.go": "package main\nfunc main() {}\n"},
			env:    []string{"GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0"},
			want:   []string{"app.exe"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeManagedBuildFixture(t, root, tt.module, tt.files)
			output, err := runManagedBuild(t, just, root, tt.env...)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(string(output), tt.wantError) {
					t.Fatalf("just build output=%q error=%v, want %q", output, err, tt.wantError)
				}
				if _, statErr := os.Stat(filepath.Join(root, "bin")); !os.IsNotExist(statErr) {
					t.Fatalf("collision wrote bin before failure: %v", statErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("just build: %v\n%s", err, output)
			}
			for _, name := range tt.want {
				if info, statErr := os.Stat(filepath.Join(root, "bin", name)); statErr != nil || !info.Mode().IsRegular() {
					t.Errorf("output %q info=%v error=%v", name, info, statErr)
				}
			}
			if len(tt.want) == 0 {
				if !strings.Contains(string(output), "No executable packages were found") {
					t.Fatalf("library build output=%q, want success guidance", output)
				}
				if _, statErr := os.Stat(filepath.Join(root, "bin")); !os.IsNotExist(statErr) {
					t.Fatalf("library build created bin: %v", statErr)
				}
			}
		})
	}
}

func TestManagedBuildRefreshesExistingBinaryWithoutWritingRoot(t *testing.T) {
	just := managedLintExecutable(t)
	root := t.TempDir()
	writeManagedBuildFixture(t, root, "example.com/project", map[string]string{
		"cmd/app/main.go": "package main\nfunc main() { println(\"first\") }\n",
	})
	writeManagedTestFile(t, root, "app", []byte("root binary\n"))

	if output, err := runManagedBuild(t, just, root); err != nil {
		t.Fatalf("first just build: %v\n%s", err, output)
	}
	first, err := os.ReadFile(filepath.Join(root, "bin", "app"))
	if err != nil {
		t.Fatal(err)
	}
	writeManagedTestFile(t, root, "cmd/app/main.go", []byte("package main\nfunc main() { println(\"second\") }\n"))
	if output, err := runManagedBuild(t, just, root); err != nil {
		t.Fatalf("second just build: %v\n%s", err, output)
	}
	second, err := os.ReadFile(filepath.Join(root, "bin", "app"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("second build did not refresh bin/app")
	}
	rootBinary, err := os.ReadFile(filepath.Join(root, "app"))
	if err != nil || string(rootBinary) != "root binary\n" {
		t.Fatalf("root binary content=%q error=%v", rootBinary, err)
	}
}

func TestManagedBuildRejectsUnsafeDestinations(t *testing.T) {
	tests := []struct {
		name      string
		wantError string
		setup     func(*testing.T, string)
	}{
		{
			name:      "bin symlink",
			wantError: "refusing to use symlink bin",
			setup: func(t *testing.T, root string) {
				outside := filepath.Join(root, "outside")
				if err := os.Mkdir(outside, 0o755); err != nil {
					t.Fatal(err)
				}
				managedSymlinkOrSkip(t, outside, filepath.Join(root, "bin"))
			},
		},
		{
			name:      "output symlink",
			wantError: "refusing to replace symlink bin/app",
			setup: func(t *testing.T, root string) {
				writeManagedTestFile(t, root, "user-target", []byte("user binary\n"))
				if err := os.Mkdir(filepath.Join(root, "bin"), 0o755); err != nil {
					t.Fatal(err)
				}
				managedSymlinkOrSkip(t, filepath.Join(root, "user-target"), filepath.Join(root, "bin", "app"))
			},
		},
		{
			name:      "output directory",
			wantError: "refusing to replace non-regular bin/app",
			setup: func(t *testing.T, root string) {
				if err := os.MkdirAll(filepath.Join(root, "bin", "app"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	just := managedLintExecutable(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeManagedBuildFixture(t, root, "example.com/project", map[string]string{
				"cmd/app/main.go": "package main\nfunc main() {}\n",
			})
			tt.setup(t, root)
			output, err := runManagedBuild(t, just, root)
			if err == nil || !strings.Contains(string(output), tt.wantError) {
				t.Fatalf("just build output=%q error=%v, want %q", output, err, tt.wantError)
			}
			if content, readErr := os.ReadFile(filepath.Join(root, "user-target")); readErr == nil && string(content) != "user binary\n" {
				t.Fatalf("symlink target changed to %q", content)
			}
		})
	}
}

func TestManagedBuildUsesBashThreeCompatibleArrays(t *testing.T) {
	content := string(renderSelectedManagedFiles(t, managedRenderConfig(true, nil))["just/tailor.just"])
	if strings.Contains(content, "declare -A") {
		t.Fatal("managed Go build uses Bash 4 associative arrays")
	}
	guard := strings.Index(content, `if [[ -z "${main_packages[*]-}" ]]; then`)
	loop := strings.Index(content, `for package in "${main_packages[@]}"`)
	if guard < 0 || loop < 0 || guard > loop {
		t.Fatal("managed Go build must handle empty arrays before expansion")
	}
	if strings.Contains(content, `${#main_packages[@]}`) {
		t.Fatal("managed Go build uses an empty array length under nounset")
	}
	if strings.Contains(content, `${!outputs[@]}`) || strings.Contains(content, `${#outputs[@]}`) {
		t.Fatal("managed Go build expands empty output arrays under nounset")
	}
	count := strings.Index(content, `output_count=0`)
	collisionLoop := strings.Index(content, `for ((index = 0; index < output_count; index++)); do`)
	appendOutput := strings.Index(content, `outputs+=("${output}")`)
	increment := strings.Index(content, `output_count=$((output_count + 1))`)
	if count < 0 || collisionLoop < count || appendOutput < collisionLoop || increment < appendOutput {
		t.Fatal("managed Go build must count stored outputs before collision checks")
	}
}

func TestManagedBuildWithBashThree(t *testing.T) {
	var bash string
	for _, candidate := range []string{"bash3", "bash32", "bash-3.2", "/bin/bash", "bash"} {
		path, err := exec.LookPath(candidate)
		if err != nil {
			continue
		}
		output, err := exec.CommandContext(t.Context(), path, "-c", `printf '%s' "${BASH_VERSINFO[0]}"`).Output()
		if err == nil && string(output) == "3" {
			bash = path
			break
		}
	}
	if bash == "" {
		t.Skip("Bash 3 executable is not available")
	}
	just := managedLintExecutable(t)
	for _, tc := range []struct {
		name      string
		files     map[string]string
		wantError string
	}{
		{name: "first executable", files: map[string]string{"cmd/app/main.go": "package main\nfunc main() {}\n"}},
		{name: "library only", files: map[string]string{"library.go": "package library\n"}},
		{name: "collision", files: map[string]string{"cmd/app/main.go": "package main\nfunc main() {}\n", "tools/app/main.go": "package main\nfunc main() {}\n"}, wantError: "both produce bin/app"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeManagedBuildFixture(t, root, "example.com/project", tc.files)
			bin := t.TempDir()
			managedSymlinkOrSkip(t, bash, filepath.Join(bin, "bash"))
			output, err := runManagedBuild(t, just, root, "PATH="+bin+":"+os.Getenv("PATH"))
			if tc.wantError != "" {
				if err == nil || !strings.Contains(string(output), tc.wantError) {
					t.Fatalf("Bash 3 collision output=%q error=%v", output, err)
				}
			} else if err != nil {
				t.Fatalf("Bash 3 build: %v\n%s", err, output)
			}
			if tc.name == "first executable" {
				if _, err := os.Stat(filepath.Join(root, "bin", "app")); err != nil {
					t.Fatal(err)
				}
			} else if _, err := os.Stat(filepath.Join(root, "bin")); !os.IsNotExist(err) {
				t.Fatalf("build created bin: %v", err)
			}
		})
	}
}

func TestManagedBuildWithoutBuildersSucceedsWithGuidance(t *testing.T) {
	root := t.TempDir()
	rendered := renderSelectedManagedFiles(t, managedRenderConfig(false, nil))
	writeManagedJustFixture(t, root, rendered)
	writeManagedTestFile(t, root, "justfile", []byte("import 'just/loader.just'\n"))
	output, err := runManagedBuild(t, managedLintExecutable(t), root)
	if err != nil || !strings.Contains(string(output), "No build recipes are enabled.") {
		t.Fatalf("just build output=%q error=%v", output, err)
	}
}

func runManagedBuild(t *testing.T, just, root string, env ...string) ([]byte, error) {
	t.Helper()
	// #nosec G204 -- The Just executable and isolated fixture are controlled by the test.
	command := exec.CommandContext(t.Context(), just, "--justfile", filepath.Join(root, "justfile"), "build")
	command.Dir = root
	command.Env = append(os.Environ(), env...)
	return command.CombinedOutput()
}

func writeManagedBuildFixture(t *testing.T, root, module string, files map[string]string) {
	t.Helper()
	rendered := renderSelectedManagedFiles(t, managedRenderConfig(true, nil))
	writeManagedJustFixture(t, root, rendered)
	writeManagedTestFile(t, root, "justfile", []byte("import 'just/loader.just'\n"))
	writeManagedTestFile(t, root, "go.mod", []byte("module "+module+"\n\ngo 1.26\n"))
	for name, content := range files {
		writeManagedTestFile(t, root, name, []byte(content))
	}
}
