package measure

import (
	"errors"
	"fmt"
	"image/png"
	"io"
	"os"
	"path"
	"strings"

	"github.com/wimpysworld/tailor/internal/model"
)

// maxSocialOGSize is GitHub's 1 MB (1,048,576-byte) Open Graph image limit.
const maxSocialOGSize = 1 << 20

// CheckSocialOG validates an existing Open Graph PNG at outputPath inside root.
// When the file does not exist, it returns nil so repositories that have not
// generated an Open Graph card yet receive no spurious warnings.
func CheckSocialOG(root *os.Root, outputPath string) []HealthResult {
	if root == nil {
		return nil
	}
	if strings.TrimSpace(outputPath) == "" {
		outputPath = model.DefaultSocialOGOutput
	}

	cleaned := path.Clean(outputPath)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "/") || strings.HasPrefix(cleaned, "../") || strings.ContainsRune(outputPath, '\\') {
		return []HealthResult{{Path: outputPath, Status: Warning, Detail: "(unsafe output path)"}}
	}

	if result, stop := checkSocialOGParents(root, outputPath, cleaned); stop {
		return result
	}

	info, err := root.Lstat(cleaned)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return []HealthResult{{Path: outputPath, Status: Warning, Detail: "(not inspected: read failed)"}}
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return []HealthResult{{Path: outputPath, Status: Warning, Detail: "(symlink is not allowed)"}}
	}
	if !info.Mode().IsRegular() {
		return []HealthResult{{Path: outputPath, Status: Warning, Detail: "(not a regular file)"}}
	}

	file, err := root.Open(cleaned)
	if err != nil {
		return []HealthResult{{Path: outputPath, Status: Warning, Detail: "(not inspected: read failed)"}}
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return []HealthResult{{Path: outputPath, Status: Warning, Detail: "(not inspected: read failed)"}}
	}
	if !stat.Mode().IsRegular() {
		return []HealthResult{{Path: outputPath, Status: Warning, Detail: "(not a regular file)"}}
	}

	cfg, err := png.DecodeConfig(io.LimitReader(file, maxSocialOGSize))
	if err != nil {
		return []HealthResult{{Path: outputPath, Status: Warning, Detail: "(invalid PNG)"}}
	}

	var violations []string
	validDimensions := (cfg.Width == 1280 && cfg.Height == 640) || (cfg.Width == 1200 && cfg.Height == 630)
	if !validDimensions {
		violations = append(violations, fmt.Sprintf("dimensions %dx%d; expected 1280x640 or 1200x630", cfg.Width, cfg.Height))
	}
	if stat.Size() >= maxSocialOGSize {
		violations = append(violations, fmt.Sprintf("exceeds 1 MB limit (%d bytes)", stat.Size()))
	}
	if len(violations) > 0 {
		return []HealthResult{{
			Path:   outputPath,
			Status: Warning,
			Detail: "(" + strings.Join(violations, "; ") + ")",
		}}
	}

	return []HealthResult{{Path: outputPath, Status: Present}}
}

func checkSocialOGParents(root *os.Root, displayPath, cleaned string) ([]HealthResult, bool) {
	parent := path.Dir(cleaned)
	if parent == "." {
		return nil, false
	}

	current := ""
	for component := range strings.SplitSeq(parent, "/") {
		if current == "" {
			current = component
		} else {
			current += "/" + component
		}
		info, err := root.Lstat(current)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, true
			}
			return []HealthResult{{Path: displayPath, Status: Warning, Detail: "(not inspected: read failed)"}}, true
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return []HealthResult{{Path: displayPath, Status: Warning, Detail: "(parent directory is a symlink)"}}, true
		}
		if !info.IsDir() {
			return nil, true
		}
	}
	return nil, false
}
