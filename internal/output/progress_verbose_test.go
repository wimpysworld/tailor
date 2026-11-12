package output

import (
	"context"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestVerboseProgressPTYHelper(t *testing.T) {
	if os.Getenv("TAILOR_VERBOSE_PROGRESS_HELPER") != "1" {
		return
	}
	p := New(os.Stdout, os.Stderr, Auto, WithVerbose(true), WithNoProgress(os.Getenv("TAILOR_VERBOSE_NO_PROGRESS") == "1"))
	progress := p.StartProgress()
	progress.Observe(StageEvent{ID: "stub", Label: "Delayed local stub", Phase: "start"})
	<-time.After(650 * time.Millisecond)
	progress.Observe(StageEvent{ID: "stub", Label: "Stub complete", Phase: "complete"})
	progress.Stop()
	p.Print(Document{Command: "baste", Context: "local fixture"}, "")
}

func TestVerboseProgressPTY(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty requires unix")
	}
	for _, noProgress := range []bool{false, true} {
		t.Run(boolDigit(noProgress), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestVerboseProgressPTYHelper$") //nolint:gosec // The current test binary is trusted.
			cmd.Env = append(os.Environ(), "TAILOR_VERBOSE_PROGRESS_HELPER=1", "TERM=xterm-256color", "LC_ALL=C.UTF-8", "TAILOR_VERBOSE_NO_PROGRESS="+boolDigit(noProgress))
			terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
			if err != nil {
				t.Fatal(err)
			}
			data, readErr := io.ReadAll(terminal)
			_ = terminal.Close()
			if err := cmd.Wait(); err != nil {
				t.Fatalf("helper: %v: %s", err, data)
			}
			if readErr != nil && len(data) == 0 {
				t.Fatal(readErr)
			}
			got := string(data)
			if strings.Contains(got, "[start]") || strings.Contains(got, "[complete]") {
				t.Fatal("diagnostic stage records leaked")
			}
			if strings.Contains(got, "Delayed local stub") == noProgress {
				t.Fatalf("incorrect progress policy: %q", got)
			}
			if !strings.Contains(got, "tailor baste") {
				t.Fatal("missing final report")
			}
		})
	}
}
