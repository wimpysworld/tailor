package swatch

import (
	"fmt"

	"github.com/wimpysworld/tailor"
)

// Content returns the embedded bytes for the swatch identified by path.
// The path is relative to swatches/, e.g. ".github/workflows/tailor.yml".
func Content(path string) ([]byte, error) {
	fsPath := "swatches/" + path
	data, err := tailor.SwatchFS.ReadFile(fsPath)
	if err != nil {
		return nil, fmt.Errorf("swatch %q not found in embedded files: %w", path, err)
	}
	return data, nil
}
