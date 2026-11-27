package social

import (
	"fmt"
	"os"
	"path"
	"strings"
	"unicode"

	"github.com/wimpysworld/tailor/internal/config"
)

// Asset size bounds for safe rooted inspection.
const (
	maxIconSize         = 2 << 20
	maxPreviewImageSize = 10 << 20
)

// Icon kind values reported in ProjectAssets.IconKind.
const (
	IconKindNone = ""
	IconKindSVG  = "svg"
	IconKindPNG  = "png"
)

// Preview mode values reported in ProjectAssets.PreviewMode.
const (
	PreviewModeNone     = "none"
	PreviewModeImage    = "image"
	PreviewModeTerminal = "terminal"
)

// ProjectAssets holds inherited project metadata, icon selection, Open Graph
// preview resolution, and footer URL resolved from .tailor.yml and project files.
type ProjectAssets struct {
	Title           string
	Tagline         string
	IconPath        string
	IconKind        string
	PreviewMode     string
	PreviewImage    string
	PreviewTerminal []string
	FooterURL       string
	Platforms       string
}

// ResolveProjectAssets resolves inherited project title, tagline, icon, Open
// Graph preview mode, and footer URL from root, pagesPath, cfg, and repoName.
func ResolveProjectAssets(root *os.Root, pagesPath string, cfg *config.Config, repoName string) (ProjectAssets, error) {
	effectivePagesPath := pagesPath
	if strings.TrimSpace(effectivePagesPath) == "" && cfg != nil && cfg.Pages != nil && cfg.Pages.Path != nil {
		effectivePagesPath = *cfg.Pages.Path
	}
	cleanedPages, err := cleanRelativeDir(effectivePagesPath)
	if err != nil {
		return ProjectAssets{}, err
	}

	title, repoOwner, repoShort := parseRepoName(repoName)

	tagline := ""
	if cfg != nil && cfg.Repository != nil && cfg.Repository.Description != nil {
		tagline = StripTrailingEmoji(*cfg.Repository.Description)
	}

	assets := ProjectAssets{
		Title:       title,
		Tagline:     tagline,
		IconKind:    IconKindNone,
		PreviewMode: PreviewModeNone,
		FooterURL:   resolveFooterURL(cfg, repoOwner, repoShort),
		Platforms:   cfg.EffectiveSocialPlatforms(),
	}

	if root != nil {
		iconPath, iconKind, err := resolveIcon(root, cleanedPages)
		if err != nil {
			return ProjectAssets{}, err
		}
		assets.IconPath = iconPath
		assets.IconKind = iconKind
	}

	if cfg != nil && cfg.EffectiveSocialOGIncludePreview() {
		if err := resolvePreview(root, cleanedPages, cfg, &assets); err != nil {
			return ProjectAssets{}, err
		}
	}

	return assets, nil
}

func parseRepoName(repoName string) (title, owner, repo string) {
	trimmed := strings.TrimSpace(repoName)
	if trimmed == "" {
		return "", "", ""
	}
	left, right, hasSlash := strings.Cut(trimmed, "/")
	if hasSlash && left != "" && right != "" && !strings.Contains(right, "/") {
		return right, left, right
	}
	return trimmed, "", ""
}

func resolveIcon(root *os.Root, pagesPath string) (string, string, error) {
	svgPath := path.Join(pagesPath, "icon.svg")
	exists, err := checkRootRegularFile(root, svgPath, maxIconSize)
	if err != nil {
		return "", IconKindNone, err
	}
	if exists {
		return svgPath, IconKindSVG, nil
	}

	pngPath := path.Join(pagesPath, "icon.png")
	exists, err = checkRootRegularFile(root, pngPath, maxIconSize)
	if err != nil {
		return "", IconKindNone, err
	}
	if exists {
		return pngPath, IconKindPNG, nil
	}

	return "", IconKindNone, nil
}

