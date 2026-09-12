package output

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/creack/pty"
	"golang.org/x/term"
)

func proposalDocument() Document {
	doc := Document{Command: "baste", Context: "wimpysworld/tailor"}
	doc.Items = append(doc.Items, Item{Domain: "Repository", Category: "Security", Outcome: Alteration, Action: "set", Name: "Generic secret patterns", After: "enabled", Provenance: "repository.secret_scanning_non_provider_patterns"})
	for range 12 {
		doc.Items = append(doc.Items, Item{Domain: "Files", Outcome: Preserved, Name: "first-fit", Reason: "first-fit, exists"})
	}
	doc.Items = append(doc.Items, Item{Domain: "Files", Outcome: Preserved, Name: "never", Reason: "mode never"})
	for domain, count := range map[string]int{"Ruleset": 20, "Repository": 18, "Labels": 12, "Files": 9, "Actions": 7, "Security": 5, "Pages": 4, "Code analysis": 4} {
		for range count {
			doc.Items = append(doc.Items, Item{Domain: domain, Outcome: Unchanged, Name: "check"})
		}
	}
	doc.Summary = Count(doc.Items)
	return doc
}

func TestPlainAndRedirectedPreserveExactBytes(t *testing.T) {
	plain := "would set:                          repository.has_wiki = true\n"
	for _, policy := range []*Policy{New(new(strings.Builder), nil, Auto), New(new(strings.Builder), nil, Plain, WithTTY(true))} {
		var out strings.Builder
		policy.stdout = &out
		policy.Print(proposalDocument(), plain)
		if out.String() != plain {
			t.Fatalf("plain output = %q", out.String())
		}
	}
}

func TestProposalShapeAndResponsiveCards(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "xterm-256color")
	for _, width := range []int{60, 80, 100} {
		var out strings.Builder
		New(&out, nil, Auto, WithTTY(true), WithWidth(width)).Print(proposalDocument(), "legacy")
		got := out.String()
		for _, want := range []string{"TAILOR · BASTE", "1 alteration · 13 preserved by policy", "ALTERATION · 1", "PRESERVED BY POLICY · 13", "ALREADY MATCHES · 79", "repository.secret_scanning_non_provider_patterns"} {
			if !strings.Contains(got, want) {
				t.Errorf("width %d missing %q:\n%s", width, want, got)
			}
		}
		if strings.Contains(got, "\x1b[") {
			t.Fatalf("width %d has colour", width)
		}
	}
}

func TestVerboseExpandsEveryResult(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "xterm")
	var out strings.Builder
	New(&out, nil, Auto, WithTTY(true), WithVerbose(true)).Print(proposalDocument(), "")
	if got := strings.Count(out.String(), "ok check"); got != 79 {
		t.Fatalf("expanded unchanged rows = %d, want 79", got)
	}
	if got := strings.Count(out.String(), "= first-fit"); got != 12 {
		t.Fatalf("expanded policy rows = %d, want 12", got)
	}
}

func TestColourPoliciesAndASCII(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("NO_COLOR", "1")
	var noColour strings.Builder
	New(&noColour, nil, Auto, WithTTY(true), WithColor(ColorAuto)).Print(proposalDocument(), "")
	if strings.Contains(noColour.String(), "\x1b[") {
		t.Fatal("NO_COLOR emitted ANSI")
	}
	var colour strings.Builder
	New(&colour, nil, Auto, WithTTY(true), WithColor(ColorAlways)).Print(proposalDocument(), "")
	if !strings.Contains(colour.String(), "\x1b[") {
		t.Fatal("always did not emit ANSI")
	}
	var ascii strings.Builder
	New(&ascii, nil, Auto, WithTTY(true), WithASCII(true), WithColor(ColorNever)).Print(proposalDocument(), "")
	if !strings.HasPrefix(ascii.String(), "+") || strings.ContainsAny(ascii.String(), "╭╮╰╯") {
		t.Fatalf("ASCII border:\n%s", ascii.String())
	}
}

