package config

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"strings"
	"text/template"

	"github.com/wimpysworld/tailor/internal/model"
)

// yamlSpecial lists characters that require quoting in YAML values.
const yamlSpecial = "{}[]#&*!|>'\"%@`\n"

// yamlVal quotes v when it contains YAML special characters, a ":" that
// reads as a mapping indicator (followed by a space or tab, or at the end),
// surrounding whitespace, or is empty. A ":" inside a word, as in URLs,
// needs no quoting.
func yamlVal(v string) string {
	if strings.ContainsAny(v, yamlSpecial) || strings.Contains(v, ": ") ||
		strings.Contains(v, ":\t") || strings.HasSuffix(v, ":") ||
		v != strings.TrimSpace(v) || v == "" {
		return fmt.Sprintf("%q", v)
	}
	return v
}

// settingLines renders one "  key: value" line per set field. Scalar fields
// keep struct order; list fields follow them, preserving the output order
// that places topics last.
func settingLines(fields []model.RepositorySettingField) []string {
	var lines, lists []string
	for _, field := range fields {
		if !field.Set {
			continue
		}
		v := field.Value.Elem()
		switch v.Kind() {
		case reflect.String:
			lines = append(lines, fmt.Sprintf("  %s: %s", field.YAMLKey, yamlVal(v.String())))
		case reflect.Bool:
			lines = append(lines, fmt.Sprintf("  %s: %t", field.YAMLKey, v.Bool()))
		case reflect.Slice:
			lists = append(lists, listLines(field.YAMLKey, v))
		}
	}
	return append(lines, lists...)
}

// listLines renders a string-list field as a block sequence, or [] when empty.
func listLines(key string, v reflect.Value) string {
	if v.Len() == 0 {
		return fmt.Sprintf("  %s: []", key)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "  %s:", key)
	for i := range v.Len() {
		fmt.Fprintf(&b, "\n    - %s", yamlVal(v.Index(i).String()))
	}
	return b.String()
}

// templateFuncs provides helpers for the config template.
var templateFuncs = template.FuncMap{
	"yamlVal": yamlVal,
	"repositoryLines": func(r *model.RepositorySettings) []string {
		return settingLines(model.RepositorySettingFields(r))
	},
	"actionsLines": func(a *model.ActionsSettings) []string {
		return settingLines(model.ActionsSettingFields(a))
	},
}

// configTemplate renders .tailor.yml in the exact format specified. It uses
// text/template rather than yaml.Marshal to control key order, blank lines
// between swatch entries, and omission of nil pointer fields.
var configTemplate = template.Must(template.New("config").Funcs(templateFuncs).Parse(
	`# {{ .Verb }} by tailor on {{ .Date }}
license: {{ .License }}

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
func Write(dir string, cfg *Config, date string, verb string) error {
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

	if err := root.WriteFile(configPath, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}
