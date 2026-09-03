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
		{name: "nil alerts", fixes: new(true), wantChanged: true, wantAlerts: new(true)},
		{name: "false alerts", alerts: new(false), fixes: new(true), wantChanged: true, wantAlerts: new(true)},
		{name: "true alerts", alerts: new(true), fixes: new(true), wantAlerts: new(true)},
		{name: "false fixes", alerts: new(false), fixes: new(false), wantAlerts: new(false)},
		{name: "nil fixes", alerts: new(false), wantAlerts: new(false)},
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
			testutil.AssertPtrEqual(t, cfg.Repository.VulnerabilityAlertsEnabled, tt.wantAlerts, "vulnerability_alerts_enabled")
			if got := NormaliseSecurityPrerequisites(cfg); got {
				t.Fatal("second NormaliseSecurityPrerequisites() call changed the config")
			}
		})
	}
}
