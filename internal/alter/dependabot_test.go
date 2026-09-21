package alter

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestDependabotSelectionFromConfig(t *testing.T) {
	enabled := true
	disabled := false
	for _, tt := range []struct {
		name string
		cfg  *config.Config
		mode ApplyMode
		want dependabotSelection
	}{
		{name: "nil", mode: DryRun, want: dependabotSelection{}},
		{name: "omitted", cfg: &config.Config{Languages: &config.LanguageSettings{Go: &enabled}}, mode: Apply, want: dependabotSelection{GoDeclared: true, GoEnabled: true}},
		{name: "enabled", cfg: &config.Config{Languages: &config.LanguageSettings{Go: &enabled}, Swatches: []config.SwatchEntry{{Path: dependabotPath, Alteration: swatch.FirstFit}}}, mode: Apply, want: dependabotSelection{Present: true, Mode: swatch.FirstFit, GoDeclared: true, GoEnabled: true}},
		{name: "disabled recut", cfg: &config.Config{Languages: &config.LanguageSettings{Go: &disabled}, Swatches: []config.SwatchEntry{{Path: dependabotPath, Alteration: swatch.Always}}}, mode: Recut, want: dependabotSelection{Present: true, Mode: swatch.Always, Recut: true, GoDeclared: true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := dependabotSelectionFromConfig(tt.cfg, tt.mode); got != tt.want {
				t.Fatalf("selection = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestDependabotBodiesUseRenderedTemplatesAndFixedFixtures(t *testing.T) {
	bodies := mustDependabotBodies(t)
	if !bytes.Equal(bodies.Enabled, dependabotEnabledV1) {
		t.Fatal("enabled compatibility fixture differs from the current rendered template")
	}
	if !bytes.Equal(bodies.Disabled, dependabotDisabledV1) {
		t.Fatal("disabled compatibility fixture differs from the current rendered template")
	}
	for name, body := range map[string][]byte{"enabled": bodies.Enabled, "disabled": bodies.Disabled} {
		if bytes.HasPrefix(body, []byte(managedMarker(dependabotPath))) {
			t.Errorf("%s body contains the ownership marker", name)
		}
		for _, ecosystem := range []string{"github-actions", `package-ecosystem: "nix"`} {
			if !bytes.Contains(body, []byte(ecosystem)) {
				t.Errorf("%s body omits %s", name, ecosystem)
			}
		}
	}
	if !bytes.Contains(bodies.Enabled, []byte("package-ecosystem: gomod")) {
		t.Fatal("enabled body omits gomod")
	}
	if bytes.Contains(bodies.Disabled, []byte("package-ecosystem: gomod")) {
		t.Fatal("disabled body contains gomod")
	}
}

func TestPlanDependabotModeAndOwnershipMatrix(t *testing.T) {
	bodies := mustDependabotBodies(t)
	unsafe := dependabotSnapshot{
		Alternate:   dependabotDestinationSnapshot{Path: dependabotAlternatePath, Kind: dependabotDestinationSymlink},
		Destination: dependabotDestinationSnapshot{Path: dependabotPath, Kind: dependabotDestinationDirectory},
	}
	for _, selection := range []dependabotSelection{
		{},
		{Present: true, Mode: swatch.Never},
		{Present: true, Mode: swatch.Never, Recut: true},
	} {
		plan, err := planDependabot(selection, unsafe, bodies)
		if err != nil || plan.Outcome != dependabotOutcomeSkip {
			t.Fatalf("inactive selection %#v = %#v, %v, want skip", selection, plan, err)
		}
	}

	for _, mode := range []swatch.AlterationMode{swatch.FirstFit, swatch.Always} {
		for _, recut := range []bool{false, true} {
			selection := dependabotSelection{Present: true, Mode: mode, Recut: recut, GoDeclared: true, GoEnabled: true}
			plan, err := planDependabot(selection, missingDependabotSnapshot(), bodies)
			if err != nil || plan.Outcome != dependabotOutcomeCreate {
				t.Fatalf("mode %s recut %v missing = %#v, %v", mode, recut, plan, err)
			}
			unmarked := regularDependabotSnapshot(bodies.Enabled)
			plan, err = planDependabot(selection, unmarked, bodies)
			if err != nil || plan.Outcome != dependabotOutcomePreserve || len(plan.Output) != 0 {
				t.Fatalf("mode %s recut %v unmarked = %#v, %v", mode, recut, plan, err)
			}
			owned := regularDependabotSnapshot(ownedDependabot("\n", bodies.Disabled))
			plan, err = planDependabot(selection, owned, bodies)
			if err != nil || plan.Outcome != dependabotOutcomeReplace || !plan.GoEnabled {
				t.Fatalf("mode %s recut %v owned = %#v, %v", mode, recut, plan, err)
			}
			conflicting := missingDependabotSnapshot()
			conflicting.Alternate.Kind = dependabotDestinationRegular
			if _, err = planDependabot(selection, conflicting, bodies); err == nil {
				t.Fatalf("mode %s recut %v accepted alternate conflict", mode, recut)
			}
		}
	}
}

func TestPlanDependabotDeclarationTransitions(t *testing.T) {
	bodies := mustDependabotBodies(t)
	enabled := ownedDependabot("\n", bodies.Enabled)
	disabled := ownedDependabot("\n", bodies.Disabled)
	custom := ownedDependabot("\n", []byte("version: 2\ncustom-secret: do-not-report\n"))

	for _, tt := range []struct {
		name     string
		snapshot dependabotSnapshot
		declared bool
		enabled  bool
		outcome  dependabotOutcome
		selected bool
		wantErr  bool
	}{
		{name: "missing true", snapshot: missingDependabotSnapshot(), declared: true, enabled: true, outcome: dependabotOutcomeCreate, selected: true},
		{name: "missing false", snapshot: missingDependabotSnapshot(), declared: true, outcome: dependabotOutcomeCreate},
		{name: "missing absent", snapshot: missingDependabotSnapshot(), outcome: dependabotOutcomeCreate, selected: true},
		{name: "enabled to false", snapshot: regularDependabotSnapshot(enabled), declared: true, outcome: dependabotOutcomeReplace},
		{name: "enabled to true", snapshot: regularDependabotSnapshot(enabled), declared: true, enabled: true, outcome: dependabotOutcomeUnchanged, selected: true},
		{name: "enabled to absent", snapshot: regularDependabotSnapshot(enabled), outcome: dependabotOutcomeUnchanged, selected: true},
		{name: "disabled to true", snapshot: regularDependabotSnapshot(disabled), declared: true, enabled: true, outcome: dependabotOutcomeReplace, selected: true},
		{name: "disabled to false", snapshot: regularDependabotSnapshot(disabled), declared: true, outcome: dependabotOutcomeUnchanged},
		{name: "disabled to absent", snapshot: regularDependabotSnapshot(disabled), outcome: dependabotOutcomeUnchanged},
		{name: "custom to true", snapshot: regularDependabotSnapshot(custom), declared: true, enabled: true, outcome: dependabotOutcomeReplace, selected: true},
		{name: "custom to false", snapshot: regularDependabotSnapshot(custom), declared: true, outcome: dependabotOutcomeReplace},
		{name: "custom to absent", snapshot: regularDependabotSnapshot(custom), wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			selection := dependabotSelection{Present: true, Mode: swatch.FirstFit, GoDeclared: tt.declared, GoEnabled: tt.enabled}
			plan, err := planDependabot(selection, tt.snapshot, bodies)
			if tt.wantErr {
				assertDependabotConflict(t, err)
				if err != nil && strings.Contains(err.Error(), "do-not-report") {
					t.Fatal("conflict disclosed old content")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if plan.Outcome != tt.outcome || !plan.GoKnown || plan.GoEnabled != tt.selected {
				t.Fatalf("plan = %#v, want outcome %v and Go %v", plan, tt.outcome, tt.selected)
			}
		})
	}
}

func TestPlanDependabotOwnershipMarker(t *testing.T) {
	bodies := mustDependabotBodies(t)
	marker := managedMarker(dependabotPath)
	for _, tt := range []struct {
		name    string
		content []byte
		owned   bool
	}{
		{name: "LF", content: ownedDependabot("\n", bodies.Enabled), owned: true},
		{name: "CRLF", content: ownedDependabot("\r\n", bodies.Enabled), owned: true},
		{name: "BOM", content: append([]byte{0xef, 0xbb, 0xbf}, ownedDependabot("\n", bodies.Enabled)...)},
		{name: "leading blank", content: append([]byte("\n"), ownedDependabot("\n", bodies.Enabled)...)},
		{name: "partial", content: []byte("# Managed by Tailor: .github/dependabot\n")},
		{name: "wrong path", content: []byte("# Managed by Tailor: .github/dependabot.yaml\n")},
		{name: "extra text", content: []byte(marker + " extra\n")},
		{name: "missing newline", content: append([]byte(marker), bodies.Enabled...)},
		{name: "matching unmarked body", content: bodies.Enabled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			selection := dependabotSelection{Present: true, Mode: swatch.Always, GoDeclared: true, GoEnabled: true}
			plan, err := planDependabot(selection, regularDependabotSnapshot(tt.content), bodies)
			if err != nil {
				t.Fatal(err)
			}
			if tt.owned && plan.Outcome == dependabotOutcomePreserve {
				t.Fatal("exact marker was not accepted")
			}
			if !tt.owned && plan.Outcome != dependabotOutcomePreserve {
				t.Fatalf("outcome = %v, want preserve", plan.Outcome)
			}
		})
	}
}

func TestPlanDependabotRecognisesMarkerAndBodyNewlinesIndependently(t *testing.T) {
	bodies := mustDependabotBodies(t)
	fixtures := append([]dependabotCanonicalBody{
		{Content: bodies.Enabled, GoEnabled: true},
		{Content: bodies.Disabled, GoEnabled: false},
	}, bodies.Compatibility...)
	for fixtureIndex, fixture := range fixtures {
		state := map[bool]string{true: "enabled", false: "disabled"}[fixture.GoEnabled]
		state += "/fixture=" + string(rune('0'+fixtureIndex))
		for _, markerNewline := range []string{"\n", "\r\n"} {
			for _, bodyCRLF := range []bool{false, true} {
				name := state + "/marker=" + map[string]string{"\n": "LF", "\r\n": "CRLF"}[markerNewline] + "/body=" + map[bool]string{false: "LF", true: "CRLF"}[bodyCRLF]
				t.Run(name, func(t *testing.T) {
					body := fixture.Content
					if bodyCRLF {
						body = dependabotCRLF(body)
					}
					plan, err := planDependabot(
						dependabotSelection{Present: true, Mode: swatch.FirstFit},
						regularDependabotSnapshot(ownedDependabot(markerNewline, body)),
						bodies,
					)
					if err != nil {
						t.Fatal(err)
					}
					if !plan.GoKnown || plan.GoEnabled != fixture.GoEnabled {
						t.Fatalf("selected Go = %v, known = %v", plan.GoEnabled, plan.GoKnown)
					}
					want := dependabotOutcomeReplace
					if markerNewline == "\n" && !bodyCRLF {
						want = dependabotOutcomeUnchanged
					}
					if plan.Outcome != want {
						t.Fatalf("outcome = %v, want %v", plan.Outcome, want)
					}
					if bytes.Contains(plan.Output, []byte("\r")) {
						t.Fatal("planned output is not LF-only")
					}
					repeat, err := planDependabot(
						dependabotSelection{Present: true, Mode: swatch.FirstFit},
						regularDependabotSnapshot(plan.Output),
						bodies,
					)
					if err != nil || repeat.Outcome != dependabotOutcomeUnchanged || repeat.GoEnabled != fixture.GoEnabled {
						t.Fatalf("repeat plan = %#v, %v", repeat, err)
					}
				})
			}
		}
	}
}

func TestPlanDependabotRejectsUnknownAndAmbiguousOwnedBodiesWhenGoIsAbsent(t *testing.T) {
	bodies := mustDependabotBodies(t)
	for _, tt := range []struct {
		name string
		body []byte
	}{
		{name: "changed whitespace", body: bytes.Replace(bodies.Enabled, []byte("version: 2"), []byte("version:  2"), 1)},
		{name: "comment", body: append([]byte("# custom\n"), bodies.Enabled...)},
		{name: "trailing bytes", body: append(bytes.Clone(bodies.Enabled), []byte("\nextra: true\n")...)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := planDependabot(
				dependabotSelection{Present: true, Mode: swatch.FirstFit},
				regularDependabotSnapshot(ownedDependabot("\n", tt.body)),
				bodies,
			)
			assertDependabotConflict(t, err)
		})
	}

	ambiguous := dependabotBodies{
		Enabled:  []byte("same\n"),
		Disabled: []byte("disabled\n"),
		Compatibility: []dependabotCanonicalBody{
			{Content: []byte("same\n"), GoEnabled: false},
		},
	}
	_, err := planDependabot(
		dependabotSelection{Present: true, Mode: swatch.Always},
		regularDependabotSnapshot(ownedDependabot("\n", []byte("same\n"))),
		ambiguous,
	)
	assertDependabotConflict(t, err)
}

func TestPlanDependabotNormalisesOnceAndComparesCompleteFile(t *testing.T) {
	bodies := mustDependabotBodies(t)
	selection := dependabotSelection{Present: true, Mode: swatch.FirstFit}
	first, err := planDependabot(selection, regularDependabotSnapshot(ownedDependabot("\r\n", dependabotCRLF(bodies.Enabled))), bodies)
	if err != nil || first.Outcome != dependabotOutcomeReplace {
		t.Fatalf("first plan = %#v, %v", first, err)
	}
	second, err := planDependabot(selection, regularDependabotSnapshot(first.Output), bodies)
	if err != nil || second.Outcome != dependabotOutcomeUnchanged {
		t.Fatalf("second plan = %#v, %v", second, err)
	}
	changed := append(bytes.Clone(first.Output), '\n')
	third, err := planDependabot(dependabotSelection{Present: true, Mode: swatch.Always, GoDeclared: true, GoEnabled: true}, regularDependabotSnapshot(changed), bodies)
	if err != nil || third.Outcome != dependabotOutcomeReplace {
		t.Fatalf("complete-file comparison plan = %#v, %v", third, err)
	}
}

func TestPlanDependabotOutputLimit(t *testing.T) {
	markerBytes := len(managedMarker(dependabotPath)) + 1
	selection := dependabotSelection{Present: true, Mode: swatch.FirstFit, GoDeclared: true, GoEnabled: true}
	for _, tt := range []struct {
		name    string
		size    int
		wantErr bool
	}{
		{name: "at limit", size: dependabotMaxBytes - markerBytes},
		{name: "over limit", size: dependabotMaxBytes - markerBytes + 1, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := bytes.Repeat([]byte{'x'}, tt.size)
			plan, err := planDependabot(selection, missingDependabotSnapshot(), dependabotBodies{Enabled: body, Disabled: []byte("disabled\n")})
			if tt.wantErr {
				assertDependabotConflict(t, err)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Output) != dependabotMaxBytes {
				t.Fatalf("output size = %d, want %d", len(plan.Output), dependabotMaxBytes)
			}
		})
	}
}

func TestPlanDependabotConflictingDestinationStates(t *testing.T) {
	bodies := mustDependabotBodies(t)
	selection := dependabotSelection{Present: true, Mode: swatch.FirstFit}
	for _, tt := range []struct {
		name      string
		kind      dependabotDestinationKind
		alternate bool
	}{
		{name: "symlink", kind: dependabotDestinationSymlink},
		{name: "directory", kind: dependabotDestinationDirectory},
		{name: "special", kind: dependabotDestinationSpecial},
		{name: "invalid", kind: dependabotDestinationKind(255)},
		{name: "alternate", kind: dependabotDestinationMissing, alternate: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := missingDependabotSnapshot()
			snapshot.Destination.Kind = tt.kind
			if tt.alternate {
				snapshot.Alternate.Kind = dependabotDestinationRegular
			}
			_, err := planDependabot(selection, snapshot, bodies)
			assertDependabotConflict(t, err)
		})
	}
}

func TestPlanDependabotOwnsSnapshotBytes(t *testing.T) {
	bodies := mustDependabotBodies(t)
	content := ownedDependabot("\n", bodies.Enabled)
	snapshot := regularDependabotSnapshot(content)
	plan, err := planDependabot(dependabotSelection{Present: true, Mode: swatch.FirstFit, GoDeclared: true, GoEnabled: true}, snapshot, bodies)
	if err != nil {
		t.Fatal(err)
	}
	content[0] = 'X'
	if plan.Snapshot.Destination.Content[0] == 'X' {
		t.Fatal("plan aliases snapshot content")
	}
}

func mustDependabotBodies(t *testing.T) dependabotBodies {
	t.Helper()
	bodies, err := renderDependabotBodies()
	if err != nil {
		t.Fatal(err)
	}
	return bodies
}

func missingDependabotSnapshot() dependabotSnapshot {
	return dependabotSnapshot{
		Alternate:   dependabotDestinationSnapshot{Path: dependabotAlternatePath, Kind: dependabotDestinationMissing},
		Destination: dependabotDestinationSnapshot{Path: dependabotPath, Kind: dependabotDestinationMissing},
	}
}

func regularDependabotSnapshot(content []byte) dependabotSnapshot {
	snapshot := missingDependabotSnapshot()
	snapshot.Destination.Kind = dependabotDestinationRegular
	snapshot.Destination.Content = content
	return snapshot
}

func ownedDependabot(newline string, body []byte) []byte {
	content := []byte(managedMarker(dependabotPath) + newline)
	return append(content, body...)
}

func assertDependabotConflict(t *testing.T, err error) {
	t.Helper()
	var conflict *dependabotConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %v, want *dependabotConflictError", err)
	}
	if conflict.Reason == "" {
		t.Fatal("conflict reason is empty")
	}
}
