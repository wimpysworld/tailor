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
	Auto  Format = "auto"
	Plain Format = "plain"
)

// Color controls ANSI colour independently from the output format.
type Color string

const (
	ColorAuto   Color = "auto"
	ColorAlways Color = "always"
	ColorNever  Color = "never"
)

// Outcome controls visibility, colour, badges, and summary counts.
type Outcome string

const (
	Alteration Outcome = "alteration"
	Applied    Outcome = "applied"
	Attention  Outcome = "needs attention"
	Preserved  Outcome = "preserved by policy"
	Unchanged  Outcome = "already matches"
	Created    Outcome = "created"
)

// Item is one typed result. Presentation never derives state from display text.
type Item struct {
	Domain, Category, OutcomeLabel, Action, Name, Before, After, Reason, Provenance string
	Outcome                                                                         Outcome
}

// StageEvent reports execution progress without display text parsing.
type StageEvent struct {
	ID, Label, Phase, Item string
	Current, Total         int
	Err                    error
}

// Notice and Guidance keep non-result information structured.
type (
	Notice   struct{ Level, Text string }
	Guidance struct {
		Order int
		Text  string
	}
)

// Summary contains explicit totals.
type Summary struct {
	Alterations, Applied, Attention, Preserved, Unchanged int
}

// Document is the renderer-independent command result.
type Document struct {
	Command, Context string
	Items            []Item
	Notices          []Notice
	Guidance         []Guidance
	Summary          Summary
}

// Option changes terminal detection and rendering for a Policy.
type Option func(*Policy)

func WithTTY(tty bool) Option       { return func(p *Policy) { p.tty = tty } }
func WithStderrTTY(tty bool) Option { return func(p *Policy) { p.stderrTTY = tty } }
func WithWidth(width int) Option    { return func(p *Policy) { p.width = width } }
func WithColor(color Color) Option  { return func(p *Policy) { p.color = color } }
func WithVerbose(v bool) Option     { return func(p *Policy) { p.verbose = v } }
func WithQuiet(v bool) Option       { return func(p *Policy) { p.quiet = v } }
func WithNoProgress(v bool) Option  { return func(p *Policy) { p.noProgress = v } }
func WithASCII(v bool) Option       { return func(p *Policy) { p.ascii = v } }

// Policy owns command writers and independent layout, colour, and progress decisions.
type Policy struct {
	stdout, stderr                                    io.Writer
	format                                            Format
	color                                             Color
	tty, stderrTTY, verbose, quiet, noProgress, ascii bool
	width                                             int
}

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

func (p *Policy) Stdout() io.Writer { return p.stdout }
func (p *Policy) Stderr() io.Writer { return p.stderr }
func (p *Policy) Rich() bool        { return p.format == Auto && p.tty && os.Getenv("TERM") != "dumb" }

func (p *Policy) StartProgress() *Progress {
	if p.format == Plain {
		return nil
	}
	animate := p.ProgressEnabled(false) && !p.verbose
	if !animate && (!p.verbose || p.quiet) {
		return nil
	}
	return startProgress(p.stderr, progressConfig{animate: animate, verbose: p.verbose, colour: p.colourEnabled(), ascii: p.ascii})
}

func (p *Policy) ProgressEnabled(noProgress bool) bool {
	return !noProgress && !p.noProgress && p.Rich() && p.stderrTTY && os.Getenv("TERM") != "dumb"
}

