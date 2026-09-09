package alter

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/model"
	"gopkg.in/yaml.v3"
)

type pagesRun struct {
	prepared    *pagesPreparation
	repository  *gh.PagesRepositoryState
	site        *gh.PagesState
	environment *gh.PagesEnvironmentState
	skipped     []RepoSettingResult
}

func pagesEnabled(cfg *config.Config) bool {
	return cfg.Pages != nil && cfg.Pages.Enabled != nil && *cfg.Pages.Enabled
}

func preflightPages(cfg *config.Config, dir string, mode ApplyMode, target RepoTarget, prepared *pagesPreparation) (*pagesRun, error) {
	if prepared == nil {
		return nil, nil
	}
	p := &pagesRun{prepared: prepared}
	if target.missingRepo("Pages") {
		p.skipped = []RepoSettingResult{{Section: "pages", Field: "enabled", Category: WouldSkipSetup, Annotation: "no repository"}}
		return p, nil
	}
	prepared.ProjectName = target.Name
	prepared.RepoURL = "https://github.com/" + target.Owner + "/" + target.Name
	if prepared.Starter {
		if _, err := processPagesStarter(cfg, dir, DryRun, prepared); err != nil {
			return nil, err
		}
	} else if prepared.Navigation {
		if _, err := processStaticPages(cfg, dir, DryRun, prepared); err != nil {
			return nil, err
		}
	}
	repository, accessErr := gh.ReadPagesRepository(target.Client, target.Owner, target.Name)
	p.repository = repository
	if accessErr != nil {
		p.skipped, accessErr = pagesErrorResults(accessErr)
		return p, accessErr
	}
	if repository != nil && repository.DefaultBranch != "" {
		if err := preparePagesWorkflow(prepared, dir, mode, repository.DefaultBranch); err != nil {
			return nil, err
		}
	}
	var err error
	p.site, err = gh.ReadPages(target.Client, target.Owner, target.Name, repository.WriteAccess)
	if err == nil {
		p.environment, err = gh.ReadPagesEnvironment(target.Client, target.Owner, target.Name, prepared.Branch, repository.AdminAccess)
	}
	if err == nil {
		err = checkPagesActions(cfg, target, prepared, true)
	}
	if err != nil {
		p.skipped, err = pagesErrorResults(err)
	}
	return p, err
}

func pagesErrorResults(err error) ([]RepoSettingResult, error) {
	result := RepoSettingResult{Section: "pages", Field: "enabled"}
	var scope *gh.ErrInsufficientScope
	var skipped *gh.ErrSetupSkipped
	var pending *gh.ErrPagesPending
	switch {
	case errors.As(err, &scope):
		result.Category, result.Annotation = WouldSkipScope, skipAnnotation
	case errors.As(err, &skipped):
		result.Category, result.Annotation = WouldSkipSetup, string(skipped.Reason)
	case errors.As(err, &pending):
		result.Category, result.Annotation = WouldSkipSetup, pending.Reason
	default:
		return nil, err
	}
	return []RepoSettingResult{result}, nil
}

