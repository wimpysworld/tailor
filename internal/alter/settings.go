package alter

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
)

// RepoSettingCategory classifies the outcome of processing a single repository setting.
type RepoSettingCategory string

const (
	WouldSet     RepoSettingCategory = "would set"
	RepoNoChange RepoSettingCategory = "no change"
)

// RepoSettingResult records the field name, category, and display value for one
// repository setting.
type RepoSettingResult struct {
	Field    string
	Category RepoSettingCategory
	Value    string
}

// ProcessRepoSettings compares declared settings against live settings
// and optionally applies them. Returns results for output formatting.
func ProcessRepoSettings(cfg *config.Config, mode ApplyMode, client *api.RESTClient, owner, name string, hasRepo bool) ([]RepoSettingResult, error) {
	if cfg.Repository == nil {
		return nil, nil
	}

	if !hasRepo {
		fmt.Fprintln(os.Stderr, "No GitHub repository context found. Repository settings will be applied once a remote is configured.")
		return nil, nil
	}

	live, err := gh.ReadRepoSettings(client, owner, name)
	if err != nil {
		return nil, err
	}

	results := compareSettings(cfg.Repository, live)

	if mode.ShouldWrite() && hasChanges(results) {
		if err := gh.ApplyRepoSettings(client, owner, name, cfg.Repository); err != nil {
			return nil, err
		}
	}

	return results, nil
}

// compareSettings iterates non-nil pointer fields in declared and compares
// each against the corresponding field in live. Returns a result per declared field.
func compareSettings(declared, live *config.RepositorySettings) []RepoSettingResult {
	dv := reflect.ValueOf(declared).Elem()
	lv := reflect.ValueOf(live).Elem()
	dt := dv.Type()

	var results []RepoSettingResult

	for i := range dt.NumField() {
		field := dt.Field(i)
		tag := field.Tag.Get("yaml")
		if tag == "" || tag == ",inline" {
			continue
		}
		key, _, _ := strings.Cut(tag, ",")

		dfv := dv.Field(i)
		if dfv.Kind() != reflect.Ptr || dfv.IsNil() {
			continue
		}

		declaredVal := dfv.Elem().Interface()
		displayVal := formatValue(declaredVal)

		lfv := lv.Field(i)
		if !lfv.IsNil() && lfv.Elem().Interface() == declaredVal {
			results = append(results, RepoSettingResult{
				Field:    key,
				Category: RepoNoChange,
				Value:    displayVal,
			})
		} else {
			results = append(results, RepoSettingResult{
				Field:    key,
				Category: WouldSet,
				Value:    displayVal,
			})
		}
	}

	return results
}

// formatValue renders a setting value for display output.
func formatValue(v any) string {
	return fmt.Sprintf("%v", v)
}

// hasChanges returns true if any result is WouldSet.
func hasChanges(results []RepoSettingResult) bool {
	for _, r := range results {
		if r.Category == WouldSet {
			return true
		}
	}
	return false
}
