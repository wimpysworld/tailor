package output

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/creack/pty"
)

func TestTerminalReportPTYHelper(t *testing.T) {
	if os.Getenv("TAILOR_REPORT_PTY_HELPER") != "1" {
		return
	}
	colour := ColorAlways
	if os.Getenv("TAILOR_REPORT_MONO") == "1" {
		colour = ColorNever
	}
	p := New(os.Stdout, os.Stderr, Auto, WithColor(colour), WithVerbose(os.Getenv("TAILOR_REPORT_VERBOSE") == "1"))
	fmt.Print("TAILOR_REPORT_BEGIN\n")
	p.Print(terminalDocument(), "legacy output")
	fmt.Print("TAILOR_REPORT_END\n")
}

// Optional captures go outside the repository. The same PTY flow runs without captures.
func TestTerminalReportPTY(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty requires unix")
	}
	for _, width := range []int{60, 80, 100} {
		for _, verbose := range []bool{false, true} {
			for _, palette := range []string{"dark", "light", "mono"} {
				name := fmt.Sprintf("%s-%d-verbose-%v", palette, width, verbose)
				t.Run(name, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
					defer cancel()
					command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTerminalReportPTYHelper$") //nolint:gosec // The current test binary is trusted.
					colourfgbg := "15;0"
					if palette == "light" {
						colourfgbg = "0;15"
					}
					command.Env = append(os.Environ(), "TAILOR_REPORT_PTY_HELPER=1", "TERM=xterm-256color", "LC_ALL=C.UTF-8", "COLORFGBG="+colourfgbg, "TAILOR_REPORT_VERBOSE="+boolDigit(verbose), "TAILOR_REPORT_MONO="+boolDigit(palette == "mono"))
					terminal, err := pty.StartWithSize(command, &pty.Winsize{Rows: 24, Cols: uint16(width)}) //nolint:gosec // Width is one of the bounded test constants.
					if err != nil {
						t.Fatal(err)
					}
					data, readErr := io.ReadAll(terminal)
					_ = terminal.Close()
					if err := command.Wait(); err != nil {
						t.Fatalf("PTY failed: %v\n%s", err, data)
					}
					if readErr != nil && len(data) == 0 {
						t.Fatal(readErr)
					}
					_, report, found := strings.Cut(string(data), "TAILOR_REPORT_BEGIN\r\n")
					got, _, ended := strings.Cut(report, "TAILOR_REPORT_END\r\n")
					if !found || !ended {
						t.Fatal("missing report boundaries")
					}
					if !strings.Contains(ansi.Strip(got), "Proposed") || strings.Contains(got, "legacy output") {
						t.Fatalf("wrong report: %q", got)
					}
					if strings.Contains(got, "\x1b[") != (palette != "mono") {
						t.Fatal("incorrect colour policy")
					}
					if dir := os.Getenv("TAILOR_CAPTURE_DIR"); dir != "" {
						if !filepath.IsAbs(dir) {
							t.Fatal("capture path must be absolute")
						}
						if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // The test operator explicitly selects the capture directory.
							t.Fatal(err)
						}
						for ext, content := range map[string]string{"ansi": got, "txt": ansi.Strip(got), "pty": string(data)} {
							if err := os.WriteFile(filepath.Join(dir, name+"."+ext), []byte(content), 0o644); err != nil { //nolint:gosec // Fixed fixture names in the operator-selected capture directory.
								t.Fatal(err)
							}
						}
					}
				})
			}
		}
	}
}

func boolDigit(value bool) string { return strconv.Itoa(map[bool]int{false: 0, true: 1}[value]) }
