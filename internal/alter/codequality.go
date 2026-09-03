package alter

import (
	"errors"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/model"
)

// ProcessCodeQuality compares the declared Code Quality setup against GitHub
// and applies only the fields that differ. A repository without the feature,
// or with a setup run in progress, reports the declared fields as skipped.
func ProcessCodeQuality(cfg *config.Config, mode ApplyMode, target RepoTarget) ([]RepoSettingResult, error) {
	if cfg.CodeQuality == nil || !target.HasRepo {
		return nil, nil
	}
	declared := compareCodeQuality(cfg.CodeQuality, &model.CodeQualitySettings{})
	if len(declared) == 0 {
		return nil, nil
	}

	live, err := gh.ReadCodeQualitySetup(target.Client, target.Owner, target.Name)
	var skipped *gh.ErrSetupSkipped
	if errors.As(err, &skipped) {
		return skipResults(declared, WouldSkipSetup, string(skipped.Reason)), nil
	}
	if err != nil {
		return nil, err
	}

	results := compareCodeQuality(cfg.CodeQuality, live)
	if !mode.ShouldWrite() || !hasChanges(results) {
		return results, nil
	}
	return applySetup(results, func() error {
		desired := changedSettings(cfg.CodeQuality, results, model.CodeQualitySettingFields)
		return gh.ApplyCodeQualitySetup(target.Client, target.Owner, target.Name, desired)
	})
}

func compareCodeQuality(declared, live *model.CodeQualitySettings) []RepoSettingResult {
	const section = "code_quality"
	var results []RepoSettingResult
	if result, ok := setupStringResult(section, "state", declared.State, live.State); ok {
		results = append(results, result)
	}
	if result, ok := setupLanguagesResult(section, declared.Languages, live.Languages); ok {
		results = append(results, result)
	}
	return results
}