func processPages(cfg *config.Config, dir string, mode ApplyMode, target RepoTarget, p *pagesRun) ([]RepoSettingResult, *SwatchResult, error) {
	if p == nil {
		return nil, nil, nil
	}
	if len(p.skipped) != 0 {
		return p.skipped, nil, nil
	}
	if mode.ShouldWrite() {
		if err := checkPagesActions(cfg, target, p.prepared, false); err != nil {
			results, err := pagesErrorResults(err)
			return results, nil, err
		}
	}
	results := comparePages(cfg, p)
	var reconcileErr error
	if mode.ShouldWrite() {
		environment, err := gh.ApplyPagesEnvironment(target.Client, target.Owner, target.Name, p.environment)
		applied := environment.Applied
		reconcileErr = err
		if err == nil {
			var site gh.ApplyResult
			site, p.site, reconcileErr = gh.ApplyPages(target.Client, target.Owner, target.Name, p.site, cfg.Pages.CNAME)
			applied = append(applied, site.Applied...)
		}
		if reconcileErr != nil {
			results = confirmedPagesResults(results, applied, false)
			skips, hardErr := pagesErrorResults(reconcileErr)
			results = append(results, skips...)
			if hardErr != nil {
				return results, nil, hardErr
			}
			var pending *gh.ErrPagesPending
			if !errors.As(reconcileErr, &pending) {
				p.skipped = skips
				return results, nil, nil
			}
			pagesPendingGuidance(target, cfg, p.site, pending)
		} else {
			results = confirmedPagesResults(results, applied, true)
		}
	}
	workflow := p.prepared.Result
	if mode.ShouldWrite() && (workflow.Category == WouldCopy || workflow.Category == WouldOverwrite) {
		root, err := os.OpenRoot(dir)
		if err != nil {
			return results, nil, err
		}
		defer root.Close()
		if err := preparePagesWorkflow(p.prepared, dir, mode, p.prepared.Branch); err != nil {
			return results, nil, err
		}
		workflow = p.prepared.Result
		if workflow.Category == WouldCopy || workflow.Category == WouldOverwrite {
			workflow, err = writeSwatch(root, p.prepared.Entry, p.prepared.Content, workflow.Category, true)
			if err != nil {
				return results, nil, err
			}
		}
	}
	if reconcileErr == nil {
		site := *p.site
		if !mode.ShouldWrite() {
			site.Exists = true
			if cfg.Pages.CNAME != nil {
				site.CNAME = *cfg.Pages.CNAME
				if site.CNAME == "" {
					site.HTMLURL = ""
				}
			}
		}
		homepage, err := processPagesHomepage(cfg, mode, target, p.repository, &site)
		results = append(results, homepage...)
		if err != nil {
			return results, &workflow, err
		}
	}
	return results, &workflow, nil
}

func comparePages(cfg *config.Config, p *pagesRun) []RepoSettingResult {
	c := resultComparer{section: "pages"}
	c.add("environment", "github-pages", !p.environment.Missing)
	c.add("branch", p.prepared.Branch, !p.environment.AddBranch)
	c.add("build_type", "workflow", p.site.Exists && p.site.BuildType == "workflow")
	if cfg.Pages.CNAME != nil {
		c.add("cname", *cfg.Pages.CNAME, *cfg.Pages.CNAME == p.site.CNAME)
	}
	c.add("https_enforced", "true", p.site.HTTPSEnforced && (cfg.Pages.CNAME == nil || *cfg.Pages.CNAME == p.site.CNAME))
	return c.results
}

func confirmedPagesResults(results []RepoSettingResult, applied []gh.Operation, complete bool) []RepoSettingResult {
	fields := make(map[string]bool)
	for _, op := range applied {
		switch op.Kind {
		case gh.OpPutPagesEnvironment:
			fields["environment"] = true
		case gh.OpPostPagesEnvironmentPolicy:
			fields["branch"] = true
		case gh.OpCreatePages, gh.OpSetPagesBuildType:
			fields["build_type"] = true
		case gh.OpSetPagesDomain:
			fields["cname"] = true
		case gh.OpEnforcePagesHTTPS:
			fields["https_enforced"] = true
		}
	}
	var confirmed []RepoSettingResult
	for _, result := range results {
		if result.Category == RepoNoChange || fields[result.Field] {
			confirmed = append(confirmed, result)
		} else if complete {
			result.Category = RepoNoChange
			confirmed = append(confirmed, result)
		}
	}
	return confirmed
}

