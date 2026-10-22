package alter

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func ffmpegConfig() *config.Config {
	return &config.Config{Languages: &config.LanguageSettings{Go: new(true), GoOptions: &config.GoOptions{FFmpegStatigo: new(true)}}}
}

func TestManagedFFmpegLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name              string
		cfg               *config.Config
		selected, enabled bool
	}{
		{"enabled", ffmpegConfig(), true, true},
		{"disabled", &config.Config{Languages: &config.LanguageSettings{Go: new(true), GoOptions: &config.GoOptions{FFmpegStatigo: new(false)}}}, true, false},
		{"go disabled", &config.Config{Languages: &config.LanguageSettings{Go: new(false)}}, true, false},
		{"option absent", &config.Config{Languages: &config.LanguageSettings{Go: new(true)}}, false, false},
		{"go absent", &config.Config{}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selections, err := selectManagedFiles(tc.cfg)
			if err != nil {
				t.Fatal(err)
			}
			rendered := renderSelectedManagedFiles(t, tc.cfg)
			for _, path := range []string{"just/go-ffmpeg-statigo.just", "nix/go-ffmpeg-statigo.nix", "nix/hooks/go-ffmpeg-statigo.nix"} {
				found := false
				for _, selection := range selections {
					if selection.Entry.Path == path {
						found = true
						if selection.Enabled != tc.enabled {
							t.Fatal(selection)
						}
					}
				}
				if found != tc.selected {
					t.Fatalf("selection %s = %v", path, found)
				}
				if !found {
					continue
				}
				for _, present := range []bool{false, true} {
					snapshots := []managedDestinationSnapshot{}
					for _, selection := range selections {
						snapshot := managedDestinationSnapshot{Path: selection.Entry.Path}
						if present && selection.Entry.Path == path {
							snapshot.Kind = managedDestinationRegular
							snapshot.Content = managedContent(path, "old")
						}
						snapshots = append(snapshots, snapshot)
					}
					plan, err := planManagedFiles(selections, rendered, snapshots)
					if err != nil {
						t.Fatal(err)
					}
					for _, file := range plan.Files {
						if file.Selection.Entry.Path == path {
							want := managedOperationNoop
							if tc.enabled {
								want = managedOperationWrite
							} else if present {
								want = managedOperationRemove
							}
							if file.Operation != want {
								t.Fatalf("%s operation %v != %v", path, file.Operation, want)
							}
						}
					}
				}
			}
			if got := bytes.Contains(rendered["just/tailor.just"], []byte("git config pull.ff only")); got != tc.enabled {
				t.Fatal("incorrect setup aggregate")
			}
			for name, content := range rendered {
				if bytes.Contains(content, []byte("setup-go-ffmpeg-statigo")) {
					t.Fatalf("legacy setup recipe in %s", name)
				}
			}
		})
	}
}

func TestManagedCGOCommands(t *testing.T) {
	for _, policy := range []*bool{nil, new(false), new(true)} {
		t.Run(fmt.Sprint(policy), func(t *testing.T) {
			cfg := &config.Config{Languages: &config.LanguageSettings{Go: new(true), GoOptions: &config.GoOptions{CGO: policy}}}
			rendered := renderSelectedManagedFiles(t, cfg)
			if policy == nil {
				original, err := swatch.Content("just/go.just")
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(original, rendered["just/go.just"]) {
					t.Fatal("absent CGO changed base Go")
				}
			}
			root := t.TempDir()
			writeManagedJustFixture(t, root, rendered)
			writeManagedTestFile(t, root, "justfile", []byte("import 'just/loader.just'\nother:\n    @go unrelated\n"))
			bin := filepath.Join(root, "stubs")
			log := filepath.Join(root, "calls")
			for _, tool := range []string{"go", "golangci-lint", "govulncheck", "gocyclo"} {
				writeManagedTestFile(t, root, "stubs/"+tool, []byte("#!/bin/sh\nprintf '%s|%s|%s\\n' \"$(basename \"$0\")\" \"$CGO_ENABLED\" \"$*\" >> \"$CALLS\"\n"))
				if err := os.Chmod(filepath.Join(bin, tool), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			for _, recipe := range []string{"lint", "build", "test", "other"} {
				// #nosec G204 -- The executable and recipe are fixed test tools in an isolated fixture.
				cmd := exec.CommandContext(t.Context(), managedLintExecutable(t), "--justfile", filepath.Join(root, "justfile"), recipe)
				cmd.Dir = root
				cmd.Env = append(managedEnvironmentWithout("BASH_ENV", "ENV", "PATH", "CGO_ENABLED", "CALLS"), "PATH="+bin+":"+os.Getenv("PATH"), "CGO_ENABLED=inherited", "CALLS="+log)
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%s: %v %s", recipe, err, output)
				}
			}
			data, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			want := "inherited"
			if policy != nil {
				want = "0"
				if *policy {
					want = "1"
				}
			}
			for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
				if !strings.Contains(line, "|"+want+"|") {
					t.Fatalf("wrong environment: %s", line)
				}
			}
		})
	}
}

