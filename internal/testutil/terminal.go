package testutil

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

// RunPTY runs a test helper with terminal input and output. It answers DEC mode
// queries as a terminal would, including replies that the terminal driver echoes
// when the child leaves input in cooked mode.
func RunPTY(t *testing.T, helper, dir string, env ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+helper+"$") //nolint:gosec
	command.Dir = dir
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
		if readErr != nil {
			break
		}
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("PTY helper: %v\n%q", err, transcript.String())
	}
	return transcript.String()
}
