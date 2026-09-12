package alter

import (
	"errors"
	"fmt"
	"strings"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/model"
)

type VariableResult struct {
	Name       string
	Category   LabelCategory
	Value      string
	Before     string
	After      string
	Annotation string
	Operation  gh.Operation
}

func ProcessVariables(cfg *config.Config, mode ApplyMode, target RepoTarget) ([]VariableResult, error) {
	if len(cfg.Variables) == 0 || target.missingRepo("Variables") {
		return nil, nil
	}
	current, err := gh.ReadVariables(target.Client, target.Owner, target.Name)
	if err != nil {
		if scope, ok := errors.AsType[*gh.ErrInsufficientScope](err); ok {
			return []VariableResult{{Category: LabelSkipScope, Annotation: skipAnnotation, Operation: scope.Operation}}, nil
		}
		return nil, err
	}
	results := compareVariables(cfg.Variables, current)
	if !mode.ShouldWrite() {
		return results, nil
	}
	ar, err := gh.ApplyVariables(target.Client, target.Owner, target.Name, cfg.Variables, current)
	return completedVariables(results, ar), err
}

func compareVariables(desired, current []model.VariableEntry) []VariableResult {
	byName := make(map[string]model.VariableEntry, len(current))
	for _, variable := range current {
		byName[strings.ToLower(variable.Name)] = variable
	}
	results := make([]VariableResult, 0, len(desired))
	for _, variable := range desired {
		existing, found := byName[strings.ToLower(variable.Name)]
		after := fmt.Sprintf("%q", *variable.Value)
		result := VariableResult{Name: variable.Name, Category: LabelNoChange, Value: after, After: after}
		switch {
		case !found:
			result.Category = WouldCreate
		case *existing.Value != *variable.Value:
			result.Category = WouldUpdate
			result.Before = fmt.Sprintf("%q", *existing.Value)
			result.Value = fmt.Sprintf("%q -> %q", *existing.Value, *variable.Value)
		}
		results = append(results, result)
	}
	return results
}

func completedVariables(results []VariableResult, ar *gh.ApplyResult) []VariableResult {
	applied := make(map[string]bool)
	if ar != nil {
		for _, operation := range ar.Applied {
			applied[strings.ToLower(operation.Variable)] = true
		}
	}
	completed := make([]VariableResult, 0, len(results))
	for _, result := range results {
		if result.Category == LabelNoChange || applied[strings.ToLower(result.Name)] {
			completed = append(completed, result)
		}
	}
	if ar != nil {
		for _, skipped := range ar.Skipped {
			completed = append(completed, VariableResult{
				Name: skipped.Operation.Variable, Category: LabelSkipScope,
				Annotation: skipAnnotation, Operation: skipped.Operation,
			})
		}
	}
	return completed
}
