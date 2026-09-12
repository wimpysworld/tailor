package alter

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/output"
	"github.com/wimpysworld/tailor/internal/swatch"
	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestBuildReportClassifiesSetupSkipsAndAddsBasteGuidance(t *testing.T) {
	report := buildReport("baste", "owner/repo", []RepoSettingResult{
		{Section: "immutable_releases", Field: "enabled", Category: WouldSkipSetup, Annotation: "enforced by owner"},
		{Section: "code_scanning", Field: "state", Category: WouldSkipSetup, Annotation: "not available"},
		{Section: "repository", Field: "has_wiki", Category: WouldSet, Value: "true"},
	}, nil, nil, nil, DryRun)

	if got := report.Document.Items[0].Outcome; got != output.Preserved {
		t.Fatalf("owner-enforced outcome = %q, want %q", got, output.Preserved)
	}
	if got := report.Document.Items[1].Outcome; got != output.Attention {
		t.Fatalf("unavailable outcome = %q, want %q", got, output.Attention)
	}
	if got := report.Document.Guidance[len(report.Document.Guidance)-1].Text; got != "No changes made. Run `tailor alter` to apply 1 alteration." {
		t.Fatalf("baste guidance = %q", got)
	}
}

func TestBuildReportConvertsEveryResultCategory(t *testing.T) {
	report := buildReport("baste", "owner/repo", []RepoSettingResult{
		{Section: "repository", Field: "description", Category: WouldSet, Before: "old", Value: "new"},
		{Section: "repository", Field: "homepage", Category: RepoNoChange, Before: "same", Value: "same"},
		{Section: "actions", Field: "enabled", Category: WouldSkipScope, Annotation: "scope"},
		{Section: "code_scanning", Field: "state", Category: WouldSkipSetup, Annotation: "not available"},
		{Section: "immutable_releases", Field: "enabled", Category: WouldSkipSetup, Annotation: "enforced by owner"},
	}, []LabelResult{
		{Name: "create", Category: WouldCreate, Value: "#111111"},
		{Name: "update", Category: WouldUpdate, Before: "#111111", Value: "#222222"},
		{Name: "match", Category: LabelNoChange, Before: "#333333", Value: "#333333"},
		{Name: "skip", Category: LabelSkipScope, Annotation: "scope"},
	}, []VariableResult{
		{Name: "CREATE", Category: WouldCreate, After: `"new"`, Value: `"new"`},
		{Name: "UPDATE", Category: WouldUpdate, Before: `"old"`, After: `"new"`, Value: `"old" -> "new"`},
		{Name: "MATCH", Category: LabelNoChange, After: `"same"`, Value: `"same"`},
		{Name: "SKIP", Category: LabelSkipScope, Annotation: "scope"},
	}, []SwatchResult{
		{Path: "copy", Category: WouldCopy},
		{Path: "overwrite", Category: WouldOverwrite},
		{Path: "remove", Category: WouldRemove},
		{Path: ".tailor.yml", Category: WouldUpdateConfig},
		{Path: "match-file", Category: NoChange},
		{Path: "preserved", Category: Skipped, Reason: SkipModeNever},
	}, DryRun)

	want := map[string]output.Item{
		"description": {Outcome: output.Alteration, Action: "set", Before: "old", After: "new"},
		"homepage":    {Outcome: output.Unchanged, Action: "match", Before: "same", After: "same"},
		"enabled":     {Outcome: output.Preserved, Action: "preserve"},
		"create":      {Outcome: output.Alteration, Action: "create", After: "#111111"},
		"update":      {Outcome: output.Alteration, Action: "update", Before: "#111111", After: "#222222"},
		"match":       {Outcome: output.Unchanged, Action: "match", Before: "#333333", After: "#333333"},
		"CREATE":      {Outcome: output.Alteration, Action: "create", After: `"new"`},
		"UPDATE":      {Outcome: output.Alteration, Action: "update", Before: `"old"`, After: `"new"`},
		"MATCH":       {Outcome: output.Unchanged, Action: "match", After: `"same"`},
		"copy":        {Outcome: output.Alteration, Action: "copy"},
		"overwrite":   {Outcome: output.Alteration, Action: "overwrite"},
		"remove":      {Outcome: output.Alteration, Action: "remove"},
		".tailor.yml": {Outcome: output.Alteration, Action: "update"},
		"match-file":  {Outcome: output.Unchanged, Action: "match"},
		"preserved":   {Outcome: output.Preserved, Action: "preserve"},
	}
	seenAttention := 0
	for _, item := range report.Document.Items {
		if item.Outcome == output.Attention {
			seenAttention++
			continue
		}
		expected, ok := want[item.Name]
		if !ok {
			t.Fatalf("unexpected item: %#v", item)
		}
		if item.Outcome != expected.Outcome || item.Action != expected.Action || item.Before != expected.Before || item.After != expected.After {
			t.Errorf("item %q = %#v, want outcome=%q action=%q before=%q after=%q", item.Name, item, expected.Outcome, expected.Action, expected.Before, expected.After)
		}
		delete(want, item.Name)
	}
	if seenAttention != 4 || len(want) != 0 {
		t.Fatalf("attention=%d remaining=%v", seenAttention, want)
	}
}

