package output

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func terminalDocument() Document {
	return Document{Command: "baste", Context: "octocat/project", Items: []Item{
		{Domain: "Repository", Category: "Security", Outcome: Alteration, Action: "set", Name: "Secret scanning", Provenance: "repository.secret_scanning", Before: "disabled", After: "enabled"},
		{Domain: "Actions", Outcome: Alteration, Action: "set", Name: "Workflow permissions", Provenance: "actions.default_workflow_permissions", Before: "write", After: "read"},
		{Domain: "Files", Outcome: Alteration, Action: "copy", Name: ".github/SECURITY.md"},
		{Domain: "Repository", Category: "Security", Outcome: Alteration, Action: "reapply", Name: "Push protection", Provenance: "repository.secret_scanning_push_protection", After: "enabled", Reason: "Reapply the enable request."},
		{Domain: "Pages", Outcome: Unchanged, Action: "match", Name: "Build type", Provenance: "pages.build_type", After: "workflow"},
		{Domain: "Ruleset", Outcome: Unchanged, Action: "match", Name: "Enforcement", Provenance: "ruleset.enforcement", After: "active"},
		{Domain: "Labels", Outcome: Unchanged, Action: "match", Name: "bug", Provenance: "label.bug", After: "d73a4a"},
		{Domain: "Files", Outcome: Preserved, Name: "justfile", Reason: "first-fit, exists"},
		{Domain: "Files", Outcome: Excluded, Name: "LICENSE", Reason: "mode never"},
	}, Guidance: []Guidance{
		{Kind: GuidanceOptional, Text: "Optional: adopt Tailor's Dependabot templates.\nReview with `tailor docket`."},
		{Kind: GuidanceFinal, Text: "Review the preview, then run `tailor alter`."},
	}}
}

func TestTerminalFixtures(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	for _, width := range []int{60, 80, 100} {
		for _, verbose := range []bool{false, true} {
			for _, ascii := range []bool{false, true} {
				name := fmt.Sprintf("%d-verbose-%v-ascii-%v", width, verbose, ascii)
				t.Run(name, func(t *testing.T) {
					doc := terminalDocument()
					p := New(nil, nil, Auto, WithTTY(true), WithWidth(width), WithVerbose(verbose), WithASCII(ascii), WithColor(ColorNever))
					got := p.Render(doc)
					for line := range strings.SplitSeq(got, "\n") {
						if lipgloss.Width(line) > width {
							t.Fatalf("overflow: %q", line)
						}
					}
					if strings.Contains(got, "\x1b") {
						t.Fatal("ANSI in monochrome")
					}
					if ascii {
						for _, r := range got {
							if r > 127 {
								t.Fatalf("non-ASCII %q", r)
							}
						}
					}
					if !strings.Contains(got, "Reapply requests") || strings.Contains(got, "enabled → enabled") {
						t.Fatal("wrong reapply state")
					}
					if !verbose && (strings.Contains(got, "first-fit, exists") || strings.Contains(got, "mode never")) {
						t.Fatal("normal view exposes policy paragraphs")
					}
					if verbose {
						for _, item := range doc.Items {
							name := item.Provenance
							if name == "" {
								name = item.Name
							}
							if !strings.Contains(got, name) {
								t.Fatalf("missing %s", name)
							}
						}
					}
					path := filepath.Join("testdata", "terminal-"+name+".golden")
					if os.Getenv("TAILOR_UPDATE_FIXTURES") == "1" {
						if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
							t.Fatal(err)
						}
					}
					want, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					if got != string(want) {
						t.Fatalf("fixture differs: %s\n%s", path, got)
					}
				})
			}
		}
	}
}

