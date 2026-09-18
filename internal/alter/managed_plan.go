package alter

import (
	"bytes"
	"fmt"
	"slices"
	"sort"
)

type managedDestinationKind uint8

const (
	managedDestinationMissing managedDestinationKind = iota
	managedDestinationRegular
	managedDestinationSymlink
	managedDestinationDirectory
	managedDestinationSpecial
)

type managedDestinationSnapshot struct {
	Path         string
	Kind         managedDestinationKind
	Content      []byte
	UnsafeParent bool
}

type managedOperation uint8

const (
	managedOperationNoop managedOperation = iota
	managedOperationWrite
	managedOperationRemove
	managedOperationPreserve
)

type managedPlanFile struct {
	Selection managedSelection
	Operation managedOperation
	Content   []byte
}

type managedPlan struct {
	Files []managedPlanFile
}

type managedRenderedFiles map[string][]byte

type managedOwnershipError struct {
	Path string
}

func (err *managedOwnershipError) Error() string {
	return fmt.Sprintf("managed destination %q is not owned by Tailor", err.Path)
}

func managedMarker(destination string) string {
	return "# Managed by Tailor: " + destination
}

func hasManagedMarker(content []byte, destination string) bool {
	marker := []byte(managedMarker(destination))
	if !bytes.HasPrefix(content, marker) {
		return false
	}
	rest := content[len(marker):]
	return bytes.HasPrefix(rest, []byte("\n")) || bytes.HasPrefix(rest, []byte("\r\n"))
}

func planManagedFiles(selections []managedSelection, rendered managedRenderedFiles, snapshots []managedDestinationSnapshot) (managedPlan, error) {
	ordered, err := orderManagedSelections(selections)
	if err != nil {
		return managedPlan{}, err
	}

	selected := make(map[string]managedSelection, len(ordered))
	for _, selection := range ordered {
		selected[selection.Entry.Path] = selection
		if !selection.Enabled {
			continue
		}
		content, ok := rendered[selection.Entry.Path]
		if !ok {
			return managedPlan{}, fmt.Errorf("rendered managed file %q is missing", selection.Entry.Path)
		}
		if selection.Entry.Policy.marked() && !hasManagedMarker(content, selection.Entry.Path) {
			return managedPlan{}, fmt.Errorf("rendered managed file %q must start with %q", selection.Entry.Path, managedMarker(selection.Entry.Path))
		}
	}

	byPath := make(map[string]managedDestinationSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		if err := validateManagedPath(snapshot.Path); err != nil {
			return managedPlan{}, fmt.Errorf("managed snapshot: %w", err)
		}
		if _, active := selected[snapshot.Path]; !active {
			return managedPlan{}, fmt.Errorf("managed snapshot destination %q is not active", snapshot.Path)
		}
		if _, exists := byPath[snapshot.Path]; exists {
			return managedPlan{}, fmt.Errorf("managed snapshots contain duplicate destination %q", snapshot.Path)
		}
		byPath[snapshot.Path] = snapshot
	}

	plan := managedPlan{Files: make([]managedPlanFile, 0, len(ordered))}
	for _, selection := range ordered {
		snapshot, ok := byPath[selection.Entry.Path]
		if !ok {
			return managedPlan{}, fmt.Errorf("managed snapshot for %q is missing", selection.Entry.Path)
		}
		planned, err := planManagedFile(selection, rendered, snapshot)
		if err != nil {
			return managedPlan{}, err
		}
		plan.Files = append(plan.Files, planned)
	}
	if err := validateManagedPlan(plan); err != nil {
		return managedPlan{}, err
	}
	return plan, nil
}

func orderManagedSelections(selections []managedSelection) ([]managedSelection, error) {
	entries := make([]managedRegistryEntry, 0, len(selections))
	seen := make(map[string]struct{}, len(selections))
	for _, selection := range selections {
		if _, exists := seen[selection.Entry.Path]; exists {
			return nil, fmt.Errorf("managed plan contains duplicate destination %q", selection.Entry.Path)
		}
		seen[selection.Entry.Path] = struct{}{}
		entries = append(entries, selection.Entry)
		if !selection.Enabled && selection.Entry.Policy != managedPolicyFragment {
			return nil, fmt.Errorf("managed destination %q has an invalid disabled policy", selection.Entry.Path)
		}
	}
	if err := validateManagedRegistry(entries); err != nil {
		return nil, err
	}

	ordered := slices.Clone(selections)
	sort.Slice(ordered, func(i, j int) bool {
		left, right := managedSelectionRank(ordered[i]), managedSelectionRank(ordered[j])
		if left != right {
			return left < right
		}
		return ordered[i].Entry.Path < ordered[j].Entry.Path
	})
	return ordered, nil
}

