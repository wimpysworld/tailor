package swatch_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestBuilderSavedVulnerabilityResults(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/builder.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name, If, Uses, Run string
				ContinueOnError     bool `yaml:"continue-on-error"`
			}
		}
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	var check, report string
	scans := 0
	for _, step := range workflow.Jobs["security"].Steps {
		if strings.HasPrefix(step.Uses, "golang/govulncheck-action@") {
			scans++
		}
		switch step.Name {
		case "Check vulnerabilities":
			check = step.Run
			if step.ContinueOnError || step.If != "${{ !cancelled() && steps.govulncheck.outcome == 'success' }}" {
				t.Fatal("vulnerability gate must propagate errors after a successful scan")
			}
		case "Generate SARIF", "Upload SARIF":
			if !strings.Contains(step.If, "!cancelled()") || strings.Contains(step.If, "success()") {
				t.Fatal("reporting must remain eligible after a vulnerability failure")
			}
			if step.Name == "Generate SARIF" {
				report = step.Run
			}
		}
	}
	const checkCommand = "govulncheck -mode convert -format text < govulncheck.json"
	const reportCommand = "govulncheck -mode convert -format sarif < govulncheck.json > govulncheck.sarif"
	if scans != 1 || check != checkCommand || report != reportCommand {
		t.Fatal("expected one scan and conversion of its saved JSON into text and SARIF")
	}
}

func TestBuilderSavedVulnerabilityConversion(t *testing.T) {
	tool, err := exec.LookPath("govulncheck")
	if err != nil {
		t.Skip("govulncheck is not installed; workflow structure is checked independently")
	}
	t.Logf("converter: %s", tool)
	t.Setenv("GOVULNDB", "file://"+t.TempDir())
	t.Setenv("GOTELEMETRY", "off")
	const config = `{"config":{"protocol_version":"v1.0.0","scanner_name":"govulncheck","scanner_version":"fixture","scan_level":"symbol","scan_mode":"source"}}
`
	const osv = `{"osv":{"id":"GO-TEST-0001","summary":"Offline fixture","details":"Offline fixture","database_specific":{"url":"https://example.invalid/GO-TEST-0001"},"affected":[{"package":{"name":"example.com/fixture","ecosystem":"Go"},"ranges":[{"type":"SEMVER","events":[{"introduced":"0"},{"fixed":"1.1.0"}]}]}]}}
`
	const reachable = `{"finding":{"osv":"GO-TEST-0001","fixed_version":"v1.1.0","trace":[{"module":"example.com/fixture","version":"v1.0.0","package":"example.com/fixture","function":"Vulnerable"},{"module":"example.com/app","version":"v1.0.0","package":"example.com/app","function":"main"}]}}
`
	const imported = `{"finding":{"osv":"GO-TEST-0001","fixed_version":"v1.1.0","trace":[{"module":"example.com/fixture","version":"v1.0.0","package":"example.com/fixture"}]}}
`
	for _, tc := range []struct {
		name, input string
		exit        int
	}{
		{"clean", config, 0},
		{"imported but not called", config + osv + imported, 0},
		{"reachable", config + osv + reachable, 3},
		{"malformed", config + "{", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "govulncheck.json"), []byte(tc.input), 0o600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.CommandContext(t.Context(), "bash", "-e", "-o", "pipefail", "-c", "govulncheck -mode convert -format text < govulncheck.json")
			cmd.Dir = dir
			output, err := cmd.CombinedOutput()
			exit := 0
			if err != nil {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) {
					t.Fatal(err)
				}
				exit = exitErr.ExitCode()
			}
			if exit != tc.exit {
				t.Fatalf("text exit = %d, want %d\n%s", exit, tc.exit, output)
			}
			if tc.name == "reachable" && !strings.Contains(string(output), "GO-TEST-0001") {
				t.Fatalf("text report omitted the vulnerability: %s", output)
			}
			if tc.name == "malformed" {
				return
			}
			cmd = exec.CommandContext(t.Context(), "bash", "-e", "-o", "pipefail", "-c", "govulncheck -mode convert -format sarif < govulncheck.json > govulncheck.sarif")
			cmd.Dir = dir
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("SARIF conversion after text exit %d: %v\n%s", exit, err, output)
			}
			sarif, err := os.ReadFile(filepath.Join(dir, "govulncheck.sarif"))
			if err != nil || len(sarif) == 0 {
				t.Fatalf("missing SARIF report: %v", err)
			}
			if tc.name == "reachable" && !strings.Contains(string(sarif), "GO-TEST-0001") {
				t.Fatal("SARIF report omitted the vulnerability")
			}
		})
	}
}
