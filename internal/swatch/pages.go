package swatch

import (
	"bytes"
	"fmt"
	"path"
	"strconv"
	"strings"
	"text/template"

	"github.com/wimpysworld/tailor"
)

const (
	// PagesDestination is the shared path for all Pages workflow variants.
	PagesDestination = ".github/workflows/tailor-pages.yml"
	// PagesMarker identifies workflows managed by Tailor.
	PagesMarker = "# Managed by Tailor: pages"
)

// PagesContent renders the selected Pages workflow without reading project files.
func PagesContent(generator, source, branch string) ([]byte, error) {
	if generator != "static" && generator != "hugo" && generator != "jekyll" {
		return nil, fmt.Errorf("unsupported pages generator %q", generator)
	}
	if source == "" || path.IsAbs(source) || path.Clean(source) != source || source == ".." || strings.HasPrefix(source, "../") || strings.ContainsAny(source, "\\\r\n\x00") || strings.Contains(source, "${{") {
		return nil, fmt.Errorf("unsafe pages path %q", source)
	}
	branchFilter, err := workflowBranchFilter(branch)
	if err != nil {
		return nil, err
	}
	data, err := tailor.SwatchFS.ReadFile("swatches/pages/" + generator + ".yml")
	if err != nil {
		return nil, fmt.Errorf("reading pages workflow: %w", err)
	}
	tmpl, err := template.New(generator).Delims("[[", "]]").Funcs(template.FuncMap{"quote": strconv.Quote}).Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("parsing pages workflow: %w", err)
	}
	var content bytes.Buffer
	err = tmpl.Execute(&content, struct{ Path, Branch string }{source, branchFilter})
	if err != nil {
		return nil, fmt.Errorf("rendering pages workflow: %w", err)
	}
	return content.Bytes(), nil
}

// workflowBranchFilter rejects unsafe branches and escapes filter syntax before YAML quoting.
func workflowBranchFilter(branch string) (string, error) {
	if branch == "" || strings.ContainsAny(branch, "\\\r\n\x00~^:?*[") || strings.Contains(branch, "..") || strings.Contains(branch, "@{") || strings.Contains(branch, "${{") || strings.HasPrefix(branch, "-") || strings.HasPrefix(branch, "/") || strings.HasSuffix(branch, "/") {
		return "", fmt.Errorf("unsafe workflow branch %q", branch)
	}
	return strings.NewReplacer("+", `\+`, "!", `\!`).Replace(branch), nil
}
