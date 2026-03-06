package swatch_test

import (
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestLookupTriggerHit(t *testing.T) {
	tc, ok := swatch.LookupTrigger(".github/workflows/tailor-automerge.yml")
	if !ok {
		t.Fatal("LookupTrigger() returned false for automerge source")
	}
	if tc.ConfigField != "allow_auto_merge" {
		t.Errorf("ConfigField = %q, want %q", tc.ConfigField, "allow_auto_merge")
	}
	if tc.Value != true {
		t.Errorf("Value = %v, want true", tc.Value)
	}
}

func TestLookupTriggerMiss(t *testing.T) {
	_, ok := swatch.LookupTrigger("nonexistent.yml")
	if ok {
		t.Fatal("LookupTrigger() returned true for unknown source")
	}
}

func boolPtr(b bool) *bool { return &b }

func TestEvaluateTriggerNoCondition(t *testing.T) {
	repo := &config.RepositorySettings{}
	if !swatch.EvaluateTrigger("no-trigger-source.yml", repo) {
		t.Error("EvaluateTrigger() = false for source with no trigger condition, want true")
	}
}

func TestEvaluateTriggerNilRepo(t *testing.T) {
	if swatch.EvaluateTrigger(".github/workflows/tailor-automerge.yml", (*config.RepositorySettings)(nil)) {
		t.Error("EvaluateTrigger() = true for nil repo, want false")
	}
}

func TestEvaluateTriggerNilInterface(t *testing.T) {
	if swatch.EvaluateTrigger(".github/workflows/tailor-automerge.yml", nil) {
		t.Error("EvaluateTrigger() = true for nil interface, want false")
	}
}

func TestEvaluateTriggerFieldTrue(t *testing.T) {
	repo := &config.RepositorySettings{AllowAutoMerge: boolPtr(true)}
	if !swatch.EvaluateTrigger(".github/workflows/tailor-automerge.yml", repo) {
		t.Error("EvaluateTrigger() = false when allow_auto_merge is true, want true")
	}
}

func TestEvaluateTriggerFieldFalse(t *testing.T) {
	repo := &config.RepositorySettings{AllowAutoMerge: boolPtr(false)}
	if swatch.EvaluateTrigger(".github/workflows/tailor-automerge.yml", repo) {
		t.Error("EvaluateTrigger() = true when allow_auto_merge is false, want false")
	}
}

func TestEvaluateTriggerFieldNil(t *testing.T) {
	repo := &config.RepositorySettings{}
	if swatch.EvaluateTrigger(".github/workflows/tailor-automerge.yml", repo) {
		t.Error("EvaluateTrigger() = true when allow_auto_merge is nil, want false")
	}
}

func TestEvaluateTriggerUnknownSource(t *testing.T) {
	repo := &config.RepositorySettings{}
	if !swatch.EvaluateTrigger("unknown-file.yml", repo) {
		t.Error("EvaluateTrigger() = false for unknown source, want true")
	}
}