func resolvePreview(root *os.Root, pagesPath string, cfg *config.Config, assets *ProjectAssets) error {
	if ogImage := strings.TrimSpace(cfg.EffectiveSocialOGImage()); ogImage != "" {
		cleanedImage, err := cleanRelativeFile(ogImage)
		if err != nil {
			return err
		}
		if root != nil {
			exists, err := checkRootRegularFile(root, cleanedImage, maxPreviewImageSize)
			if err != nil {
				return err
			}
			if !exists {
				return fmt.Errorf("social.og.image %q does not exist", cleanedImage)
			}
		}
		assets.PreviewMode = PreviewModeImage
		assets.PreviewImage = cleanedImage
		return nil
	}

	if root != nil {
		demoStill := path.Join(pagesPath, "demo-still.png")
		exists, err := checkRootRegularFile(root, demoStill, maxPreviewImageSize)
		if err != nil {
			return err
		}
		if exists {
			assets.PreviewMode = PreviewModeImage
			assets.PreviewImage = demoStill
			return nil
		}
	}

	if terminal := cfg.EffectiveSocialOGTerminal(); len(terminal) > 0 {
		assets.PreviewMode = PreviewModeTerminal
		assets.PreviewTerminal = terminal
	}
	return nil
}

func cleanRelativeFile(file string) (string, error) {
	trimmed := strings.TrimSpace(file)
	if trimmed == "" || strings.ContainsRune(trimmed, '\\') || strings.HasPrefix(trimmed, "/") {
		return "", fmt.Errorf("social asset path %q must be a relative path", file)
	}
	cleaned := path.Clean(trimmed)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("social asset path %q escapes project root", file)
	}
	return cleaned, nil
}

func resolveFooterURL(cfg *config.Config, repoOwner, repoShort string) string {
	if cfg != nil && cfg.Pages != nil && cfg.Pages.CNAME != nil {
		cname := strings.TrimSpace(*cfg.Pages.CNAME)
		if cname != "" {
			if strings.HasPrefix(cname, "https://") || strings.HasPrefix(cname, "http://") {
				return cname
			}
			return "https://" + cname
		}
	}
	if cfg != nil && cfg.Repository != nil && cfg.Repository.Homepage != nil {
		homepage := strings.TrimSpace(*cfg.Repository.Homepage)
		if homepage != "" {
			return homepage
		}
	}
	if repoOwner != "" && repoShort != "" {
		return "https://github.com/" + repoOwner + "/" + repoShort
	}
	return ""
}

// StripTrailingEmoji trims trailing Unicode emoji, variation selectors
// (U+FE0F, U+FE0E), ZWJ sequences, keycap combining marks (such as 1️⃣),
// regional indicators, and trailing whitespace from s.
func StripTrailingEmoji(s string) string {
	runes := []rune(s)
	end := len(runes)

	for end > 0 {
		for end > 0 && unicode.IsSpace(runes[end-1]) {
			end--
		}
		if end == 0 {
			break
		}
		next := stripOneTrailingEmojiCluster(runes[:end])
		if next == end {
			break
		}
		end = next
	}

	return string(runes[:end])
}

func stripOneTrailingEmojiCluster(runes []rune) int {
	n := len(runes)
	if n == 0 {
		return 0
	}

	// Keycap combining sequence: [0-9#*] + optional VS15/VS16 + U+20E3.
	if runes[n-1] == '\u20E3' {
		j := n - 1
		if j > 0 && isVariationSelector(runes[j-1]) {
			j--
		}
		if j > 0 && isKeycapBase(runes[j-1]) {
			j--
		}
		return j
	}

	// Subdivision flag tag sequence: base emoji + tag chars + U+E007F.
	if runes[n-1] == 0xE007F {
		j := n - 1
		for j > 0 && runes[j-1] >= 0xE0020 && runes[j-1] <= 0xE007E {
			j--
		}
		if j > 0 && isEmojiBase(runes[j-1]) {
			j--
		}
		return j
	}

	// Regional indicator flag pair (or trailing regional indicator).
	if isRegionalIndicator(runes[n-1]) {
		j := n - 1
		if j > 0 && isRegionalIndicator(runes[j-1]) {
			j--
		}
		return j
	}

	// Stray trailing ZWJ.
	if runes[n-1] == '\u200D' {
		return n - 1
	}

	next := stripOneTrailingEmojiAtom(runes)
	if next == n {
		return n
	}

	// Walk back across ZWJ-linked emoji sequences (such as 👨‍💻 or 🏴‍☠️).
	for next > 0 && runes[next-1] == '\u200D' {
		beforeZWJ := next - 1
		prev := stripOneTrailingEmojiAtom(runes[:beforeZWJ])
		if prev == beforeZWJ {
			next = beforeZWJ
			break
		}
		next = prev
	}

	return next
}

