package alter

import (
	"errors"
	"testing"
)

func TestResult_AddApplied(t *testing.T) {
	var r Result
	r.AddApplied("patch repo settings")
	r.AddApplied("enable vulnerability alerts", "set topics")

	if len(r.Applied) != 3 {
		t.Fatalf("expected 3 applied, got %d", len(r.Applied))
	}
	if r.Applied[0] != "patch repo settings" {
		t.Errorf("Applied[0] = %q, want %q", r.Applied[0], "patch repo settings")
	}
	if r.Applied[2] != "set topics" {
		t.Errorf("Applied[2] = %q, want %q", r.Applied[2], "set topics")
	}
}

func TestResult_AddSkipped(t *testing.T) {
	var r Result
	r.AddSkipped("enable vulnerability alerts", "insufficient role (need: admin)")
	r.AddSkipped("enable automated security fixes", "insufficient scope")

	if len(r.Skipped) != 2 {
		t.Fatalf("expected 2 skipped, got %d", len(r.Skipped))
	}
	if r.Skipped[0].Name != "enable vulnerability alerts" {
		t.Errorf("Skipped[0].Name = %q, want %q", r.Skipped[0].Name, "enable vulnerability alerts")
	}
	if r.Skipped[0].Reason != "insufficient role (need: admin)" {
		t.Errorf("Skipped[0].Reason = %q, want %q", r.Skipped[0].Reason, "insufficient role (need: admin)")
	}
	if r.Skipped[1].Name != "enable automated security fixes" {
		t.Errorf("Skipped[1].Name = %q, want %q", r.Skipped[1].Name, "enable automated security fixes")
	}
}

func TestResult_AddError(t *testing.T) {
	var r Result
	r.AddError(errors.New("network timeout"))
	r.AddError(errors.New("rate limited"))

	if len(r.Errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(r.Errors))
	}
	if r.Errors[0].Error() != "network timeout" {
		t.Errorf("Errors[0] = %q, want %q", r.Errors[0].Error(), "network timeout")
	}
}

func TestResult_HasSkipped(t *testing.T) {
	var r Result
	if r.HasSkipped() {
		t.Error("HasSkipped() = true on empty result, want false")
	}
	r.AddSkipped("op", "reason")
	if !r.HasSkipped() {
		t.Error("HasSkipped() = false after AddSkipped, want true")
	}
}

func TestResult_HasErrors(t *testing.T) {
	var r Result
	if r.HasErrors() {
		t.Error("HasErrors() = true on empty result, want false")
	}
	r.AddError(errors.New("fail"))
	if !r.HasErrors() {
		t.Error("HasErrors() = false after AddError, want true")
	}
}

func TestResult_ZeroValue(t *testing.T) {
	var r Result
	if r.Applied != nil {
		t.Error("zero-value Applied should be nil")
	}
	if r.Skipped != nil {
		t.Error("zero-value Skipped should be nil")
	}
	if r.Errors != nil {
		t.Error("zero-value Errors should be nil")
	}
	if r.HasSkipped() {
		t.Error("zero-value HasSkipped() should be false")
	}
	if r.HasErrors() {
		t.Error("zero-value HasErrors() should be false")
	}
}

func TestSkippedOperation_Fields(t *testing.T) {
	s := SkippedOperation{
		Name:   "enable private vulnerability reporting",
		Reason: "insufficient role (need: admin or security manager)",
	}
	if s.Name != "enable private vulnerability reporting" {
		t.Errorf("Name = %q, want %q", s.Name, "enable private vulnerability reporting")
	}
	if s.Reason != "insufficient role (need: admin or security manager)" {
		t.Errorf("Reason = %q, want %q", s.Reason, "insufficient role (need: admin or security manager)")
	}
}