func TestDumbTerminalUsesPlainAndProgressNeedsBothTTYs(t *testing.T) {
	t.Setenv("TERM", "dumb")
	var out strings.Builder
	p := New(&out, nil, Auto, WithTTY(true), WithStderrTTY(true))
	p.Print(proposalDocument(), "exact\n")
	if out.String() != "exact\n" || p.ProgressEnabled(false) {
		t.Fatalf("dumb mode output=%q progress=%v", out.String(), p.ProgressEnabled(false))
	}
	t.Setenv("TERM", "xterm")
	if New(nil, nil, Auto, WithTTY(true), WithStderrTTY(false)).ProgressEnabled(false) {
		t.Fatal("non-TTY stderr enabled progress")
	}
	if New(nil, nil, Plain, WithTTY(true), WithStderrTTY(true)).ProgressEnabled(false) {
		t.Fatal("plain enabled progress")
	}
}

func TestQuietAppliesToPlainAndRedirectedOutput(t *testing.T) {
	for _, format := range []Format{Auto, Plain} {
		var out strings.Builder
		New(&out, nil, format, WithQuiet(true)).Print(proposalDocument(), "legacy detail\n")
		if got, want := out.String(), "1 alteration · 13 preserved by policy · 79 already match\n"; got != want {
			t.Fatalf("format %s quiet output = %q, want %q", format, got, want)
		}
	}
}

func TestResponsiveGridActionsOrderAndBorders(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "xterm")
	doc := proposalDocument()
	doc.Items = append(doc.Items,
		Item{Domain: "Files", Outcome: Alteration, Action: "remove", Name: "retired.yml"},
		Item{Domain: "Configuration", Outcome: Created, Action: "create", Name: ".tailor.yml"},
		Item{Domain: "Files", Outcome: Attention, Action: "skip", Name: "blocked"},
	)
	for _, width := range []int{20, 60, 100} {
		got := New(nil, nil, Auto, WithTTY(true), WithWidth(width), WithColor(ColorNever)).Render(doc)
		for line := range strings.SplitSeq(strings.TrimSuffix(got, "\n"), "\n") {
			if rendered := lipgloss.Width(line); rendered > width {
				t.Fatalf("width %d line is %d columns: %q", width, rendered, line)
			}
		}
		if !strings.Contains(got, "remove") {
			t.Fatalf("width %d omitted action:\n%s", width, got)
		}
		if strings.Index(got, ".tailor.yml") > strings.Index(got, "blocked") {
			t.Fatalf("width %d did not show created result before attention:\n%s", width, got)
		}
		if width == 100 {
			gridRow := false
			for line := range strings.SplitSeq(got, "\n") {
				if strings.Contains(line, "Actions") && strings.Contains(line, "Code analysis") {
					gridRow = true
				}
			}
			if !gridRow {
				t.Fatalf("wide summary is not a grid:\n%s", got)
			}
		}
	}
}

func TestAutomaticASCIIAndColourDetection(t *testing.T) {
	t.Setenv("TERM", "xterm")
	t.Setenv("LC_ALL", "C")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("LANG", "")
	t.Setenv("NO_COLOR", "")
	var out strings.Builder
	New(&out, nil, Auto, WithTTY(true)).Print(proposalDocument(), "")
	if !strings.HasPrefix(out.String(), "\x1b[") || !strings.Contains(out.String(), "+") || strings.ContainsAny(out.String(), "╭╮╰╯") {
		t.Fatalf("automatic ASCII/colour output:\n%s", out.String())
	}
	if New(nil, nil, Auto, WithTTY(true), WithStderrTTY(true), WithNoProgress(true)).ProgressEnabled(false) {
		t.Fatal("--no-progress enabled progress")
	}
}

func TestProgressWarningWriterAndStop(t *testing.T) {
	var stderr strings.Builder
	progress := StartProgress(&stderr)
	progress.Observe(StageEvent{ID: "repository", Label: "Measuring repository", Phase: "start"})
	if _, err := progress.WarningWriter(&stderr).Write([]byte("warning: limited access\n")); err != nil {
		t.Fatal(err)
	}
	progress.Observe(StageEvent{ID: "repository", Label: "Repository measured", Phase: "complete"})
	progress.Stop()
	if !strings.Contains(stderr.String(), "warning: limited access") {
		t.Fatalf("warning missing from progress output: %q", stderr.String())
	}
}

