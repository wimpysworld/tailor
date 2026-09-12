package alter

import (
	"errors"
	"fmt"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
)

// ProcessImmutableReleases applies only a declared toggle that differs.
func ProcessImmutableReleases(cfg *config.Config, mode ApplyMode, target RepoTarget) ([]RepoSettingResult, error) {
	if cfg.ImmutableReleases == nil || cfg.ImmutableReleases.Enabled == nil || target.missingRepo("Immutable releases") {
		return nil, nil
	}
	enabled := *cfg.ImmutableReleases.Enabled
	result := RepoSettingResult{Section: "immutable_releases", Field: "enabled", Category: WouldSet, Value: fmt.Sprint(enabled)}
	live, err := gh.ReadImmutableReleases(target.Client, target.Owner, target.Name)
	if err == nil {
		result.Before = fmt.Sprint(*live.Enabled)
		switch {
		case !enabled && *live.EnforcedByOwner:
			result.Category = WouldSkipSetup
			result.Annotation = "enforced by owner"
		case enabled == *live.Enabled:
			result.Category = RepoNoChange
		case mode.ShouldWrite():
			err = gh.ApplyImmutableReleases(target.Client, target.Owner, target.Name, enabled)
		}
	}
	if err != nil {
		if _, ok := errors.AsType[*gh.ErrInsufficientScope](err); !ok {
			return nil, err
		}
		result.Category = WouldSkipSetup
		result.Annotation = "insufficient scope"
	}
	return []RepoSettingResult{result}, nil
}