func TestBuildReportConvertsCompletedWritesToApplied(t *testing.T) {
	report := buildReport("alter", "owner/repo",
		[]RepoSettingResult{{Field: "has_wiki", Category: WouldSet, Before: "false", Value: "true"}},
		[]LabelResult{{Name: "bug", Category: WouldCreate, Value: "#ff0000"}, {Name: "docs", Category: WouldUpdate, Before: "#000000", Value: "#ffffff"}},
		[]VariableResult{{Name: "ONE", Category: WouldCreate, After: `"1"`}, {Name: "TWO", Category: WouldUpdate, Before: `"1"`, After: `"2"`}},
		[]SwatchResult{{Path: "copy", Category: WouldCopy}, {Path: "overwrite", Category: WouldOverwrite}, {Path: "remove", Category: WouldRemove}, {Path: ".tailor.yml", Category: WouldUpdateConfig}}, Apply)
	for _, item := range report.Document.Items {
		if item.Outcome != output.Applied {
			t.Errorf("%s outcome = %q, want %q", item.Name, item.Outcome, output.Applied)
		}
	}
}

func TestResultComparerKeepsAvailableBeforeValues(t *testing.T) {
	declaredString, liveString := "new", "old"
	declaredBool, liveBool := true, false
	declaredInt, liveInt := 30, 14
	declaredSet, liveSet := []string{"new"}, []string{"old"}
	comparer := resultComparer{section: "test"}
	comparer.str("string", &declaredString, &liveString)
	comparer.boolean("boolean", &declaredBool, &liveBool)
	comparer.enabled("enabled", &declaredBool, &liveBool)
	comparer.count("count", &declaredInt, &liveInt)
	comparer.set("set", &declaredSet, &liveSet)
	for _, result := range comparer.results {
		if result.Before == "" {
			t.Errorf("%s omitted available before value", result.Field)
		}
	}
	comparer = resultComparer{section: "test"}
	comparer.str("unknown", &declaredString, nil)
	if comparer.results[0].Before != "" {
		t.Fatalf("fabricated unavailable before value %q", comparer.results[0].Before)
	}
}

