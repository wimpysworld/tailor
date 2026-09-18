package alter

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
)

type managedApplyResult struct {
	Path      string
	Operation managedOperation
}

func applyManagedFiles(dir string, plan managedPlan) ([]managedApplyResult, error) {
	return applyManagedFilesWithHooks(dir, plan, managedApplyHooks{})
}

func applyManagedFilesWithHooks(dir string, plan managedPlan, hooks managedApplyHooks) (results []managedApplyResult, retErr error) {
	if err := validateManagedPlan(plan); err != nil {
		return nil, err
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("opening managed project root %q: %w", dir, err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("closing managed project root %q: %w", dir, err))
		}
	}()

	files := slices.Clone(plan.Files)
	sort.Slice(files, func(i, j int) bool {
		left, right := managedSelectionRank(files[i].Selection), managedSelectionRank(files[j].Selection)
		if left != right {
			return left < right
		}
		return files[i].Selection.Entry.Path < files[j].Selection.Entry.Path
	})

	for _, file := range files {
		if file.Operation != managedOperationWrite && file.Operation != managedOperationRemove {
			continue
		}

		current, err := recheckManagedPlanFile(root, file)
		if err != nil {
			return results, err
		}
		if current.Operation == managedOperationNoop || current.Operation == managedOperationPreserve {
			continue
		}
		if current.Operation != file.Operation {
			return results, fmt.Errorf("managed destination %q changed to an incompatible operation", file.Selection.Entry.Path)
		}

		var changed bool
		switch current.Operation {
		case managedOperationWrite:
			changed, err = writeManagedFile(root, current.Selection.Entry.Path, current.Content, hooks)
		case managedOperationRemove:
			changed, err = removeManagedFile(root, current.Selection.Entry.Path, hooks)
		}
		if changed {
			results = append(results, managedApplyResult{
				Path:      current.Selection.Entry.Path,
				Operation: current.Operation,
			})
		}
		if err != nil {
			return results, err
		}
	}

	return results, nil
}

func recheckManagedPlanFile(root *os.Root, file managedPlanFile) (managedPlanFile, error) {
	snapshot, err := snapshotManagedDestination(root, file.Selection)
	if err != nil {
		return managedPlanFile{}, err
	}

	var rendered managedRenderedFiles
	if file.Selection.Enabled {
		rendered = managedRenderedFiles{file.Selection.Entry.Path: file.Content}
	}
	current, err := planManagedFile(file.Selection, rendered, snapshot)
	if err != nil {
		return managedPlanFile{}, err
	}
	return current, nil
}
