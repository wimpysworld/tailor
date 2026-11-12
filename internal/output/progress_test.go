package output

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
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
		testutil.WaitForPTYProbeEcho(t)
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
	if runtime.GOOS == "windows" {
		t.Skip("pty requires unix")
	}
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

func TestProgressPromptPTYHelper(t *testing.T) {
	mode := os.Getenv("TAILOR_PROMPT_PROGRESS_HELPER")
	if mode == "" {
		return
	}
	progress := New(os.Stdout, os.Stderr, Auto, WithColor(ColorNever), WithVerbose(true)).StartProgress()
	if progress == nil || !progress.config.animate {
		t.Fatal("verbose disabled progress")
	}
	defer progress.Stop()
	progress.Observe(StageEvent{ID: "wiki", Label: "Inspecting wiki", Phase: "start"})
	progress.activate(progress.generation)
	program := progress.program
	injected := errors.New("prompt input failed")
	approved := false
	err := progress.WithPaused(func() error {
		fmt.Fprintln(os.Stderr, "PROMPT START")
		fmt.Fprintln(os.Stderr, "Approve only this wiki import? [y/N]")
		approved = mode == "approved"
		fmt.Fprintln(os.Stderr, "PROMPT END")
		if mode == "failed" {
			return injected
		}
		return nil
	})
	if errors.Is(err, injected) != (mode == "failed") || approved != (mode == "approved") {
		t.Fatalf("approved=%t err=%v", approved, err)
	}
	if progress.program != program || progress.stopped {
		t.Fatal("prompt stopped or replaced progress")
	}
	progress.Observe(StageEvent{ID: "continued", Label: "Progress after prompt", Phase: "start"})
	// A second handoff waits for the resumed event loop without a timer.
	if err := progress.WithPaused(func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	progress.Stop()
	fmt.Fprintln(os.Stderr, "HANDOFF COMPLETE")
}

func TestProgressPromptTerminalHandoff(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty requires unix")
	}
	for _, mode := range []string{"approved", "declined", "failed"} {
		t.Run(mode, func(t *testing.T) {
			got := testutil.RunPTY(t, "TestProgressPromptPTYHelper", "", "TAILOR_PROMPT_PROGRESS_HELPER="+mode)
			before, rest, found := strings.Cut(got, "PROMPT START")
			prompt, after, ended := strings.Cut(rest, "PROMPT END")
			if !found || !ended || !strings.Contains(before, "\x1b[?25h") || !strings.Contains(after, "\x1b[?25l") {
				t.Fatalf("terminal was not released and restored: %q", got)
			}
			if strings.Contains(prompt, "\x1b") || strings.Count(got, "[y/N]") != 1 || !strings.Contains(after, "HANDOFF COMPLETE") {
				t.Fatalf("progress interfered with confirmation: %q", got)
			}
		})
	}
}

func TestProgressPromptBeforeActivation(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(fmt.Sprint(failed), func(t *testing.T) {
			progress := StartProgress(io.Discard)
			defer progress.Stop()
			progress.Observe(StageEvent{ID: "wiki", Label: "Inspecting wiki", Phase: "start"})
			generation := progress.generation
			injected := errors.New("prompt failed")
			activeDuringPrompt := false
			err := progress.WithPaused(func() error {
				activeDuringPrompt = progress.timer != nil || progress.program != nil
				if failed {
					return injected
				}
				return nil
			})
			if activeDuringPrompt {
				t.Fatal("progress active during prompt")
			}
			if errors.Is(err, injected) != failed || progress.timer == nil || progress.generation <= generation {
				t.Fatalf("progress did not resume: %v", err)
			}
		})
	}
}

func TestProgressConcurrentStopAndActivation(t *testing.T) {
	for range 20 {
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
