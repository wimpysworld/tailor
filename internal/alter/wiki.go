package alter

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"reflect"
	"regexp"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
	"gopkg.in/yaml.v3"
)

type wikiRun struct {
	enabled bool
	content []byte
	entry   config.SwatchEntry
	skipped []RepoSettingResult
}

func preflightWiki(cfg *config.Config, dir string, mode ApplyMode, target RepoTarget, declared bool) (*wikiRun, error) {
	if !declared || cfg.Repository == nil || cfg.Repository.HasWiki == nil {
		return nil, nil
	}
	p := &wikiRun{enabled: *cfg.Repository.HasWiki, entry: config.SwatchEntry{Path: swatch.WikiDestination, Alteration: swatch.Always}}
	for _, entry := range cfg.Swatches {
		if entry.Path == swatch.WikiDestination {
			p.entry = entry
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if p.enabled {
		if err := checkWikiSource(root); err != nil {
			return nil, err
		}
	}
	// Check ownership before repository writes, even when metadata is unavailable.
	if _, err := wikiWorkflow(root, p, mode); err != nil {
		return nil, err
	}
	if !p.enabled {
		return p, nil
	}
	skip := func(reason string) *wikiRun {
		p.skipped = []RepoSettingResult{{Section: "wiki", Field: "publishing", Category: WouldSkipSetup, Annotation: reason}}
		return p
	}
	if !target.HasRepo {
		return skip("no repository"), nil
	}
	var repository struct {
		Private       *bool  `json:"private"`
		DefaultBranch string `json:"default_branch"`
	}
	if err := target.Client.Get(fmt.Sprintf("repos/%s/%s", target.Owner, target.Name), &repository); err != nil {
		var httpErr *api.HTTPError
		if errors.As(err, &httpErr) && (httpErr.StatusCode == http.StatusForbidden || httpErr.StatusCode == http.StatusNotFound) {
			return skip("not available"), nil
		}
		return nil, fmt.Errorf("reading wiki repository: %w", err)
	}
	if repository.Private == nil || repository.DefaultBranch == "" {
		return skip("repository metadata unavailable"), nil
	}
	if *repository.Private {
		return skip("not available for private repositories"), nil
	}
	p.content, err = swatch.WikiContent(repository.DefaultBranch)
	if err != nil {
		return nil, err
	}
	_, err = wikiWorkflow(root, p, mode)
	if err != nil {
		return nil, err
	}
	if _, err := root.Lstat(swatch.WikiBaseline); errors.Is(err, os.ErrNotExist) {
		fmt.Fprintln(target.stderr(), "warning: wiki: before the first publish, initialise the GitHub wiki, import its files into wiki/, and record its commit in wiki/.tailor-wiki-base")
	}
	return p, nil
}

var wikiCommit = regexp.MustCompile(`^[0-9a-f]{40}$`)

func checkWikiSource(root *os.Root) error {
	info, err := root.Lstat("wiki")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking wiki source: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("wiki source must be a directory, not a file or symlink")
	}
	if err := fs.WalkDir(root.FS(), "wiki", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if strings.EqualFold(path.Base(name), ".git") || entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("wiki source %q must not contain symlinks or git metadata", name)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("wiki source %q is not a regular file", name)
		}
		if swatch.IsWiki(name) && info.IsDir() {
			return fmt.Errorf("wiki starter destination %q is a directory", name)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("checking wiki source: %w", err)
	}
	info, err = root.Lstat(swatch.WikiBaseline)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > 128 {
		return fmt.Errorf("wiki/.tailor-wiki-base must contain one full 40-character commit ID")
	}
	baseline, err := root.ReadFile(swatch.WikiBaseline)
	if err != nil {
		return err
	}
	if !wikiCommit.Match(bytes.TrimSpace(baseline)) {
		return fmt.Errorf("wiki/.tailor-wiki-base must contain one full 40-character commit ID")
	}
	return nil
}

func wikiWorkflow(root *os.Root, p *wikiRun, mode ApplyMode) (*SwatchResult, error) {
	if err := checkParents(root, swatch.WikiDestination, "wiki workflow parent"); err != nil {
		return nil, err
	}
	result := &SwatchResult{Path: swatch.WikiDestination, Category: WouldCopy}
	info, err := root.Lstat(swatch.WikiDestination)
	if errors.Is(err, os.ErrNotExist) {
		if !p.enabled {
			return nil, nil
		}
		if p.entry.Alteration == swatch.Never {
			return nil, fmt.Errorf("wiki workflow is missing with alteration mode never")
		}
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("wiki workflow must be a regular file, not a directory or symlink")
	}
	data, err := root.ReadFile(swatch.WikiDestination)
	if err != nil {
		return nil, err
	}
	owned := bytes.HasPrefix(data, []byte(swatch.WikiMarker+"\n")) || bytes.HasPrefix(data, []byte(swatch.WikiMarker+"\r\n"))
	if !owned {
		if p.enabled {
			return nil, fmt.Errorf("wiki workflow ownership conflict: %s lacks %q", swatch.WikiDestination, swatch.WikiMarker)
		}
		result.Category, result.Reason = Skipped, "not managed by Tailor, remove manually to stop publishing"
		return result, nil
	}
	if !p.enabled {
		result.Category = WouldRemove
		return result, nil
	}
	if p.content == nil {
		return result, nil
	}
	protected := p.entry.Alteration == swatch.Never || (p.entry.Alteration == swatch.FirstFit && mode != Recut)
	if protected {
		var actual, expected any
		if yaml.Unmarshal(data, &actual) != nil || yaml.Unmarshal(p.content, &expected) != nil || !reflect.DeepEqual(actual, expected) {
			return nil, fmt.Errorf("wiki workflow is incompatible with the default branch under mode %s", p.entry.Alteration)
		}
		result.Category, result.Reason = Skipped, SkipModeNever
		if p.entry.Alteration == swatch.FirstFit {
			result.Reason = SkipFirstFitExists
		}
		return result, nil
	}
	result.Category = WouldOverwrite
	if sha256.Sum256(data) == sha256.Sum256(p.content) {
		result.Category = NoChange
	}
	return result, nil
}

func processWiki(cfg *config.Config, dir string, mode ApplyMode, p *wikiRun) ([]RepoSettingResult, []SwatchResult, error) {
	if p == nil || len(p.skipped) != 0 {
		if p != nil {
			return p.skipped, nil, nil
		}
		return nil, nil, nil
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()
	if p.enabled {
		if err := checkWikiSource(root); err != nil {
			return nil, nil, err
		}
	}
	workflow, err := wikiWorkflow(root, p, mode)
	if err != nil {
		return nil, nil, err
	}
	var results []SwatchResult
	if p.enabled {
		for _, entry := range cfg.Swatches {
			if !swatch.IsWiki(entry.Path) || entry.Path == swatch.WikiDestination {
				continue
			}
			content, err := swatch.Content(entry.Path)
			if err != nil {
				return nil, results, err
			}
			// Starter pages remain user content after their first creation, including recut.
			if entry.Alteration != swatch.Never {
				entry.Alteration = swatch.FirstFit
			}
			pageMode := mode
			if mode == Recut {
				pageMode = Apply
			}
			result, err := processSwatch(root, entry, content, pageMode)
			if err != nil {
				return nil, results, err
			}
			results = append(results, result)
		}
	}
	if workflow != nil {
		if mode.ShouldWrite() {
			switch workflow.Category {
			case WouldCopy, WouldOverwrite:
				if err := writeFile(root, swatch.WikiDestination, p.content); err != nil {
					return nil, results, err
				}
			case WouldRemove:
				if err := root.Remove(swatch.WikiDestination); err != nil {
					return nil, results, fmt.Errorf("removing wiki workflow: %w", err)
				}
			}
		}
		results = append(results, *workflow)
	}
	return nil, results, nil
}
