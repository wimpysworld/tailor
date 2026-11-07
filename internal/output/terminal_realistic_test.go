package output

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// realisticDocument reproduces the mixed statuses and long fields of a full preview.
func realisticDocument() Document {
	doc := terminalDocument()
	doc.Items = append(doc.Items,
		Item{Domain: "Repository", Outcome: Unchanged, Action: "match", Provenance: "repository.topics", AfterItems: []string{"community-health-files", "github-community-health", "github-config", "github-actions", "github-api", "github-security", "github-label-management", "github-templates", "github-automation", "github-repository-management", "yaml-configuration", "github-settings"}},
		Item{Domain: "Actions", Outcome: Unchanged, Action: "match", Provenance: "actions.patterns_allowed", AfterItems: []string{"freerangebytes/setup-actionlint@*", "golang/govulncheck-action@*", "golangci/golangci-lint-action@*", "nick-fields/retry@*", "robherley/go-test-action@*", "softprops/action-gh-release@*", "owner/action@{one,two}"}},
		Item{Domain: "Ruleset", Outcome: Unchanged, Action: "match", Provenance: "ruleset.rules.pull_request.parameters.require_extra_approval_for_unattributed_changes", After: "true"},
		Item{Domain: "Ruleset", Outcome: Unchanged, Action: "match", Provenance: "ruleset.rules.pull_request.parameters.required_review_thread_resolution", After: "true"},
		Item{Domain: "Pages", Outcome: Alteration, Action: "set", Provenance: "pages.https_enforced", Before: "false", After: "true"},
		Item{Domain: "Other", Outcome: Unchanged, Action: "match", Name: "Long exact key", Provenance: "custom." + strings.Repeat("qualified_identifier_", 6), After: "true"},
		Item{Domain: "Files", Outcome: Applied, Action: "update", Name: "applied.txt"},
		Item{Domain: "Files", Outcome: Created, Action: "create", Name: "created.txt"},
		Item{Domain: "Wiki", Outcome: Attention, Name: "Wiki readiness", Reason: "Review with `git status --short`."},
	)
	for i := range 24 {
		doc.Items = append(doc.Items, Item{Domain: "Files", Outcome: Preserved, Name: fmt.Sprintf("preserved/%02d.yml", i), Reason: "existing shared settings preserved"}, Item{Domain: "Files", Outcome: Unchanged, Name: fmt.Sprintf("matching/%02d.yml", i)}, Item{Domain: "Files", Outcome: Excluded, Name: fmt.Sprintf("excluded/%02d.yml", i), Reason: "mode never"})
	}
	return doc
}

func TestRealisticTerminalReport(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	doc := realisticDocument()
	for _, width := range []int{60, 80, 100} {
		for _, ascii := range []bool{false, true} {
			for _, colour := range []Color{ColorAlways, ColorNever} {
				t.Run(fmt.Sprintf("%d/%v/%s", width, ascii, colour), func(t *testing.T) {
					p := New(nil, nil, Auto, WithTTY(true), WithWidth(width), WithVerbose(true), WithASCII(ascii), WithColor(colour))
					got := p.Render(doc)
					clean := ansi.Strip(got)
					if colour == ColorNever && got != clean {
						t.Fatal("ANSI in monochrome")
					}
					for _, title := range []string{"Repository", "Actions", "Ruleset", "Files", "Other"} {
						marker := "╭─ " + title + " "
						if ascii {
							marker = "+- " + title + " "
						}
						if !strings.Contains(clean, marker) {
							t.Errorf("missing stable frame %s", title)
						}
					}
					filesMarker, endMarker := "╭─ Files ", "\n╰"
					if ascii {
						filesMarker, endMarker = "+- Files ", "\n+"
					}
					_, files, ok := strings.Cut(clean, filesMarker)
					if !ok {
						t.Fatal("missing Files panel")
					}
					files, _, ok = strings.Cut(files, endMarker)
					if !ok {
						t.Fatal("missing Files panel closing frame")
					}
					for _, group := range []struct {
						title string
						want  int
					}{
						{"Proposed", 1},
						{"Applied", 1},
						{"Created", 1},
						{"Reapply requests", 0},
						{"Already matches", 1},
						{"Preserved", 1},
						{"Excluded", 1},
					} {
						if got := strings.Count(files, group.title); got != group.want {
							t.Errorf("Files subgroup %q count = %d, want %d", group.title, got, group.want)
						}
					}
					if strings.Count(clean, "existing shared settings preserved") != 1 {
						t.Fatal("repeated preservation reason")
					}
					if strings.Index(clean, " Security ") > strings.Index(clean, " Repository ") {
						t.Fatal("matching domain before changes")
					}
					if strings.Index(clean, " Next ") > strings.Index(clean, "Optional:") {
						t.Fatal("optional advice before Next")
					}
					for _, item := range doc.Items {
						for _, value := range append(append([]string{}, item.BeforeItems...), item.AfterItems...) {
							if !strings.Contains(clean, value) {
								t.Errorf("lost list item %q", value)
							}
						}
						key := item.Provenance
						if key == "" {
							key = item.Name
						}
						prefix := "ruleset.rules.pull_request.parameters."
						if strings.HasPrefix(key, prefix) {
							key = strings.TrimPrefix(key, prefix)
							if !strings.Contains(clean, strings.TrimSuffix(prefix, ".")) {
								t.Fatal("lost qualified prefix")
							}
						}
						if !strings.Contains(clean, key) {
							t.Errorf("lost key %q", key)
						}
					}
					for line := range strings.SplitSeq(got, "\n") {
						plain := ansi.Strip(line)
						framed := strings.HasPrefix(plain, "│") || strings.HasPrefix(plain, "╭") || strings.HasPrefix(plain, "╰")
						if ascii {
							framed = strings.HasPrefix(plain, "|") || strings.HasPrefix(plain, "+")
						}
						if framed && lipgloss.Width(line) != width {
							t.Errorf("frame width %q", plain)
						}
						if framed && colour == ColorAlways {
							if !strings.HasPrefix(line, "\x1b[") || strings.HasPrefix(line, "\x1b[2m") {
								t.Errorf("uncoloured or faint frame %q", line)
							}
						}
					}
					if !strings.Contains(got, p.code("`git status --short`")) {
						t.Fatal("wiki command not highlighted")
					}
				})
			}
		}
	}
	for _, format := range []Format{Auto, Plain} {
		var out strings.Builder
		p := New(&out, nil, format, WithTTY(false), WithVerbose(true), WithColor(ColorAlways))
		p.Print(doc, "legacy rows\n")
		if strings.Contains(out.String(), "\x1b") {
			t.Fatal("ANSI in pipe")
		}
		if format == Plain && out.String() != "legacy rows\n" {
			t.Fatal("legacy contract changed")
		}
	}
}
