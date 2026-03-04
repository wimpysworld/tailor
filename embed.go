package tailor

import "embed"

// SwatchFS holds the embedded swatch files from the swatches/ directory.
// The "all:" prefix includes dotfiles such as .gitignore and .envrc.
//
//go:embed all:swatches
var SwatchFS embed.FS
