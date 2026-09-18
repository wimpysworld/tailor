package config

// PagesDeclared reports whether pages.enabled is explicitly set, including false.
func (cfg *Config) PagesDeclared() bool {
	return cfg != nil && cfg.Pages != nil && cfg.Pages.Enabled != nil
}

// PagesEnabled reports whether pages.enabled is explicitly true.
func (cfg *Config) PagesEnabled() bool {
	return cfg.PagesDeclared() && *cfg.Pages.Enabled
}
