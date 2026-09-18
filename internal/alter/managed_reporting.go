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
	plan    managedPlan
	planned []SwatchResult
}

func prepareManagedExecution(cfg *config.Config, dir string, renderer managedRenderer, availableOnly ...bool) (*managedExecution, error) {
	selections, err := selectManagedFiles(cfg)
	if err != nil {
		return nil, err
	}
	if len(availableOnly) != 0 && availableOnly[0] {
		selections = slices.DeleteFunc(selections, func(selection managedSelection) bool {
			return !selection.Entry.Available
		})
	}
	rendered, err := renderer(slices.Clone(selections))
	if err != nil {
		return nil, fmt.Errorf("rendering managed files: %w", err)
	}
	if err := validateManagedRecipeNames(rendered); err != nil {
		return nil, err
	}
	plan, err := preflightManagedFiles(dir, selections, rendered)
	if err != nil {
		return nil, err
	}
	planned, err := managedPlannedResults(dir, plan)
	if err != nil {
		return nil, err
	}
	planned, err = appendPreservedManagedRoots(dir, planned)
	if err != nil {
		return nil, err
	}

	return &managedExecution{plan: plan, planned: planned}, nil
}

func managedExcludedPaths() map[string]struct{} {
	excluded := make(map[string]struct{})
	for _, entry := range fixedManagedRegistry() {
		excluded[entry.Path] = struct{}{}
	}
	return excluded
}

func validateManagedRecipeNames(rendered managedRenderedFiles) error {
	paths := make([]string, 0, len(rendered))
	for name := range rendered {
		if name == "justfile" || strings.HasSuffix(name, ".just") {
			paths = append(paths, name)
		}
	}
	sort.Strings(paths)

	owners := make(map[string]string)
	for _, name := range paths {
		for line := range strings.SplitSeq(string(rendered[name]), "\n") {
			if line == "" || strings.TrimLeft(line, " \t") != line || strings.HasPrefix(line, "#") {
				continue
			}
			colon := strings.IndexByte(line, ':')
			if colon < 1 || (colon+1 < len(line) && line[colon+1] == '=') {
				continue
			}
			fields := strings.Fields(line[:colon])
			if len(fields) == 0 || !validManagedRecipeName(fields[0]) {
				continue
			}
			recipe := fields[0]
			if owner, exists := owners[recipe]; exists {
				return fmt.Errorf("generated managed files contain duplicate recipe %q in %q and %q", recipe, owner, name)
			}
			owners[recipe] = name
		}
	}
	return nil
}

func validManagedRecipeName(name string) bool {
	for index, character := range name {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character == '_' || index > 0 && (character >= '0' && character <= '9' || character == '-') {
			continue
		}
		return false
	}
	return name != ""
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

func appendPreservedManagedRoots(dir string, results []SwatchResult) (_ []SwatchResult, retErr error) {
	reported := make(map[string]struct{}, len(results))
	for _, result := range results {
		reported[result.Path] = struct{}{}
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("opening managed project root %q for protected root reporting: %w", dir, err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("closing managed project root %q after protected root reporting: %w", dir, err))
		}
	}()

	for _, entry := range fixedManagedRegistry() {
		if entry.Policy != managedPolicyRoot {
			continue
		}
		if _, exists := reported[entry.Path]; exists {
			continue
		}
		snapshot, err := snapshotManagedDestination(root, managedSelection{Entry: entry})
		if err != nil {
			return nil, err
		}
		switch snapshot.Kind {
		case managedDestinationMissing:
			continue
		case managedDestinationRegular, managedDestinationSymlink:
			results = append(results, SwatchResult{Path: entry.Path, Category: Skipped, Reason: SkipManagedRootExists})
		case managedDestinationDirectory:
			return nil, fmt.Errorf("managed destination %q is a directory", entry.Path)
		case managedDestinationSpecial:
			return nil, fmt.Errorf("managed destination %q is not a regular file or symlink", entry.Path)
		default:
			return nil, fmt.Errorf("managed destination %q has an invalid snapshot kind", entry.Path)
		}
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

func appendManagedReporting(report *Report, results []SwatchResult) {
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
			guidance = append(guidance, "If absent, add `import 'just/loader.just'` to the preserved `justfile`.")
		case "flake.nix":
			guidance = append(guidance, "If absent, add `++ import ./nix/loader.nix { inherit pkgs; }` to the existing package list in the preserved `flake.nix`.")
		}
	}
	if len(newNix) != 0 {
		sort.Strings(newNix)
		appendManagedNotice(report, "review and add new Nix files to Git because Nix flakes exclude untracked files: "+managedPathList(newNix))
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
