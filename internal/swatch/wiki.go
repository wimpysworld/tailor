package swatch

const (
	// WikiDestination is the wiki publisher workflow path.
	WikiDestination = ".github/workflows/tailor-wiki.yml"
	// WikiMarker identifies workflows that Tailor can replace or remove.
	WikiMarker = "# Managed by Tailor: wiki"
	// WikiBaseline records the imported remote commit and is excluded from publication.
	WikiBaseline = "wiki/.tailor-wiki-base"
)

// IsWiki reports whether a path is one of the four conditional wiki swatches.
func IsWiki(path string) bool {
	return path == WikiDestination || path == "wiki/Home.md" || path == "wiki/_Sidebar.md" || path == "wiki/_Footer.md"
}