// Print writes the typed document, or the exact legacy bytes outside rich mode.
func (p *Policy) Print(doc Document, plain string) {
	if p.quiet {
		summary := doc.Summary
		if summary == (Summary{}) {
			summary = Count(doc.Items)
		}
		fmt.Fprintln(p.stdout, summaryLine(summary))
		return
	}
	if !p.Rich() {
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
	if p.color == ColorNever {
		return false
	}
	if p.color == ColorAlways {
		return true
	}
	return os.Getenv("NO_COLOR") == ""
}

func (p *Policy) style(text, colour string) string {
	if !p.colourEnabled() {
		return text
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(colour)).Bold(true).Render(text)
}

// Render creates responsive change-first cards from explicit typed values.
func (p *Policy) Render(doc Document) string {
	doc.Command = termtext.EscapeControlText(doc.Command)
	doc.Context = termtext.EscapeControlText(doc.Context)
	var b strings.Builder
	summary := doc.Summary
	if summary == (Summary{}) {
		summary = Count(doc.Items)
	}
	if p.quiet {
		fmt.Fprintf(&b, "%s\n", summaryLine(summary))
		return b.String()
	}
	b.WriteString(p.card("TAILOR · "+strings.ToUpper(doc.Command), []string{doc.Context, p.richSummaryLine(summary)}, "5"))
	b.WriteString("\n")
	for _, outcome := range []Outcome{Alteration, Applied, Created, Attention, Preserved, Unchanged} {
		items := filter(doc.Items, outcome)
		if len(items) == 0 {
			continue
		}
		if !p.verbose && (outcome == Preserved || outcome == Unchanged) {
			b.WriteString(p.summaryCard(outcome, items))
		} else {
			b.WriteString(p.itemsCard(outcome, items))
		}
		b.WriteString("\n")
	}
	for _, notice := range doc.Notices {
		fmt.Fprintf(&b, "%s: %s\n", notice.Level, termtext.EscapeControlText(notice.Text))
	}
	sort.SliceStable(doc.Guidance, func(i, j int) bool { return doc.Guidance[i].Order < doc.Guidance[j].Order })
	for _, guidance := range doc.Guidance {
		b.WriteString(termtext.EscapeControlText(guidance.Text) + "\n")
	}
	return b.String()
}

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
		case Unchanged:
			s.Unchanged++
		}
	}
	return s
}

func summaryLine(s Summary) string {
	parts := []string{}
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
		parts = append(parts, fmt.Sprintf("%d preserved by policy", s.Preserved))
	}
	if s.Unchanged > 0 {
		parts = append(parts, fmt.Sprintf("%d already match", s.Unchanged))
	}
	if len(parts) == 0 {
		return "No results."
	}
	return strings.Join(parts, " · ")
}

