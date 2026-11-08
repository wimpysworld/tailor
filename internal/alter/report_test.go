package alter

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/output"
	"github.com/wimpysworld/tailor/internal/swatch"
	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestDisabledWikiCommandGuidance(t *testing.T) {
	text := wikiPreviewGuidance(&gh.WikiReadinessError{Reason: gh.WikiDisabled}, "https://github.com/owner/repo/wiki", "https://github.com/owner/repo.wiki.git", false)
	var report Report
	appendGuidance(&report, text, output.GuidanceRequired)
	if report.Plain != text || !strings.Contains(report.Document.Guidance[0].Text, "1. Run: `tailor alter`") {
		t.Fatal("wiki command markup changed legacy bytes or omitted emphasis")
	}
}

func TestRepeatedAlertsEnablementReport(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	const reason = "reapply enable request for Dependency Graph"
	for _, tc := range []struct {
		name       string
		mode       ApplyMode
		status     int
		wantWrites int32
		wantError  bool
		outcome    output.Outcome
	}{
		{"baste", DryRun, http.StatusNoContent, 0, false, output.Alteration},
		{"alter", Apply, http.StatusNoContent, 1, false, output.Applied},
		{"recut", Recut, http.StatusNoContent, 1, false, output.Applied},
		{"scope denied", Apply, http.StatusForbidden, 1, false, output.Attention},
		{"write failed", Apply, http.StatusUnprocessableEntity, 1, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var writes atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut && r.URL.Path == "/repos/owner/repo/vulnerability-alerts" {
					writes.Add(1)
					w.WriteHeader(tc.status)
					if tc.status != http.StatusNoContent {
						_, _ = io.WriteString(w, `{"message":"Resource not accessible by integration"}`)
					}
					return
				}
				if r.Method != http.MethodGet {
					t.Errorf("unexpected write: %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
					return
				}
				switch r.URL.Path {
				case "/repos/owner/repo":
					_, _ = io.WriteString(w, `{"permissions":{"admin":true}}`)
				case "/repos/owner/repo/actions/permissions/workflow":
					_, _ = io.WriteString(w, `{"default_workflow_permissions":"read"}`)
				case "/repos/owner/repo/private-vulnerability-reporting", "/repos/owner/repo/automated-security-fixes":
					_, _ = io.WriteString(w, `{"enabled":true}`)
				case "/repos/owner/repo/vulnerability-alerts":
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected read: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)
			cfg := &config.Config{Repository: &model.RepositorySettings{VulnerabilityAlertsEnabled: new(true)}}
			results, err := ProcessRepoSettings(cfg, tc.mode, RepoTarget{
				Client: testutil.NewTestClient(t, server), Owner: "owner", Name: "repo", HasRepo: true,
			})
			if (err != nil) != tc.wantError {
				t.Fatalf("ProcessRepoSettings() error = %v, want error = %t", err, tc.wantError)
			}
			if got := writes.Load(); got != tc.wantWrites {
				t.Errorf("writes = %d, want %d", got, tc.wantWrites)
			}
			command := stageLabel(tc.mode, "baste", "alter")
			report := buildReport(command, "owner/repo", results, nil, nil, nil, tc.mode)
			if tc.wantError {
				if len(report.Document.Items) != 0 || report.Plain != "" {
					t.Fatalf("failed write reported results: %#v", report)
				}
				return
			}
			if len(report.Document.Items) != 1 {
				t.Fatalf("items = %#v, want one item", report.Document.Items)
			}
			item := report.Document.Items[0]
			if item.Outcome != tc.outcome {
				t.Fatalf("outcome = %q, want %q", item.Outcome, tc.outcome)
			}
			if tc.outcome == output.Attention {
				if item.Action != "skip" || strings.Contains(report.Plain, "repository.vulnerability_alerts_enabled = true") || strings.Contains(report.Plain, reason) {
					t.Fatalf("denied write reported success: %#v", report)
				}
				return
			}
			if len(results) != 1 || results[0].Category != WouldSet || results[0].Before != "true" || results[0].Value != "true" || results[0].Annotation != reason {
				t.Fatalf("raw results = %#v", results)
			}
			wantAction := "set"
			wantValue := "true -> true"
			if tc.mode == DryRun {
				wantAction = "reapply"
				wantValue = "would reapply = true"
			}
			if item.Action != wantAction || item.Before != "true" || item.After != "true" || item.Reason != reason || item.Provenance != "repository.vulnerability_alerts_enabled" {
				t.Errorf("typed item = %#v", item)
			}
			rich := output.New(io.Discard, io.Discard, output.Auto, output.WithWidth(140), output.WithColor(output.ColorNever)).Render(report.Document)
			if !strings.Contains(rich, wantValue) || !strings.Contains(rich, reason) || (tc.mode == DryRun && strings.Contains(rich, "true -> true")) {
				t.Errorf("rich output omitted observed values or reason: %q", rich)
			}
			if tc.mode == DryRun && (report.Document.Summary.Alterations != 1 || report.Document.Summary.Applied != 0) {
				t.Errorf("dry-run summary = %#v", report.Document.Summary)
			}
			if tc.mode == DryRun {
				preview := buildReport(command, "owner/repo", results, nil, nil, []SwatchResult{
					{Path: "SECURITY.md", Category: WouldCopy},
					{Path: "SUPPORT.md", Category: WouldCopy},
				}, DryRun)
				const advice = "Review the preview, then run `tailor alter`."
				if got := preview.Document.Guidance[len(preview.Document.Guidance)-1].Text; got != advice {
					t.Fatalf("final advice = %q, want %q", got, advice)
				}
				for _, width := range []int{60, 80} {
					terminal := output.New(io.Discard, io.Discard, output.Auto, output.WithTTY(true), output.WithWidth(width), output.WithColor(output.ColorNever)).Render(preview.Document)
					for _, want := range []string{"Reapply requests", "1 reapply request, 2 alterations", "Next", advice} {
						if !strings.Contains(terminal, want) {
							t.Errorf("width %d: terminal preview omitted %q:\n%s", width, want, terminal)
						}
					}
					if strings.Contains(terminal, "3 alterations") {
						t.Errorf("width %d: terminal preview counted a reapply as an alteration:\n%s", width, terminal)
					}
				}
			}
		})
	}
}

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
	if got := report.Document.Guidance[len(report.Document.Guidance)-1].Text; got != "Resolve the items that need attention, then run `tailor baste` again." {
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
		"preserved":   {Outcome: output.Excluded, Action: "exclude"},
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

func TestBuildReportClassifiesProtectedIgnoreAsPreserved(t *testing.T) {
	result := SwatchResult{Path: ignoreRootPath, Category: NoChange, Reason: SkipManagedRootExists}
	for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
		report := buildReport("alter", "owner/repo", nil, nil, nil, []SwatchResult{result}, mode)
		if len(report.Document.Items) != 1 {
			t.Fatalf("mode %d items = %d, want 1", mode, len(report.Document.Items))
		}
		item := report.Document.Items[0]
		if item.Outcome != output.Preserved || item.Action != "preserve" || item.Reason != string(SkipManagedRootExists) {
			t.Errorf("mode %d item = %#v, want preserved ignore root", mode, item)
		}
		if strings.Contains(report.Plain, "overwrite") || strings.Contains(report.Plain, "copy") {
			t.Errorf("mode %d reports replacement: %q", mode, report.Plain)
		}
	}

	unrelated := SwatchResult{Path: "unrelated", Category: NoChange, Reason: SkipManagedRootExists}
	report := buildReport("alter", "owner/repo", nil, nil, nil, []SwatchResult{unrelated}, Apply)
	item := report.Document.Items[0]
	if item.Outcome != output.Unchanged || item.Action != "match" {
		t.Fatalf("unrelated item = %#v, want unchanged match", item)
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

func TestWikiGuidanceKeepsCommandsAndAppendOrder(t *testing.T) {
	report := buildReport("baste", "owner/repo", nil, nil, nil, []SwatchResult{{Path: "new.txt", Category: WouldCopy}}, DryRun)
	appendManagedGuidance(&report, []string{"Verify the preserved justfile."})
	appendDependabotReporting(&report, dependabotPlan{}, []SwatchResult{{Path: dependabotPath, Category: Skipped, Reason: ManagedNotOwned}})
	wiki := wikiPreviewGuidance(&gh.WikiReadinessError{Reason: gh.WikiBaselineMissing}, "https://github.com/owner/repo/wiki", "https://github.com/owner/repo.wiki.git", true)
	appendGuidance(&report, wiki, output.GuidanceRequired)
	if !strings.Contains(report.Plain, wiki) {
		t.Fatal("plain report lost wiki guidance")
	}
	if len(report.Document.Guidance) != 4 {
		t.Fatalf("guidance blocks = %#v", report.Document.Guidance)
	}
	for i, block := range report.Document.Guidance[1:] {
		if block.Order != i+2 {
			t.Fatalf("append order = %d, want %d", block.Order, i+2)
		}
	}
	for _, width := range []int{60, 80} {
		got := output.New(nil, nil, output.Auto, output.WithWidth(width)).Render(report.Document)
		last := -1
		for _, text := range []string{"Verify the preserved justfile.", "Optional: adopt", "Use `tailor baste --verbose`", "Next steps:", "1. Import the wiki:", "Run tailor baste to inspect the import plan after the wiki can be read safely.", "Run tailor alter. Tailor imports the wiki automatically.", "Tailor preserves local-only files and backs up replacements in private Git metadata.", "2. Run `tailor baste` again."} {
			index := strings.Index(strings.Join(strings.Fields(got), " "), text)
			if index <= last {
				t.Fatalf("width %d: missing or out-of-order %q:\n%s", width, text, got)
			}
			last = index
		}
		if strings.Contains(got, "Review the preview") || strings.Contains(got, "1. Next steps:") || strings.Contains(got, "rsync") || strings.Contains(got, "git clone") {
			t.Fatalf("nested numbering or unsafe final advice:\n%s", got)
		}
	}
}

func TestOptionalDependabotGuidanceKeepsNextAction(t *testing.T) {
	for _, changes := range []bool{false, true} {
		results := []SwatchResult{{Path: dependabotPath, Category: Skipped, Reason: ManagedNotOwned}}
		want := "No changes needed."
		if changes {
			results = append(results, SwatchResult{Path: "new.txt", Category: WouldCopy})
			want = "Review the preview, then run `tailor alter`."
		}
		report := buildReport("baste", "owner/repo", nil, nil, nil, results, DryRun)
		appendDependabotReporting(&report, dependabotPlan{}, results)
		if report.Document.Guidance[1].Kind != output.GuidanceOptional {
			t.Fatalf("adoption is not optional: %#v", report.Document.Guidance)
		}
		got := output.New(nil, nil, output.Auto, output.WithWidth(120)).Render(report.Document)
		if !strings.Contains(got, want) || !strings.Contains(got, "Optional: adopt") || strings.Contains(got, "need attention") {
			t.Fatalf("incorrect optional advice:\n%s", got)
		}
		if strings.Index(got, want) < strings.Index(got, "Optional: adopt") {
			t.Fatalf("next action must follow optional advice:\n%s", got)
		}
	}
}

func TestAppendGuidanceKeepsPlainAndTypedReports(t *testing.T) {
	report := Report{Plain: "results\n"}
	appendGuidance(&report, "Next steps:\nRun `tailor baste`.\n", output.GuidanceRequired)
	if report.Plain != "results\nNext steps:\nRun `tailor baste`.\n" {
		t.Fatalf("plain guidance = %q", report.Plain)
	}
	if len(report.Document.Guidance) != 1 || report.Document.Guidance[0].Text != "Next steps:\nRun `tailor baste`." {
		t.Fatalf("typed guidance = %#v", report.Document.Guidance)
	}
}
