// Package output renders typed command results and controls terminal progress.
package output

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/wimpysworld/tailor/internal/termtext"
	"golang.org/x/term"
)

// Format selects the command output format.
type Format string

const (
	// Auto uses concise reports on terminals and redirected streams.
	Auto Format = "auto"
	// Plain uses the command's stable text format without live progress.
	Plain Format = "plain"
)

// Color controls ANSI colour independently from the output format.
type Color string

const (
	// ColorAuto honours NO_COLOR.
	ColorAuto Color = "auto"
	// ColorAlways enables colour on capable terminals regardless of NO_COLOR.
	ColorAlways Color = "always"
	// ColorNever disables ANSI colour.
	ColorNever Color = "never"
)

// Outcome controls visibility, colour, and summary counts.
type Outcome string

const (
	// Alteration identifies a planned change.
	Alteration Outcome = "alteration"
	// Applied identifies a completed change.
	Applied Outcome = "applied"
	// Attention identifies a result that needs user action.
	Attention Outcome = "needs attention"
	// Preserved identifies an item left unchanged by policy.
	Preserved Outcome = "preserved by policy"
	// Excluded identifies an item excluded from management.
	Excluded Outcome = "excluded"
	// Unchanged identifies an item that already matches the declared state.
	Unchanged Outcome = "already matches"
	// Created identifies a new item and counts towards the applied total.
	Created Outcome = "created"
)

// Item is one typed result. Presentation never derives state from display text.
type Item struct {
	Domain, Category, OutcomeLabel, Action, Name, Before, After, Reason, Provenance string
	Outcome                                                                         Outcome
	// List values retain item boundaries without parsing display strings.
	BeforeItems, AfterItems []string
}

// StageEvent reports execution progress without display text parsing.
type StageEvent struct {
	ID, Label, Phase, Item string
	Current, Total         int
	Err                    error
}

// GuidanceKind controls whether advice replaces the final next action.
type GuidanceKind int

const (
	// GuidanceRequired identifies manual edits or verification steps.
	GuidanceRequired GuidanceKind = iota
	// GuidanceOptional identifies advice that does not block the next action.
	GuidanceOptional
	// GuidanceInfo identifies context that does not require action.
	GuidanceInfo
	// GuidanceFinal identifies the next action after the report.
	GuidanceFinal
)

// Notice and Guidance keep non-result information structured.
type (
	Notice   struct{ Level, Text string }
	Guidance struct {
		Order int
		Text  string
		Kind  GuidanceKind
	}
)

// Summary contains explicit totals.
type (
	Summary struct{ Alterations, Applied, Attention, Preserved, Unchanged, Excluded int }
	// Document is the renderer-independent command result.
	Document struct {
		Command, Context string
		Items            []Item
		Notices          []Notice
		Guidance         []Guidance
		Summary          Summary
	}
)

// Option changes terminal detection and rendering for a Policy.
type Option func(*Policy)

// WithTTY overrides terminal detection for standard output.
func WithTTY(tty bool) Option { return func(p *Policy) { p.tty = tty } }

// WithStderrTTY overrides terminal detection for standard error.
func WithStderrTTY(tty bool) Option { return func(p *Policy) { p.stderrTTY = tty } }

// WithWidth sets the available number of terminal columns.
func WithWidth(width int) Option { return func(p *Policy) { p.width = width } }

// WithColor sets the ANSI colour policy independently of layout.
func WithColor(color Color) Option { return func(p *Policy) { p.color = color } }

// WithVerbose expands result groups without changing live progress.
func WithVerbose(v bool) Option { return func(p *Policy) { p.verbose = v } }

// WithQuiet limits output to totals and essential guidance.
func WithQuiet(v bool) Option { return func(p *Policy) { p.quiet = v } }

// WithNoProgress disables animation without changing final output.
func WithNoProgress(v bool) Option { return func(p *Policy) { p.noProgress = v } }

