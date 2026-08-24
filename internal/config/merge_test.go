package config

import (
	"reflect"
	"testing"

	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/swatch"
	"github.com/wimpysworld/tailor/internal/testutil"
)

// allNonConfigSwatches returns every registered swatch except .tailor.yml.
func allNonConfigSwatches() []swatch.Swatch {
	var out []swatch.Swatch
	for _, s := range swatch.All() {
		if s.Path != ConfigSwatchPath {
			out = append(out, s)
		}
	}
	return out
}

func TestMergeAllPresent(t *testing.T) {
	var entries []SwatchEntry
	for _, s := range allNonConfigSwatches() {
		entries = append(entries, SwatchEntry{
			Path:       s.Path,
			Alteration: s.DefaultAlteration,
		})
	}
	cfg := &Config{Swatches: entries}
	origLen := len(cfg.Swatches)

	added := MergeDefaultSwatches(cfg)

	if len(added) != 0 {
		t.Fatalf("expected no additions, got %d", len(added))
	}
	if len(cfg.Swatches) != origLen {
		t.Fatalf("swatches length changed from %d to %d", origLen, len(cfg.Swatches))
	}
}

func TestMergeSubset(t *testing.T) {
	cfg := &Config{
		Swatches: []SwatchEntry{
			{Path: ".gitignore", Alteration: swatch.FirstFit},
			{Path: "SECURITY.md", Alteration: swatch.Always},
		},
	}

	added := MergeDefaultSwatches(cfg)

	expected := allNonConfigSwatches()
	wantAdded := len(expected) - 2 // two already present
	if len(added) != wantAdded {
		t.Fatalf("expected %d additions, got %d", wantAdded, len(added))
	}
	if len(cfg.Swatches) != len(expected) {
		t.Fatalf("expected %d total swatches, got %d", len(expected), len(cfg.Swatches))
	}

	// Added entries use their registry alteration modes.
	addedByPath := make(map[string]SwatchEntry, len(added))
	for _, e := range added {
		addedByPath[e.Path] = e
	}
	for _, s := range expected {
		if s.Path == ".gitignore" || s.Path == "SECURITY.md" {
			continue
		}
		e, ok := addedByPath[s.Path]
		if !ok {
			t.Errorf("missing added entry for path %q", s.Path)
			continue
		}
		if e.Alteration != s.DefaultAlteration {
			t.Errorf("path %q: alteration = %q, want %q", s.Path, e.Alteration, s.DefaultAlteration)
		}
	}
}

func TestMergeNeverNotDuplicated(t *testing.T) {
	cfg := &Config{
		Swatches: []SwatchEntry{
			{Path: ".gitignore", Alteration: swatch.Never},
		},
	}

	added := MergeDefaultSwatches(cfg)

	// .gitignore already present (with never), should not be duplicated.
	count := 0
	for _, e := range cfg.Swatches {
		if e.Path == ".gitignore" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf(".gitignore appears %d times, want 1", count)
	}

	// Should not be in the added slice either.
	for _, e := range added {
		if e.Path == ".gitignore" {
			t.Fatal(".gitignore should not appear in added slice")
		}
	}
}

func TestMergeEmptyConfig(t *testing.T) {
	cfg := &Config{}

	added := MergeDefaultSwatches(cfg)

	expected := allNonConfigSwatches()
	if len(added) != len(expected) {
		t.Fatalf("expected %d additions, got %d", len(expected), len(added))
	}
	if len(cfg.Swatches) != len(expected) {
		t.Fatalf("expected %d total swatches, got %d", len(expected), len(cfg.Swatches))
	}

	// The config swatch is not merged into cfg.Swatches.
	for _, e := range cfg.Swatches {
		if e.Path == ConfigSwatchPath {
			t.Fatal("config swatch should not be added by merge")
		}
	}
}

func TestMergeDefaultsChangedByEachSection(t *testing.T) {
	tests := []struct {
		name  string
		alter func(*Config)
	}{
		{
			name: "swatches",
			alter: func(cfg *Config) {
				cfg.Swatches = cfg.Swatches[1:]
			},
		},
		{
			name: "repository",
			alter: func(cfg *Config) {
				cfg.Repository.HasIssues = nil
			},
		},
		{
			name: "actions",
			alter: func(cfg *Config) {
				cfg.Actions.Enabled = nil
			},
		},
		{
			name: "labels",
			alter: func(cfg *Config) {
				cfg.Labels = nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultConfig(t)
			tt.alter(cfg)

			changed, err := MergeDefaults(cfg)
			if err != nil {
				t.Fatalf("MergeDefaults() error = %v", err)
			}
			if !changed {
				t.Fatal("MergeDefaults() changed = false, want true")
			}
			changed, err = MergeDefaults(cfg)
			if err != nil {
				t.Fatalf("second MergeDefaults() error = %v", err)
			}
			if changed {
				t.Fatal("second MergeDefaults() changed = true, want false")
			}
		})
	}
}

