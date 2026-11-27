package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/output"
	"github.com/wimpysworld/tailor/internal/social"
)

// SocialCmd generates themed social SVG assets and optionally updates README markers.
type SocialCmd struct {
	Static       bool   `help:"Generate static social SVGs (default when --live is not set)."`
	Live         bool   `help:"Generate live social SVGs using GitHub and community APIs."`
	OutputDir    string `help:"Override the SVG output directory." name:"output-dir"`
	UpdateReadme bool   `help:"Update existing tailor:social marker blocks in README.md." name:"update-readme"`
	AssetsSHA    string `help:"Commit SHA of the live assets branch for commit-pinned URLs in README.md." name:"assets-sha"`
	DryRun       bool   `help:"Preview planned files without writing." name:"dry-run"`

	dir     string
	fetcher social.LiveFetcher
	stdout  io.Writer
	stderr  io.Writer
	output  *output.Policy
}

// Run executes the social subcommand.
func (s *SocialCmd) Run() (runErr error) {
	policy := s.output
	if policy == nil {
		policy = output.New(s.stdout, s.stderr, output.Auto)
	}
	progress := policy.StartProgress()
	defer progress.Stop()
	defer func() {
		if runErr != nil {
			progress.Observe(output.StageEvent{ID: "social", Label: "Generating social assets", Phase: "error", Err: runErr})
		}
	}()
	progress.Observe(output.StageEvent{ID: "social", Label: "Generating social assets", Phase: "start"})

	dir := s.dir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
	}

	repo, hasRepo, err := gh.RepoContextAt(dir)
	if err != nil {
		return err
	}

	fetcher := s.fetcher
	if s.Live && fetcher == nil {
		if err := gh.CheckAuth(repo.Host); err != nil {
			return err
		}
		if !hasRepo {
			return fmt.Errorf("tailor social --live requires a GitHub git remote to identify the repository")
		}
		fetcher, err = social.NewGitHubLiveFetcher(gh.ResolveHost(repo.Host))
		if err != nil {
			return err
		}
	}

	res, err := social.Generate(context.Background(), social.GenerateOptions{
		Dir:          dir,
		Static:       s.Static,
		Live:         s.Live,
		DryRun:       s.DryRun,
		OutputDir:    s.OutputDir,
		UpdateReadme: s.UpdateReadme,
		AssetsSHA:    s.AssetsSHA,
		Owner:        repo.Owner,
		Repo:         repo.Name,
		Fetcher:      fetcher,
	})
	if err != nil {
		return err
	}

	progress.Observe(output.StageEvent{ID: "social", Label: "Social assets generated", Phase: "complete"})
	progress.Stop()

	contextLabel := projectName(dir)
	if hasRepo {
		contextLabel = repo.Owner + "/" + repo.Name
	} else if abs, absErr := filepath.Abs(dir); absErr == nil {
		contextLabel = filepath.Base(abs)
	}

	doc, plain := formatSocialReport(contextLabel, res)
	policy.Print(doc, plain)
	return nil
}

func formatSocialReport(contextLabel string, res social.GenerateResult) (output.Document, string) {
	doc := output.Document{
		Command: "social",
		Context: contextLabel,
		Items:   make([]output.Item, 0, len(res.Assets)),
	}
	var plain strings.Builder
	for _, asset := range res.Assets {
		switch asset.Status {
		case social.StatusCreated:
			if res.DryRun {
				fmt.Fprintf(&plain, "would create %s\n", asset.Path)
				doc.Items = append(doc.Items, output.Item{
					Domain:     "Social",
					Outcome:    output.Alteration,
					Action:     "create",
					Name:       asset.Path,
					Provenance: asset.Path,
				})
			} else {
				fmt.Fprintf(&plain, "created %s\n", asset.Path)
				doc.Items = append(doc.Items, output.Item{
					Domain:     "Social",
					Outcome:    output.Created,
					Action:     "create",
					Name:       asset.Path,
					Provenance: asset.Path,
				})
			}
		case social.StatusUpdated:
			if res.DryRun {
				fmt.Fprintf(&plain, "would update %s\n", asset.Path)
				doc.Items = append(doc.Items, output.Item{
					Domain:     "Social",
					Outcome:    output.Alteration,
					Action:     "update",
					Name:       asset.Path,
					Provenance: asset.Path,
				})
			} else {
				fmt.Fprintf(&plain, "updated %s\n", asset.Path)
				doc.Items = append(doc.Items, output.Item{
					Domain:     "Social",
					Outcome:    output.Applied,
					Action:     "update",
					Name:       asset.Path,
					Provenance: asset.Path,
				})
			}
		case social.StatusUnchanged:
			fmt.Fprintf(&plain, "unchanged %s\n", asset.Path)
			doc.Items = append(doc.Items, output.Item{
				Domain:     "Social",
				Outcome:    output.Unchanged,
				Action:     "unchanged",
				Name:       asset.Path,
				Provenance: asset.Path,
			})
		case social.StatusSkipped:
			if asset.Reason != "" {
				fmt.Fprintf(&plain, "skipped %s (%s)\n", asset.Path, asset.Reason)
			} else {
				fmt.Fprintf(&plain, "skipped %s\n", asset.Path)
			}
			doc.Items = append(doc.Items, output.Item{
				Domain:     "Social",
				Outcome:    output.Excluded,
				Action:     "skipped",
				Name:       asset.Path,
				Provenance: asset.Path,
				Reason:     asset.Reason,
			})
		}
	}
	doc.Summary = output.Count(doc.Items)
	return doc, plain.String()
}
