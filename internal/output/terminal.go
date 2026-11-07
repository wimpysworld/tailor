package output

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/wimpysworld/tailor/internal/termtext"
)

// Terminal colours use the user's palette, without background queries.
const (
	identityColour  = "5"
	changeColour    = "6"
	attentionColour = "3"
	neutralColour   = "4"
)

func (p *Policy) code(text string) string {
	if !p.colourEnabled() {
		return text
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(identityColour)).Bold(true).TabWidth(-1).Render(text)
}

func safeSnippet(text string) string {
	parts := strings.Split(text, "\t")
	for i := range parts {
		parts[i] = termtext.EscapeControlText(parts[i])
	}
	return strings.Join(parts, "\t")
}

// inlineCode styles explicit code spans only. Delimiters remain copy-visible in monochrome.
func (p *Policy) inlineCode(text string) string {
	var b strings.Builder
	for {
		start := strings.IndexByte(text, '`')
		if start < 0 {
			b.WriteString(text)
			break
		}
		end := strings.IndexByte(text[start+1:], '`')
		if end < 0 {
			b.WriteString(text)
			break
		}
		end += start + 2
		b.WriteString(text[:start])
		b.WriteString(p.code(text[start:end]))
		text = text[end:]
	}
	return b.String()
}

func (p *Policy) muted(text string) string {
	if !p.colourEnabled() {
		return text
	}
	return lipgloss.NewStyle().Faint(true).Render(text)
}

func (p *Policy) renderTerminal(doc Document) string {
	var b strings.Builder
	command := termtext.EscapeControlText("tailor " + doc.Command)
	context := termtext.EscapeControlText(doc.Context)
	gap := p.width - lipgloss.Width(command) - lipgloss.Width(context)
	if gap >= 2 {
		fmt.Fprintln(&b, p.code(command)+strings.Repeat(" ", gap)+p.muted(context))
	} else {
		fmt.Fprintln(&b, p.code(command))
		fmt.Fprintln(&b, p.muted(context))
	}
	mode := map[string]string{
		"baste":   "[PREVIEW] No changes made",
		"measure": "[READ-ONLY] Inspection only",
		"docket":  "[READ-ONLY] Inspection only",
		"alter":   "[RESULTS] Confirmed changes only",
		"fit":     "[CREATED] Created configuration",
	}[doc.Command]
	badge, detail, _ := strings.Cut(mode, " ")
	if p.colourEnabled() {
		badge = lipgloss.NewStyle().Bold(true).Reverse(true).Render(badge)
	}
	if lipgloss.Width(badge+" "+detail) > p.width {
		fmt.Fprintln(&b, badge)
		p.line(&b, detail, "", "")
	} else {
		fmt.Fprintln(&b, badge+" "+detail)
	}
	b.WriteByte('\n')
	for _, notice := range doc.Notices {
		p.line(&b, notice.Level+": "+notice.Text, "", "  ")
	}
	if len(doc.Notices) > 0 {
		b.WriteByte('\n')
	}
	attention := filter(doc.Items, Attention)
	if len(attention) > 0 {
		p.resultPanel(&b, "Needs attention", attention, attentionColour, false)
	}
	if p.verbose {
		p.domainPanels(&b, doc.Items)
	} else {
		var changes, reapply, unchanged []Item
		for _, item := range doc.Items {
			switch item.Outcome {
			case Alteration, Applied, Created:
				if item.Action == "reapply" {
					reapply = append(reapply, item)
				} else {
					changes = append(changes, item)
				}
			case Unchanged:
				if doc.Command == "docket" {
					unchanged = append(unchanged, item)
				}
			}
		}
		title := "Proposed changes"
		if doc.Command != "baste" {
			title = "Results"
		}
		if len(changes) > 0 {
			p.resultPanel(&b, title, changes, changeColour, true)
		}
		if len(reapply) > 0 {
			p.resultPanel(&b, "Reapply requests", reapply, neutralColour, true)
		}
		if len(unchanged) > 0 {
			p.resultPanel(&b, "Identity", unchanged, neutralColour, true)
		}
	}
	summary := doc.Summary
	if summary == (Summary{}) {
		summary = Count(doc.Items)
	}
	p.terminalSummary(&b, summary, doc.Items)
	if !p.verbose && len(doc.Items) > 0 {
		p.line(&b, "Details: `tailor "+doc.Command+" --verbose`", "", "")
	}
	var advice strings.Builder
	advicePolicy := *p
	if p.width >= 60 {
		advicePolicy.width = p.width - 4
	}
	next := doc
	next.Guidance = nil
	other := doc
	other.Guidance = nil
	for _, g := range doc.Guidance {
		if g.Kind == GuidanceOptional || g.Kind == GuidanceInfo {
			other.Guidance = append(other.Guidance, g)
		} else {
			next.Guidance = append(next.Guidance, g)
		}
	}
	if len(other.Guidance) > 0 {
		var notes strings.Builder
		p.guidance(&notes, other)
		for line := range strings.SplitSeq(strings.TrimSuffix(notes.String(), "\n"), "\n") {
			text := ansi.Strip(line)
			if strings.HasPrefix(text, "    ") || strings.HasPrefix(text, "\t") {
				fmt.Fprintln(&b, p.code(text))
			} else {
				fmt.Fprintln(&b, p.inlineCode(p.muted(text)))
			}
		}
	}
	advicePolicy.guidance(&advice, next)
	if text := strings.Trim(advice.String(), "\n"); text != "" {
		b.WriteByte('\n')
		p.panel(&b, "Next", text, identityColour)
	}
	return b.String()
}