func stripOneTrailingEmojiAtom(runes []rune) int {
	n := len(runes)
	j := n

	hadVS := false
	for j > 0 && isVariationSelector(runes[j-1]) {
		j--
		hadVS = true
	}

	hadMod := false
	if j > 0 && isEmojiModifier(runes[j-1]) {
		j--
		hadMod = true
		for j > 0 && isVariationSelector(runes[j-1]) {
			j--
		}
	}

	if j > 0 && (isEmojiBase(runes[j-1]) || (hadVS && isTextDefaultEmoji(runes[j-1]))) {
		j--
		return j
	}
	if hadMod || hadVS {
		return j
	}
	return n
}

func isVariationSelector(r rune) bool {
	return r == '\uFE0E' || r == '\uFE0F'
}

func isKeycapBase(r rune) bool {
	return (r >= '0' && r <= '9') || r == '#' || r == '*'
}

func isRegionalIndicator(r rune) bool {
	return r >= 0x1F1E6 && r <= 0x1F1FF
}

func isEmojiModifier(r rune) bool {
	return r >= 0x1F3FB && r <= 0x1F3FF
}

func isEmojiBase(r rune) bool {
	switch {
	case r >= 0x1F300 && r <= 0x1F5FF: // Misc Symbols and Pictographs
		return true
	case r >= 0x1F600 && r <= 0x1F64F: // Emoticons
		return true
	case r >= 0x1F680 && r <= 0x1F6FF: // Transport and Map Symbols
		return true
	case r >= 0x1F900 && r <= 0x1F9FF: // Supplemental Symbols and Pictographs
		return true
	case r >= 0x1FA00 && r <= 0x1FAFF: // Chess and Extended-A Symbols
		return true
	case r >= 0x2600 && r <= 0x26FF: // Misc Symbols
		return true
	case r >= 0x2700 && r <= 0x27BF: // Dingbats
		return true
	case r >= 0x1F000 && r <= 0x1F0FF: // Mahjong and Playing Cards
		return true
	case r >= 0x1F100 && r <= 0x1F2FF: // Enclosed Alphanumeric/Ideographic Supplement
		return true
	case r >= 0x1F7E0 && r <= 0x1F7EB, r == 0x1F7F0: // Geometric Shapes Extended emoji
		return true
	case r == 0x231A, r == 0x231B,
		(r >= 0x23E9 && r <= 0x23F3),
		(r >= 0x23F8 && r <= 0x23FA),
		(r >= 0x2B05 && r <= 0x2B07),
		r == 0x2B1B, r == 0x2B1C, r == 0x2B50, r == 0x2B55:
		return true
	default:
		return false
	}
}

func isTextDefaultEmoji(r rune) bool {
	switch {
	case r == 0x00A9, r == 0x00AE, // © ®
		r == 0x203C, r == 0x2049, // ‼ ⁉
		r == 0x2122, r == 0x2139, // ™ ℹ
		(r >= 0x2194 && r <= 0x2199),
		r == 0x21A9, r == 0x21AA,
		r == 0x2328, r == 0x23CF, r == 0x24C2,
		r == 0x25AA, r == 0x25AB, r == 0x25B6, r == 0x25C0,
		(r >= 0x25FB && r <= 0x25FE),
		r == 0x2934, r == 0x2935,
		r == 0x3030, r == 0x303D, r == 0x3297, r == 0x3299:
		return true
	default:
		return false
	}
}