func managedSelectionRank(selection managedSelection) int {
	switch selection.Entry.Policy {
	case managedPolicyLoader:
		return 0
	case managedPolicyRoot, managedPolicySharedStarter:
		return 1
	case managedPolicyCore:
		return 2
	case managedPolicyFragment:
		if selection.Enabled {
			return 2
		}
		return 3
	default:
		return 4
	}
}

func planManagedFile(selection managedSelection, rendered managedRenderedFiles, snapshot managedDestinationSnapshot) (managedPlanFile, error) {
	planned := managedPlanFile{Selection: selection, Operation: managedOperationNoop}
	if snapshot.UnsafeParent {
		return managedPlanFile{}, fmt.Errorf("managed destination %q has an unsafe parent", selection.Entry.Path)
	}
	if snapshot.Kind > managedDestinationSpecial {
		return managedPlanFile{}, fmt.Errorf("managed destination %q has an invalid snapshot kind", selection.Entry.Path)
	}

	if selection.Entry.Policy.protected() {
		switch snapshot.Kind {
		case managedDestinationMissing:
			planned.Operation = managedOperationWrite
			planned.Content = bytes.Clone(rendered[selection.Entry.Path])
		case managedDestinationRegular, managedDestinationSymlink:
			planned.Operation = managedOperationPreserve
		case managedDestinationDirectory:
			return managedPlanFile{}, fmt.Errorf("managed destination %q is a directory", selection.Entry.Path)
		case managedDestinationSpecial:
			return managedPlanFile{}, fmt.Errorf("managed destination %q is not a regular file or symlink", selection.Entry.Path)
		}
		return planned, nil
	}

	switch snapshot.Kind {
	case managedDestinationMissing:
		if selection.Enabled {
			planned.Operation = managedOperationWrite
			planned.Content = bytes.Clone(rendered[selection.Entry.Path])
		}
	case managedDestinationRegular:
		if !hasManagedMarker(snapshot.Content, selection.Entry.Path) {
			return managedPlanFile{}, &managedOwnershipError{Path: selection.Entry.Path}
		}
		if !selection.Enabled {
			planned.Operation = managedOperationRemove
		} else if !bytes.Equal(snapshot.Content, rendered[selection.Entry.Path]) {
			planned.Operation = managedOperationWrite
			planned.Content = bytes.Clone(rendered[selection.Entry.Path])
		}
	case managedDestinationSymlink:
		if selection.Enabled {
			planned.Operation = managedOperationWrite
			planned.Content = bytes.Clone(rendered[selection.Entry.Path])
		} else {
			planned.Operation = managedOperationRemove
		}
	case managedDestinationDirectory:
		return managedPlanFile{}, fmt.Errorf("managed destination %q is a directory", selection.Entry.Path)
	case managedDestinationSpecial:
		return managedPlanFile{}, fmt.Errorf("managed destination %q is not a regular file or symlink", selection.Entry.Path)
	}
	return planned, nil
}

func validateManagedPlan(plan managedPlan) error {
	seen := make(map[string]struct{}, len(plan.Files))
	for _, file := range plan.Files {
		path := file.Selection.Entry.Path
		if err := validateManagedPath(path); err != nil {
			return fmt.Errorf("managed plan: %w", err)
		}
		if _, exists := seen[path]; exists {
			return fmt.Errorf("managed plan contains duplicate destination %q", path)
		}
		seen[path] = struct{}{}

		switch file.Operation {
		case managedOperationNoop:
		case managedOperationWrite:
			if !file.Selection.Enabled {
				return fmt.Errorf("managed plan writes disabled destination %q", path)
			}
			if file.Selection.Entry.Policy.marked() && !hasManagedMarker(file.Content, path) {
				return fmt.Errorf("managed plan content for %q has an invalid ownership marker", path)
			}
		case managedOperationRemove:
			if file.Selection.Enabled || file.Selection.Entry.Policy != managedPolicyFragment {
				return fmt.Errorf("managed plan removes active destination %q", path)
			}
		case managedOperationPreserve:
			if !file.Selection.Entry.Policy.protected() {
				return fmt.Errorf("managed plan preserves unprotected destination %q", path)
			}
		default:
			return fmt.Errorf("managed plan destination %q has an invalid operation", path)
		}
	}
	return nil
}
