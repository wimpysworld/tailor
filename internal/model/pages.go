package model

// PagesSettings holds the opt-in GitHub Pages settings. A nil CNAME preserves
// the remote domain, while an empty string clears it.
type PagesSettings struct {
	Enabled   *bool          `yaml:"enabled,omitempty"`
	Generator *string        `yaml:"generator,omitempty"`
	Path      *string        `yaml:"path,omitempty"`
	Branch    *string        `yaml:"branch,omitempty"`
	CNAME     *string        `yaml:"cname,omitempty"`
	Extra     map[string]any `yaml:",inline"`
}

// PagesSettingFields returns Pages settings in struct order.
func PagesSettingFields(settings *PagesSettings) []SettingField {
	return settingFields(settings)
}
