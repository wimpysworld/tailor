package alter

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/swatch"
)

// processPagesIgnore appends a generator output rule after ordinary swatches so Recut cannot remove it.
// Existing rules remain unchanged, and never mode prevents the addition.
func processPagesIgnore(cfg *config.Config, dir string, mode ApplyMode, prepared *pagesPreparation) (*SwatchResult, error) {
	if prepared == nil || prepared.Generator == "static" {
		return nil, nil
	}
	var output string
	switch prepared.Generator {
	case "hugo":
		output = "public"
	case "jekyll":
		output = "_site"
	default:
		return nil, fmt.Errorf("unknown Pages generator %q", prepared.Generator)
	}

	entry := config.SwatchEntry{Path: ".gitignore", Alteration: swatch.Always}
	for _, configured := range cfg.Swatches {
		if configured.Path == entry.Path && configured.Alteration == swatch.Never {
			return &SwatchResult{Path: entry.Path, Category: Skipped, Reason: SkipModeNever}, nil
		}
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("opening project root %q: %w", dir, err)
	}
	defer root.Close()
	exists, err := prepareSwatchDestination(root, entry.Path, mode.ShouldWrite())
	if err != nil {
		return nil, fmt.Errorf("checking Pages ignore file: %w", err)
	}
	var content []byte
	if exists {
		content, err = root.ReadFile(entry.Path)
		if err != nil {
			return nil, fmt.Errorf("reading Pages ignore file: %w", err)
		}
	}

	path := strings.TrimSuffix(prepared.Path, "/")
	if path == "." {
		path = ""
	}
	if path != "" {
		path += "/"
	}
	rule := "/" + strings.NewReplacer(
		`\`, `\\`, "*", `\*`, "?", `\?`, "[", `\[`, "]", `\]`,
		" ", `\ `, "#", `\#`, "!", `\!`,
	).Replace(path) + output + "/"
	for line := range bytes.SplitSeq(content, []byte("\n")) {
		if string(bytes.TrimSuffix(line, []byte("\r"))) == rule {
			return &SwatchResult{Path: entry.Path, Category: NoChange}, nil
		}
	}
	if len(content) > 0 && content[len(content)-1] != '\n' {
		content = append(content, '\n')
	}
	content = append(content, rule...)
	content = append(content, '\n')
	category := WouldCopy
	if exists {
		category = WouldOverwrite
	}
	result, err := writeSwatch(root, entry, content, category, mode.ShouldWrite())
	if err != nil {
		return nil, err
	}
	return &result, nil
}