func TestTerminalSupportingTextFullIntensity(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	faint := regexp.MustCompile(`\x1b\[(?:[0-9]+;)*2(?:;[0-9]+)*m`)
	for _, width := range []int{60, 80, 100} {
		for _, verbose := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/verbose=%t", width, verbose), func(t *testing.T) {
				p := New(nil, nil, Auto, WithTTY(true), WithWidth(width), WithVerbose(verbose), WithColor(ColorAlways))
				doc := terminalDocument()
				doc.Guidance[0].Text = "Optional: use `tailor docket`."
				got := p.Render(doc)
				if faint.MatchString(got) {
					t.Fatal("final report contains SGR faint")
				}
				if p.muted("supporting value") != "supporting value" {
					t.Fatal("supporting text overrides the terminal foreground or weight")
				}
				clean := ansi.Strip(got)
				for _, text := range []string{"octocat/project", "3 unchanged", "Optional: use `tailor docket`."} {
					if !strings.Contains(clean, text) {
						t.Errorf("missing supporting text %q", text)
					}
				}
				if verbose && (!strings.Contains(clean, "workflow") || !strings.Contains(clean, "active") || !strings.Contains(clean, "d73a4a")) {
					t.Fatal("verbose report lost matching values")
				}
				if !strings.Contains(got, p.code("`tailor docket`")) {
					t.Fatal("optional command lost full-intensity highlighting")
				}
			})
		}
	}
}

func TestTerminalSummaryKeepsMetricsTogether(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	for _, verbose := range []bool{false, true} {
		for _, colour := range []Color{ColorNever, ColorAlways} {
			p := New(nil, nil, Auto, WithTTY(true), WithWidth(60), WithVerbose(verbose), WithColor(colour))
			got := p.Render(terminalDocument())
			want := "1 reapply request, 3 alterations, 1 preserved, 1 excluded,\n3 unchanged\n"
			if !strings.Contains(ansi.Strip(got), want) {
				t.Errorf("verbose %t, colour %s: summary split a metric:\n%s", verbose, colour, got)
			}
			for _, metric := range []struct{ text, colour string }{
				{"1 reapply request", neutralColour},
				{"3 alterations", changeColour},
			} {
				if !strings.Contains(got, p.style(metric.text, metric.colour)) {
					t.Errorf("summary omitted styled metric %q", metric.text)
				}
			}
		}
	}

	p := New(nil, nil, Auto, WithTTY(true), WithWidth(60), WithColor(ColorAlways))
	var summary strings.Builder
	p.terminalSummary(&summary, Summary{Alterations: 2, Applied: 3, Attention: 4, Preserved: 5, Excluded: 6, Unchanged: 7}, nil)
	want := "2 alterations, 3 applied, 4 need attention, 5 preserved,\n6 excluded, 7 unchanged\n"
	if got := ansi.Strip(summary.String()); got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
	if !strings.Contains(summary.String(), p.style("3 applied", changeColour)) || !strings.Contains(summary.String(), p.style("4 need attention", attentionColour)) {
		t.Fatal("summary lost applied or attention colour")
	}
}

func TestTerminalCommandSpansAndLongSource(t *testing.T) {
	t.Setenv("TERM", "xterm")
	command := "`tailor baste --config " + strings.Repeat("long-path/", 12) + ".tailor.yml`"
	snippet := "\targs = [\"sh\", \"-c\", \"exact source bytes\"]"
	doc := terminalDocument()
	doc.Items = append(doc.Items, Item{Outcome: Attention, Name: "Wiki readiness", Reason: "Review the imported source before another preview."})
	doc.Guidance = []Guidance{{Text: "Recheck with " + command + ".\n" + snippet + "\n    git clone https://github.com/octocat/project.wiki.git ../wiki-import"}}
	for _, width := range []int{40, 60, 80, 100} {
		p := New(nil, nil, Auto, WithTTY(true), WithWidth(width), WithASCII(false), WithColor(ColorAlways))
		got := p.Render(doc)
		if !strings.Contains(got, p.code(command)) || !strings.Contains(got, p.code(snippet)) || !strings.Contains(got, p.code("    git clone https://github.com/octocat/project.wiki.git ../wiki-import")) {
			t.Fatalf("unstyled command/source: %q", got)
		}
		clean := ansi.Strip(got)
		if !strings.Contains(clean, command) || !strings.Contains(clean, snippet) {
			t.Fatal("copyable bytes changed")
		}
		if strings.Index(clean, "Wiki readiness") > strings.Index(clean, "Secret scanning") {
			t.Fatal("attention follows changes")
		}
		for line := range strings.SplitSeq(clean, "\n") {
			if strings.Contains(line, "long-path/") && strings.ContainsAny(line, "│╭╮") {
				t.Fatal("long command inside frame")
			}
		}
		if width < 60 && strings.ContainsAny(clean, "│╭╮") {
			t.Fatal("narrow view has frames")
		}
	}
}

