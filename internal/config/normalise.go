package config

// NormaliseSecurityPrerequisites enables vulnerability alerts when automated
// security fixes are enabled. It reports whether it changed the config.
func NormaliseSecurityPrerequisites(cfg *Config) bool {
	if cfg.Repository == nil ||
		cfg.Repository.AutomatedSecurityFixesEnabled == nil ||
		!*cfg.Repository.AutomatedSecurityFixesEnabled ||
		(cfg.Repository.VulnerabilityAlertsEnabled != nil && *cfg.Repository.VulnerabilityAlertsEnabled) {
		return false
	}

	enabled := true
	cfg.Repository.VulnerabilityAlertsEnabled = &enabled
	return true
}
