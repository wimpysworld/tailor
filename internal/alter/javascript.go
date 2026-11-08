package alter

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

type JavaScriptPolicy struct {
	Reason           string
	AdoptionRequired bool
	Present          bool
}

var javaScriptPolicyFiles = map[string]string{
	".oxlintrc.json":      "oxlint",
	".oxlintrc.jsonc":     "unverified",
	"oxlint.config.ts":    "unverified",
	"oxlint.config.mts":   "unverified",
	".prettierrc":         "prettier",
	".prettierrc.json":    "prettier",
	".prettierrc.json5":   "prettier",
	".prettierrc.yml":     "prettier",
	".prettierrc.yaml":    "prettier",
	".prettierrc.toml":    "prettier",
	".prettierrc.js":      "prettier",
	".prettierrc.cjs":     "prettier",
	".prettierrc.mjs":     "prettier",
	".prettierrc.ts":      "prettier",
	".prettierrc.cts":     "prettier",
	".prettierrc.mts":     "prettier",
	"prettier.config.js":  "prettier",
	"prettier.config.cjs": "prettier",
	"prettier.config.mjs": "prettier",
	"prettier.config.ts":  "prettier",
	"prettier.config.cts": "prettier",
	"prettier.config.mts": "prettier",
	".prettierignore":     "ignore",
	".eslintrc":           "conflict",
	".eslintrc.js":        "conflict",
	".eslintrc.cjs":       "conflict",
	".eslintrc.json":      "conflict",
	".eslintrc.yml":       "conflict",
	".eslintrc.yaml":      "conflict",
	"eslint.config.js":    "conflict",
	"eslint.config.mjs":   "conflict",
	"eslint.config.cjs":   "conflict",
	"eslint.config.ts":    "conflict",
	"eslint.config.mts":   "conflict",
	"eslint.config.cts":   "conflict",
	"biome.json":          "conflict",
	"biome.jsonc":         "conflict",
	"package.json":        "metadata",
	"package.yaml":        "metadata",
}

func javaScriptPolicyWarning(path, cause string) JavaScriptPolicy {
	return JavaScriptPolicy{AdoptionRequired: true, Reason: fmt.Sprintf("JavaScript manual adoption required; preserved %s: %s; native Oxlint/Prettier lint stays enabled, review policy before running lint", path, cause)}
}

// InspectJavaScriptPolicy checks fixed root and ancestor paths without evaluating policy code.
func InspectJavaScriptPolicy(dir, destination string) (JavaScriptPolicy, error) {
	tool := javaScriptPolicyFiles[destination]
	if destination != ".oxlintrc.json" && destination != ".prettierrc.json" && destination != ".prettierignore" {
		return JavaScriptPolicy{}, nil
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return JavaScriptPolicy{}, err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return javaScriptPolicyWarning(root, "unsafe project directory"), nil
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(root))
	if err != nil {
		return javaScriptPolicyWarning(root, "cannot resolve project parent"), nil
	}
	root = filepath.Join(parent, filepath.Base(root))
	var policy JavaScriptPolicy
	localCanonical := false
	names := slices.Sorted(maps.Keys(javaScriptPolicyFiles))
	for current := root; ; current = filepath.Dir(current) {
		matches := make(map[string]int)
		for _, name := range names {
			kind := javaScriptPolicyFiles[name]
			if current != root && kind == "ignore" {
				continue
			}
			path := filepath.Join(current, name)
			info, err := os.Lstat(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o444 == 0 {
				return javaScriptPolicyWarning(path, "cannot safely inspect a readable regular file"), nil
			}
			content, cause := readManagedInspection(current, name)
			if cause != "" {
				return javaScriptPolicyWarning(path, cause), nil
			}
			if kind == "metadata" {
				prettier, eslint, err := javaScriptPackagePolicy(name, content)
				if err != nil {
					return javaScriptPolicyWarning(path, "invalid or ambiguous package metadata"), nil
				}
				if eslint {
					kind = "conflict"
				} else if prettier {
					kind = "prettier"
				}
			}
			if kind == "conflict" {
				return javaScriptPolicyWarning(path, "existing ESLint or Biome policy"), nil
			}
			if kind == "unverified" && tool == "oxlint" {
				return javaScriptPolicyWarning(path, "Oxlint configuration needs manual verification"), nil
			}
			if current != root && localCanonical && (kind == tool || tool == "ignore" && kind == "prettier") {
				continue
			}
			if kind == "prettier" || kind == "oxlint" {
				matches[kind]++
				if matches[kind] > 1 && (kind == tool || tool == "ignore" && kind == "prettier") {
					return javaScriptPolicyWarning(current, "multiple policy files need manual review"), nil
				}
			}
			if current == root && name == destination {
				policy.Present = true
				localCanonical = true
				continue
			}
			if current == root && name == ".prettierrc.json" && tool == "ignore" {
				localCanonical = true
				continue
			}
			if kind == tool || tool == "ignore" && kind == "prettier" {
				if policy.Reason == "" {
					policy.Reason = "existing JavaScript policy preserved: " + path
				}
			}
		}
		if current == filepath.Dir(current) {
			break
		}
	}
	return policy, nil
}

func javaScriptPackagePolicy(name string, content []byte) (prettier, eslint bool, err error) {
	var document map[string]any
	if name == "package.json" {
		decoder := json.NewDecoder(bytes.NewReader(content))
		value, parseErr := readManagedJSONValue(decoder, 0)
		if parseErr != nil {
			return false, false, parseErr
		}
		if _, err := decoder.Token(); err != io.EOF {
			return false, false, fmt.Errorf("trailing package metadata")
		}
		var ok bool
		document, ok = value.(map[string]any)
		if !ok {
			return false, false, fmt.Errorf("package metadata must be an object")
		}
	} else {
		decoder := yaml.NewDecoder(bytes.NewReader(content))
		if err := decoder.Decode(&document); err != nil {
			return false, false, err
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) || document == nil {
			return false, false, fmt.Errorf("invalid package metadata")
		}
	}
	if value, exists := document["prettier"]; exists {
		switch value := value.(type) {
		case map[string]any, map[any]any:
			prettier = true
		case string:
			prettier = strings.TrimSpace(value) != ""
		}
		if !prettier {
			return false, false, fmt.Errorf("prettier metadata must be an object or non-empty string")
		}
	}
	_, eslint = document["eslintConfig"]
	return prettier, eslint, nil
}
