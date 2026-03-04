package swatch

import (
	"fmt"

	"github.com/wimpysworld/tailor"
)

// Content returns the embedded bytes for the swatch identified by source.
// The source is the path relative to swatches/, e.g. ".github/workflows/tailor.yml".
func Content(source string) ([]byte, error) {
	path := "swatches/" + source
	data, err := tailor.SwatchFS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("swatch %q not found in embedded files: %w", source, err)
	}
	return data, nil
}