func TestPTYProgressHelper(t *testing.T) {
	if os.Getenv("TAILOR_PTY_HELPER") != "1" {
		return
	}
	if !term.IsTerminal(int(os.Stderr.Fd())) {
		t.Fatal("helper stderr is not a TTY")
	}
	policy := New(os.Stdout, os.Stderr, Auto, WithColor(ColorNever))
	progress := policy.StartProgress()
	time.Sleep(30 * time.Millisecond)
	progress.Observe(StageEvent{ID: "local", Label: "Delayed local work", Phase: "start"})
	time.Sleep(420 * time.Millisecond)
	_, _ = io.WriteString(progress.WarningWriter(os.Stderr), "warning: survives\n")
	progress.Observe(StageEvent{ID: "local", Label: "Local work complete", Phase: "complete"})
	progress.Stop()
	_, _ = io.WriteString(os.Stdout, "FINAL STDOUT\n")
}

func TestProgressOnOperatingSystemPTY(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	// os.Args[0] is the current test binary, not external input.
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPTYProgressHelper$") //nolint:gosec
	command.Env = append(os.Environ(), "TAILOR_PTY_HELPER=1", "TERM=xterm-256color", "LANG=C.UTF-8")
	terminal, err := pty.Start(command)
	if err != nil {
		t.Fatal(err)
	}
	if err := pty.Setsize(terminal, &pty.Winsize{Rows: 24, Cols: 80}); err != nil {
		t.Fatal(err)
	}
	outputBytes, readErr := io.ReadAll(terminal)
	_ = terminal.Close()
	waitErr := command.Wait()
	if waitErr != nil {
		t.Fatalf("PTY helper failed: %v\n%s", waitErr, outputBytes)
	}
	if readErr != nil && len(outputBytes) == 0 {
		t.Fatal(readErr)
	}
	got := string(outputBytes)
	stage := strings.Index(got, "Delayed local work")
	warning := strings.Index(got, "warning: survives")
	final := strings.Index(got, "FINAL STDOUT")
	restored := strings.LastIndex(got, "\x1b[?25h")
	if stage < 0 || warning < 0 || final < 0 || stage >= warning || warning >= restored || restored >= final {
		t.Fatalf("PTY output order is wrong: stage=%d warning=%d restored=%d final=%d\n%q", stage, warning, restored, final, got)
	}
	spinner := -1
	for _, frame := range []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"} {
		if index := strings.Index(got, frame); index >= 0 && (spinner < 0 || index < spinner) {
			spinner = index
		}
	}
	if spinner < 0 || spinner > warning {
		t.Fatalf("spinner did not appear before delayed work completed: spinner=%d\n%q", spinner, got)
	}
	if strings.Contains(got, "\x1b[3") {
		t.Fatalf("--color=never emitted colour: %q", got)
	}
}

func TestTerminalProgressIntegration(t *testing.T) {
	model := newProgressModel(false, true)
	updated, delay := model.Update(StageEvent{ID: "labels", Label: "Measuring\nlabels", Phase: "start", Item: "bad\rname"})
	started := time.Now()
	activation := delay()
	if elapsed := time.Since(started); elapsed < 280*time.Millisecond {
		t.Fatalf("spinner activated after %s, want at least 280ms", elapsed)
	}
	updated, _ = updated.(ProgressModel).Update(activation)
	view := updated.(ProgressModel).View().Content
	for _, want := range []string{"Measuring\\x0alabels", "bad\\x0dname"} {
		if !strings.Contains(view, want) {
			t.Fatalf("live view missing escaped text %q: %q", want, view)
		}
	}
	if strings.ContainsAny(view, "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏") || strings.Contains(view, "\x1b[") {
		t.Fatalf("ASCII colourless progress used Unicode or colour: %q", view)
	}

	var transcript strings.Builder
	progress := startProgress(&transcript, progressConfig{animate: true, ascii: true})
	if _, err := progress.WarningWriter(&transcript).Write([]byte("warning: preserved\n")); err != nil {
		t.Fatal(err)
	}
	progress.Stop()
	transcript.WriteString("FINAL REPORT\n")
	if got := transcript.String(); !strings.Contains(got, "warning: preserved") || !strings.HasSuffix(got, "FINAL REPORT\n") {
		t.Fatalf("warning/final output ordering = %q", got)
	}

	var verbose strings.Builder
	verboseProgress := startProgress(&verbose, progressConfig{verbose: true, ascii: true})
	verboseProgress.Observe(StageEvent{ID: "wiki", Label: "Wiki\ncheck", Phase: "start", Item: "Home\r.md"})
	verboseProgress.Observe(StageEvent{ID: "wiki", Label: "Wiki checked", Phase: "complete"})
	verboseProgress.Stop()
	if got := verbose.String(); got != "[start] · Wiki\\x0acheck · Home\\x0d.md\n[complete] · Wiki checked\n" || strings.Contains(got, "\x1b[") {
		t.Fatalf("verbose stage records = %q", got)
	}
}

