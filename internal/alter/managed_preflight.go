package alter

import (
	"errors"
	"fmt"
	"os"
)

func preflightManagedFiles(dir string, selections []managedSelection, rendered managedRenderedFiles) (managedPlan, error) {
	ordered, err := orderManagedSelections(selections)
	if err != nil {
		return managedPlan{}, err
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return managedPlan{}, fmt.Errorf("opening managed project root %q: %w", dir, err)
	}
	defer root.Close()

	snapshots := make([]managedDestinationSnapshot, 0, len(ordered))
	for _, selection := range ordered {
		snapshot, err := snapshotManagedDestination(root, selection)
		if err != nil {
			return managedPlan{}, err
		}
		snapshots = append(snapshots, snapshot)
	}

	return planManagedFiles(ordered, rendered, snapshots)
}

func snapshotManagedDestination(root *os.Root, selection managedSelection) (managedDestinationSnapshot, error) {
	destination := selection.Entry.Path
	snapshot := managedDestinationSnapshot{Path: destination}
	if err := checkParents(root, destination, "managed destination parent"); err != nil {
		return managedDestinationSnapshot{}, err
	}

	info, err := root.Lstat(destination)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return snapshot, nil
		}
		return managedDestinationSnapshot{}, fmt.Errorf("checking managed destination %q: %w", destination, err)
	}

	switch {
	case info.Mode()&os.ModeSymlink != 0:
		snapshot.Kind = managedDestinationSymlink
	case info.IsDir():
		snapshot.Kind = managedDestinationDirectory
	case !info.Mode().IsRegular():
		snapshot.Kind = managedDestinationSpecial
	default:
		snapshot.Kind = managedDestinationRegular
		if !selection.Entry.Policy.protected() {
			snapshot.Content, err = root.ReadFile(destination)
			if err != nil {
				return managedDestinationSnapshot{}, fmt.Errorf("reading managed destination %q: %w", destination, err)
			}
		}
	}

	return snapshot, nil
}