// WithASCII selects ASCII spinner frames.
func WithASCII(v bool) Option { return func(p *Policy) { p.ascii = v } }

// Policy owns command writers and independent layout, colour, and progress decisions.
type Policy struct {
	stdout, stderr                                    io.Writer
	format                                            Format
	color                                             Color
	tty, stderrTTY, verbose, quiet, noProgress, ascii bool
	width                                             int
}

// New detects terminal capabilities, then applies options in order.
// Nil writers use the corresponding process streams.
func New(stdout, stderr io.Writer, format Format, options ...Option) *Policy {
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	p := &Policy{stdout: stdout, stderr: stderr, format: format, color: ColorAuto, width: 80, ascii: !unicodeLocale()}
	if f, ok := stdout.(*os.File); ok {
		p.tty = term.IsTerminal(int(f.Fd()))
		if width, _, err := term.GetSize(int(f.Fd())); err == nil {
			p.width = width
		}
	}
	if f, ok := stderr.(*os.File); ok {
		p.stderrTTY = term.IsTerminal(int(f.Fd()))
	}
	for _, option := range options {
		option(p)
	}
	return p
}

// Stdout returns the writer for durable command results.
func (p *Policy) Stdout() io.Writer { return p.stdout }

// Stderr returns the writer for progress, warnings and errors.
func (p *Policy) Stderr() io.Writer { return p.stderr }

// Rich reports whether automatic format and terminal capabilities permit colour and progress.
func (p *Policy) Rich() bool { return p.format == Auto && p.tty && os.Getenv("TERM") != "dumb" }

// StartProgress selects live animation on capable terminals.
func (p *Policy) StartProgress() *Progress {
	if p.format == Plain {
		return nil
	}
	if !p.ProgressEnabled(false) || p.quiet {
		return nil
	}
	return startProgress(p.stderr, progressConfig{animate: true, colour: p.colourEnabled(), ascii: p.ascii})
}

// ProgressEnabled checks terminal and progress flags.
func (p *Policy) ProgressEnabled(noProgress bool) bool {
	return !noProgress && !p.noProgress && p.Rich() && p.stderrTTY && os.Getenv("TERM") != "dumb"
}

// Print writes a concise report unless the caller explicitly selects legacy plain output.
func (p *Policy) Print(doc Document, plain string) {
	if p.format == Plain && !p.quiet {
		fmt.Fprint(p.stdout, plain)
		return
	}
	fmt.Fprint(p.stdout, p.Render(doc))
}

func unicodeLocale() bool {
	locale := os.Getenv("LC_ALL")
	if locale == "" {
		locale = os.Getenv("LC_CTYPE")
	}
	if locale == "" {
		locale = os.Getenv("LANG")
	}
	locale = strings.ToLower(locale)
	return strings.Contains(locale, "utf-8") || strings.Contains(locale, "utf8")
}

func (p *Policy) colourEnabled() bool {
	if !p.Rich() || p.color == ColorNever {
		return false
	}
	return p.color == ColorAlways || os.Getenv("NO_COLOR") == ""
}

func (p *Policy) style(text, colour string) string {
	if !p.colourEnabled() {
		return text
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(colour)).Bold(true).Render(text)
}

