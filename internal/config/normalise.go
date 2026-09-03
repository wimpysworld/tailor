package config

import "fmt"

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

// NormaliseSecretScanningPrerequisites enables secret scanning when push
// protection or non-provider patterns are enabled. It returns one warning
// per requiring feature, and an empty list when it changed nothing.
func NormaliseSecretScanningPrerequisites(cfg *Config) []string {
	if cfg.Repository == nil ||
		(cfg.Repository.SecretScanning != nil && *cfg.Repository.SecretScanning == "enabled") {
		return nil
	}

	var warnings []string
	for _, feature := range []struct {
		name  string
		value *string
	}{
		{"secret_scanning_push_protection", cfg.Repository.SecretScanningPushProtection},
		{"secret_scanning_non_provider_patterns", cfg.Repository.SecretScanningNonProviderPatterns},
	} {
		if feature.value != nil && *feature.value == "enabled" {
			warnings = append(warnings, fmt.Sprintf("warning: set secret_scanning to enabled because %s requires secret scanning", feature.name))
		}
	}
	if len(warnings) == 0 {
		return nil
	}

	enabled := "enabled"
	cfg.Repository.SecretScanning = &enabled
	return warnings
}
