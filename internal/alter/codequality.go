package alter

import (
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/model"
)

// ProcessCodeQuality previews or applies changed fields in the declared Code Quality setup.
// Unavailable or busy setup skips all declared fields on reads, and only changed fields on writes.
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