func (p *Policy) terminalSummary(b *strings.Builder, summary Summary, items []Item) {
	reapply := 0
	for _, item := range items {
		if item.Outcome == Alteration && item.Action == "reapply" {
			reapply++
		}
	}
	summary.Alterations = max(0, summary.Alterations-reapply)
	var parts []string
	for _, metric := range []struct {
		count         int
		label, colour string
	}{
		{reapply, "reapply request" + plural(reapply), neutralColour},
		{summary.Alterations, "alteration" + plural(summary.Alterations), changeColour},
		{summary.Applied, "applied", changeColour},
		{summary.Attention, "need attention", attentionColour},
		{summary.Preserved, "preserved", ""},
		{summary.Excluded, "excluded", ""},
		{summary.Unchanged, "unchanged", ""},
	} {
		if metric.count > 0 {
			text := fmt.Sprintf("%d %s", metric.count, metric.label)
			if metric.colour != "" {
				text = p.style(text, metric.colour)
			}
			parts = append(parts, text)
		}
	}
	if len(parts) == 0 {
		fmt.Fprintln(b, p.style("No results.", changeColour))
		return
	}
	width := 0
	for i, part := range parts {
		if i < len(parts)-1 {
			part += ","
		}
		cells := lipgloss.Width(part)
		if width > 0 {
			if width+1+cells > p.width {
				b.WriteByte('\n')
				width = 0
			} else {
				b.WriteByte(' ')
				width++
			}
		}
		b.WriteString(part)
		width += cells
	}
	b.WriteByte('\n')
}

// domainPanels retains input order within each domain and every result in verbose mode.
func (p *Policy) domainPanels(b *strings.Builder, items []Item) {
	var domains []string
	groups := map[string][]Item{}
	for _, item := range items {
		if item.Outcome == Attention {
			continue
		}
		domain := terminalDomain(item)
		if _, exists := groups[domain]; !exists {
			domains = append(domains, domain)
		}
		groups[domain] = append(groups[domain], item)
	}
	for _, domain := range domains {
		colour := ""
		for _, item := range groups[domain] {
			if item.Outcome == Alteration || item.Outcome == Applied || item.Outcome == Created {
				if item.Action == "reapply" && colour != changeColour {
					colour = neutralColour
				} else if item.Action != "reapply" {
					colour = changeColour
				}
			}
		}
		p.resultPanel(b, domain, groups[domain], colour, false)
	}
}

func terminalDomain(item Item) string {
	if item.Category == "Security" {
		return "Security"
	}
	if item.Domain != "" {
		return item.Domain
	}
	return "Results"
}

func terminalStatus(item Item) string {
	if item.Action == "reapply" {
		return "Reapply requests"
	}
	switch item.Outcome {
	case Alteration:
		return "Proposed"
	case Applied:
		return "Applied"
	case Created:
		return "Created"
	case Preserved:
		return "Preserved"
	case Excluded:
		return "Excluded"
	case Unchanged:
		return "Already matches"
	case Attention:
		return "Needs attention"
	}
	return string(item.Outcome)
}

