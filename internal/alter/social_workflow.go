package alter

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

const (
	socialWorkflowSource      = "social/tailor-social.yml"
	socialWorkflowDestination = ".github/workflows/tailor-social.yml"
	socialWorkflowMarker      = "# Managed by Tailor: social"
)

type socialWorkflowPreparation struct {
	Enabled bool
	Content []byte
	Result  *SwatchResult
}

func hasSocialWorkflowMarker(content []byte) bool {
	return bytes.HasPrefix(content, []byte(socialWorkflowMarker+"\n")) || bytes.HasPrefix(content, []byte(socialWorkflowMarker+"\r\n"))
}

func renderSocialWorkflow(cfg *config.Config) ([]byte, error) {
	if err := config.ValidateSocial(cfg); err != nil {
		return nil, fmt.Errorf("validating social workflow: %w", err)
	}
	schedule := cfg.EffectiveSocialLiveSchedule()
	branch := cfg.EffectiveSocialLiveBranch()
	if strings.ContainsAny(schedule, "'\r\n\x00") || strings.Contains(schedule, "${{") {
		return nil, fmt.Errorf("unsafe social workflow schedule %q", schedule)
	}
	if strings.ContainsAny(branch, "'\r\n\x00") || strings.Contains(branch, "${{") {
		return nil, fmt.Errorf("unsafe social workflow branch %q", branch)
	}

	content, err := swatch.Content(socialWorkflowSource)
	if err != nil {
		return nil, fmt.Errorf("reading social workflow template: %w", err)
	}
	if !hasSocialWorkflowMarker(content) {
		return nil, fmt.Errorf("social workflow template lacks %q", socialWorkflowMarker)
	}
	schedulePlaceholder := []byte("{{SCHEDULE}}")
	branchPlaceholder := []byte("{{BRANCH}}")
	if bytes.Count(content, schedulePlaceholder) != 1 {
		return nil, fmt.Errorf("social workflow template must contain one {{SCHEDULE}} placeholder")
	}
	if bytes.Count(content, branchPlaceholder) != 1 {
		return nil, fmt.Errorf("social workflow template must contain one {{BRANCH}} placeholder")
	}

	rendered := bytes.Replace(content, schedulePlaceholder, []byte(schedule), 1)
	rendered = bytes.Replace(rendered, branchPlaceholder, []byte(branch), 1)
	if !hasSocialWorkflowMarker(rendered) {
		return nil, fmt.Errorf("rendered social workflow lacks %q", socialWorkflowMarker)
	}
	return rendered, nil
}

func prepareSocialWorkflow(cfg *config.Config, dir string) (*socialWorkflowPreparation, error) {
	if !cfg.SocialDeclared() {
		return nil, nil
	}
	prepared := &socialWorkflowPreparation{Enabled: cfg.SocialEnabled()}
	if prepared.Enabled {
		content, err := renderSocialWorkflow(cfg)
		if err != nil {
			return nil, err
		}
		prepared.Content = content
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("opening social project: %w", err)
	}
	defer root.Close()

	if err := checkParents(root, socialWorkflowDestination, "social workflow parent"); err != nil {
		return nil, err
	}

	info, err := root.Lstat(socialWorkflowDestination)
	if errors.Is(err, os.ErrNotExist) {
		if prepared.Enabled {
			prepared.Result = &SwatchResult{Path: socialWorkflowDestination, Category: WouldCopy}
		}
		return prepared, nil
	}
	if err != nil {
		return nil, fmt.Errorf("checking social workflow: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("social workflow must be a regular file, not a directory or symlink")
	}

	onDisk, err := root.ReadFile(socialWorkflowDestination)
	if err != nil {
		return nil, fmt.Errorf("reading social workflow: %w", err)
	}
	owned := hasSocialWorkflowMarker(onDisk)
	if !owned {
		if prepared.Enabled {
			return nil, fmt.Errorf("social workflow ownership conflict: %s lacks %q", socialWorkflowDestination, socialWorkflowMarker)
		}
		prepared.Result = &SwatchResult{
			Path:     socialWorkflowDestination,
			Category: Skipped,
			Reason:   "not managed by Tailor, remove manually to disable social workflow",
		}
		return prepared, nil
	}

	if !prepared.Enabled {
		prepared.Result = &SwatchResult{Path: socialWorkflowDestination, Category: WouldRemove}
		return prepared, nil
	}

	category := WouldOverwrite
	if sha256.Sum256(onDisk) == sha256.Sum256(prepared.Content) {
		category = NoChange
	}
	prepared.Result = &SwatchResult{Path: socialWorkflowDestination, Category: category}
	return prepared, nil
}

func processSocialWorkflowWithParentObserver(cfg *config.Config, dir string, mode ApplyMode, prepared *socialWorkflowPreparation, observer *parentCreationObserver) (*SwatchResult, error) {
	if prepared == nil {
		return nil, nil
	}
	if !mode.ShouldWrite() {
		return prepared.Result, nil
	}

	rechecked, err := prepareSocialWorkflow(cfg, dir)
	if err != nil {
		return nil, err
	}
	if rechecked == nil || rechecked.Result == nil {
		return nil, nil
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("opening social project: %w", err)
	}
	defer root.Close()

	switch rechecked.Result.Category {
	case WouldCopy, WouldOverwrite:
		if err := writeFileWithParentObserver(root, socialWorkflowDestination, rechecked.Content, observer); err != nil {
			return nil, err
		}
	case WouldRemove:
		if err := root.Remove(socialWorkflowDestination); err != nil {
			return nil, fmt.Errorf("removing social workflow: %w", err)
		}
	}
	return rechecked.Result, nil
}
