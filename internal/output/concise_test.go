package output

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestAutoKeepsConciseReportWithoutTerminal(t *testing.T) {
	for _, term := range []string{"xterm", "dumb"} {
		t.Run(term, func(t *testing.T) {
			t.Setenv("TERM", term)
			for _, tty := range []bool{false, true} {
				var out strings.Builder
				p := New(&out, nil, Auto, WithTTY(tty), WithColor(ColorAlways))
				p.Print(proposalDocument(), "legacy rows")
				got := out.String()
				mode := "Preview - no changes made"
				if tty && term != "dumb" {
					mode = "No changes made"
				}
				if !strings.Contains(got, mode) || strings.Contains(got, "legacy rows") {
					t.Fatalf("wrong contract: %s", got)
				}
				if (!tty || term == "dumb") && strings.Contains(got, "\x1b") {
					t.Fatalf("ANSI on unsupported stream: %q", got)
				}
			}
		})
	}
}

func TestConciseSafetyGuidanceAndProvenance(t *testing.T) {
	doc := Document{Command: "baste", Context: "owner/repo", Items: []Item{
		{Outcome: Alteration, Action: "reapply", Name: "humanised field", Provenance: "repository.field", Before: "true", After: "true", Reason: "reapply enable request"},
		{Outcome: Attention, Action: "resolve", Name: "blocked"},
		{Outcome: Preserved, Name: "custom", Reason: "first-fit, exists"},
		{Outcome: Excluded, Name: "excluded", Reason: "mode never"},
	}, Notices: []Notice{{Level: "warning", Text: "Keep normal approvals."}}, Guidance: []Guidance{
		{Order: 1000, Kind: GuidanceFinal, Text: "Run `tailor alter`."},
		{Order: 2, Text: "Verify `flake.nix`: inspection is unknown. Keep existing settings."},
		{Order: 1, Text: "Add `import 'just/loader.just'` to `justfile`."},
		{Order: 2, Text: "Verify `flake.nix`: inspection is unknown. Keep existing settings."},
		{Order: 3, Kind: GuidanceOptional, Text: "Optional: review the custom file."},
	}}
	for _, quiet := range []bool{false, true} {
		got := New(nil, nil, Auto, WithQuiet(quiet), WithWidth(60)).Render(doc)
		for _, want := range []string{"blocked", "Keep normal approvals.", "Verify `flake.nix`", "1 excluded", "1 preserved"} {
			if !strings.Contains(got, want) {
				t.Errorf("quiet=%v missing %q: %s", quiet, want, got)
			}
		}
		if strings.Count(got, "Verify `flake.nix`") != 1 || strings.Contains(got, "Run `tailor alter`") {
			t.Fatalf("unsafe or repeated guidance: %s", got)
		}
		if !quiet {
			if strings.Count(got, "repository.field") != 1 || strings.Contains(got, "humanised field") || !strings.Contains(got, "would reapply = true") || strings.Contains(got, "true -> true") || !strings.Contains(got, "reapply enable request") {
				t.Fatalf("incorrect change row: %s", got)
			}
			if strings.Index(got, "blocked") > strings.Index(got, "repository.field") {
				t.Fatal("attention follows changes")
			}
		} else if strings.Contains(got, "Optional:") {
			t.Fatal("quiet includes optional advice")
		}
	}
	if got := Count(doc.Items); got.Excluded != 1 || got.Preserved != 1 {
		t.Fatalf("counts: %#v", got)
	}
}

