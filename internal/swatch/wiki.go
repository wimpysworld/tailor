package swatch

const (
	WikiDestination = ".github/workflows/tailor-wiki.yml"
	WikiMarker      = "# Managed by Tailor: wiki"
	WikiBaseline    = "wiki/.tailor-wiki-base"
)

// IsWiki reports whether a path is one of the four conditional wiki swatches.
func IsWiki(path string) bool {
	return path == WikiDestination || path == "wiki/Home.md" || path == "wiki/_Sidebar.md" || path == "wiki/_Footer.md"
}
