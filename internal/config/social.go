package config

import (
	"fmt"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/wimpysworld/tailor/internal/model"
)

// SocialDeclared reports whether social.enabled is explicitly set, including false.
func (cfg *Config) SocialDeclared() bool {
	return cfg != nil && cfg.Social != nil && cfg.Social.Enabled != nil
}

// SocialEnabled reports whether social.enabled is explicitly true.
func (cfg *Config) SocialEnabled() bool {
	return cfg.SocialDeclared() && *cfg.Social.Enabled
}

// EffectiveSocialTheme returns the configured social theme or the default ("dark").
func (cfg *Config) EffectiveSocialTheme() string {
	if cfg != nil && cfg.Social != nil && cfg.Social.Theme != nil && *cfg.Social.Theme != "" {
		return *cfg.Social.Theme
	}
	return model.DefaultSocialTheme
}

// EffectiveSocialAccent returns the configured social accent hex colour, or empty when unset.
func (cfg *Config) EffectiveSocialAccent() string {
	if cfg != nil && cfg.Social != nil && cfg.Social.Accent != nil {
		return *cfg.Social.Accent
	}
	return ""
}

// EffectiveSocialBackground returns the configured social background hex colour, or empty when unset.
func (cfg *Config) EffectiveSocialBackground() string {
	if cfg != nil && cfg.Social != nil && cfg.Social.Background != nil {
		return *cfg.Social.Background
	}
	return ""
}

// EffectiveSocialPlatforms returns the configured social platforms footer line, or empty when unset.
func (cfg *Config) EffectiveSocialPlatforms() string {
	if cfg != nil && cfg.Social != nil && cfg.Social.Platforms != nil {
		return *cfg.Social.Platforms
	}
	return ""
}

// EffectiveSocialOGOutput returns the configured Open Graph output path or the default ("pages/og.png").
func (cfg *Config) EffectiveSocialOGOutput() string {
	if cfg != nil && cfg.Social != nil && cfg.Social.OG != nil && cfg.Social.OG.Output != nil && *cfg.Social.OG.Output != "" {
		return *cfg.Social.OG.Output
	}
	return model.DefaultSocialOGOutput
}

// EffectiveSocialOGIncludePreview reports whether the Open Graph card includes a preview image or terminal block.
func (cfg *Config) EffectiveSocialOGIncludePreview() bool {
	if cfg != nil && cfg.Social != nil && cfg.Social.OG != nil && cfg.Social.OG.IncludePreview != nil {
		return *cfg.Social.OG.IncludePreview
	}
	return false
}

// EffectiveSocialOGImage returns the configured Open Graph preview image path, or empty when unset.
func (cfg *Config) EffectiveSocialOGImage() string {
	if cfg != nil && cfg.Social != nil && cfg.Social.OG != nil && cfg.Social.OG.Image != nil {
		return *cfg.Social.OG.Image
	}
	return ""
}

// EffectiveSocialOGTerminal returns a copy of the configured Open Graph terminal lines, or nil when unset.
func (cfg *Config) EffectiveSocialOGTerminal() []string {
	if cfg != nil && cfg.Social != nil && cfg.Social.OG != nil && cfg.Social.OG.Terminal != nil {
		return slices.Clone(*cfg.Social.OG.Terminal)
	}
	return nil
}

// EffectiveSocialReadmeHeader reports whether the animated README hero banner is enabled (default true).
func (cfg *Config) EffectiveSocialReadmeHeader() bool {
	if cfg != nil && cfg.Social != nil && cfg.Social.Readme != nil && cfg.Social.Readme.Header != nil {
		return *cfg.Social.Readme.Header
	}
	return true
}

// EffectiveSocialPreferredLinks returns a copy of the configured README preferred link keys, or nil when unset.
func (cfg *Config) EffectiveSocialPreferredLinks() []string {
	if cfg != nil && cfg.Social != nil && cfg.Social.Readme != nil && cfg.Social.Readme.PreferredLinks != nil {
		return slices.Clone(*cfg.Social.Readme.PreferredLinks)
	}
	return nil
}

// EffectiveSocialCommunity returns the configured README community mode or the default ("auto").
func (cfg *Config) EffectiveSocialCommunity() string {
	if cfg != nil && cfg.Social != nil && cfg.Social.Readme != nil && cfg.Social.Readme.Community != nil && *cfg.Social.Readme.Community != "" {
		return *cfg.Social.Readme.Community
	}
	return model.DefaultSocialCommunity
}

// EffectiveSocialPackages returns a copy of the configured README Haberdasher package hosts, or nil when unset.
func (cfg *Config) EffectiveSocialPackages() []string {
	if cfg != nil && cfg.Social != nil && cfg.Social.Readme != nil && cfg.Social.Readme.Packages != nil {
		return slices.Clone(*cfg.Social.Readme.Packages)
	}
	return nil
}

// EffectiveSocialReadmeStats reports whether the live README stats strip is enabled (default true).
func (cfg *Config) EffectiveSocialReadmeStats() bool {
	if cfg != nil && cfg.Social != nil && cfg.Social.Readme != nil && cfg.Social.Readme.Stats != nil {
		return *cfg.Social.Readme.Stats
	}
	return true
}

// EffectiveSocialStarHistoryEnabled reports whether the live animated star history chart is enabled (default true).
func (cfg *Config) EffectiveSocialStarHistoryEnabled() bool {
	if cfg != nil && cfg.Social != nil && cfg.Social.Readme != nil && cfg.Social.Readme.StarHistory != nil && cfg.Social.Readme.StarHistory.Enabled != nil {
		return *cfg.Social.Readme.StarHistory.Enabled
	}
	return true
}

