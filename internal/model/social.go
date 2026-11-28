package model

import (
	"strconv"

	"gopkg.in/yaml.v3"
)

// Default values for optional social asset settings.
const (
	DefaultSocialTheme               = "dark"
	DefaultSocialOGOutput            = "pages/og.png"
	DefaultSocialCommunity           = "auto"
	DefaultSocialDemo                = true
	DefaultSocialSponsors            = "auto"
	DefaultSocialContributors        = true
	DefaultSocialFooter              = true
	DefaultSocialStarHistoryMinStars = 25
	DefaultSocialStarHistoryMinDays  = 30
	DefaultSocialLiveBranch          = "social-assets"
	DefaultSocialLiveSchedule        = "0 4 * * *"
)

// Supported enum and option lists for social asset settings.
var (
	SocialThemes            = []string{"dark", "light"}
	SocialCommunities       = []string{"auto", "none", "discord", "telegram", "matrix", "slack", "irc", "forum", "discussions"}
	SocialPackages          = []string{"apt", "rpm", "pkg", "apk", "flatpak", "brew", "scoop", "nix"}
	SocialSponsorsModes     = []string{"auto", "true", "false"}
	SocialOGImageExtensions = []string{".png", ".webp", ".jpg", ".jpeg", ".avif"}
)

// SocialSettings holds the opt-in social asset configuration.
// Pointer fields distinguish absent settings from explicit zero values.
type SocialSettings struct {
	Enabled    *bool                 `yaml:"enabled,omitempty"`
	Theme      *string               `yaml:"theme,omitempty"`
	Accent     *string               `yaml:"accent,omitempty"`
	Background *string               `yaml:"background,omitempty"`
	Platforms  *string               `yaml:"platforms,omitempty"`
	OG         *SocialOGSettings     `yaml:"og,omitempty"`
	Readme     *SocialReadmeSettings `yaml:"readme,omitempty"`
	Live       *SocialLiveSettings   `yaml:"live,omitempty"`

	// Extra captures any YAML keys not mapped to struct fields above.
	// ValidateSocial uses this to reject unrecognised settings.
	Extra map[string]any `yaml:",inline"`
}

// SocialOGSettings controls Open Graph social preview card generation.
type SocialOGSettings struct {
	Output         *string   `yaml:"output,omitempty"`
	IncludePreview *bool     `yaml:"include_preview,omitempty"`
	Image          *string   `yaml:"image,omitempty"`
	Terminal       *[]string `yaml:"terminal,omitempty"`

	Extra map[string]any `yaml:",inline"`
}

// SocialReadmeSettings controls the generated README SVG suite and marker blocks.
type SocialReadmeSettings struct {
	Header         *bool                      `yaml:"header,omitempty"`
	PreferredLinks *[]string                  `yaml:"preferred_links,omitempty"`
	Community      *string                    `yaml:"community,omitempty"`
	Packages       *[]string                  `yaml:"packages,omitempty"`
	Demo           *bool                      `yaml:"demo,omitempty"`
	Stats          *bool                      `yaml:"stats,omitempty"`
	StarHistory    *SocialStarHistorySettings `yaml:"star_history,omitempty"`
	Sponsors       *string                    `yaml:"sponsors,omitempty"`
	Contributors   *bool                      `yaml:"contributors,omitempty"`
	Footer         *bool                      `yaml:"footer,omitempty"`

	Extra map[string]any `yaml:",inline"`
}

// UnmarshalYAML normalises YAML boolean values for sponsors to "true" or "false".
func (r *SocialReadmeSettings) UnmarshalYAML(node *yaml.Node) error {
	type plain SocialReadmeSettings
	var decoded plain
	if err := node.Decode(&decoded); err != nil {
		return err
	}
	*r = SocialReadmeSettings(decoded)
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == "sponsors" && node.Content[i+1].Kind == yaml.ScalarNode && node.Content[i+1].Tag == "!!bool" {
			var b bool
			if err := node.Content[i+1].Decode(&b); err == nil {
				value := strconv.FormatBool(b)
				r.Sponsors = &value
			}
		}
	}
	return nil
}

// SocialStarHistorySettings controls the live animated star history chart.
type SocialStarHistorySettings struct {
	Enabled  *bool `yaml:"enabled,omitempty"`
	MinStars *int  `yaml:"min_stars,omitempty"`
	MinDays  *int  `yaml:"min_days,omitempty"`

	Extra map[string]any `yaml:",inline"`
}

// SocialLiveSettings controls the scheduled workflow that updates live SVGs.
type SocialLiveSettings struct {
	Branch   *string `yaml:"branch,omitempty"`
	Schedule *string `yaml:"schedule,omitempty"`

	Extra map[string]any `yaml:",inline"`
}

// Sorted YAML key names for each social nesting level. Validation reports them
// when it rejects an unrecognised key.
var (
	SocialSettingNames            = yamlKeys[SocialSettings]()
	SocialOGSettingNames          = yamlKeys[SocialOGSettings]()
	SocialReadmeSettingNames      = yamlKeys[SocialReadmeSettings]()
	SocialStarHistorySettingNames = yamlKeys[SocialStarHistorySettings]()
	SocialLiveSettingNames        = yamlKeys[SocialLiveSettings]()
)
