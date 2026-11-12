package alter

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const managedInspectionLimit = 1 << 20

type managedSetupState uint8

const (
	managedSetupUnknown managedSetupState = iota
	managedSetupMissing
	managedSetupPresent
)

type managedSetupInspection struct {
	state managedSetupState
	cause string
}

func unknownManagedSetup(cause string) managedSetupInspection {
	return managedSetupInspection{cause: cause}
}

func readManagedInspection(dir, name string) ([]byte, string) {
	if dir == "" {
		return nil, "the project directory was not supplied"
	}
	if strings.Contains(name, "\\") || !filepath.IsLocal(name) || path.Clean(name) != name {
		return nil, "the path is not a local clean path"
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, "the project directory cannot be opened"
	}
	defer func() { _ = root.Close() }()
	current := ""
	parts := strings.Split(name, "/")
	for index, part := range parts {
		current = path.Join(current, part)
		info, err := root.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil, "the file or a parent is absent"
		}
		if err != nil {
			return nil, "the file or a parent cannot be inspected"
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, "the file or a parent is a symlink"
		}
		if index < len(parts)-1 && !info.IsDir() {
			return nil, "a parent is not a directory"
		}
		if index == len(parts)-1 && !info.Mode().IsRegular() {
			return nil, "the file is not regular"
		}
	}
	file, err := openManagedInspectionFile(root, name)
	if err != nil {
		return nil, "the file cannot be opened without following symlinks"
	}
	defer func() { _ = file.Close() }()
	before, err := file.Stat()
	if err != nil || !before.Mode().IsRegular() {
		return nil, "the opened file is not regular"
	}
	if before.Size() > managedInspectionLimit {
		return nil, "the file exceeds the 1 MiB inspection limit"
	}
	content, err := io.ReadAll(io.LimitReader(file, managedInspectionLimit+1))
	if err != nil {
		return nil, "the file cannot be read"
	}
	if len(content) > managedInspectionLimit {
		return nil, "the file exceeds the 1 MiB inspection limit"
	}
	after, err := file.Stat()
	currentInfo, pathErr := root.Lstat(name)
	if err != nil || pathErr != nil || !os.SameFile(before, currentInfo) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return nil, "the file changed during inspection"
	}
	return content, ""
}

func inspectManagedClient(content []byte, destination string) (map[string]any, string) {
	var document map[string]any
	collection := "mcpServers"
	if destination == ".codex/config.toml" {
		collection = "mcp_servers"
		if err := toml.Unmarshal(content, &document); err != nil {
			return nil, "invalid TOML"
		}
	} else {
		if destination == "opencode.json" {
			collection = "mcp"
		}
		decoder := json.NewDecoder(bytes.NewReader(content))
		value, err := readManagedJSONValue(decoder, 0)
		if err != nil {
			return nil, "invalid or ambiguous JSON"
		}
		if _, err := decoder.Token(); err != io.EOF {
			return nil, "invalid or ambiguous JSON"
		}
		var ok bool
		document, ok = value.(map[string]any)
		if !ok {
			return nil, "the JSON root is not an object"
		}
	}
	value, exists := document[collection]
	if !exists {
		return map[string]any{}, ""
	}
	entries, ok := value.(map[string]any)
	if !ok {
		return nil, "the server collection is not an object or table"
	}
	return entries, ""
}

// Duplicate keys make missing-entry claims unsafe, even when a JSON decoder accepts them.
func readManagedJSONValue(decoder *json.Decoder, depth int) (any, error) {
	if depth > 64 {
		return nil, fmt.Errorf("JSON nesting exceeds inspection limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch token {
	case json.Delim('{'):
		object := make(map[string]any)
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("JSON object key is not a string")
			}
			if _, exists := object[name]; exists {
				return nil, fmt.Errorf("duplicate JSON key")
			}
			value, err := readManagedJSONValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			object[name] = value
		}
		_, err := decoder.Token()
		return object, err
	case json.Delim('['):
		for decoder.More() {
			if _, err := readManagedJSONValue(decoder, depth+1); err != nil {
				return nil, err
			}
		}
		_, err := decoder.Token()
		return []any{}, err
	default:
		return token, nil
	}
}

func inspectManagedJust(content []byte) managedSetupInspection {
	present := false
	composition := ""
	for line := range strings.SplitSeq(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.Contains(trimmed, "'''") || strings.Contains(trimmed, `"""`) || strings.Contains(trimmed, "```") {
			return unknownManagedSetup("multiline Just expressions need manual inspection")
		}
		if strings.TrimLeft(line, " \t") != line {
			continue
		}
		if strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "import? ") {
			rest := strings.TrimSpace(line[strings.IndexByte(line, ' ')+1:])
			if len(rest) < 2 || rest[0] != '\'' && rest[0] != '"' {
				return unknownManagedSetup("a Just import is not a literal path")
			}
			end := strings.IndexByte(rest[1:], rest[0])
			if end < 0 {
				return unknownManagedSetup("a Just import is incomplete")
			}
			importPath := rest[1 : end+1]
			if strings.ContainsAny(importPath, "\\\n\r") {
				return unknownManagedSetup("a Just import uses an escaped path")
			}
			tail := strings.TrimSpace(rest[end+2:])
			if tail != "" && !strings.HasPrefix(tail, "#") {
				return unknownManagedSetup("a Just import has an unsupported expression")
			}
			if importPath == "just/loader.just" || importPath == "./just/loader.just" {
				present = true
			} else {
				composition = "another Just import can provide the loader"
			}
			continue
		}
		if strings.HasPrefix(line, "mod ") || strings.HasPrefix(line, "mod? ") {
			composition = "Just module composition needs manual inspection"
			continue
		}
		if strings.Contains(line, ":") || strings.HasPrefix(line, "set ") || strings.HasPrefix(line, "[") {
			continue
		}
		return unknownManagedSetup("unsupported Just syntax")
	}
	if present {
		return managedSetupInspection{state: managedSetupPresent}
	}
	if composition != "" {
		return unknownManagedSetup(composition)
	}
	return managedSetupInspection{state: managedSetupMissing}
}
