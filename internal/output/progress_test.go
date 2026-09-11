package output

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestFastProgressPTYHelper(t *testing.T) {
	mode := os.Getenv("TAILOR_FAST_PROGRESS_HELPER")
	if mode == "" {
		return
	}
	if mode == "probe-control" {
		_, _ = io.WriteString(os.Stderr, "\x1b[?2026$p\x1b[?2027$p")
		time.Sleep(100 * time.Millisecond)
		return
	}
	progress := New(os.Stdout, os.Stderr, Auto, WithColor(ColorNever)).StartProgress()
	warnings := progress.WarningWriter(os.Stderr)
	_, _ = io.WriteString(warnings, "warning: before start\n")
	if mode != "idle" {
		progress.Observe(StageEvent{ID: "first", Label: "First stage", Phase: "start"})
		if mode == "error" {
			progress.Observe(StageEvent{ID: "first", Label: "Failed", Phase: "error"})
		} else if mode != "stop" {
			progress.Observe(StageEvent{ID: "first", Label: "Finished", Phase: "complete"})
			progress.Observe(StageEvent{ID: "second", Label: "Second stage", Phase: "start"})
			progress.Observe(StageEvent{ID: "second", Label: "Finished", Phase: "complete"})
		}
	}
	if mode != "stop" {
		time.Sleep(350 * time.Millisecond)
	}
	progress.Stop()
	progress.Stop()
	progress.Observe(StageEvent{ID: "late", Label: "Late stage", Phase: "start"})
	_, _ = io.WriteString(warnings, "warning: after stop\n")
	_, _ = io.WriteString(os.Stdout, "FINAL STDOUT\n")
	// Keep the slave open so an incorrect probe can receive an echoed reply.
	time.Sleep(350 * time.Millisecond)
}

func TestFastProgressDoesNotNegotiateTerminal(t *testing.T) {
	for _, mode := range []string{"complete", "error", "stop", "idle", "probe-control"} {
		t.Run(mode, func(t *testing.T) {
			got := testutil.RunPTY(t, "TestFastProgressPTYHelper", "", "TAILOR_FAST_PROGRESS_HELPER="+mode)
			if mode == "probe-control" {
				for _, reply := range []string{"^[[?2026;2$y", "^[[?2027;1$y"} {
					if !strings.Contains(got, reply) {
						t.Fatalf("PTY did not echo reply %q: %q", reply, got)
					}
				}
				return
			}
			if strings.Contains(got, "\x1b") || strings.Contains(got, "^[[?") {
				t.Fatalf("short progress touched terminal state: %q", got)
			}
			for _, want := range []string{"warning: before start", "warning: after stop", "FINAL STDOUT"} {
				if !strings.Contains(got, want) {
					t.Fatalf("missing %q: %q", want, got)
				}
			}
		})
	}
}

func TestProgressConcurrentStopAndActivation(t *testing.T) {
	for i := 0; i < 20; i++ {
		var transcript strings.Builder
		progress := StartProgress(&transcript)
		progress.Observe(StageEvent{ID: "stage", Label: "Work", Phase: "start"})
		progress.mu.Lock()
		generation := progress.generation
		progress.mu.Unlock()
		var wg sync.WaitGroup
		wg.Go(func() { progress.activate(generation) })
		wg.Go(func() { progress.Stop() })
		wg.Go(func() {
			_, _ = fmt.Fprintln(progress.WarningWriter(&transcript), "warning: concurrent")
		})
		wg.Wait()
		if !strings.Contains(transcript.String(), "warning: concurrent") {
			t.Fatalf("warning lost: %q", transcript.String())
		}
	}
}