// defaultRepoDefaults returns the default RepositorySettings from the embedded
// config, for comparison in merge tests.
func defaultRepoDefaults(t *testing.T) *model.RepositorySettings {
	t.Helper()
	defaults, err := DefaultConfig("_")
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	if defaults.Repository == nil {
		t.Fatal("DefaultConfig returned nil Repository")
	}
	return defaults.Repository
}

func defaultConfig(t *testing.T) *Config {
	t.Helper()
	defaults, err := DefaultConfig("_")
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	return defaults
}

func mergeRepoDefaultsForTest(t *testing.T, cfg *Config) bool {
	t.Helper()
	return mergeRepoSettingsFrom(cfg, defaultConfig(t))
}

func mergeLabelsDefaultsForTest(t *testing.T, cfg *Config) bool {
	t.Helper()
	return mergeLabelsFrom(cfg, defaultConfig(t))
}

// countMergeableFields returns the number of pointer fields in the default
// RepositorySettings that are non-nil and not in repoSettingsSkipFields.
func countMergeableFields(t *testing.T) int {
	t.Helper()
	def := defaultRepoDefaults(t)
	count := 0
	for _, field := range model.RepositorySettingFields(def) {
		if _, skip := repoSettingsSkipFields[field.YAMLKey]; skip {
			continue
		}
		if field.Set {
			count++
		}
	}
	return count
}

func TestMergeRepoSettingsNilRepository(t *testing.T) {
	cfg := &Config{}

	changed := mergeRepoDefaultsForTest(t, cfg)

	if !changed {
		t.Fatal("expected changed=true for nil Repository")
	}
	if cfg.Repository == nil {
		t.Fatal("Repository should be allocated after merge")
	}

	def := defaultRepoDefaults(t)
	cv := reflect.ValueOf(cfg.Repository).Elem()

	merged := 0
	for _, field := range model.RepositorySettingFields(def) {
		if _, skip := repoSettingsSkipFields[field.YAMLKey]; skip {
			continue
		}
		if !field.Set {
			continue
		}
		cfv := cv.Field(field.Index)
		if cfv.IsNil() {
			t.Errorf("field %s should be set from defaults", field.YAMLKey)
			continue
		}
		if !reflect.DeepEqual(cfv.Elem().Interface(), field.Value.Elem().Interface()) {
			t.Errorf("field %s: got %v, want %v", field.YAMLKey, cfv.Elem().Interface(), field.Value.Elem().Interface())
		}
		merged++
	}
	want := countMergeableFields(t)
	if merged != want {
		t.Errorf("merged %d fields, want %d", merged, want)
	}
}

func TestMergeRepoSettingsPartialRepository(t *testing.T) {
	customWiki := true
	customCanApprove := true
	customTitle := "CUSTOM_TITLE"
	cfg := &Config{
		Repository: &model.RepositorySettings{
			HasWiki:                      &customWiki,
			CanApprovePullRequestReviews: &customCanApprove,
			SquashMergeCommitTitle:       &customTitle,
		},
	}

	changed := mergeRepoDefaultsForTest(t, cfg)

	if !changed {
		t.Fatal("expected changed=true for partial Repository")
	}

	// Existing values must be preserved.
	if *cfg.Repository.HasWiki != customWiki {
		t.Errorf("HasWiki changed: got %v, want %v", *cfg.Repository.HasWiki, customWiki)
	}
	if *cfg.Repository.CanApprovePullRequestReviews != customCanApprove {
		t.Errorf("CanApprovePullRequestReviews changed: got %v, want %v", *cfg.Repository.CanApprovePullRequestReviews, customCanApprove)
	}
	if *cfg.Repository.SquashMergeCommitTitle != customTitle {
		t.Errorf("SquashMergeCommitTitle changed: got %q, want %q", *cfg.Repository.SquashMergeCommitTitle, customTitle)
	}

	// Nil fields receive values from defaults.
	def := defaultRepoDefaults(t)
	cv := reflect.ValueOf(cfg.Repository).Elem()

	for _, field := range model.RepositorySettingFields(def) {
		if _, skip := repoSettingsSkipFields[field.YAMLKey]; skip {
			continue
		}
		if !field.Set {
			continue
		}
		cfv := cv.Field(field.Index)
		if cfv.IsNil() {
			t.Errorf("field %s should be set from defaults", field.YAMLKey)
		}
	}
}

func TestMergeRepoSettingsAddsSecurityDefaults(t *testing.T) {
	cfg := &Config{Repository: &model.RepositorySettings{HasWiki: new(false)}}

	if !mergeRepoDefaultsForTest(t, cfg) {
		t.Fatal("expected changed=true for missing security settings")
	}

	testutil.AssertBoolPtr(t, cfg.Repository.PrivateVulnerabilityReportEnabled, false, true, "private_vulnerability_reporting_enabled")
	testutil.AssertBoolPtr(t, cfg.Repository.VulnerabilityAlertsEnabled, false, true, "vulnerability_alerts_enabled")
	testutil.AssertBoolPtr(t, cfg.Repository.AutomatedSecurityFixesEnabled, false, true, "automated_security_fixes_enabled")
}

