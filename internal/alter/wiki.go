package alter

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"reflect"
	"regexp"
	"strings"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/swatch"
	"gopkg.in/yaml.v3"
)

type wikiRun struct {
	enabled       bool
	enabledViaAPI bool
	content       []byte
	entry         config.SwatchEntry
	skipped       []RepoSettingResult
	nextSteps     string
}

func (p *wikiRun) didEnable() bool { return p != nil && p.enabledViaAPI }

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
	host := target.Host
	if host == "" {
		host = "github.com"
	}
	wikiURL := fmt.Sprintf("https://%s/%s/%s/wiki", host, target.Owner, target.Name)
	remoteURL := fmt.Sprintf("https://%s/%s/%s.wiki.git", host, target.Owner, target.Name)
	_, sourceErr := root.Lstat("wiki")
	hasSource := !errors.Is(sourceErr, os.ErrNotExist)
	blocked := func(err error) (*wikiRun, error) {
		if mode == DryRun {
			p.nextSteps = wikiPreviewGuidance(err, wikiURL, remoteURL, hasSource)
			return p, nil
		}
		return nil, errors.New(wikiReadinessGuidance(err, wikiURL, remoteURL, hasSource))
	}
	p.content, err = swatch.Content(swatch.WikiDestination)
	if err != nil {
		return nil, err
	}
	if _, err := wikiWorkflow(root, p, mode); err != nil {
		return nil, err
	}
	baseline, err := readWikiBaseline(root)
	if err != nil {
		if mode.ShouldWrite() {
			return blocked(err)
		}
		return blocked(err)
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
		HasWiki       *bool  `json:"has_wiki"`
		DefaultBranch string `json:"default_branch"`
	}
	if err := target.Client.Get(fmt.Sprintf("repos/%s/%s", target.Owner, target.Name), &repository); err != nil {
		return blocked(fmt.Errorf("reading wiki repository metadata: %w", err))
	}
	if repository.Private != nil && *repository.Private {
		return skip("not available for private repositories"), nil
	}
	if repository.Private == nil || repository.DefaultBranch == "" || repository.HasWiki == nil {
		return blocked(errors.New("repository metadata is incomplete; check repository access and retry"))
	}
	if !*repository.HasWiki {
		if mode == DryRun {
			return blocked(errors.New("wiki is disabled; tailor alter will enable repository.has_wiki through the API before checking readiness"))
		}
		if err := target.Client.Patch(fmt.Sprintf("repos/%s/%s", target.Owner, target.Name), strings.NewReader(`{"has_wiki":true}`), nil); err != nil {
			return blocked(fmt.Errorf("enabling repository.has_wiki through the API: %w", err))
		}
		fmt.Fprintln(target.stderr(), "set: repository.has_wiki = true")
		p.enabledViaAPI = true
	}
	if err := gh.InspectWiki(dir, remoteURL, baseline); err != nil {
		return blocked(err)
	}
	return p, nil
}

func wikiReadinessGuidance(err error, wikiURL, remoteURL string, hasSource bool) string {
	return "wiki is not ready\n" + strings.TrimSuffix(wikiPreviewGuidance(err, wikiURL, remoteURL, hasSource), "\n")
}

func wikiImportGuidance(remoteURL string, hasSource bool) string {
	guidance := fmt.Sprintf(`   Requires rsync. Run each command from the project root, stopping if a command fails.
   Use an unused path for ../tailor-wiki-import.
     git clone %s ../tailor-wiki-import
`, remoteURL)
	if hasSource {
		guidance += `   Copy missing files without replacing local files:
     rsync -a --ignore-existing --exclude=.git ../tailor-wiki-import/ wiki/
     rsync -naci --exclude=.git ../tailor-wiki-import/ wiki/
   Review the listed differences and merge changes before recording the imported version:
`
	} else {
		guidance += "     rsync -a --exclude=.git ../tailor-wiki-import/ wiki/\n"
	}
	return guidance + "     git -C ../tailor-wiki-import rev-parse HEAD > wiki/.tailor-wiki-base\n"
}

