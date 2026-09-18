package alter

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/output"
)

type managedRenderer func([]managedSelection) (managedRenderedFiles, error)

type managedExecution struct {
	plan                       managedPlan
	planned                    []SwatchResult
	retainedPlaywrightSettings []string
}

func prepareManagedExecution(cfg *config.Config, dir string, renderer managedRenderer) (*managedExecution, error) {
	selections, err := selectManagedFiles(cfg)
	if err != nil {
		return nil, err
	}
	rendered, err := renderer(slices.Clone(selections))
	if err != nil {
		return nil, fmt.Errorf("rendering managed files: %w", err)
	}
	plan, err := preflightManagedFiles(dir, selections, rendered)
	if err != nil {
		return nil, err
	}
	planned, err := managedPlannedResults(dir, plan)
	if err != nil {
		return nil, err
	}

	execution := &managedExecution{plan: plan, planned: planned}
	if cfg.PlaywrightDeclared() && !cfg.PlaywrightEnabled() {
		execution.retainedPlaywrightSettings = retainedManagedSharedSettings(dir)
	}
	return execution, nil
}

func managedExcludedPaths() map[string]struct{} {
	excluded := make(map[string]struct{})
	for _, entry := range fixedManagedRegistry() {
		excluded[entry.Path] = struct{}{}
	}
	return excluded
}

func managedPlannedResults(dir string, plan managedPlan) (results []SwatchResult, retErr error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("opening managed project root %q for reporting: %w", dir, err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("closing managed project root %q after reporting: %w", dir, err))
		}
	}()

	results = make([]SwatchResult, 0, len(plan.Files))
	for _, file := range plan.Files {
		result := SwatchResult{Path: file.Selection.Entry.Path}
		switch file.Operation {
		case managedOperationWrite:
			_, err := root.Lstat(result.Path)
			switch {
			case errors.Is(err, os.ErrNotExist):
				result.Category = WouldCopy
			case err != nil:
				return nil, fmt.Errorf("checking managed destination %q for reporting: %w", result.Path, err)
			default:
				result.Category = WouldOverwrite
			}
		case managedOperationRemove:
			result.Category = WouldRemove
		case managedOperationNoop:
			result.Category = NoChange
		case managedOperationPreserve:
			result.Category = Skipped
			switch file.Selection.Entry.Policy {
			case managedPolicyRoot:
				result.Reason = SkipManagedRootExists
			case managedPolicySharedStarter:
				result.Reason = SkipManagedSharedExists
			}
		default:
			return nil, fmt.Errorf("managed destination %q has an invalid reporting operation", result.Path)
		}
		results = append(results, result)
	}
	return results, nil
}

func managedConfirmedResults(planned []SwatchResult, confirmed []managedApplyResult) ([]SwatchResult, error) {
	byPath := make(map[string]SwatchResult, len(planned))
	results := make([]SwatchResult, 0, len(planned))
	for _, result := range planned {
		if _, exists := byPath[result.Path]; exists {
			return nil, fmt.Errorf("managed reporting contains duplicate destination %q", result.Path)
		}
		byPath[result.Path] = result
		if result.Category == NoChange || result.Category == Skipped {
			results = append(results, result)
		}
	}

	seen := make(map[string]struct{}, len(confirmed))
	for _, applied := range confirmed {
		if _, exists := seen[applied.Path]; exists {
			return nil, fmt.Errorf("managed confirmed results contain duplicate destination %q", applied.Path)
		}
		seen[applied.Path] = struct{}{}
		result, ok := byPath[applied.Path]
		if !ok {
			return nil, fmt.Errorf("managed confirmed destination %q was not planned", applied.Path)
		}
		switch applied.Operation {
		case managedOperationWrite:
			if result.Category != WouldCopy && result.Category != WouldOverwrite {
				return nil, fmt.Errorf("managed confirmed write for %q does not match its planned result", applied.Path)
			}
		case managedOperationRemove:
			if result.Category != WouldRemove {
				return nil, fmt.Errorf("managed confirmed removal for %q does not match its planned result", applied.Path)
			}
		default:
			return nil, fmt.Errorf("managed confirmed destination %q has an invalid operation", applied.Path)
		}
		results = append(results, result)
	}
	return results, nil
}

func managedConflictResult(err error) (SwatchResult, bool) {
	var ownershipErr *managedOwnershipError
	if !errors.As(err, &ownershipErr) {
		return SwatchResult{}, false
	}
	return SwatchResult{Path: ownershipErr.Path, Category: ManagedConflict, Reason: ManagedNotOwned}, true
}

func retainedManagedSharedSettings(dir string) []string {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil
	}
	defer root.Close()

	var retained []string
	for _, entry := range fixedManagedRegistry() {
		if entry.Policy != managedPolicySharedStarter {
			continue
		}
		if _, err := root.Lstat(entry.Path); err == nil {
			retained = append(retained, entry.Path)
		}
	}
	sort.Strings(retained)
	return retained
}

func appendManagedReporting(report *Report, results []SwatchResult, retainedPlaywrightSettings []string) {
	var newNix []string
	var guidance []string
	for _, result := range results {
		if result.Category == WouldCopy && (result.Path == "flake.nix" || strings.HasSuffix(result.Path, ".nix")) {
			newNix = append(newNix, result.Path)
		}
		if result.Category != Skipped || result.Reason != SkipManagedRootExists {
			continue
		}
		switch result.Path {
		case "justfile":
			guidance = append(guidance, "Connect the preserved `justfile` to `just/loader.just` manually.")
		case "flake.nix":
			guidance = append(guidance, "Connect the preserved `flake.nix` to `nix/loader.nix` manually.")
		}
	}
	if len(newNix) != 0 {
		sort.Strings(newNix)
		appendManagedNotice(report, "review and add new Nix files to Git because Nix flakes exclude untracked files: "+managedPathList(newNix))
	}
	if len(retainedPlaywrightSettings) != 0 {
		appendManagedNotice(report, "mcp.playwright is false, but retained shared settings can invoke the unavailable Playwright MCP executable: "+managedPathList(retainedPlaywrightSettings))
	}
	if len(guidance) != 0 {
		appendGuidance(report, strings.Join(guidance, "\n")+"\n")
	}
}

func appendManagedNotice(report *Report, text string) {
	report.Plain += "warning: " + text + "\n"
	report.Document.Notices = append(report.Document.Notices, output.Notice{Level: "warning", Text: text})
}

func managedPathList(paths []string) string {
	quoted := make([]string, len(paths))
	for i, path := range paths {
		quoted[i] = "`" + path + "`"
	}
	return strings.Join(quoted, ", ")
}