// Render selects terminal layout without changing report state.
func (p *Policy) Render(doc Document) string {
	if p.Rich() && !p.quiet {
		return p.renderTerminal(doc)
	}
	var b strings.Builder
	summary := doc.Summary
	if summary == (Summary{}) {
		summary = Count(doc.Items)
	}
	if !p.quiet {
		p.line(&b, "tailor "+doc.Command+" - "+doc.Context, "", "")
		mode := map[string]string{"baste": "Preview - no changes made", "measure": "Read-only inspection", "docket": "Read-only inspection", "alter": "Results - confirmed changes only", "fit": "Created configuration"}[doc.Command]
		if mode != "" {
			p.line(&b, mode, "", "")
		}
		b.WriteByte('\n')
	}
	for _, notice := range doc.Notices {
		p.line(&b, notice.Level+": "+notice.Text, "", "  ")
	}
	for _, outcome := range []Outcome{Attention, Alteration, Applied, Created, Preserved, Excluded, Unchanged} {
		items := filter(doc.Items, outcome)
		if len(items) == 0 || (p.quiet && outcome != Attention) {
			continue
		}
		if !p.verbose && (outcome == Preserved || outcome == Excluded || (outcome == Unchanged && doc.Command != "docket")) {
			continue
		}
		title := map[Outcome]string{Attention: "Needs attention", Alteration: "Planned changes", Applied: "Applied", Created: "Created", Preserved: "Preserved", Excluded: "Excluded", Unchanged: "Unchanged"}[outcome]
		fmt.Fprintln(&b, p.style(title, outcomeColour(outcome)))
		for _, item := range items {
			p.item(&b, item)
		}
		b.WriteByte('\n')
	}
	p.line(&b, summaryLine(summary), "", "")
	if !p.quiet && !p.verbose {
		for _, outcome := range []Outcome{Preserved, Excluded} {
			counts := map[string]int{}
			for _, item := range filter(doc.Items, outcome) {
				if item.Reason != "" {
					counts[item.Reason]++
				}
			}
			keys := make([]string, 0, len(counts))
			for key := range counts {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			parts := make([]string, 0, len(keys))
			for _, key := range keys {
				parts = append(parts, reasonSummary(counts[key], key))
			}
			if len(parts) > 0 {
				p.line(&b, strings.Join(parts, "; "), "  ", "  ")
			}
		}
	}
	p.guidance(&b, doc)
	return b.String()
}

func (p *Policy) item(b *strings.Builder, item Item) {
	name := item.Provenance
	if name == "" {
		name = item.Name
	}
	action := item.Action
	if item.Outcome == Alteration && action != "" {
		action = "would " + action
	}
	if item.Outcome == Applied || item.Outcome == Created {
		if past, ok := map[string]string{"set": "set", "copy": "copied", "overwrite": "overwritten", "remove": "removed", "update": "updated", "create": "created"}[action]; ok {
			action = past
		}
	}
	if action == "match" || (item.Action == "set" && item.Outcome == Alteration) {
		action = ""
	}
	line := name
	if action != "" {
		line += ": " + action
	}
	value := item.After
	if item.Before != "" && item.Action != "reapply" {
		value = item.Before + " -> " + value
	}
	if value != "" {
		line += " = " + value
	}
	p.line(b, line, "  ", "    ")
	if item.Reason != "" {
		p.line(b, item.Reason, "    ", "    ")
	}
}

func (p *Policy) guidance(b *strings.Builder, doc Document) {
	guidance := append([]Guidance(nil), doc.Guidance...)
	sort.SliceStable(guidance, func(i, j int) bool {
		if (guidance[i].Kind == GuidanceFinal) != (guidance[j].Kind == GuidanceFinal) {
			return guidance[j].Kind == GuidanceFinal
		}
		return guidance[i].Order < guidance[j].Order
	})
	seen := map[string]bool{}
	manual := false
	for _, g := range guidance {
		if g.Kind == GuidanceRequired && strings.TrimSpace(g.Text) != "" {
			manual = true
		}
	}
	for _, g := range guidance {
		text := strings.Trim(g.Text, "\n")
		if strings.TrimSpace(text) == "" || seen[text] {
			continue
		}
		seen[text] = true
		// Final advice is subordinate to manual checks appended after report construction.
		if g.Kind == GuidanceFinal && manual {
			continue
		}
		if p.quiet && ((g.Kind == GuidanceFinal && Count(doc.Items).Attention == 0) || g.Kind == GuidanceOptional || g.Kind == GuidanceInfo) {
			continue
		}
		if g.Kind == GuidanceOptional && !p.verbose {
			if first, _, multiline := strings.Cut(text, "\n"); multiline {
				command := "--verbose"
				if doc.Command != "" {
					command = "tailor " + doc.Command + " --verbose"
				}
				text = first + "\nUse `" + command + "` for adoption steps and safeguards."
			}
		}
		b.WriteByte('\n')
		for line := range strings.SplitSeq(text, "\n") {
			// Indented source snippets are copyable and must not be reflowed.
			if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
				fmt.Fprintln(b, p.code(safeSnippet(line)))
				continue
			}
			prose := strings.TrimLeft(line, " ")
			indent := line[:len(line)-len(prose)]
			p.line(b, prose, indent, indent)
		}
	}
}

