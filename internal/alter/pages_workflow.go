package alter

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
	"gopkg.in/yaml.v3"
)

type pagesPreparation struct {
	Generator string
	Path      string
	Branch    string
	Content   []byte
	Entry     config.SwatchEntry
	Result    SwatchResult
}

func preparePagesWorkflow(prepared *pagesPreparation, dir string, mode ApplyMode, branch string) error {
	if prepared == nil {
		return nil
	}
	if prepared.Branch == "" {
		prepared.Branch = branch
	}
	content, err := swatch.PagesContent(prepared.Generator, prepared.Path, prepared.Branch)
	if err != nil {
		return err
	}
	prepared.Content = content
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("opening pages project: %w", err)
	}
	defer root.Close()
	if err := checkParents(root, swatch.PagesDestination, "pages workflow parent"); err != nil {
		return err
	}
	prepared.Result = SwatchResult{Path: swatch.PagesDestination, Category: WouldCopy}
	info, err := root.Lstat(swatch.PagesDestination)
	if errors.Is(err, os.ErrNotExist) {
		if prepared.Entry.Alteration == swatch.Never {
			return fmt.Errorf("pages workflow is missing with alteration mode never")
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking pages workflow: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("pages workflow must be a regular file, not a directory or symlink")
	}
	onDisk, err := root.ReadFile(swatch.PagesDestination)
	if err != nil {
		return fmt.Errorf("reading pages workflow: %w", err)
	}
	if !bytes.HasPrefix(onDisk, []byte(swatch.PagesMarker+"\n")) && !bytes.HasPrefix(onDisk, []byte(swatch.PagesMarker+"\r\n")) {
		return fmt.Errorf("pages workflow ownership conflict: %s lacks %q", swatch.PagesDestination, swatch.PagesMarker)
	}
	protected := prepared.Entry.Alteration == swatch.Never || (prepared.Entry.Alteration == swatch.FirstFit && mode != Recut)
	if protected {
		// YAML comparison preserves harmless formatting and comments without
		// accepting changes to the workflow's execution or permissions.
		var existing, expected any
		if yaml.Unmarshal(onDisk, &existing) != nil || yaml.Unmarshal(content, &expected) != nil {
			return fmt.Errorf("pages workflow is incompatible with the requested generator, path or branch")
		}
		a, aerr := yaml.Marshal(existing)
		b, berr := yaml.Marshal(expected)
		if aerr != nil || berr != nil || !bytes.Equal(a, b) {
			return fmt.Errorf("pages workflow is incompatible with the requested generator, path or branch under mode %s", prepared.Entry.Alteration)
		}
		prepared.Result.Category = Skipped
		prepared.Result.Reason = SkipModeNever
		if prepared.Entry.Alteration == swatch.FirstFit {
			prepared.Result.Reason = SkipFirstFitExists
		}
		return nil
	}
	prepared.Result.Category = WouldOverwrite
	if sha256.Sum256(onDisk) == sha256.Sum256(content) {
		prepared.Result.Category = NoChange
	}
	return nil
}