func (p *Policy) richSummaryLine(s Summary) string {
	parts := []string{}
	if s.Alterations > 0 {
		parts = append(parts, p.style(fmt.Sprintf("%d", s.Alterations), outcomeColour(Alteration))+" alteration"+plural(s.Alterations))
	}
	if s.Applied > 0 {
		parts = append(parts, p.style(fmt.Sprintf("%d", s.Applied), outcomeColour(Applied))+" applied")
	}
	if s.Attention > 0 {
		parts = append(parts, p.style(fmt.Sprintf("%d", s.Attention), outcomeColour(Attention))+" need attention")
	}
	if s.Preserved > 0 {
		parts = append(parts, p.style(fmt.Sprintf("%d", s.Preserved), outcomeColour(Preserved))+" preserved by policy")
	}
	if s.Unchanged > 0 {
		parts = append(parts, p.style(fmt.Sprintf("%d", s.Unchanged), outcomeColour(Unchanged))+" already match")
	}
	if len(parts) == 0 {
		return "No results."
	}
	return strings.Join(parts, " · ")
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func filter(items []Item, outcome Outcome) []Item {
	var out []Item
	for _, i := range items {
		if i.Outcome == outcome {
			out = append(out, i)
		}
	}
	return out
}

func (p *Policy) summaryCard(outcome Outcome, items []Item) string {
	counts := map[string]int{}
	if outcome == Preserved {
		for _, i := range items {
			counts[i.Reason]++
		}
	} else {
		for _, i := range items {
			counts[i.Domain]++
		}
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	entries := make([]string, 0, len(keys))
	for _, key := range keys {
		count := counts[key]
		if key == "" {
			key = "Other"
		}
		if outcome == Preserved {
			key = preservedReason(key, count)
		}
		entries = append(entries, p.style(fmt.Sprintf("%3d", count), outcomeColour(outcome))+"  "+key)
	}
	lines := entries
	if outcome == Unchanged && p.width >= 72 {
		columns := 3
		cellWidth := (p.width - 4) / columns
		lines = lines[:0]
		for i := 0; i < len(entries); i += columns {
			var row strings.Builder
			for j := 0; j < columns && i+j < len(entries); j++ {
				cell := entries[i+j]
				row.WriteString(cell)
				if i+j+1 < len(entries) && j < columns-1 {
					padding := max(cellWidth-lipgloss.Width(cell), 2)
					row.WriteString(strings.Repeat(" ", padding))
				}
			}
			lines = append(lines, row.String())
		}
	}
	if outcome == Preserved {
		lines = append(lines, "", "Tailor will leave these files untouched.", "Run `tailor baste --verbose` to list them.")
	}
	return p.card(strings.ToUpper(string(outcome))+fmt.Sprintf(" · %d", len(items)), lines, outcomeColour(outcome))
}

func preservedReason(reason string, count int) string {
	switch reason {
	case "first-fit, exists":
		if count == 1 {
			return "first-fit file already exists"
		}
		return "first-fit files already exist"
	case "mode never":
		if count == 1 {
			return "file has mode never"
		}
		return "files have mode never"
	default:
		return reason
	}
}

func (p *Policy) itemsCard(outcome Outcome, items []Item) string {
	lines := []string{}
	last := ""
	for _, item := range items {
		group := termtext.EscapeControlText(item.Domain)
		if item.Category != "" {
			group += " › " + termtext.EscapeControlText(item.Category)
		}
		if group != last {
			if last != "" {
				lines = append(lines, "")
			}
			lines = append(lines, group)
			last = group
		}
		symbol := map[Outcome]string{Alteration: "~", Applied: "ok", Attention: "!", Preserved: "=", Unchanged: "ok", Created: "+"}[outcome]
		value := termtext.EscapeControlText(item.After)
		if item.Before != "" {
			value = termtext.EscapeControlText(item.Before) + " → " + value
		}
		action := termtext.EscapeControlText(item.Action)
		if action != "" && action != "match" {
			switch {
			case value == "":
				value = action
			case action == "set" || action == "create" || action == "update":
				value = action + " to " + value
			default:
				value = action + " " + value
			}
		}
		line := p.style(symbol, outcomeColour(outcome)) + " " + termtext.EscapeControlText(item.Name)
		if value != "" && p.width >= 72 {
			line += "  " + p.style(value, outcomeColour(outcome))
		} else if value != "" {
			lines = append(lines, line)
			line = "  " + p.style(value, outcomeColour(outcome))
		}
		lines = append(lines, line)
		if item.Provenance != "" {
			lines = append(lines, "  "+termtext.EscapeControlText(item.Provenance))
		}
		if item.Reason != "" {
			lines = append(lines, "  "+termtext.EscapeControlText(item.Reason))
		}
	}
	return p.card(strings.ToUpper(string(outcome))+fmt.Sprintf(" · %d", len(items)), lines, outcomeColour(outcome))
}

func outcomeColour(outcome Outcome) string {
	switch outcome {
	case Applied, Created, Unchanged:
		return "2"
	case Attention:
		return "3"
	case Preserved:
		return "5"
	case Alteration:
		return "6"
	}
	return "7"
}

func (p *Policy) card(title string, lines []string, colour string) string {
	width := max(p.width, 4)
	inner := width - 4
	clean := make([]string, 0, len(lines))
	for _, line := range lines {
		if line != "" || len(clean) > 0 {
			clean = append(clean, line)
		}
	}
	border := lipgloss.RoundedBorder()
	if p.ascii || os.Getenv("TERM") == "dumb" {
		border = lipgloss.Border{Top: "-", Bottom: "-", Left: "|", Right: "|", TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+"}
	}
	body := p.style(title, colour)
	if len(clean) > 0 {
		body += "\n" + strings.Join(clean, "\n")
	}
	style := lipgloss.NewStyle().Width(inner).Padding(0, 1).Border(border)
	if p.colourEnabled() {
		style = style.BorderForeground(lipgloss.Color(colour))
	}
	return style.Render(body) + "\n"
}
