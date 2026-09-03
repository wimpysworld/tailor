package config

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"reflect"
	"slices"
	"strings"
	"text/template"

	"github.com/wimpysworld/tailor/internal/model"
	"gopkg.in/yaml.v3"
)

// yamlDoubleQuoted preserves the established style. yaml.v3 handles all
// quoting that YAML syntax requires.
const yamlDoubleQuoted = "{}[]#&*!|>'\"%@`"

// yamlVal encodes v as one YAML string scalar. Newlines use double quotes so
// the result stays on one line when the template adds its indentation.
func yamlVal(v string) (string, error) {
	var node yaml.Node
	node.SetString(v)
	if strings.ContainsAny(v, yamlDoubleQuoted) || strings.Contains(v, "\n") {
		node.Style = yaml.DoubleQuotedStyle
	}

	encoded, err := yaml.Marshal(&node)
	if err != nil {
		return "", fmt.Errorf("encoding YAML scalar: %w", err)
	}
	return strings.TrimSuffix(string(encoded), "\n"), nil
}

// settingLines renders one "  key: value" line per set field. Scalar fields
// keep struct order; list fields follow them, preserving the output order
// that places topics last.
func settingLines(fields []model.SettingField) ([]string, error) {
	var lines, lists []string
	for _, field := range fields {
		if !field.Set {
			continue
		}
		v := field.Value.Elem()
		switch v.Kind() {
		case reflect.String:
			value, err := yamlVal(v.String())
			if err != nil {
				return nil, err
			}
			lines = append(lines, fmt.Sprintf("  %s: %s", field.YAMLKey, value))
		case reflect.Bool:
			lines = append(lines, fmt.Sprintf("  %s: %t", field.YAMLKey, v.Bool()))
		case reflect.Slice:
			list, err := listLines(field.YAMLKey, v)
			if err != nil {
				return nil, err
			}
			lists = append(lists, list)
		}
	}
	return append(lines, lists...), nil
}

// listLines renders a string-list field as a block sequence, or [] when empty.
func listLines(key string, v reflect.Value) (string, error) {
	if v.Len() == 0 {
		return fmt.Sprintf("  %s: []", key), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "  %s:", key)
	for i := range v.Len() {
		value, err := yamlVal(v.Index(i).String())
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "\n    - %s", value)
	}
	return b.String(), nil
}

// templateFuncs provides helpers for the config template.
var templateFuncs = template.FuncMap{
	"yamlVal": yamlVal,
	"repositoryLines": func(r *model.RepositorySettings) ([]string, error) {
		return settingLines(model.RepositorySettingFields(r))
	},
	"actionsLines": func(a *model.ActionsSettings) ([]string, error) {
		return settingLines(model.ActionsSettingFields(a))
	},
	"codeScanningLines": func(c *model.CodeScanningSettings) ([]string, error) {
		lines, err := settingLines(model.CodeScanningSettingFields(c))
		return withLanguagesComment(lines, model.CodeScanningLanguages), err
	},
	"codeQualityLines": func(c *model.CodeQualitySettings) ([]string, error) {
		lines, err := settingLines(model.CodeQualitySettingFields(c))
		return withLanguagesComment(lines, model.CodeQualityLanguages), err
	},
}

// withLanguagesComment inserts the languages guidance comment before the
// languages line, so the valid values in the config match the model.
func withLanguagesComment(lines []string, valid []string) []string {
	index := slices.IndexFunc(lines, func(line string) bool {
		return strings.HasPrefix(line, "  languages:")
	})
	if index == -1 {
		return lines
	}
	comment := []string{
		"  # An empty list means GitHub detects the languages. Valid values:",
		"  # " + strings.Join(valid, ", "),
	}
	return slices.Insert(lines, index, comment...)
}

// configTemplate renders .tailor.yml in the exact format specified. It uses
// text/template rather than yaml.Marshal to control key order, blank lines
// between swatch entries, and omission of nil pointer fields.
var configTemplate = template.Must(template.New("config").Funcs(templateFuncs).Parse(
	`# {{ .Verb }} by tailor on {{ .Date }}
license: {{ yamlVal .License }}

{{- if .Repository }}

repository:
{{- range repositoryLines .Repository }}
{{ . }}
{{- end }}
{{- end }}
{{- if .Actions }}

actions:
{{- range actionsLines .Actions }}
{{ . }}
{{- end }}
{{- end }}
{{- if .CodeScanning }}

code_scanning:
{{- range codeScanningLines .CodeScanning }}
{{ . }}
{{- end }}
{{- end }}
{{- if .CodeQuality }}

code_quality:
{{- range codeQualityLines .CodeQuality }}
{{ . }}
{{- end }}
{{- end }}
{{- if .Labels }}

labels:
{{- range $i, $l := .Labels }}
{{ if $i }}
{{ end }}  - name: {{ yamlVal $l.Name }}
    color: {{ yamlVal $l.Color }}
    description: {{ yamlVal $l.Description }}
{{- end }}
{{- end }}

swatches:
{{- range $i, $s := .Swatches }}
{{ if $i }}
{{ end }}  - path: {{ $s.Path }}
    alteration: {{ $s.Alteration }}
{{- end }}
`))

// Write renders cfg to <dir>/.tailor.yml with the given header date and verb.
func Write(dir string, cfg *Config, date string, verb string) (retErr error) {
	var buf bytes.Buffer
	if err := configTemplate.Execute(&buf, struct {
		Date string
		Verb string
		*Config
	}{
		Date:   date,
		Verb:   verb,
		Config: cfg,
	}); err != nil {
		return fmt.Errorf("rendering config template: %w", err)
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("opening project root %q: %w", dir, err)
	}
	defer root.Close()

	mode := os.FileMode(0o644)
	info, err := root.Lstat(configPath)
	switch {
	case err == nil:
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("writing config: %s is a symbolic link", configPath)
		}
		if info.Mode().IsRegular() {
			mode = info.Mode().Perm()
		}
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("inspecting config: %w", err)
	}

	tempPath := configPath + ".tmp-" + rand.Text()
	temp, err := root.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("creating temporary config: %w", err)
	}
	tempOpen := true
	defer func() {
		if tempOpen {
			if err := temp.Close(); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("closing temporary config: %w", err))
			}
		}
		if tempPath != "" {
			if err := root.Remove(tempPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
				retErr = errors.Join(retErr, fmt.Errorf("removing temporary config: %w", err))
			}
		}
	}()

	if _, err := temp.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("writing temporary config: %w", err)
	}
	if err := temp.Chmod(mode); err != nil {
		return fmt.Errorf("setting temporary config permissions: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("syncing temporary config: %w", err)
	}
	if err := temp.Close(); err != nil {
		tempOpen = false
		return fmt.Errorf("closing temporary config: %w", err)
	}
	tempOpen = false

	if err := root.Rename(tempPath, configPath); err != nil {
		return fmt.Errorf("replacing config: %w", err)
	}
	tempPath = ""

	rootDir, err := root.Open(".")
	if err != nil {
		return fmt.Errorf("opening config directory: %w", err)
	}
	if err := rootDir.Sync(); err != nil {
		_ = rootDir.Close()
		return fmt.Errorf("syncing config directory: %w", err)
	}
	if err := rootDir.Close(); err != nil {
		return fmt.Errorf("closing config directory: %w", err)
	}

	return nil
}
