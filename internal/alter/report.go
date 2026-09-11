package alter

import (
	"strconv"
	"strings"

	"github.com/wimpysworld/tailor/internal/output"
)

// Report retains typed and plain forms so legacy output remains exact.
type Report struct {
	Document output.Document
	Plain    string
}

func buildReport(command, context string, repo []RepoSettingResult, labels []LabelResult, variables []VariableResult, swatches []SwatchResult, mode ApplyMode) Report {
	if mode.ShouldWrite() {
		repo = removeSkipped(repo, repoSkippedKind, repoActionKind)
		labels = removeSkipped(labels, labelSkippedName, labelActionName)
	}
	doc := output.Document{Command: command, Context: context}
	for _, r := range repo {
		outcome := output.Unchanged
		action := "match"
		switch r.Category {
		case WouldSet:
			if mode.ShouldWrite() {
				outcome, action = output.Applied, "set"
			} else {
				outcome, action = output.Alteration, "set"
			}
		case WouldSkipScope:
			outcome, action = output.Attention, "skip"
		case WouldSkipSetup:
			outcome, action = output.Attention, "skip"
			if r.Annotation == "enforced by owner" {
				outcome, action = output.Preserved, "preserve"
			}
		}
		name := r.Field
		if name == "" {
			name = r.Operation.String()
		}
		domain, category := repoDomain(r.Section, r.Field)
		doc.Items = append(doc.Items, output.Item{Domain: domain, Category: category, Outcome: outcome, Action: action, Name: displayName(name), Before: r.Before, After: r.Value, Reason: r.Annotation, Provenance: qualified(r.Section, r.Field)})
	}
	for _, r := range labels {
		outcome, action := output.Unchanged, "match"
		if r.Category == WouldCreate || r.Category == WouldUpdate {
			if mode.ShouldWrite() {
				outcome = output.Applied
			} else {
				outcome = output.Alteration
			}
			action = strings.TrimPrefix(string(r.Category), "would ")
		}
		if r.Category == LabelSkipScope {
			outcome, action = output.Attention, "skip"
		}
		name := r.Name
		if name == "" {
			name = r.Operation.String()
		}
		doc.Items = append(doc.Items, output.Item{Domain: "Labels", Outcome: outcome, Action: action, Name: name, Before: r.Before, After: r.Value, Reason: r.Annotation, Provenance: qualified("label", r.Name)})
	}
	for _, r := range variables {
		outcome, action := output.Unchanged, "match"
		if r.Category == WouldCreate || r.Category == WouldUpdate {
			if mode.ShouldWrite() {
				outcome = output.Applied
			} else {
				outcome = output.Alteration
			}
			action = strings.TrimPrefix(string(r.Category), "would ")
		}
		if r.Category == LabelSkipScope {
			outcome, action = output.Attention, "skip"
		}
		name := r.Name
		if name == "" {
			name = r.Operation.String()
		}
		after := r.After
		if after == "" {
			after = r.Value
		}
		doc.Items = append(doc.Items, output.Item{Domain: "Variables", Outcome: outcome, Action: action, Name: name, Before: r.Before, After: after, Reason: r.Annotation, Provenance: qualified("variable", r.Name)})
	}
	for _, r := range swatches {
		outcome, action := output.Unchanged, "match"
		switch r.Category {
		case WouldCopy, WouldOverwrite, WouldRemove, WouldUpdateConfig:
			if mode.ShouldWrite() {
				outcome = output.Applied
			} else {
				outcome = output.Alteration
			}
			action = strings.TrimPrefix(string(r.Category), "would ")
		case Skipped:
			outcome, action = output.Preserved, "preserve"
		}
		doc.Items = append(doc.Items, output.Item{Domain: "Files", Outcome: outcome, Action: action, Name: r.Path, Reason: string(r.Reason), Provenance: r.Path})
	}
	doc.Summary = output.Count(doc.Items)
	if mode == DryRun {
		if doc.Summary.Alterations > 0 {
			doc.Guidance = append(doc.Guidance, output.Guidance{Order: 1000, Text: "No changes made. Run `tailor alter` to apply " + alterationCount(doc.Summary.Alterations) + "."})
		} else {
			doc.Guidance = append(doc.Guidance, output.Guidance{Order: 1000, Text: "No changes needed."})
		}
	}
	return Report{Document: doc, Plain: FormatOutput(repo, labels, variables, swatches, mode)}
}

func alterationCount(count int) string {
	if count == 1 {
		return "1 alteration"
	}
	return strconv.Itoa(count) + " alterations"
}

func repoDomain(section, field string) (string, string) {
	switch section {
	case "actions":
		return "Actions", "Policy"
	case "code_scanning":
		return "Code analysis", "Code scanning"
	case "code_quality":
		return "Code analysis", "Code quality"
	case "ruleset":
		return "Ruleset", "Branch rules"
	case "pages":
		return "Pages", "Publishing"
	}
	if strings.Contains(field, "vulnerability") || strings.Contains(field, "security") || strings.Contains(field, "secret_scanning") {
		return "Repository", "Security"
	}
	return "Repository", "Settings"
}
func displayName(name string) string { return strings.ReplaceAll(name, "_", " ") }
func qualified(section, field string) string {
	if section == "" {
		section = "repository"
	}
	if field == "" {
		return ""
	}
	return section + "." + field
}
