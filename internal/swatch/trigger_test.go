package swatch_test

import (
	"testing"

	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestLookupTriggerMiss(t *testing.T) {
	_, ok := swatch.LookupTrigger("nonexistent.yml")
	if ok {
		t.Fatal("LookupTrigger() returned true for unknown path")
	}
}

func TestEvaluateTriggerNoCondition(t *testing.T) {
	repo := &model.RepositorySettings{}
	if !swatch.EvaluateTrigger("no-trigger-path.yml", repo) {
		t.Error("EvaluateTrigger() = false for path with no trigger condition, want true")
	}
}

func TestEvaluateTriggerUnknownSource(t *testing.T) {
	repo := &model.RepositorySettings{}
	if !swatch.EvaluateTrigger("unknown-file.yml", repo) {
		t.Error("EvaluateTrigger() = false for unknown path, want true")
	}
}
