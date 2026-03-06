package swatch

import (
	"fmt"
	"sort"
)

// Category classifies a swatch as either a community health file or a
// development tooling file.
type Category string

const (
	Health      Category = "health"
	Development Category = "development"
)

// AlterationMode controls how a swatch is applied to a project.
type AlterationMode string

const (
	Always    AlterationMode = "always"
	FirstFit  AlterationMode = "first-fit"
	Triggered AlterationMode = "triggered"
	Never     AlterationMode = "never"
)

// LicenseDestination is the destination path for the licence file.
// Licences are not embedded swatches; they are fetched via gh at alter time.
const LicenseDestination = "LICENSE"

// Swatch describes a single template file with its source-to-destination
// mapping, default alteration mode, and category.
type Swatch struct {
	Source            string
	Destination       string
	DefaultAlteration AlterationMode
	Category          Category
}

// registry is the ordered list of all built-in swatches.
var registry = []Swatch{
	{Source: ".gitignore", Destination: ".gitignore", DefaultAlteration: FirstFit, Category: Development},
	{Source: ".envrc", Destination: ".envrc", DefaultAlteration: FirstFit, Category: Development},
	{Source: "SECURITY.md", Destination: "SECURITY.md", DefaultAlteration: Always, Category: Health},
	{Source: "CODE_OF_CONDUCT.md", Destination: "CODE_OF_CONDUCT.md", DefaultAlteration: Always, Category: Health},
	{Source: "CONTRIBUTING.md", Destination: "CONTRIBUTING.md", DefaultAlteration: Always, Category: Health},
	{Source: "SUPPORT.md", Destination: "SUPPORT.md", DefaultAlteration: Always, Category: Health},
	{Source: "flake.nix", Destination: "flake.nix", DefaultAlteration: FirstFit, Category: Development},
	{Source: "justfile", Destination: "justfile", DefaultAlteration: FirstFit, Category: Development},
	{Source: ".github/FUNDING.yml", Destination: ".github/FUNDING.yml", DefaultAlteration: FirstFit, Category: Health},
	{Source: ".github/dependabot.yml", Destination: ".github/dependabot.yml", DefaultAlteration: FirstFit, Category: Health},
	{Source: ".github/ISSUE_TEMPLATE/bug_report.yml", Destination: ".github/ISSUE_TEMPLATE/bug_report.yml", DefaultAlteration: Always, Category: Health},
	{Source: ".github/ISSUE_TEMPLATE/feature_request.yml", Destination: ".github/ISSUE_TEMPLATE/feature_request.yml", DefaultAlteration: Always, Category: Health},
	{Source: ".github/ISSUE_TEMPLATE/config.yml", Destination: ".github/ISSUE_TEMPLATE/config.yml", DefaultAlteration: FirstFit, Category: Health},
	{Source: ".github/pull_request_template.md", Destination: ".github/pull_request_template.md", DefaultAlteration: Always, Category: Health},
	{Source: ".github/workflows/tailor.yml", Destination: ".github/workflows/tailor.yml", DefaultAlteration: Always, Category: Development},
	{Source: ".github/workflows/tailor-automerge.yml", Destination: ".github/workflows/tailor-automerge.yml", DefaultAlteration: Triggered, Category: Development},
	{Source: ".tailor/config.yml", Destination: ".tailor/config.yml", DefaultAlteration: Always, Category: Development},
}

// All returns every registered swatch in definition order.
func All() []Swatch {
	out := make([]Swatch, len(registry))
	copy(out, registry)
	return out
}

// BySource returns the swatch matching the given source path, or an error if
// no such swatch exists.
func BySource(source string) (Swatch, error) {
	for _, s := range registry {
		if s.Source == source {
			return s, nil
		}
	}
	return Swatch{}, fmt.Errorf("unknown swatch source: %q", source)
}

// SourceNames returns the source names of all registered swatches, sorted
// lexicographically.
func SourceNames() []string {
	names := make([]string, len(registry))
	for i, s := range registry {
		names[i] = s.Source
	}
	sort.Strings(names)
	return names
}

// HealthSwatches returns only the swatches categorised as health.
func HealthSwatches() []Swatch {
	var out []Swatch
	for _, s := range registry {
		if s.Category == Health {
			out = append(out, s)
		}
	}
	return out
}
