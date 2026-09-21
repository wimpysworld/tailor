package alter

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

const (
	dependabotPath          = ".github/dependabot.yml"
	dependabotAlternatePath = ".github/dependabot.yaml"
	dependabotMaxBytes      = 1 << 20
)

//go:embed testdata/dependabot/enabled-v1.yml
var dependabotEnabledV1 []byte

//go:embed testdata/dependabot/disabled-v1.yml
var dependabotDisabledV1 []byte

type dependabotSelection struct {
	Present    bool
	Mode       swatch.AlterationMode
	Recut      bool
	GoDeclared bool
	GoEnabled  bool
}

type dependabotCanonicalBody struct {
	Content   []byte
	GoEnabled bool
}

type dependabotBodies struct {
	Enabled       []byte
	Disabled      []byte
	Compatibility []dependabotCanonicalBody
}

type dependabotDestinationKind uint8

const (
	dependabotDestinationMissing dependabotDestinationKind = iota
	dependabotDestinationRegular
	dependabotDestinationSymlink
	dependabotDestinationDirectory
	dependabotDestinationSpecial
)

type dependabotIdentitySnapshot struct {
	Path     string
	Exists   bool
	Identity os.FileInfo
}

type dependabotDestinationSnapshot struct {
	Path     string
	Kind     dependabotDestinationKind
	Content  []byte
	Identity os.FileInfo
}

type dependabotSnapshot struct {
	Root        dependabotIdentitySnapshot
	Parent      dependabotIdentitySnapshot
	Alternate   dependabotDestinationSnapshot
	Destination dependabotDestinationSnapshot
}

type dependabotOutcome uint8

const (
	dependabotOutcomeSkip dependabotOutcome = iota
	dependabotOutcomePreserve
	dependabotOutcomeUnchanged
	dependabotOutcomeCreate
	dependabotOutcomeReplace
)

type dependabotPlan struct {
	Snapshot  dependabotSnapshot
	Outcome   dependabotOutcome
	GoKnown   bool
	GoEnabled bool
	Output    []byte
}

type dependabotConflictError struct {
	Reason string
}

func (err *dependabotConflictError) Error() string {
	return fmt.Sprintf("dependabot destination %q conflicts: %s", dependabotPath, err.Reason)
}

func dependabotSelectionFromConfig(cfg *config.Config, mode ApplyMode) dependabotSelection {
	selection := dependabotSelection{Recut: mode == Recut}
	if cfg == nil {
		return selection
	}
	selection.GoDeclared = cfg.GoDeclared()
	selection.GoEnabled = cfg.GoEnabled()
	for _, entry := range cfg.Swatches {
		if entry.Path == dependabotPath {
			selection.Present = true
			selection.Mode = entry.Alteration
			break
		}
	}
	return selection
}

func renderDependabotBodies() (dependabotBodies, error) {
	enabled, err := swatch.Render(dependabotPath, swatch.Options{GoDeclared: true, GoEnabled: true})
	if err != nil {
		return dependabotBodies{}, fmt.Errorf("rendering Go-enabled Dependabot body: %w", err)
	}
	disabled, err := swatch.Render(dependabotPath, swatch.Options{GoDeclared: true, GoEnabled: false})
	if err != nil {
		return dependabotBodies{}, fmt.Errorf("rendering Go-disabled Dependabot body: %w", err)
	}
	for name, body := range map[string][]byte{"enabled": enabled, "disabled": disabled} {
		if bytes.Contains(body, []byte("\r")) {
			return dependabotBodies{}, fmt.Errorf("rendered %s Dependabot body is not LF-only", name)
		}
	}
	return dependabotBodies{
		Enabled:  bytes.Clone(enabled),
		Disabled: bytes.Clone(disabled),
		Compatibility: []dependabotCanonicalBody{
			{Content: bytes.Clone(dependabotEnabledV1), GoEnabled: true},
			{Content: bytes.Clone(dependabotDisabledV1), GoEnabled: false},
		},
	}, nil
}