func TestExecuteEmitsEveryAlterStage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"login":"tailor"}`)
	}))
	defer server.Close()
	var events []output.StageEvent
	_, err := Execute(&config.Config{}, t.TempDir(), DryRun, testutil.NewTestClient(t, server), io.Discard, Options{Observer: func(event output.StageEvent) {
		events = append(events, event)
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"config", "auth", "pages-preflight", "wiki-preflight", "retired-workflows", "repository", "labels", "variables", "pages", "wiki", "licence", "swatches", "complete"} {
		if !slices.ContainsFunc(events, func(event output.StageEvent) bool { return event.ID == id }) {
			t.Errorf("missing %s stage: %#v", id, events)
		}
	}
	for _, event := range events {
		if strings.Contains(event.Label, "Measuring") {
			t.Errorf("dry-run stage uses mutation-ambiguous label %q", event.Label)
		}
	}
	if !slices.ContainsFunc(events, func(event output.StageEvent) bool { return event.Label == "Reading GitHub settings" }) ||
		!slices.ContainsFunc(events, func(event output.StageEvent) bool { return event.Label == "Planning swatches" }) {
		t.Fatalf("dry-run labels are not mode-accurate: %#v", events)
	}

	events = nil
	_, err = Execute(&config.Config{}, t.TempDir(), Apply, testutil.NewTestClient(t, server), io.Discard, Options{Observer: func(event output.StageEvent) {
		events = append(events, event)
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"Applying GitHub settings", "Applying labels", "Applying variables", "Applying Pages changes", "Applying wiki changes", "Writing licence", "Writing swatches"} {
		if !slices.ContainsFunc(events, func(event output.StageEvent) bool { return event.Label == label }) {
			t.Errorf("missing mutation label %q: %#v", label, events)
		}
	}
}

func TestExecuteRetainsLicenceResult(t *testing.T) {
	for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
		for _, failSwatch := range []bool{false, true} {
			name := stageLabel(mode, "dry-run", "apply")
			if mode == Recut {
				name = "recut"
			}
			if failSwatch {
				name += "/swatch-failure"
			} else {
				name += "/success"
			}
			t.Run(name, func(t *testing.T) {
				dir := t.TempDir()
				if failSwatch {
					if err := os.Mkdir(filepath.Join(dir, "SECURITY.md"), 0o755); err != nil {
						t.Fatal(err)
					}
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/user":
						_, _ = io.WriteString(w, `{"login":"tailor"}`)
					case "/licenses/MIT":
						_, _ = io.WriteString(w, `{"body":"MIT licence text"}`)
					default:
						t.Errorf("unexpected request: %s", r.URL.Path)
						http.NotFound(w, r)
					}
				}))
				defer server.Close()
				cfg := &config.Config{License: "MIT", Swatches: []config.SwatchEntry{{Path: "SECURITY.md", Alteration: swatch.FirstFit}}}
				report, err := Execute(cfg, dir, mode, testutil.NewTestClient(t, server), io.Discard, Options{})
				if failSwatch {
					if err == nil || !strings.Contains(err.Error(), "SECURITY.md") {
						t.Fatalf("expected swatch failure, got %v", err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
				count := 0
				for _, item := range report.Document.Items {
					if item.Name == licenceDestination {
						count++
						want := output.Alteration
						if mode.ShouldWrite() {
							want = output.Applied
						}
						if item.Outcome != want || item.Action != "copy" {
							t.Errorf("licence item = %#v", item)
						}
					}
				}
				if count != 1 || strings.Count(report.Plain, licenceDestination) != 1 {
					t.Fatalf("expected one licence result, got %d typed and plain %q", count, report.Plain)
				}
				if mode.ShouldWrite() {
					data, err := os.ReadFile(filepath.Join(dir, licenceDestination))
					if err != nil || string(data) != "MIT licence text" {
						t.Fatalf("licence file = %q, error = %v", data, err)
					}
				}
			})
		}
	}
}

func TestAppendGuidanceKeepsPlainAndTypedReports(t *testing.T) {
	report := Report{Plain: "results\n"}
	appendGuidance(&report, "Next steps:\nRun `tailor baste`.\n")
	if report.Plain != "results\nNext steps:\nRun `tailor baste`.\n" {
		t.Fatalf("plain guidance = %q", report.Plain)
	}
	if len(report.Document.Guidance) != 2 || report.Document.Guidance[1].Text != "Run `tailor baste`." {
		t.Fatalf("typed guidance = %#v", report.Document.Guidance)
	}
}
