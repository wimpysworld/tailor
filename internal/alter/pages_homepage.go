package alter

import (
	"strings"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/model"
)

// processPagesHomepage replaces only an undeclared homepage that still points to the repository itself.
// Explicit declarations and other live values, including an empty homepage, remain unchanged.
func processPagesHomepage(cfg *config.Config, mode ApplyMode, target RepoTarget, repository *gh.PagesRepositoryState, site *gh.PagesState) ([]RepoSettingResult, error) {
	if cfg.HomepageDeclared() || repository == nil || site == nil || !site.Exists {
		return nil, nil
	}
	repoURL := "https://github.com/" + target.Owner + "/" + target.Name
	if strings.TrimSuffix(repository.Homepage, "/") != repoURL {
		return nil, nil
	}
	homepage := site.HTMLURL
	if site.CNAME != "" {
		homepage = "https://" + site.CNAME + "/"
	}
	if homepage == "" && !mode.ShouldWrite() {
		return []RepoSettingResult{{Field: "homepage", Category: WouldSkipSetup, Annotation: "after Pages reports its URL"}}, nil
	}
	if homepage == "" || homepage == repository.Homepage {
		return nil, nil
	}
	result := RepoSettingResult{Field: "homepage", Category: WouldSet, Value: homepage, Before: repository.Homepage}
	if mode.ShouldWrite() {
		applied, err := gh.ApplyRepoSettings(target.Client, target.Owner, target.Name, &model.RepositorySettings{Homepage: &homepage}, &model.RepositorySettings{Homepage: &repository.Homepage})
		if err != nil {
			return nil, err
		}
		if len(applied.Skipped) != 0 {
			return skippedToResults(applied), nil
		}
	}
	return []RepoSettingResult{result}, nil
}
