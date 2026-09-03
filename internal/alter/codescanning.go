package alter

import (
	"errors"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/model"
)

// ProcessCodeScanning compares the declared code scanning default setup
// against GitHub and applies only the fields that differ. A repository
// without the feature, or with a setup run in progress, reports the declared
// fields as skipped.
func ProcessCodeScanning(cfg *config.Config, mode ApplyMode, target RepoTarget) ([]RepoSettingResult, error) {
	if cfg.CodeScanning == nil || !target.HasRepo {
		return nil, nil
	}
	declared := compareCodeScanning(cfg.CodeScanning, &model.CodeScanningSettings{})
	if len(declared) == 0 {
		return nil, nil
	}

	live, err := gh.ReadCodeScanningSetup(target.Client, target.Owner, target.Name)
	var skipped *gh.ErrSetupSkipped
	if errors.As(err, &skipped) {
		return setupSkipResults(declared, skipped.Reason), nil
	}
	if err != nil {
		return nil, err
	}

	results := compareCodeScanning(cfg.CodeScanning, live)
	if !mode.ShouldWrite() || !hasChanges(results) {
		return results, nil
	}
	return applySetup(results, func() error {
		desired := changedSettings(cfg.CodeScanning, results, model.CodeScanningSettingFields)
		return gh.ApplyCodeScanningSetup(target.Client, target.Owner, target.Name, desired)
	})
}

func compareCodeScanning(declared, live *model.CodeScanningSettings) []RepoSettingResult {
	const section = "code_scanning"
	var results []RepoSettingResult
	if result, ok := setupStringResult(section, "state", declared.State, live.State); ok {
		results = append(results, result)
	}
	if result, ok := setupStringResult(section, "query_suite", declared.QuerySuite, live.QuerySuite); ok {
		results = append(results, result)
	}
	if result, ok := setupStringResult(section, "threat_model", declared.ThreatModel, live.ThreatModel); ok {
		results = append(results, result)
	}
	if result, ok := setupLanguagesResult(section, declared.Languages, live.Languages); ok {
		results = append(results, result)
	}
	return results
}
