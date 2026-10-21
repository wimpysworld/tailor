package alter

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func managedQualityBody(name string) ([]byte, error) {
	return managedBuild.contribution(name)
}

func (operation managedRecipeOperation) contribution(name string) ([]byte, error) {
	directory := "quality"
	if operation == managedSetup {
		directory = "setup"
	}
	body, err := swatch.Content(directory + "/" + name + ".sh")
	if err != nil || len(bytes.TrimSpace(body)) == 0 {
		return nil, fmt.Errorf("missing managed quality contribution %q", name)
	}
	return body, nil
}

func renderManagedQuality(cfg *config.Config, content []byte, registry []managedRegistryEntry, operation managedRecipeOperation) ([]byte, error) {
	if cfg == nil {
		return nil, fmt.Errorf("managed %s requires a config", operation)
	}
	if err := validateManagedRegistry(registry); err != nil {
		return nil, err
	}
	prefix := "[[TAILOR_MANAGED_" + strings.ToUpper(string(operation)) + "_"
	placeholder := []byte(prefix + "BODY]]")
	if operation == managedSetup {
		placeholder = []byte(prefix + "COMMAND]]")
	}
	if bytes.Count(content, placeholder) != 1 {
		return nil, fmt.Errorf("managed core template must contain one %s body placeholder", operation)
	}
	var entries []managedRegistryEntry
	for _, entry := range registry {
		declared, enabled := managedCapabilityState(cfg, entry.Capability)
		if declared && enabled && operation.body(entry) != "" {
			entries = append(entries, entry)
		}
	}
	slices.SortFunc(entries, func(a, b managedRegistryEntry) int {
		return strings.Compare(a.Capability.canonicalName(), b.Capability.canonicalName())
	})
	var body strings.Builder
	for _, entry := range entries {
		if entry.Capability != managedCapabilityRust {
			continue
		}
		preflight, err := managedQualityBody("rust-preflight")
		if err != nil {
			return nil, err
		}
		tools := ""
		if operation == managedLint {
			tools = "rustfmt cargo-fmt cargo-clippy clippy-driver cargo-audit"
		}
		body.Write(bytes.ReplaceAll(preflight, []byte("[[TAILOR_RUST_TOOLS]]"), []byte(tools)))
	}
	if operation == managedSetup && len(entries) > 0 {
		body.WriteString("#!/usr/bin/env bash\nset -e\n")
	}
	if operation == managedLint {
		workflows, err := managedQualityBody("workflows")
		if err != nil {
			return nil, err
		}
		body.Write(workflows)
		body.WriteString("check_workflows\n")
	}
	for _, entry := range entries {
		contribution, err := operation.contribution(string(operation.body(entry)))
		if err != nil {
			return nil, err
		}
		body.WriteString("(\n")
		body.Write(contribution)
		body.WriteString(")\n")
	}
	if len(entries) == 0 && operation != managedLint {
		if operation == managedSetup {
			body.WriteString("@")
		}
		body.WriteString("echo \"No " + string(operation) + " recipes are enabled.\"\n")
	}
	if operation != managedSetup {
		project, err := managedQualityBody("project")
		if err != nil {
			return nil, err
		}
		arguments := ""
		if operation == managedLint {
			arguments = ` "${mode}"`
		}
		project = bytes.ReplaceAll(project, []byte("[[TAILOR_OPERATION]]"), []byte(operation))
		project = bytes.ReplaceAll(project, []byte(" [[TAILOR_ARGUMENTS]]"), []byte(arguments))
		body.Write(project)
	}
	rendered := indentManagedQualityBody([]byte(body.String()))
	content = bytes.Replace(content, placeholder, rendered, 1)
	if bytes.Contains(content, []byte(prefix)) {
		return nil, fmt.Errorf("managed core template contains an unresolved %s placeholder", operation)
	}
	return content, nil
}

func indentManagedQualityBody(body []byte) []byte {
	lines := strings.Split(strings.TrimSuffix(string(body), "\n"), "\n")
	for index := 1; index < len(lines); index++ {
		if lines[index] != "" {
			lines[index] = "    " + lines[index]
		}
	}
	return []byte(strings.Join(lines, "\n"))
}
