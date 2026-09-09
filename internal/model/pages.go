package model

import "gopkg.in/yaml.v3"

// PagesSettings holds the opt-in GitHub Pages settings. A nil CNAME preserves
// the remote domain, while an empty string clears it.
type PagesSettings struct {
	Enabled    *bool              `yaml:"enabled,omitempty"`
	Generator  *string            `yaml:"generator,omitempty"`
	Path       *string            `yaml:"path,omitempty"`
	Branch     *string            `yaml:"branch,omitempty"`
	CNAME      *string            `yaml:"cname,omitempty"`
	Links      *map[string]string `yaml:"links,omitempty"`
	Extra      map[string]any     `yaml:",inline"`
	linksOrder []string
}

// UnmarshalYAML preserves the declared link order alongside the values.
func (p *PagesSettings) UnmarshalYAML(node *yaml.Node) error {
	type plain PagesSettings
	var decoded plain
	if err := node.Decode(&decoded); err != nil {
		return err
	}
	*p = PagesSettings(decoded)
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == "links" {
			links := node.Content[i+1]
			for j := 0; j < len(links.Content); j += 2 {
				p.linksOrder = append(p.linksOrder, links.Content[j].Value)
			}
		}
	}
	return nil
}

// PagesSettingFields returns Pages settings in struct order.
func PagesSettingFields(settings *PagesSettings) []SettingField {
	return settingFields(settings)
}
