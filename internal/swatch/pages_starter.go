package swatch

// PagesStarterPaths lists the fixed sources for the static Pages starter.
var PagesStarterPaths = []string{"pages/index.html", "pages/style.css", "pages/theme.js", "pages/icon.svg"}

// IsPagesStarter identifies files that the Pages stage handles as one starter.
func IsPagesStarter(name string) bool {
	for _, source := range PagesStarterPaths {
		if name == source {
			return true
		}
	}
	return false
}