// EffectiveSocialStarHistoryMinStars returns the configured star threshold or the default (25).
func (cfg *Config) EffectiveSocialStarHistoryMinStars() int {
	if cfg != nil && cfg.Social != nil && cfg.Social.Readme != nil && cfg.Social.Readme.StarHistory != nil && cfg.Social.Readme.StarHistory.MinStars != nil {
		return *cfg.Social.Readme.StarHistory.MinStars
	}
	return model.DefaultSocialStarHistoryMinStars
}

// EffectiveSocialStarHistoryMinDays returns the configured age threshold in days or the default (30).
func (cfg *Config) EffectiveSocialStarHistoryMinDays() int {
	if cfg != nil && cfg.Social != nil && cfg.Social.Readme != nil && cfg.Social.Readme.StarHistory != nil && cfg.Social.Readme.StarHistory.MinDays != nil {
		return *cfg.Social.Readme.StarHistory.MinDays
	}
	return model.DefaultSocialStarHistoryMinDays
}

// EffectiveSocialSponsors returns the configured README sponsors mode or the default ("auto").
func (cfg *Config) EffectiveSocialSponsors() string {
	if cfg != nil && cfg.Social != nil && cfg.Social.Readme != nil && cfg.Social.Readme.Sponsors != nil && *cfg.Social.Readme.Sponsors != "" {
		return *cfg.Social.Readme.Sponsors
	}
	return model.DefaultSocialSponsors
}

// EffectiveSocialLiveBranch returns the configured live assets branch or the default ("social-assets").
func (cfg *Config) EffectiveSocialLiveBranch() string {
	if cfg != nil && cfg.Social != nil && cfg.Social.Live != nil && cfg.Social.Live.Branch != nil && *cfg.Social.Live.Branch != "" {
		return *cfg.Social.Live.Branch
	}
	return model.DefaultSocialLiveBranch
}

// EffectiveSocialLiveSchedule returns the configured live workflow cron schedule or the default ("0 4 * * *").
func (cfg *Config) EffectiveSocialLiveSchedule() string {
	if cfg != nil && cfg.Social != nil && cfg.Social.Live != nil && cfg.Social.Live.Schedule != nil && *cfg.Social.Live.Schedule != "" {
		return *cfg.Social.Live.Schedule
	}
	return model.DefaultSocialLiveSchedule
}

// validateSocialNodes checks YAML node types in the social section before decoding can coerce scalars.
func validateSocialNodes(document *yaml.Node) error {
	sections := make(map[string]yaml.Node)
	if err := document.Decode(&sections); err != nil {
		return err
	}
	node, ok := sections["social"]
	if !ok {
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("social must be a mapping")
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		switch key.Value {
		case "enabled":
			if err := requireYAMLTag("social.enabled", value, "!!bool"); err != nil {
				return err
			}
		case "theme", "accent", "background", "platforms":
			if err := requireYAMLTag("social."+key.Value, value, "!!str"); err != nil {
				return err
			}
		case "og":
			if err := validateSocialOGNodes(value); err != nil {
				return err
			}
		case "readme":
			if err := validateSocialReadmeNodes(value); err != nil {
				return err
			}
		case "live":
			if err := validateSocialLiveNodes(value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateSocialOGNodes(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("social.og must be a mapping")
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		switch key.Value {
		case "output", "image":
			if err := requireYAMLTag("social.og."+key.Value, value, "!!str"); err != nil {
				return err
			}
		case "include_preview":
			if err := requireYAMLTag("social.og.include_preview", value, "!!bool"); err != nil {
				return err
			}
		case "terminal":
			if err := requireYAMLStringSequence("social.og.terminal", value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateSocialReadmeNodes(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("social.readme must be a mapping")
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		switch key.Value {
		case "header", "stats":
			if err := requireYAMLTag("social.readme."+key.Value, value, "!!bool"); err != nil {
				return err
			}
		case "community":
			if err := requireYAMLTag("social.readme.community", value, "!!str"); err != nil {
				return err
			}
		case "preferred_links", "packages":
			if err := requireYAMLStringSequence("social.readme."+key.Value, value); err != nil {
				return err
			}
		case "sponsors":
			if value.Kind != yaml.ScalarNode || (value.Tag != "!!str" && value.Tag != "!!bool") {
				return fmt.Errorf("social.readme.sponsors must be a string or bool")
			}
		case "star_history":
			if err := validateSocialStarHistoryNodes(value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateSocialStarHistoryNodes(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("social.readme.star_history must be a mapping")
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		switch key.Value {
		case "enabled":
			if err := requireYAMLTag("social.readme.star_history.enabled", value, "!!bool"); err != nil {
				return err
			}
		case "min_stars", "min_days":
			if err := requireYAMLTag("social.readme.star_history."+key.Value, value, "!!int"); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateSocialLiveNodes(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("social.live must be a mapping")
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		switch key.Value {
		case "branch", "schedule":
			if err := requireYAMLTag("social.live."+key.Value, value, "!!str"); err != nil {
				return err
			}
		}
	}
	return nil
}

func requireYAMLTag(field string, value *yaml.Node, tag string) error {
	if value.Kind != yaml.ScalarNode || value.Tag != tag {
		label := tag
		switch tag {
		case "!!str":
			label = "string"
		case "!!bool":
			label = "bool"
		case "!!int":
			label = "int"
		}
		return fmt.Errorf("%s must be a %s", field, label)
	}
	return nil
}

func requireYAMLStringSequence(field string, value *yaml.Node) error {
	if value.Kind != yaml.SequenceNode {
		return fmt.Errorf("%s must be a sequence", field)
	}
	for i, item := range value.Content {
		if item.Kind != yaml.ScalarNode || item.Tag != "!!str" {
			return fmt.Errorf("%s[%d] must be a string", field, i)
		}
	}
	return nil
}
