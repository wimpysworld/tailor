package config

import (
	"testing"

	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestNormaliseSecurityPrerequisites(t *testing.T) {
	tests := []struct {
		name        string
		alerts      *bool
		fixes       *bool
		wantChanged bool
		wantAlerts  *bool
	}{
		{name: "nil alerts", fixes: boolPointer(true), wantChanged: true, wantAlerts: boolPointer(true)},
		{name: "false alerts", alerts: boolPointer(false), fixes: boolPointer(true), wantChanged: true, wantAlerts: boolPointer(true)},
		{name: "true alerts", alerts: boolPointer(true), fixes: boolPointer(true), wantAlerts: boolPointer(true)},
		{name: "false fixes", alerts: boolPointer(false), fixes: boolPointer(false), wantAlerts: boolPointer(false)},
		{name: "nil fixes", alerts: boolPointer(false), wantAlerts: boolPointer(false)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Repository: &model.RepositorySettings{
				VulnerabilityAlertsEnabled:    tt.alerts,
				AutomatedSecurityFixesEnabled: tt.fixes,
			}}

			if got := NormaliseSecurityPrerequisites(cfg); got != tt.wantChanged {
				t.Fatalf("NormaliseSecurityPrerequisites() = %t, want %t", got, tt.wantChanged)
			}
			testutil.AssertBoolPtr(t, cfg.Repository.VulnerabilityAlertsEnabled, tt.wantAlerts == nil, valueOrFalse(tt.wantAlerts), "vulnerability_alerts_enabled")
			if got := NormaliseSecurityPrerequisites(cfg); got {
				t.Fatal("second NormaliseSecurityPrerequisites() call changed the config")
			}
		})
	}
}

func boolPointer(value bool) *bool { return &value }

func valueOrFalse(value *bool) bool {
	if value == nil {
		return false
	}
	return *value
}