// line wraps prose at spaces, keeping backtick commands and long paths intact.
func (p *Policy) line(b *strings.Builder, text, first, rest string) {
	text = termtext.EscapeControlText(text)
	var tokens []string
	var token strings.Builder
	quoted := false
	for _, r := range text {
		if r == '`' {
			quoted = !quoted
		}
		if r == ' ' && !quoted {
			if token.Len() > 0 {
				tokens = append(tokens, token.String())
				token.Reset()
			}
		} else {
			token.WriteRune(r)
		}
	}
	if token.Len() > 0 {
		tokens = append(tokens, token.String())
	}
	line, prefix := first, first
	for _, word := range tokens {
		sep := ""
		if line != prefix {
			sep = " "
		}
		if line != prefix && lipgloss.Width(line+sep+word) > max(p.width, 20) {
			fmt.Fprintln(b, p.inlineCode(line))
			line, prefix, sep = rest, rest, ""
		}
		line += sep + word
	}
	fmt.Fprintln(b, p.inlineCode(line))
}

// Count totals recognised outcomes, counting Created as Applied.
func Count(items []Item) Summary {
	var s Summary
	for _, item := range items {
		switch item.Outcome {
		case Alteration:
			s.Alterations++
		case Applied, Created:
			s.Applied++
		case Attention:
			s.Attention++
		case Preserved:
			s.Preserved++
		case Excluded:
			s.Excluded++
		case Unchanged:
			s.Unchanged++
		}
	}
	return s
}

func summaryLine(s Summary) string {
	var parts []string
	if s.Alterations > 0 {
		parts = append(parts, fmt.Sprintf("%d alteration%s", s.Alterations, plural(s.Alterations)))
	}
	if s.Applied > 0 {
		parts = append(parts, fmt.Sprintf("%d applied", s.Applied))
	}
	if s.Attention > 0 {
		parts = append(parts, fmt.Sprintf("%d need attention", s.Attention))
	}
	if s.Preserved > 0 {
		parts = append(parts, fmt.Sprintf("%d preserved", s.Preserved))
	}
	if s.Excluded > 0 {
		parts = append(parts, fmt.Sprintf("%d excluded", s.Excluded))
	}
	if s.Unchanged > 0 {
		parts = append(parts, fmt.Sprintf("%d unchanged", s.Unchanged))
	}
	if len(parts) == 0 {
		return "No results."
	}
	return strings.Join(parts, ", ")
}

func reasonSummary(count int, reason string) string {
	switch reason {
	case "first-fit, exists":
		if count == 1 {
			return "1 first-fit file already exists"
		}
		return fmt.Sprintf("%d first-fit files already exist", count)
	case "mode never":
		return fmt.Sprintf("%d file%s excluded by mode never", count, plural(count))
	}
	return fmt.Sprintf("%d %s", count, reason)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func filter(items []Item, outcome Outcome) []Item {
	var out []Item
	for _, item := range items {
		if item.Outcome == outcome {
			out = append(out, item)
		}
	}
	return out
}

func outcomeColour(outcome Outcome) string {
	switch outcome {
	case Applied, Created, Unchanged:
		return "2"
	case Attention:
		return "3"
	case Preserved, Excluded:
		return "5"
	case Alteration:
		return "6"
	}
	return "7"
}