func checkPagesActions(cfg *config.Config, target RepoTarget, prepared *pagesPreparation, planned bool) error {
	policy, warnings, err := gh.ReadActionsPolicy(target.Client, target.Owner, target.Name, true)
	if err != nil {
		return err
	}
	if len(warnings) != 0 {
		return warnings[0]
	}
	if planned && cfg.Actions != nil {
		desired := cfg.Actions
		if desired.Enabled != nil {
			policy.Enabled = desired.Enabled
		}
		if desired.AllowedActions != nil {
			policy.AllowedActions = desired.AllowedActions
		}
		if desired.GitHubOwnedAllowed != nil {
			policy.GitHubOwnedAllowed = desired.GitHubOwnedAllowed
		}
		if desired.VerifiedAllowed != nil {
			policy.VerifiedAllowed = desired.VerifiedAllowed
		}
		if desired.PatternsAllowed != nil {
			policy.PatternsAllowed = desired.PatternsAllowed
		}
	}
	if policy.Enabled == nil || !*policy.Enabled {
		return fmt.Errorf("pages requires GitHub Actions, but the effective Actions policy disables it")
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Uses string `yaml:"uses"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(prepared.Content, &workflow); err != nil {
		return err
	}
	var blocked []string
	for _, job := range workflow.Jobs {
		for _, step := range job.Steps {
			if step.Uses != "" && !pagesActionAllowed(policy, step.Uses) {
				blocked = append(blocked, step.Uses)
			}
		}
	}
	if len(blocked) != 0 {
		slices.Sort(blocked)
		return fmt.Errorf("pages required actions are blocked by the effective Actions policy: %s", strings.Join(slices.Compact(blocked), ", "))
	}
	return nil
}

func pagesActionAllowed(policy *model.ActionsSettings, action string) bool {
	if policy.AllowedActions == nil {
		return false
	}
	if *policy.AllowedActions == "all" {
		return true
	}
	if *policy.AllowedActions != "selected" {
		return false
	}
	allowed := policy.GitHubOwnedAllowed != nil && *policy.GitHubOwnedAllowed && strings.HasPrefix(action, "actions/")
	// Verified creator: https://github.com/marketplace/actions/setup-ruby-jruby-and-truffleruby
	const verifiedAction = "ruby/setup-ruby"
	if policy.VerifiedAllowed != nil && *policy.VerifiedAllowed && strings.HasPrefix(action, verifiedAction+"@") {
		allowed = true
	}
	if policy.PatternsAllowed != nil {
		for _, pattern := range *policy.PatternsAllowed {
			excluded := strings.HasPrefix(pattern, "!")
			pattern = strings.TrimPrefix(pattern, "!")
			if !strings.Contains(pattern, "@") {
				pattern += "@*"
			}
			expression := "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), `\*`, ".*") + "$"
			if regexp.MustCompile(expression).MatchString(action) {
				if excluded {
					return false
				}
				allowed = true
			}
		}
	}
	return allowed
}

func pagesPendingGuidance(target RepoTarget, cfg *config.Config, site *gh.PagesState, pending *gh.ErrPagesPending) {
	domain := site.CNAME
	if cfg.Pages.CNAME != nil {
		domain = *cfg.Pages.CNAME
	}
	reason := strings.ToLower(pending.Reason)
	switch {
	case strings.Contains(reason, "verif") || strings.Contains(reason, "ownership"):
		fmt.Fprintf(target.stderr(), "pages: verify %s with the TXT record shown in account Pages settings (https://github.com/settings/pages) or organisation Pages settings (https://github.com/organizations/%s/settings/pages), then rerun tailor alter\n", domain, target.Owner)
	case strings.Contains(reason, "resolve") || strings.Contains(reason, "dns"):
		fmt.Fprintf(target.stderr(), "pages: for a subdomain, point %s by CNAME to %s.github.io. For an apex domain, use https://docs.github.com/en/pages/configuring-a-custom-domain-for-your-github-pages-site/managing-a-custom-domain-for-your-github-pages-site\n", domain, target.Owner)
	default:
		fmt.Fprintln(target.stderr(), "pages: wait for the HTTPS certificate, then rerun tailor alter")
	}
}
