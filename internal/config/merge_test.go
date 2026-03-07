package config

import (
	"reflect"
	"testing"

	"github.com/wimpysworld/tailor/internal/swatch"
)

// allNonConfigSwatches returns every registered swatch except .tailor/config.yml.
func allNonConfigSwatches() []swatch.Swatch {
	var out []swatch.Swatch
	for _, s := range swatch.All() {
		if s.Source != ConfigSwatchSource {
			out = append(out, s)
		}
	}
	return out
}

func TestMergeAllPresent(t *testing.T) {
	var entries []SwatchEntry
	for _, s := range allNonConfigSwatches() {
		entries = append(entries, SwatchEntry{
			Source:      s.Source,
			Destination: s.Destination,
			Alteration:  s.DefaultAlteration,
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
			{Source: ".gitignore", Destination: ".gitignore", Alteration: swatch.FirstFit},
			{Source: "SECURITY.md", Destination: "SECURITY.md", Alteration: swatch.Always},
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

	// Verify each added entry has the correct alteration mode from the registry.
	addedBySource := make(map[string]SwatchEntry, len(added))
	for _, e := range added {
		addedBySource[e.Source] = e
	}
	for _, s := range expected {
		if s.Source == ".gitignore" || s.Source == "SECURITY.md" {
			continue
		}
		e, ok := addedBySource[s.Source]
		if !ok {
			t.Errorf("missing added entry for source %q", s.Source)
			continue
		}
		if e.Alteration != s.DefaultAlteration {
			t.Errorf("source %q: alteration = %q, want %q", s.Source, e.Alteration, s.DefaultAlteration)
		}
		if e.Destination != s.Destination {
			t.Errorf("source %q: destination = %q, want %q", s.Source, e.Destination, s.Destination)
		}
	}
}

func TestMergeNeverNotDuplicated(t *testing.T) {
	cfg := &Config{
		Swatches: []SwatchEntry{
			{Source: ".gitignore", Destination: ".gitignore", Alteration: swatch.Never},
		},
	}

	added := MergeDefaultSwatches(cfg)

	// .gitignore already present (with never), should not be duplicated.
	count := 0
	for _, e := range cfg.Swatches {
		if e.Source == ".gitignore" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf(".gitignore appears %d times, want 1", count)
	}

	// Should not be in the added slice either.
	for _, e := range added {
		if e.Source == ".gitignore" {
			t.Fatal(".gitignore should not appear in added slice")
		}
	}
}

func TestMergeRemappedDestination(t *testing.T) {
	cfg := &Config{
		Swatches: []SwatchEntry{
			{Source: ".gitignore", Destination: "custom/.gitignore", Alteration: swatch.FirstFit},
		},
	}

	added := MergeDefaultSwatches(cfg)

	// Source matches, so it should not be treated as missing.
	for _, e := range added {
		if e.Source == ".gitignore" {
			t.Fatal(".gitignore with remapped destination should not be added again")
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

	// Verify no config.yml entry was added.
	for _, e := range cfg.Swatches {
		if e.Source == ConfigSwatchSource {
			t.Fatal("config.yml swatch should not be added by merge")
		}
	}
}

// defaultRepoDefaults returns the default RepositorySettings from the embedded
// config, for comparison in merge tests.
func defaultRepoDefaults(t *testing.T) *RepositorySettings {
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

// countMergeableFields returns the number of pointer fields in the default
// RepositorySettings that are non-nil and not in repoSettingsSkipFields.
func countMergeableFields(t *testing.T) int {
	t.Helper()
	def := defaultRepoDefaults(t)
	dv := reflect.ValueOf(def).Elem()
	dt := dv.Type()
	count := 0
	for i := range dt.NumField() {
		f := dt.Field(i)
		if _, skip := repoSettingsSkipFields[f.Name]; skip {
			continue
		}
		if f.Tag.Get("yaml") == "" || f.Tag.Get("yaml") == ",inline" {
			continue
		}
		if dv.Field(i).Kind() == reflect.Ptr && !dv.Field(i).IsNil() {
			count++
		}
	}
	return count
}

func TestMergeRepoSettingsNilRepository(t *testing.T) {
	cfg := &Config{}

	changed := MergeDefaultRepoSettings(cfg)

	if !changed {
		t.Fatal("expected changed=true for nil Repository")
	}
	if cfg.Repository == nil {
		t.Fatal("Repository should be allocated after merge")
	}

	def := defaultRepoDefaults(t)
	dv := reflect.ValueOf(def).Elem()
	cv := reflect.ValueOf(cfg.Repository).Elem()
	dt := dv.Type()

	merged := 0
	for i := range dt.NumField() {
		f := dt.Field(i)
		if _, skip := repoSettingsSkipFields[f.Name]; skip {
			continue
		}
		if f.Tag.Get("yaml") == "" || f.Tag.Get("yaml") == ",inline" {
			continue
		}
		dfv := dv.Field(i)
		if dfv.Kind() != reflect.Ptr || dfv.IsNil() {
			continue
		}
		cfv := cv.Field(i)
		if cfv.IsNil() {
			t.Errorf("field %s should be set from defaults", f.Name)
			continue
		}
		if !reflect.DeepEqual(cfv.Elem().Interface(), dfv.Elem().Interface()) {
			t.Errorf("field %s: got %v, want %v", f.Name, cfv.Elem().Interface(), dfv.Elem().Interface())
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
	customTitle := "CUSTOM_TITLE"
	cfg := &Config{
		Repository: &RepositorySettings{
			HasWiki:                &customWiki,
			SquashMergeCommitTitle: &customTitle,
		},
	}

	changed := MergeDefaultRepoSettings(cfg)

	if !changed {
		t.Fatal("expected changed=true for partial Repository")
	}

	// Existing values must be preserved.
	if *cfg.Repository.HasWiki != customWiki {
		t.Errorf("HasWiki changed: got %v, want %v", *cfg.Repository.HasWiki, customWiki)
	}
	if *cfg.Repository.SquashMergeCommitTitle != customTitle {
		t.Errorf("SquashMergeCommitTitle changed: got %q, want %q", *cfg.Repository.SquashMergeCommitTitle, customTitle)
	}

	// Nil fields should now be filled from defaults.
	def := defaultRepoDefaults(t)
	dv := reflect.ValueOf(def).Elem()
	cv := reflect.ValueOf(cfg.Repository).Elem()
	dt := dv.Type()

	for i := range dt.NumField() {
		f := dt.Field(i)
		if _, skip := repoSettingsSkipFields[f.Name]; skip {
			continue
		}
		if f.Tag.Get("yaml") == "" || f.Tag.Get("yaml") == ",inline" {
			continue
		}
		dfv := dv.Field(i)
		if dfv.Kind() != reflect.Ptr || dfv.IsNil() {
			continue
		}
		cfv := cv.Field(i)
		if cfv.IsNil() {
			t.Errorf("field %s should be set from defaults", f.Name)
		}
	}
}

func TestMergeRepoSettingsFullRepository(t *testing.T) {
	def := defaultRepoDefaults(t)

	// Deep-copy default into a new RepositorySettings so every field is set.
	full := &RepositorySettings{}
	dv := reflect.ValueOf(def).Elem()
	fv := reflect.ValueOf(full).Elem()
	dt := dv.Type()
	for i := range dt.NumField() {
		f := dt.Field(i)
		if f.Tag.Get("yaml") == "" || f.Tag.Get("yaml") == ",inline" {
			continue
		}
		dfv := dv.Field(i)
		if dfv.Kind() != reflect.Ptr || dfv.IsNil() {
			continue
		}
		newVal := reflect.New(dfv.Elem().Type())
		newVal.Elem().Set(dfv.Elem())
		fv.Field(i).Set(newVal)
	}

	cfg := &Config{Repository: full}

	changed := MergeDefaultRepoSettings(cfg)

	if changed {
		t.Fatal("expected changed=false for full Repository")
	}
}

// defaultLabelDefaults returns the default Labels from the embedded config.
func defaultLabelDefaults(t *testing.T) []LabelEntry {
	t.Helper()
	defaults, err := DefaultConfig("_")
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	return defaults.Labels
}

func TestMergeLabelsNilLabels(t *testing.T) {
	cfg := &Config{}

	changed := MergeDefaultLabels(cfg)

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
	cfg := &Config{Labels: []LabelEntry{}}

	changed := MergeDefaultLabels(cfg)

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
	custom := []LabelEntry{
		{Name: "custom", Color: "ff0000", Description: "a custom label"},
	}
	cfg := &Config{Labels: custom}

	changed := MergeDefaultLabels(cfg)

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
		Repository: &RepositorySettings{},
	}

	MergeDefaultRepoSettings(cfg)

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
