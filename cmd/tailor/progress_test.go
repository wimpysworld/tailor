package main

import (
	"os"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestMeasurePTYHelper(t *testing.T) {
	if os.Getenv("TAILOR_MEASURE_PTY_HELPER") != "1" {
		return
	}
	// Local filesystem and PTY I/O do not advance the bubble's clock, so
	// slow real I/O cannot trigger the progress activation timer.
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		if code := run([]string{"measure", "--color=never"}, os.Stdout, os.Stderr); code != 0 {
			t.Fatalf("measure exit status: %d", code)
		}
		if elapsed := time.Since(start); elapsed != 0 {
			t.Fatalf("fast measure advanced the progress clock by %s", elapsed)
		}
	})
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
