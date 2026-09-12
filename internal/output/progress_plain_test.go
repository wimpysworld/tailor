package output

import (
	"fmt"
	"strings"
	"testing"
)

func TestPlainDisablesProgressRegardlessOfFlags(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	for _, tty := range []bool{false, true} {
		for _, verbose := range []bool{false, true} {
			for _, quiet := range []bool{false, true} {
				for _, noProgress := range []bool{false, true} {
					name := fmt.Sprintf("tty=%t/verbose=%t/quiet=%t/no-progress=%t", tty, verbose, quiet, noProgress)
					t.Run(name, func(t *testing.T) {
						var stdout, stderr strings.Builder
						policy := New(&stdout, &stderr, Plain, WithTTY(tty), WithStderrTTY(tty), WithVerbose(verbose), WithQuiet(quiet), WithNoProgress(noProgress))
						progress := policy.StartProgress()
						progress.Observe(StageEvent{ID: "stage", Label: "Work", Phase: "start"})
						progress.Observe(StageEvent{ID: "stage", Label: "Work", Phase: "complete"})
						progress.Stop()
						if progress != nil {
							t.Error("plain format started progress")
						}
						if stdout.Len() != 0 || stderr.Len() != 0 {
							t.Fatalf("plain progress wrote stdout=%q stderr=%q", stdout.String(), stderr.String())
						}
					})
				}
			}
		}
	}
}

func TestAutoVerboseRetainsStageRecords(t *testing.T) {
	var stdout, stderr strings.Builder
	policy := New(&stdout, &stderr, Auto, WithTTY(false), WithStderrTTY(false), WithVerbose(true))
	progress := policy.StartProgress()
	progress.Observe(StageEvent{ID: "stage", Label: "Work", Phase: "start"})
	progress.Stop()
	if !strings.Contains(stderr.String(), "Work") {
		t.Fatalf("missing verbose stage record: %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("progress wrote stdout: %q", stdout.String())
	}
}