func TestManagedFFmpegSetupStubbed(t *testing.T) {
	for _, scenario := range []string{"update", "current", "init", "no-tag", "fetch-failure", "download-failure", "source", "fallback", "checkout-failure", "config-failure", "untagged", "fallback-non-v", "fallback-no-tag", "api-failure", "fallback-api-failure", "jq-failure"} {
		t.Run(scenario, func(t *testing.T) {
			root := testTempDir(t)
			bin := filepath.Join(root, "stubs")
			log := filepath.Join(root, "calls")
			rendered := renderSelectedManagedFiles(t, ffmpegConfig())
			writeManagedJustFixture(t, root, rendered)
			writeManagedTestFile(t, root, "justfile", []byte("import 'just/loader.just'\n"))
			module := "module example.com/consumer\n"
			if scenario == "source" {
				module = "module github.com/linuxmatters/ffmpeg-statigo\n"
			}
			writeManagedTestFile(t, root, "go.mod", []byte(module))
			if scenario != "init" {
				writeManagedTestFile(t, root, "third_party/ffmpeg-statigo/go.mod", []byte("module github.com/linuxmatters/ffmpeg-statigo\n"))
			}
			writeManagedTestFile(t, root, "third_party/ffmpeg-statigo/lib/linux_amd64/libffmpeg.a", []byte("fixture"))
			scripts := map[string]string{
				"git": `case "$*" in
"submodule update --init --recursive") printf 'module github.com/linuxmatters/ffmpeg-statigo\n' > third_party/ffmpeg-statigo/go.mod ;;
"describe --tags --exact-match") if [ "$SCENARIO" = current ]; then echo v1.2.3; elif [ "$SCENARIO" = untagged ]; then exit 1; else echo v1.0.0; fi ;;
"config pull.ff only") [ "$SCENARIO" != config-failure ] || exit 6 ;;
"checkout v1.2.3") [ "$SCENARIO" != checkout-failure ] || exit 7 ;;
"fetch --no-tags origin refs/tags/v1.2.3:refs/tags/v1.2.3") [ "$SCENARIO" != fetch-failure ] || exit 4 ;;
"diff --cached --quiet third_party/ffmpeg-statigo") [ "$SCENARIO" = current ] ;;
esac`,
				"curl": `case "$SCENARIO" in
api-failure|fallback-api-failure) printf '{"message":"API error"}\n'; exit 22 ;;
esac
printf '[\n'
case "$SCENARIO" in
fallback-non-v|fallback-no-tag)
    printf '{\n"tag_name": "release-1.2.3",\n"prerelease": false\n}'
    if [ "$SCENARIO" = fallback-no-tag ]; then printf '\n]\n'; exit 0; fi
    printf ',\n' ;;
esac
printf '{\n"tag_name": "v1.2.3",\n"prerelease": false\n}\n]\n'`,
				"jq": `cat >/dev/null
if [ "$SCENARIO" = jq-failure ]; then echo v1.2.3; echo 'jq: invalid response' >&2; exit 5; fi
if [ "$SCENARIO" = no-tag ]; then echo null; else echo v1.2.3; fi`,
				"go": `[ "$SCENARIO" != download-failure ] || exit 5`,
				"rm": `:`,
			}
			for tool, body := range scripts {
				if tool == "jq" && strings.HasPrefix(scenario, "fallback") {
					continue
				}
				writeManagedTestFile(t, root, "stubs/"+tool, []byte("#!/bin/sh\nprintf '%s|%s|%s|%s\\n' \"$(basename \"$0\")\" \"$PWD\" \"${CGO_ENABLED:-}\" \"$*\" >> \"$CALLS\"\n"+body+"\n"))
				if err := os.Chmod(filepath.Join(bin, tool), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			// An isolated PATH ensures that setup cannot use real mutating tools.
			for _, tool := range []string{"bash", "sh", "env", "grep", "head", "cut", "cat", "basename", "just"} {
				source, err := exec.LookPath(tool)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(source, filepath.Join(bin, tool)); err != nil {
					t.Fatal(err)
				}
			}
			// #nosec G204 -- The executable is a test tool and setup uses only isolated stubs.
			cmd := exec.CommandContext(t.Context(), managedLintExecutable(t), "--justfile", filepath.Join(root, "justfile"), "setup")
			cmd.Dir = root
			cmd.Env = []string{"PATH=" + bin, "CALLS=" + log, "SCENARIO=" + scenario, "CGO_ENABLED=0", "HOME=" + root}
			output, err := cmd.CombinedOutput()
			tagFailure := scenario == "no-tag" || scenario == "fallback-no-tag" || scenario == "api-failure" || scenario == "fallback-api-failure" || scenario == "jq-failure"
			failure := tagFailure || scenario == "fetch-failure" || scenario == "download-failure" || scenario == "source" || scenario == "config-failure" || scenario == "checkout-failure"
			if (err != nil) != failure {
				t.Fatalf("error=%v output=%s", err, output)
			}
			data, _ := os.ReadFile(log)
			calls := string(data)
			if scenario == "source" {
				if calls != "" || !strings.Contains(string(output), "consumer repositories only") {
					t.Fatalf("source guard: %s %s", calls, output)
				}
				return
			}
			if scenario == "config-failure" {
				if strings.Contains(calls, "curl|") || strings.Contains(calls, "submodule.recurse") {
					t.Fatal(calls)
				}
				return
			}
			for _, args := range []string{"config pull.ff only", "config submodule.recurse true", "-fsS https://api.github.com/repos/linuxmatters/ffmpeg-statigo/releases"} {
				if !strings.Contains(calls, "|"+args+"\n") {
					t.Fatalf("missing %s: %s", args, calls)
				}
			}
			if tagFailure {
				if !strings.Contains(string(output), "Error: Could not fetch latest release tag") {
					t.Fatal(string(output))
				}
				for _, forbidden := range []string{"|fetch ", "|submodule ", "|checkout ", "|add ", "rm|", "go|"} {
					if strings.Contains(calls, forbidden) {
						t.Fatal(calls)
					}
				}
				return
			}
			if !strings.Contains(calls, "git|"+root+"/third_party/ffmpeg-statigo|1|fetch --no-tags origin refs/tags/v1.2.3:refs/tags/v1.2.3\n") {
				t.Fatal(calls)
			}
			if scenario == "fetch-failure" {
				if strings.Contains(calls, "checkout") {
					t.Fatal(calls)
				}
				return
			}
			if scenario == "init" && !strings.Contains(calls, "|submodule update --init --recursive\n") {
				t.Fatal(calls)
			}
			if scenario == "checkout-failure" {
				if strings.Contains(calls, "rm|") || strings.Contains(calls, "|add ") || strings.Contains(calls, "\ngo|") {
					t.Fatal(calls)
				}
				return
			}
			if scenario == "current" {
				if strings.Contains(calls, "|checkout") || strings.Contains(calls, "rm|") || strings.Contains(calls, "|add ") {
					t.Fatal(calls)
				}
			} else {
				for _, args := range []string{"checkout v1.2.3", "-f third_party/ffmpeg-statigo/lib/linux_amd64/libffmpeg.a", "add third_party/ffmpeg-statigo"} {
					if !strings.Contains(calls, "|"+args+"\n") {
						t.Fatalf("missing %s: %s", args, calls)
					}
				}
			}
			if !strings.Contains(calls, "go|"+root+"/third_party/ffmpeg-statigo|1|run ./cmd/download-lib\n") {
				t.Fatal(calls)
			}
			if scenario == "download-failure" {
				if strings.Contains(calls, "|diff ") {
					t.Fatal(calls)
				}
				return
			}
			if !strings.Contains(calls, "|diff --cached --quiet third_party/ffmpeg-statigo\n") {
				t.Fatal(calls)
			}
			if scenario != "current" && !strings.Contains(string(output), "Don't forget to commit:") {
				t.Fatal(string(output))
			}
		})
	}
}
