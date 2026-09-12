package testutil

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
)

// WaitForPTYProbeEcho waits for RunPTY to observe both cooked-mode probe replies.
// The acknowledgement uses a separate pipe so terminal input stays in cooked mode.
func WaitForPTYProbeEcho(t *testing.T) {
	t.Helper()
	if err := syscall.SetNonblock(3, true); err != nil {
		t.Fatal(err)
	}
	ack := os.NewFile(3, "PTY probe acknowledgement")
	if ack == nil {
		t.Fatal("missing PTY acknowledgement pipe")
	}
	defer ack.Close()
	if err := ack.SetReadDeadline(time.Now().Add(4 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var received [1]byte
	if _, err := io.ReadFull(ack, received[:]); err != nil {
		t.Fatalf("waiting for PTY probe echo: %v", err)
	}
	if received[0] != 1 {
		t.Fatalf("invalid PTY acknowledgement: %v", received[0])
	}
}

// RunPTY runs a test helper with terminal input and output. It answers DEC mode
// queries as a terminal would, including replies that the terminal driver echoes
// when the child leaves input in cooked mode.
func RunPTY(t *testing.T, helper, dir string, env ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+helper+"$") //nolint:gosec
	command.Dir = dir
	ackRead, ackWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer ackRead.Close()
	defer ackWrite.Close()
	command.ExtraFiles = []*os.File{ackRead}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if key != "TERM" && key != "TERM_PROGRAM" && key != "SSH_TTY" && key != "WT_SESSION" {
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env, "TERM=xterm-256color", "TERM_PROGRAM=tailor-test")
	command.Env = append(command.Env, env...)
	terminal, err := pty.StartWithSize(command, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	var transcript strings.Builder
	queries := []string{"\x1b[?2026$p", "\x1b[?2027$p"}
	replies := []string{"\x1b[?2026;2$y", "\x1b[?2027;1$y"}
	answered := make([]int, len(queries))
	acknowledged := false
	buf := make([]byte, 4096)
	for {
		n, readErr := terminal.Read(buf)
		transcript.Write(buf[:n])
		for i, query := range queries {
			count := strings.Count(transcript.String(), query)
			for answered[i] < count {
				if _, err := io.WriteString(terminal, replies[i]); err != nil {
					t.Errorf("terminal reply: %v", err)
				}
				answered[i]++
			}
		}
		if !acknowledged && answered[0] > 0 && answered[1] > 0 &&
			strings.Contains(transcript.String(), "^[[?2026;2$y") &&
			strings.Contains(transcript.String(), "^[[?2027;1$y") {
			if _, err := ackWrite.Write([]byte{1}); err != nil {
				t.Errorf("PTY acknowledgement: %v", err)
			}
			acknowledged = true
		}
		if readErr != nil {
			break
		}
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("PTY helper: %v\n%q", err, transcript.String())
	}
	return transcript.String()
}