func TestProgressPolicyControlsColourASCIIAndVerbose(t *testing.T) {
	t.Setenv("TERM", "xterm")
	var stderr strings.Builder
	policy := New(nil, &stderr, Auto, WithTTY(true), WithStderrTTY(true), WithColor(ColorNever), WithASCII(true))
	progress := policy.StartProgress()
	time.Sleep(20 * time.Millisecond)
	progress.Observe(StageEvent{ID: "swatches", Label: "Measuring swatches", Phase: "start"})
	time.Sleep(350 * time.Millisecond)
	progress.Stop()
	if strings.Contains(stderr.String(), "\x1b[3") || strings.ContainsAny(stderr.String(), "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏") {
		t.Fatalf("policy was not applied to progress: %q", stderr.String())
	}

	stderr.Reset()
	verbosePolicy := New(nil, &stderr, Auto, WithTTY(true), WithStderrTTY(true), WithVerbose(true), WithColor(ColorNever), WithASCII(true))
	verboseProgress := verbosePolicy.StartProgress()
	verboseProgress.Observe(StageEvent{ID: "licence", Label: "Measuring licence", Phase: "start"})
	verboseProgress.Stop()
	if got := stderr.String(); got != "[start] · Measuring licence\n" {
		t.Fatalf("verbose policy output = %q", got)
	}
}

func TestProgressSpinnerIgnoresPreviousStart(t *testing.T) {
	for _, phase := range []string{"complete", "error", "start"} {
		t.Run(phase, func(t *testing.T) {
			model := NewProgressModel()
			updated, oldCmd := model.Update(StageEvent{ID: "auth", Phase: "start"})
			updated, _ = updated.(ProgressModel).Update(StageEvent{ID: "auth", Phase: phase})
			updated, currentCmd := updated.(ProgressModel).Update(StageEvent{ID: "auth", Phase: "start"})
			updated, cmd := updated.(ProgressModel).Update(oldCmd())
			if updated.(ProgressModel).active || cmd != nil {
				t.Fatal("previous start activated the spinner for a reused stage ID")
			}
			updated, cmd = updated.(ProgressModel).Update(currentCmd())
			if !updated.(ProgressModel).active || cmd == nil {
				t.Fatal("current start did not activate the spinner")
			}
		})
	}
}

func TestProgressSpinnerDelayAndLifecycle(t *testing.T) {
	model := NewProgressModel()
	updated, cmd := model.Update(StageEvent{ID: "auth", Label: "Authenticating", Phase: "start"})
	started := updated.(ProgressModel)
	if started.active || cmd == nil {
		t.Fatal("spinner must wait and schedule activation")
	}
	updated, _ = started.Update(activateSpinner{id: "auth", generation: started.generation})
	active := updated.(ProgressModel)
	if !active.active || !strings.Contains(active.View().Content, "Authenticating") {
		t.Fatal("spinner did not activate")
	}
	updated, _ = active.Update(StageEvent{ID: "auth", Label: "Authenticated", Phase: "complete"})
	if updated.(ProgressModel).active {
		t.Fatal("completed stage remained active")
	}
	t.Setenv("NO_COLOR", "1")
	failed, _ := active.Update(StageEvent{ID: "auth", Label: "Authentication failed", Phase: "error", Current: 2, Total: 3, Item: "token", Err: errors.New("denied\x1b")})
	view := failed.(ProgressModel).View().Content
	for _, want := range []string{"!  Authentication failed", "2 of 3", "token", "denied\\x1b"} {
		if !strings.Contains(view, want) {
			t.Fatalf("error progress missing %q: %q", want, view)
		}
	}
}