func (p *Policy) resultPanel(b *strings.Builder, title string, items []Item, colour string, domains bool) {
	inner := *p
	if p.width >= 60 {
		inner.width = p.width - 4
	}
	var content strings.Builder
	last := ""
	for _, item := range items {
		heading := terminalStatus(item)
		if domains {
			heading = terminalDomain(item)
		}
		if heading != last && heading != title {
			if last != "" && p.verbose {
				content.WriteByte('\n')
			}
			c := colour
			if item.Action == "reapply" {
				c = neutralColour
			}
			if item.Outcome == Unchanged || item.Outcome == Preserved || item.Outcome == Excluded {
				c = ""
			}
			fmt.Fprintln(&content, p.panelStyle(heading, c))
		}
		last = heading
		inner.terminalItem(&content, item)
	}
	p.panel(b, title, strings.TrimSuffix(content.String(), "\n"), colour)
	b.WriteByte('\n')
}

func (p *Policy) terminalItem(b *strings.Builder, item Item) {
	name := item.Provenance
	if !p.verbose && item.Name != "" {
		name = item.Name
		if label, ok := terminalLabels[item.Provenance]; ok {
			name = label
		}
	}
	if name == "" {
		name = item.Name
	}
	// Keep typed category identity when readable names overlap between services.
	if !p.verbose && item.Category != "" && item.Category != "Settings" && item.Category != "Security" {
		name = item.Category + ": " + name
	}
	name = termtext.EscapeControlText(name)
	action := item.Action
	if action == "match" || action == "set" || action == "preserve" || action == "exclude" {
		action = ""
	}
	if item.Outcome == Alteration && action != "" {
		action = "would " + action
	}
	if item.Outcome == Applied || item.Outcome == Created {
		if past, ok := map[string]string{"copy": "copied", "overwrite": "overwritten", "remove": "removed", "update": "updated", "create": "created"}[action]; ok {
			action = past
		}
	}
	if action != "" {
		name += ": " + termtext.EscapeControlText(action)
	}
	valueColour := changeColour
	if item.Action == "reapply" {
		valueColour = neutralColour
	}
	if item.Outcome == Unchanged || item.Outcome == Preserved || item.Outcome == Excluded {
		valueColour = ""
	}
	value := p.panelStyle(termtext.EscapeControlText(item.After), valueColour)
	if item.Before != "" && item.Action != "reapply" && (item.Outcome != Unchanged || item.Before != item.After) {
		arrow := " → "
		if p.ascii {
			arrow = " -> "
		}
		value = p.muted(termtext.EscapeControlText(item.Before)) + arrow + value
	}
	if item.After != "" || item.Before != "" {
		if p.width < 76 || lipgloss.Width(name+" = "+value) > p.width {
			p.line(b, name, "", "")
			fmt.Fprintln(b, "  "+value)
		} else {
			fmt.Fprintln(b, name+" = "+value)
		}
	} else {
		p.line(b, name, "", "")
	}
	if item.Reason != "" {
		p.line(b, item.Reason, "  ", "  ")
	}
}

var terminalLabels = map[string]string{ //nolint:gosec // Public setting names, not credentials.
	"repository.secret_scanning_non_provider_patterns": "Non-provider secret patterns",
	"repository.vulnerability_alerts_enabled":          "Vulnerability alerts",
	"pages.https_enforced":                             "HTTPS enforcement",
}

func (p *Policy) panelStyle(text, colour string) string {
	if colour == "" {
		return p.muted(text)
	}
	return p.style(text, colour)
}

// panel never clips copyable data. Oversized blocks use a labelled, unboxed layout.
func (p *Policy) panel(b *strings.Builder, title, content, colour string) {
	lines := strings.Split(content, "\n")
	fits := p.width >= 60 && lipgloss.Width(title)+6 <= p.width
	for _, line := range lines {
		if lipgloss.Width(line) > p.width-4 || strings.ContainsRune(line, '\t') || strings.HasPrefix(ansi.Strip(line), "    ") {
			fits = false
		}
	}
	if !fits {
		fmt.Fprintln(b, p.panelStyle(title, colour))
		fmt.Fprintln(b, content)
		return
	}
	tl, tr, bl, br, h, v := "╭", "╮", "╰", "╯", "─", "│"
	if p.ascii {
		tl, tr, bl, br, h, v = "+", "+", "+", "+", "-", "|"
	}
	top := tl + h + " " + title + " " + strings.Repeat(h, p.width-lipgloss.Width(title)-5) + tr
	fmt.Fprintln(b, p.panelStyle(top, colour))
	for _, line := range lines {
		fmt.Fprintln(b, p.panelStyle(v, colour)+" "+line+strings.Repeat(" ", p.width-4-lipgloss.Width(line))+" "+p.panelStyle(v, colour))
	}
	fmt.Fprintln(b, p.panelStyle(bl+strings.Repeat(h, p.width-2)+br, colour))
}
