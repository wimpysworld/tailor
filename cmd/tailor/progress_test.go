package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestMeasurePTYHelper(t *testing.T) {
	if os.Getenv("TAILOR_MEASURE_PTY_HELPER") != "1" {
		return
	}
	if code := run([]string{"measure", "--color=never"}, os.Stdout, os.Stderr); code != 0 {
		t.Fatalf("measure exit status: %d", code)
	}
	time.Sleep(100 * time.Millisecond)
}

func TestMeasurePTYDoesNotNegotiateTerminal(t *testing.T) {
	got := testutil.RunPTY(t, "TestMeasurePTYHelper", t.TempDir(), "TAILOR_MEASURE_PTY_HELPER=1")
	for _, unwanted := range []string{"\x1b[?2026", "\x1b[?2027", "^[[?", "\x1b[?25"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("measure emitted terminal negotiation or cursor controls: %q", got)
		}
	}
	if !strings.Contains(got, "README.md") {
		t.Fatalf("measure report missing: %q", got)
	}
}
