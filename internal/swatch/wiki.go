package swatch

import (
	"bytes"
	"fmt"
	"strconv"
	"text/template"

	"github.com/wimpysworld/tailor"
)

const (
	WikiDestination = ".github/workflows/tailor-wiki.yml"
	WikiMarker      = "# Managed by Tailor: wiki"
	WikiBaseline    = "wiki/.tailor-wiki-base"
)

// IsWiki reports whether a path is one of the four conditional wiki swatches.
func IsWiki(path string) bool {
	return path == WikiDestination || path == "wiki/Home.md" || path == "wiki/_Sidebar.md" || path == "wiki/_Footer.md"
}

// WikiContent renders the wiki workflow for the repository default branch.
func WikiContent(branch string) ([]byte, error) {
	filter, err := workflowBranchFilter(branch)
	if err != nil {
		return nil, err
	}
	data, err := tailor.SwatchFS.ReadFile("swatches/" + WikiDestination)
	if err != nil {
		return nil, fmt.Errorf("reading wiki workflow: %w", err)
	}
	tmpl, err := template.New("wiki").Delims("[[", "]]").Funcs(template.FuncMap{"quote": strconv.Quote}).Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("parsing wiki workflow: %w", err)
	}
	var content bytes.Buffer
	if err := tmpl.Execute(&content, struct{ Branch string }{filter}); err != nil {
		return nil, fmt.Errorf("rendering wiki workflow: %w", err)
	}
	return content.Bytes(), nil
}