func TestOptionalGuidanceVerbosity(t *testing.T) {
	const first = "Optional: adopt Tailor's Dependabot templates."
	const full = first + "\nBack up the file and review both templates.\nSet languages.go explicitly.\nAdd the exact marker only after review.\nResolve the alternate path.\nPreview changes and keep normal approvals."
	for _, command := range []string{"", "baste"} {
		for _, width := range []int{60, 80} {
			doc := Document{Command: command, Guidance: []Guidance{{Kind: GuidanceOptional, Text: full}}}
			got := New(nil, nil, Auto, WithWidth(width)).Render(doc)
			hint := "`--verbose`"
			if command != "" {
				hint = "`tailor " + command + " --verbose`"
			}
			if !strings.Contains(got, first) || !strings.Contains(got, hint) || !strings.Contains(strings.Join(strings.Fields(got), " "), "for adoption steps and safeguards.") {
				t.Fatalf("missing optional summary or discovery: %s", got)
			}
			for _, line := range strings.Split(full, "\n")[1:] {
				if strings.Contains(got, line) {
					t.Fatalf("default includes adoption instructions: %s", got)
				}
				verbose := New(nil, nil, Auto, WithVerbose(true), WithWidth(width)).Render(doc)
				if !strings.Contains(verbose, line) {
					t.Fatalf("verbose lacks safeguard %q: %s", line, verbose)
				}
			}
			for _, verbose := range []bool{false, true} {
				quiet := New(nil, nil, Auto, WithQuiet(true), WithVerbose(verbose)).Render(doc)
				if strings.Contains(quiet, "Optional:") || strings.Contains(quiet, "--verbose") {
					t.Fatalf("quiet includes optional guidance: %s", quiet)
				}
				var out strings.Builder
				New(&out, nil, Plain, WithVerbose(verbose)).Print(doc, full)
				if out.String() != full {
					t.Fatalf("plain changed original bytes: %q", out.String())
				}
			}
		}
	}
}

func TestPlannedSettingRowShowsValuesWithoutRedundantAction(t *testing.T) {
	doc := Document{Command: "baste", Items: []Item{{Outcome: Alteration, Action: "set", Provenance: "repository.has_wiki", Before: "false", After: "true"}}}
	got := New(nil, nil, Auto).Render(doc)
	if !strings.Contains(got, "repository.has_wiki = false -> true") || strings.Contains(got, "would set") {
		t.Fatalf("setting row = %s", got)
	}
}

func TestGuidanceKindControlsFinalAdvice(t *testing.T) {
	for _, kind := range []GuidanceKind{GuidanceRequired, GuidanceOptional, GuidanceInfo} {
		doc := Document{Guidance: []Guidance{{Kind: GuidanceFinal, Text: "Final action"}, {Kind: kind, Text: "Advice without a role in its text"}}}
		got := New(nil, nil, Auto).Render(doc)
		if strings.Contains(got, "Final action") != (kind != GuidanceRequired) {
			t.Fatalf("kind %d final advice: %s", kind, got)
		}
		quiet := New(nil, nil, Auto, WithQuiet(true)).Render(doc)
		if strings.Contains(quiet, "Advice") != (kind == GuidanceRequired) {
			t.Fatalf("kind %d quiet advice: %s", kind, quiet)
		}
	}
}

func TestDocketKeepsIdentityAndGuidanceDoesNotMutateDocument(t *testing.T) {
	doc := Document{Command: "docket", Items: []Item{{Outcome: Unchanged, Name: "Account", After: "octocat"}}, Guidance: []Guidance{{Order: 2, Text: "Second"}, {Order: 1, Text: "First"}}}
	got := New(nil, nil, Auto).Render(doc)
	if !strings.Contains(got, "Account = octocat") {
		t.Fatalf("identity hidden: %s", got)
	}
	if doc.Guidance[0].Order != 2 {
		t.Fatal("render changed input order")
	}
}

func TestGuidanceWrapKeepsCommandsAndSourceBytes(t *testing.T) {
	command := "`++ import ./nix/loader.nix { inherit pkgs; }`"
	snippet := "    args = [\"sh\", \"-c\", \"a deliberately long command that must stay on one line\"]"
	doc := Document{Command: "baste", Guidance: []Guidance{{Text: "Add the following exact expression to the existing package list: " + command + ".\nLocation: `flake.nix`. Preserve the existing package declarations.\n" + snippet}, {Text: snippet}}}
	for _, width := range []int{60, 80} {
		got := New(nil, nil, Auto, WithWidth(width)).Render(doc)
		if !strings.Contains(got, command) || strings.Count(got, "\n"+snippet+"\n") != 2 {
			t.Fatalf("broken copyable content: %s", got)
		}
		for line := range strings.SplitSeq(got, "\n") {
			if !strings.Contains(line, "args =") && lipgloss.Width(line) > width {
				t.Fatalf("prose wider than %d: %q", width, line)
			}
		}
	}
}
