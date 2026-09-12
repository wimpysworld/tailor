package swatch

import (
	"fmt"

	"github.com/wimpysworld/tailor"
)

// Content returns the embedded bytes for the swatch identified by path.
// The path is relative to swatches/, for example ".github/dependabot.yml".
// The Pages destination renders the static workflow with source pages and branch main.
func Content(path string) ([]byte, error) {
	if path == PagesDestination {
		return PagesContent("static", "pages", "main")
	}
	fsPath := "swatches/" + path
	data, err := tailor.SwatchFS.ReadFile(fsPath)
	if err != nil {
		return nil, fmt.Errorf("swatch %q not found in embedded files: %w", path, err)
	}
	return data, nil
}
