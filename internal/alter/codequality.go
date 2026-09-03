package alter

import (
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
	return processSetup(compareCodeQuality(cfg.CodeQuality, &model.CodeQualitySettings{}), mode,
		func() ([]RepoSettingResult, error) {
			live, err := gh.ReadCodeQualitySetup(target.Client, target.Owner, target.Name)
			if err != nil {
				return nil, err
			}
			return compareCodeQuality(cfg.CodeQuality, live), nil
		},
		func(results []RepoSettingResult) error {
			desired := changedSettings(cfg.CodeQuality, results, model.CodeQualitySettingFields)
			return gh.ApplyCodeQualitySetup(target.Client, target.Owner, target.Name, desired)
		})
}

func compareCodeQuality(declared, live *model.CodeQualitySettings) []RepoSettingResult {
	c := &resultComparer{section: "code_quality"}
	c.str("state", declared.State, live.State)
	c.languages(declared.Languages, live.Languages)
	return c.results
}
