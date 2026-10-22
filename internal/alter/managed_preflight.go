package alter

import (
	"bytes"
	"errors"
	"fmt"
	"os"
)

func preflightManagedFiles(dir string, selections []managedSelection, rendered managedRenderedFiles) (managedPlan, error) {
	ordered, err := orderManagedSelections(selections)
	if err != nil {
		return managedPlan{}, err
	}

	if err := validateEnabledManagedRecipes(ordered, rendered); err != nil {
		return managedPlan{}, err
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return managedPlan{}, fmt.Errorf("opening managed project root %q: %w", dir, err)
	}
	defer root.Close()

	if err := preflightLegacyGoRecipes(root, ordered); err != nil {
		return managedPlan{}, err
	}

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

func validateEnabledManagedRecipes(selections []managedSelection, rendered managedRenderedFiles) error {
	for _, selection := range selections {
		if !selection.Enabled {
			continue
		}
		_, exists := rendered[selection.Entry.Path]
		for _, operation := range managedOperations() {
			name := operation.body(selection.Entry)
			if name != "" {
				body, err := operation.contribution(string(name))
				if err != nil {
					return err
				}
				indented := indentManagedQualityBody(body)
				if !exists || !bytes.Contains(rendered["just/tailor.just"], indented) {
					return fmt.Errorf("enabled managed %s contribution %q is missing from core", operation, name)
				}
				continue
			}
		}
	}
	return nil
}

func preflightLegacyGoRecipes(root *os.Root, selections []managedSelection) error {
	for _, selection := range selections {
		if selection.Entry.Path == "just/go.just" {
			return nil
		}
	}
	const destination = "just/go.just"
	if err := checkParents(root, destination, "legacy Go fragment parent"); err != nil {
		return err
	}
	info, err := root.Lstat(destination)
	if errors.Is(err, os.ErrNotExist) || err == nil && !info.Mode().IsRegular() {
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking retained Go fragment %q: %w", destination, err)
	}
	content, err := root.ReadFile(destination)
	if err != nil {
		return fmt.Errorf("reading retained Go fragment %q: %w", destination, err)
	}
	marked := hasManagedMarker(content, destination)
	for _, recipe := range []string{"build", "test", "lint"} {
		if !marked && hasManagedRecipe(content, recipe) {
			return fmt.Errorf("retained fragment %q defines the core %s recipe; remove or rename that recipe before adopting the managed loader", destination, recipe)
		}
	}
	if marked {
		for _, recipe := range []string{"build", "test", "lint", "build-go", "test-go", "lint-go", "_lint-go", "check-go", "vet-go", "fmt-go", "fmt-check-go", "tidy-go", "tidy-check-go", "coverage-go", "complexity-go"} {
			if hasManagedRecipe(content, recipe) {
				return fmt.Errorf("retained managed fragment %q still defines the legacy %s recipe; set languages.go to true to migrate it, or remove or rename that recipe before retrying", destination, recipe)
			}
		}
	}
	return nil
}

func hasManagedRecipe(content []byte, recipe string) bool {
	for line := range bytes.SplitSeq(content, []byte("\n")) {
		line = bytes.TrimSuffix(line, []byte("\r"))
		line = bytes.TrimPrefix(line, []byte("@"))
		rest, found := bytes.CutPrefix(line, []byte(recipe))
		if !found || len(rest) == 0 || rest[0] != ':' && rest[0] != ' ' && rest[0] != '\t' {
			continue
		}
		colon := bytes.IndexByte(rest, ':')
		if colon >= 0 && (colon+1 == len(rest) || rest[colon+1] != '=') {
			return true
		}
	}
	return false
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