func TestTerminalHierarchy(t *testing.T) {
	t.Setenv("TERM", "xterm")
	p := New(nil, nil, Auto, WithTTY(true), WithWidth(80), WithColor(ColorAlways), WithVerbose(true), WithASCII(false))
	assertMatchingRow := func(got, frameColour string) {
		t.Helper()
		row := "pages.https_enforced = true"
		border := p.style("│", frameColour)
		want := border + " " + row + strings.Repeat(" ", p.width-4-lipgloss.Width(row)) + " " + border + "\n"
		if !strings.Contains(got, "\n"+want) {
			t.Fatalf("missing unstyled matching row %q in:\n%q", want, got)
		}
	}
	doc := Document{Command: "baste", Context: "owner/repo", Items: []Item{{Domain: "Pages", Outcome: Unchanged, Action: "match", Provenance: "pages.https_enforced", Before: "true", After: "true"}}, Guidance: []Guidance{
		{Kind: GuidanceRequired, Text: "Verify with `tailor baste`."},
		{Kind: GuidanceOptional, Text: "Optional: use `tailor docket`.\n    git status --short"},
		{Kind: GuidanceInfo, Text: "Local configuration is preserved."},
		{Kind: GuidanceFinal, Text: "Run `tailor alter`."},
	}}
	got := p.Render(doc)
	clean := ansi.Strip(got)
	if !strings.Contains(got, p.code("tailor baste")) || !strings.Contains(got, p.muted("owner/repo")) {
		t.Fatal("header hierarchy missing")
	}
	if strings.Count(clean, "PREVIEW") != 1 || strings.Count(clean, "No changes made") != 1 {
		t.Fatal("duplicate preview label")
	}
	if strings.Contains(clean, "true → true") {
		t.Fatal("matching values show a false transition")
	}
	assertMatchingRow(got, settledColour)
	if strings.Index(clean, "Optional:") < strings.Index(clean, " Next ") || strings.Contains(clean, "Run `tailor alter`") {
		t.Fatal("incorrect guidance priority")
	}
	if !strings.Contains(got, p.code("`tailor docket`")) || !strings.Contains(got, p.code("    git status --short")) {
		t.Fatal("optional command is not highlighted")
	}
	doc.Items = append(doc.Items, Item{Domain: "Pages", Outcome: Alteration, Action: "set", Name: "Build type", Before: "legacy", After: "workflow"})
	got = p.Render(doc)
	assertMatchingRow(got, changeColour)
	if !strings.Contains(got, p.style("workflow", changeColour)) || !strings.Contains(got, p.muted("Already matches")) {
		t.Fatal("mixed domain does not separate matching values from changes")
	}
	p.verbose = false
	for key, label := range terminalLabels {
		doc.Items = []Item{{Domain: "Settings", Outcome: Alteration, Name: key, Provenance: key, After: "true"}}
		if !strings.Contains(ansi.Strip(p.Render(doc)), label) {
			t.Fatalf("missing label for %s", key)
		}
	}
	doc.Items = []Item{{Outcome: Alteration, Name: "custom_setting_ID", After: "value"}}
	if !strings.Contains(ansi.Strip(p.Render(doc)), "custom_setting_ID") {
		t.Fatal("unknown identifier changed")
	}
}

func TestTerminalSafeValuesAndCellWidths(t *testing.T) {
	t.Setenv("TERM", "xterm")
	doc := Document{Command: "baste", Context: "owner/界", Items: []Item{{Domain: "Labels", Outcome: Alteration, Action: "set", Name: "Cafe\u0301 界", Before: "old", After: "new\x1b[31m"}}}
	p := New(nil, nil, Auto, WithTTY(true), WithWidth(60), WithASCII(false), WithColor(ColorAlways))
	clean := ansi.Strip(p.Render(doc))
	if !strings.Contains(clean, `new\x1b[31m`) {
		t.Fatal("unsafe value")
	}
	for line := range strings.SplitSeq(clean, "\n") {
		if strings.HasPrefix(line, "│") && lipgloss.Width(line) != 60 {
			t.Fatalf("misaligned: %q", line)
		}
	}
}