func wikiPreviewGuidance(err error, wikiURL, remoteURL string, hasSource bool) string {
	if strings.HasPrefix(err.Error(), "wiki is disabled") {
		return fmt.Sprintf(`
Next steps:
The wiki is off. Other changes will wait until it is ready.

1. Run: tailor alter

2. If the wiki has no pages, create and save one at:
   %s

3. If Tailor asks you to import the wiki:
%s
4. %s
`, wikiURL, wikiImportGuidance(remoteURL, hasSource), wikiRecheckGuidance)
	}
	const waiting = " Other changes will wait until it is ready.\n\n"
	guidance := "\nNext steps:\n"
	step := 2
	var access *gh.WikiAccessError
	switch {
	case errors.As(err, &access):
		switch {
		case access.State == "wiki has no commits":
			guidance += "The wiki has no pages." + waiting
			guidance += fmt.Sprintf("1. Create and save the first page at:\n   %s\n\n", wikiURL)
			guidance += "2. Import the wiki:\n" + wikiImportGuidance(remoteURL, hasSource) + "\n"
			step = 3
		case strings.Contains(access.State, "distinguish"):
			guidance += "Tailor could not open the wiki." + waiting
			guidance += fmt.Sprintf("1. Open %s\n   If the wiki has no pages, create and save one.\n   If you cannot open the wiki, check your access.\n\n", wikiURL)
		case strings.HasPrefix(access.State, "git could not start"):
			guidance += "Tailor could not start Git." + waiting
			guidance += "1. Check that Git is installed and runs from this terminal.\n\n"
		case strings.HasPrefix(access.State, "wiki access was denied"):
			guidance += "GitHub denied access to the wiki." + waiting
			guidance += "1. Check your repository access.\n\n"
		default:
			guidance += "Tailor could not read the wiki." + waiting
			guidance += "1. Check your network connection and repository access.\n\n"
		}
	case strings.HasPrefix(err.Error(), "reading wiki repository"), strings.HasPrefix(err.Error(), "repository metadata"), strings.HasPrefix(err.Error(), "enabling repository.has_wiki"):
		guidance += "Tailor could not check or enable the wiki." + waiting
		guidance += "1. Check GitHub authentication, repository permissions and network access.\n\n"
	default:
		if strings.HasPrefix(err.Error(), "wiki published source is not in local HEAD history") || strings.HasPrefix(err.Error(), "wiki published source tree is unavailable") {
			guidance += "The project history for the published wiki is missing locally." + waiting
			guidance += "1. Fetch the project history and switch to the current project branch.\n\n"
			guidance += "2. If the history is still unavailable, review and import the wiki again:\n" + wikiImportGuidance(remoteURL, hasSource) + "\n"
			step = 3
			break
		}
		guidance += wikiReviewReason(err) + waiting
		if strings.Contains(err.Error(), "wiki/.tailor-wiki-base is missing") {
			guidance += "1. Import the wiki:\n"
		} else {
			guidance += "1. Review the wiki files, then import the wiki again:\n"
		}
		guidance += wikiImportGuidance(remoteURL, hasSource) + "\n"
	}
	return guidance + fmt.Sprintf("%d. %s\n", step, wikiRecheckGuidance)
}

const wikiRecheckGuidance = "Run tailor baste again. When the wiki checks pass, run tailor alter."

func wikiReviewReason(err error) string {
	switch message := err.Error(); {
	case strings.Contains(message, "wiki/.tailor-wiki-base is missing"):
		return "Tailor needs an imported copy of the wiki in wiki/."
	case strings.HasPrefix(message, "wiki/.tailor-wiki-base must contain"):
		return "wiki/.tailor-wiki-base must contain the full 40-character ID of the imported wiki version."
	case strings.HasPrefix(message, "wiki import is incomplete:"):
		detail, _, _ := strings.Cut(strings.TrimPrefix(message, "wiki import is incomplete: "), ";")
		return "The wiki import is incomplete: " + detail + "."
	case strings.Contains(message, "unsafe file"), strings.Contains(message, "unsupported file"):
		return "The wiki contains an unsafe or unsupported file. Review the wiki files before importing them."
	case strings.Contains(message, "remote changed outside the publisher"), strings.Contains(message, "wiki remote differs from its published source"):
		return "The wiki changed outside Tailor. Review those changes before importing the wiki again."
	default:
		return "Tailor could not check the wiki: " + message + "."
	}
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
	return nil
}

func readWikiBaseline(root *os.Root) (string, error) {
	info, err := root.Lstat(swatch.WikiBaseline)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() > 128 {
		return "", fmt.Errorf("wiki/.tailor-wiki-base must contain one full 40-character commit ID")
	}
	baseline, err := root.ReadFile(swatch.WikiBaseline)
	if err != nil {
		return "", err
	}
	if !wikiCommit.Match(bytes.TrimSpace(baseline)) {
		return "", fmt.Errorf("wiki/.tailor-wiki-base must contain one full 40-character commit ID")
	}
	return strings.TrimSpace(string(baseline)), nil
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
