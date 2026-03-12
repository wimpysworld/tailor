package swatch

import (
	"slices"
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

// Swatch describes a single template file with its path, default alteration
// mode, and category.
type Swatch struct {
	Path              string
	DefaultAlteration AlterationMode
	Category          Category
}

// registry is the ordered list of all built-in swatches.
var registry = []Swatch{
	{Path: ".gitignore", DefaultAlteration: FirstFit, Category: Development},
	{Path: ".envrc", DefaultAlteration: FirstFit, Category: Development},
	{Path: "SECURITY.md", DefaultAlteration: Always, Category: Health},
	{Path: "CODE_OF_CONDUCT.md", DefaultAlteration: Always, Category: Health},
	{Path: "CONTRIBUTING.md", DefaultAlteration: Always, Category: Health},
	{Path: "SUPPORT.md", DefaultAlteration: Always, Category: Health},
	{Path: "flake.nix", DefaultAlteration: FirstFit, Category: Development},
	{Path: "cubic.yaml", DefaultAlteration: FirstFit, Category: Development},
	{Path: "justfile", DefaultAlteration: FirstFit, Category: Development},
	{Path: ".github/FUNDING.yml", DefaultAlteration: FirstFit, Category: Health},
	{Path: ".github/dependabot.yml", DefaultAlteration: FirstFit, Category: Health},
	{Path: ".github/ISSUE_TEMPLATE/bug_report.yml", DefaultAlteration: Always, Category: Health},
	{Path: ".github/ISSUE_TEMPLATE/feature_request.yml", DefaultAlteration: Always, Category: Health},
	{Path: ".github/ISSUE_TEMPLATE/config.yml", DefaultAlteration: FirstFit, Category: Health},
	{Path: ".github/pull_request_template.md", DefaultAlteration: Always, Category: Health},
	{Path: ".github/workflows/tailor.yml", DefaultAlteration: Always, Category: Development},
	{Path: ".github/workflows/tailor-automerge.yml", DefaultAlteration: Triggered, Category: Development},
	{Path: ".tailor.yml", DefaultAlteration: Always, Category: Development},
}

// All returns every registered swatch in definition order.
func All() []Swatch {
	out := make([]Swatch, len(registry))
	copy(out, registry)
	return out
}

// Paths returns the paths of all registered swatches, sorted
// lexicographically.
func Paths() []string {
	names := make([]string, len(registry))
	for i, s := range registry {
		names[i] = s.Path
	}
	slices.Sort(names)
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