func planDependabot(selection dependabotSelection, snapshot dependabotSnapshot, bodies dependabotBodies) (dependabotPlan, error) {
	plan := dependabotPlan{Snapshot: cloneDependabotSnapshot(snapshot)}
	if !selection.Present || selection.Mode == swatch.Never {
		plan.Outcome = dependabotOutcomeSkip
		return plan, nil
	}
	if selection.Mode != swatch.FirstFit && selection.Mode != swatch.Always {
		return dependabotPlan{}, dependabotConflict("the alteration mode is invalid")
	}
	if snapshot.Alternate.Kind != dependabotDestinationMissing {
		return dependabotPlan{}, dependabotConflict("the alternate .github/dependabot.yaml entry exists")
	}

	switch snapshot.Destination.Kind {
	case dependabotDestinationMissing:
		selected := selection.GoEnabled
		if !selection.GoDeclared {
			selected = true
		}
		return buildDependabotPlan(plan, dependabotOutcomeCreate, selected, bodies)
	case dependabotDestinationRegular:
	case dependabotDestinationSymlink:
		return dependabotPlan{}, dependabotConflict("the destination is a symlink")
	case dependabotDestinationDirectory:
		return dependabotPlan{}, dependabotConflict("the destination is a directory")
	case dependabotDestinationSpecial:
		return dependabotPlan{}, dependabotConflict("the destination is not a regular file")
	default:
		return dependabotPlan{}, dependabotConflict("the destination type is invalid")
	}

	if !hasManagedMarker(snapshot.Destination.Content, dependabotPath) {
		plan.Outcome = dependabotOutcomePreserve
		return plan, nil
	}

	selected := selection.GoEnabled
	if !selection.GoDeclared {
		var known bool
		selected, known = recogniseDependabotState(snapshot.Destination.Content, bodies)
		if !known {
			return dependabotPlan{}, dependabotConflict("the owned content does not identify one canonical Go state")
		}
	}
	return buildDependabotPlan(plan, dependabotOutcomeReplace, selected, bodies)
}

func buildDependabotPlan(plan dependabotPlan, changed dependabotOutcome, enabled bool, bodies dependabotBodies) (dependabotPlan, error) {
	body := bodies.Disabled
	if enabled {
		body = bodies.Enabled
	}
	marker := []byte(managedMarker(dependabotPath) + "\n")
	if len(marker)+len(body) > dependabotMaxBytes {
		return dependabotPlan{}, dependabotConflict("the planned output exceeds 1048576 bytes")
	}
	plan.GoKnown = true
	plan.GoEnabled = enabled
	plan.Output = make([]byte, 0, len(marker)+len(body))
	plan.Output = append(plan.Output, marker...)
	plan.Output = append(plan.Output, body...)
	if changed == dependabotOutcomeReplace && bytes.Equal(plan.Snapshot.Destination.Content, plan.Output) {
		plan.Outcome = dependabotOutcomeUnchanged
	} else {
		plan.Outcome = changed
	}
	return plan, nil
}

func recogniseDependabotState(content []byte, bodies dependabotBodies) (bool, bool) {
	marker := []byte(managedMarker(dependabotPath))
	if !bytes.HasPrefix(content, marker) {
		return false, false
	}
	body := content[len(marker):]
	switch {
	case bytes.HasPrefix(body, []byte("\r\n")):
		body = body[2:]
	case bytes.HasPrefix(body, []byte("\n")):
		body = body[1:]
	default:
		return false, false
	}

	candidates := make([]dependabotCanonicalBody, 0, 2+len(bodies.Compatibility))
	candidates = append(candidates,
		dependabotCanonicalBody{Content: bodies.Enabled, GoEnabled: true},
		dependabotCanonicalBody{Content: bodies.Disabled, GoEnabled: false},
	)
	candidates = append(candidates, bodies.Compatibility...)

	var selected bool
	matched := false
	for _, candidate := range candidates {
		if !bytes.Equal(body, candidate.Content) && !bytes.Equal(body, dependabotCRLF(candidate.Content)) {
			continue
		}
		if matched && selected != candidate.GoEnabled {
			return false, false
		}
		selected = candidate.GoEnabled
		matched = true
	}
	return selected, matched
}

func dependabotCRLF(content []byte) []byte {
	return bytes.ReplaceAll(content, []byte("\n"), []byte("\r\n"))
}

func dependabotConflict(reason string) error {
	return &dependabotConflictError{Reason: reason}
}

func cloneDependabotSnapshot(snapshot dependabotSnapshot) dependabotSnapshot {
	snapshot.Alternate.Content = bytes.Clone(snapshot.Alternate.Content)
	snapshot.Destination.Content = bytes.Clone(snapshot.Destination.Content)
	return snapshot
}
