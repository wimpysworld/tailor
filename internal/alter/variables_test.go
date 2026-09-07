package alter_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/measure"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func serveAlterVariables(t *testing.T, sc *alterServerConfig, w http.ResponseWriter, r *http.Request, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodGet {
		if sc.variableReadError != 0 {
			w.WriteHeader(sc.variableReadError)
			fmt.Fprint(w, `{"message":"denied"}`)
			return
		}
		variables := []map[string]string{}
		for name, value := range sc.variables {
			variables = append(variables, map[string]string{"name": name, "value": value})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"total_count": len(variables), "variables": variables})
		return
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Errorf("variable payload: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	name := payload["name"]
	if r.Method == http.MethodPatch {
		name = filepath.Base(r.URL.Path)
	}
	if status := sc.variableErrors[name]; status != 0 {
		w.WriteHeader(status)
		fmt.Fprint(w, `{"message":"denied"}`)
		return
	}
	if r.Method != http.MethodPost && r.Method != http.MethodPatch {
		t.Errorf("unexpected variable method %s", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if sc.variables == nil {
		sc.variables = map[string]string{}
	}
	sc.variables[name] = payload["value"]
	if r.Method == http.MethodPost {
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{}`)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

const variableConfig = `license: none
variables:
  - name: NEW
    value: ""
  - name: changed
    value: " next\n\t\"\\é "
  - name: SAME
    value: "{{GITHUB_USERNAME}}"
swatches:
  - path: .gitignore
    alteration: always
`

func TestAlterVariablesPreviewAndRepeatedApply(t *testing.T) {
	tc := setupAlterTest(t, variableConfig, func(sc *alterServerConfig) {
		sc.variables = map[string]string{"CHANGED": "old\n", "same": "{{GITHUB_USERNAME}}", "UNDECLARED": "keep"}
	})
	cfg := loadTestConfig(t, tc.Dir)
	preview := captureAlterRun(t, cfg, tc.Dir, alter.DryRun, tc.Client)
	requireContains(t, preview, `variable.NEW = ""`)
	requireContains(t, preview, `variable.changed = "old\n" -> " next\n\t\"\\é "`)
	requireContains(t, preview, `variable.SAME (already "{{GITHUB_USERNAME}}")`)
	for _, call := range tc.Calls() {
		if call.Method != http.MethodGet {
			t.Errorf("preview wrote: %v", call)
		}
	}
	if data, err := os.ReadFile(filepath.Join(tc.Dir, ".tailor.yml")); err != nil || string(data) != variableConfig {
		t.Fatalf("preview changed config: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(tc.Dir, ".gitignore")); !os.IsNotExist(err) {
		t.Fatalf("preview created swatch: %v", err)
	}
	first := captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)
	requireContains(t, first, "created:")
	requireContains(t, first, "updated:")
	before := len(tc.Calls())
	second := captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)
	requireContains(t, second, `variable.changed (already " next\n\t\"\\é ")`)
	for _, call := range tc.Calls()[before:] {
		if call.Method != http.MethodGet {
			t.Errorf("repeat apply wrote: %v", call)
		}
	}
	var writes []apiCall
	for _, call := range tc.Calls() {
		if call.Method != http.MethodGet {
			writes = append(writes, call)
		}
	}
	if len(writes) != 2 || writes[0].Method != http.MethodPost || writes[1].Method != http.MethodPatch || !strings.HasSuffix(writes[1].Path, "/CHANGED") {
		t.Errorf("variable writes = %v", writes)
	}
}

func TestAlterVariablesSkipAndPartialFailure(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusUnprocessableEntity, http.StatusNotFound, http.StatusTooManyRequests} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			tc := setupAlterTest(t, `license: none
variables:
  - name: DONE
    value: "new"
  - name: SKIP
    value: "new"
  - name: FAIL
    value: "new"
  - name: LATER
    value: "new"
swatches:
  - path: .gitignore
    alteration: always
`, func(sc *alterServerConfig) {
				sc.variables = map[string]string{"FAIL": "old"}
				sc.variableErrors = map[string]int{"SKIP": http.StatusForbidden, "FAIL": status}
			})
			output, _, err := captureAlterRunWithStderr(t, loadTestConfig(t, tc.Dir), tc.Dir, alter.Apply, tc.Client)
			requireContains(t, output, `variable.DONE = "new"`)
			requireContains(t, output, "would skip (insufficient scope: token missing required scope): variable.SKIP")
			if strings.Contains(output, "variable.SKIP =") || strings.Contains(output, "variable.FAIL =") {
				t.Errorf("failed write reported as successful: %s", output)
			}
			if status == http.StatusForbidden {
				if err != nil {
					t.Fatal(err)
				}
				requireContains(t, output, `variable.LATER = "new"`)
				return
			}
			if err == nil || !strings.Contains(err.Error(), "1 applied, 2 remaining") {
				t.Fatalf("error = %v, want partial counts", err)
			}
			if strings.Contains(output, "variable.LATER") {
				t.Errorf("unattempted write reported: %s", output)
			}
			if _, err := os.Stat(filepath.Join(tc.Dir, ".gitignore")); !os.IsNotExist(err) {
				t.Errorf("swatches ran after failure: %v", err)
			}
		})
	}
}

func TestAlterVariablesReadFailures(t *testing.T) {
	for _, status := range []int{403, 404, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			tc := setupAlterTest(t, variableConfig, func(sc *alterServerConfig) { sc.variableReadError = status })
			output, _, err := captureAlterRunWithStderr(t, loadTestConfig(t, tc.Dir), tc.Dir, alter.Apply, tc.Client)
			if status == 403 || status == 404 {
				if err != nil {
					t.Fatal(err)
				}
				requireContains(t, output, "would skip (insufficient scope: token missing required scope): fetch variables")
			} else if err == nil {
				t.Fatal("expected read error")
			}
			for _, call := range tc.Calls() {
				if call.Method != http.MethodGet {
					t.Errorf("write after failed list: %v", call)
				}
			}
		})
	}
}

func TestAlterVariablesRecutAndOmission(t *testing.T) {
	for _, section := range []string{"", "variables: []\n", "variables:\n  - name: KEEP\n    value: \"\"\n"} {
		t.Run(fmt.Sprintf("section_%d", len(section)), func(t *testing.T) {
			tc := setupAlterTest(t, "license: none\n"+section+"swatches:\n  - path: .tailor.yml\n    alteration: first-fit\n")
			cfg := loadTestConfig(t, tc.Dir)
			captureAlterRun(t, cfg, tc.Dir, alter.Recut, tc.Client)
			written := loadTestConfig(t, tc.Dir)
			if len(written.Variables) != len(cfg.Variables) {
				t.Fatalf("recut changed declarations: %v", written.Variables)
			}
			if len(written.Variables) != 0 {
				if written.Variables[0].Name != "KEEP" || *written.Variables[0].Value != "" {
					t.Errorf("recut changed variable: %v", written.Variables)
				}
			} else {
				for _, call := range tc.Calls() {
					if strings.Contains(call.Path, "/variables") {
						t.Errorf("undeclared variable request: %v", call)
					}
				}
			}
		})
	}
}

func TestProcessVariablesMissingRepo(t *testing.T) {
	var stderr strings.Builder
	results, err := alter.ProcessVariables(&config.Config{Variables: []model.VariableEntry{{Name: "A", Value: new("")}}}, alter.Apply, alter.RepoTarget{Stderr: &stderr})
	if err != nil || len(results) != 0 {
		t.Fatalf("results %v, error %v", results, err)
	}
	requireContains(t, stderr.String(), "Variables will be applied once a remote is configured.")
}

func TestProcessVariablesNoDeclarations(t *testing.T) {
	for _, variables := range [][]model.VariableEntry{nil, {}} {
		for _, mode := range []alter.ApplyMode{alter.DryRun, alter.Apply, alter.Recut} {
			var stderr strings.Builder
			results, err := alter.ProcessVariables(&config.Config{Variables: variables}, mode, alter.RepoTarget{HasRepo: true, Stderr: &stderr})
			if err != nil || results != nil || stderr.Len() != 0 {
				t.Errorf("empty declarations: results %v, error %v, stderr %q", results, err, stderr.String())
			}
		}
	}
}

func TestMeasureVariablesOffline(t *testing.T) {
	tc := setupAlterTest(t, variableConfig)
	health := measure.CheckHealth(tc.Dir)
	diff := measure.CheckConfigDiff(loadTestConfig(t, tc.Dir), swatch.All())
	if output := measure.FormatOutput(health, diff, true); output == "" {
		t.Fatal("measure output is empty")
	}
	if len(tc.Calls()) != 0 {
		t.Errorf("measure made API calls: %v", tc.Calls())
	}
}
