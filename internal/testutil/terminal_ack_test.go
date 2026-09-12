package testutil

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestPTYAcknowledgementHelper(t *testing.T) {
	switch os.Getenv("TAILOR_PTY_ACK_HELPER") {
	case "":
		return
	case "probes":
		_, _ = io.WriteString(os.Stderr, "\x1b[?2026$p\x1b[?2027$p")
		WaitForPTYProbeEcho(t)
	case "missing":
		WaitForPTYProbeEcho(t)
	}
	_, _ = io.WriteString(os.Stdout, "HELPER FINISHED\n")
}

func TestPTYAcknowledgement(t *testing.T) {
	for _, mode := range []string{"probes", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			got := RunPTY(t, "TestPTYAcknowledgementHelper", "", "TAILOR_PTY_ACK_HELPER="+mode)
			finished := strings.Index(got, "HELPER FINISHED")
			if finished < 0 {
				t.Fatalf("helper did not finish: %q", got)
			}
			if mode == "probes" {
				for _, reply := range []string{"^[[?2026;2$y", "^[[?2027;1$y"} {
					if index := strings.Index(got, reply); index < 0 || index >= finished {
						t.Fatalf("helper finished before echoed reply %q: %q", reply, got)
					}
				}
			}
		})
	}
}

func TestPTYAcknowledgementTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 7*time.Second)
	defer cancel()
	ackRead, ackWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer ackRead.Close()
	defer ackWrite.Close()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPTYAcknowledgementHelper$") //nolint:gosec
	command.Env = append(os.Environ(), "TAILOR_PTY_ACK_HELPER=missing")
	command.ExtraFiles = []*os.File{ackRead}
	got, err := command.CombinedOutput()
	if err == nil || ctx.Err() != nil || !strings.Contains(string(got), "waiting for PTY probe echo: read PTY probe acknowledgement: i/o timeout") {
		t.Fatalf("expected acknowledgement timeout, got %v (context %v): %s", err, ctx.Err(), got)
	}
}
