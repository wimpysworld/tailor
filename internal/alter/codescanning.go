package alter

import (
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
	return processSetup(compareCodeScanning(cfg.CodeScanning, &model.CodeScanningSettings{}), mode,
		func() ([]RepoSettingResult, error) {
			live, err := gh.ReadCodeScanningSetup(target.Client, target.Owner, target.Name)
			if err != nil {
				return nil, err
			}
			return compareCodeScanning(cfg.CodeScanning, live), nil
		},
		func(results []RepoSettingResult) error {
			desired := changedSettings(cfg.CodeScanning, results, model.CodeScanningSettingFields)
			return gh.ApplyCodeScanningSetup(target.Client, target.Owner, target.Name, desired)
		})
}

func compareCodeScanning(declared, live *model.CodeScanningSettings) []RepoSettingResult {
	c := &resultComparer{section: "code_scanning"}
	c.str("state", declared.State, live.State)
	c.str("query_suite", declared.QuerySuite, live.QuerySuite)
	c.str("threat_model", declared.ThreatModel, live.ThreatModel)
	c.languages(declared.Languages, live.Languages)
	return c.results
}