func TestMergeRepoSettingsPreservesExplicitFalseSecuritySettings(t *testing.T) {
	cfg := &Config{Repository: &model.RepositorySettings{
		PrivateVulnerabilityReportEnabled: new(false),
		VulnerabilityAlertsEnabled:        new(false),
		AutomatedSecurityFixesEnabled:     new(false),
	}}

	if !mergeRepoDefaultsForTest(t, cfg) {
		t.Fatal("expected changed=true for other missing repository settings")
	}

	testutil.AssertBoolPtr(t, cfg.Repository.PrivateVulnerabilityReportEnabled, false, false, "private_vulnerability_reporting_enabled")
	testutil.AssertBoolPtr(t, cfg.Repository.VulnerabilityAlertsEnabled, false, false, "vulnerability_alerts_enabled")
	testutil.AssertBoolPtr(t, cfg.Repository.AutomatedSecurityFixesEnabled, false, false, "automated_security_fixes_enabled")
}

func TestMergeRepoSettingsFullRepository(t *testing.T) {
	def := defaultRepoDefaults(t)

	// Deep-copy default into a new RepositorySettings so every field is set.
	full := &model.RepositorySettings{}
	dv := reflect.ValueOf(def).Elem()
	fv := reflect.ValueOf(full).Elem()
	dt := dv.Type()
	for i := range dt.NumField() {
		f := dt.Field(i)
		if f.Tag.Get("yaml") == "" || f.Tag.Get("yaml") == ",inline" {
			continue
		}
		dfv := dv.Field(i)
		if dfv.Kind() != reflect.Pointer || dfv.IsNil() {
			continue
		}
		newVal := reflect.New(dfv.Elem().Type())
		newVal.Elem().Set(dfv.Elem())
		fv.Field(i).Set(newVal)
	}

	cfg := &Config{Repository: full}

	changed := mergeRepoDefaultsForTest(t, cfg)

	if changed {
		t.Fatal("expected changed=false for full Repository")
	}
}

// defaultLabelDefaults returns the default Labels from the embedded config.
func defaultLabelDefaults(t *testing.T) []model.LabelEntry {
	t.Helper()
	defaults, err := DefaultConfig("_")
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	return defaults.Labels
}

func TestMergeLabelsNilLabels(t *testing.T) {
	cfg := &Config{}

	changed := mergeLabelsDefaultsForTest(t, cfg)

	if !changed {
		t.Fatal("expected changed=true for nil Labels")
	}

	def := defaultLabelDefaults(t)
	if len(cfg.Labels) != len(def) {
		t.Fatalf("got %d labels, want %d", len(cfg.Labels), len(def))
	}
	if !reflect.DeepEqual(cfg.Labels, def) {
		t.Error("labels do not match defaults")
	}
}

func TestMergeLabelsEmptySlice(t *testing.T) {
	cfg := &Config{Labels: []model.LabelEntry{}}

	changed := mergeLabelsDefaultsForTest(t, cfg)

	if !changed {
		t.Fatal("expected changed=true for empty Labels slice")
	}

	def := defaultLabelDefaults(t)
	if len(cfg.Labels) != len(def) {
		t.Fatalf("got %d labels, want %d", len(cfg.Labels), len(def))
	}
	if !reflect.DeepEqual(cfg.Labels, def) {
		t.Error("labels do not match defaults")
	}
}

func TestMergeLabelsNonEmpty(t *testing.T) {
	custom := []model.LabelEntry{
		{Name: "custom", Color: "ff0000", Description: "a custom label"},
	}
	cfg := &Config{Labels: custom}

	changed := mergeLabelsDefaultsForTest(t, cfg)

	if changed {
		t.Fatal("expected changed=false for non-empty Labels")
	}
	if len(cfg.Labels) != 1 {
		t.Fatalf("got %d labels, want 1", len(cfg.Labels))
	}
	if cfg.Labels[0].Name != "custom" {
		t.Errorf("label name = %q, want %q", cfg.Labels[0].Name, "custom")
	}
}

func TestMergeLabelsDefaultCount(t *testing.T) {
	def := defaultLabelDefaults(t)
	const wantCount = 12
	if len(def) != wantCount {
		t.Fatalf("embedded default label count = %d, want %d", len(def), wantCount)
	}
}

func TestMergeRepoSettingsSkipsDescriptionHomepageTopics(t *testing.T) {
	cfg := &Config{
		Repository: &model.RepositorySettings{},
	}

	mergeRepoDefaultsForTest(t, cfg)

	if cfg.Repository.Description != nil {
		t.Error("Description should remain nil after merge")
	}
	if cfg.Repository.Homepage != nil {
		t.Error("Homepage should remain nil after merge")
	}
	if cfg.Repository.Topics != nil {
		t.Error("Topics should remain nil after merge")
	}
}
