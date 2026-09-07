package config

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/model"
)

func TestVariablesYAML(t *testing.T) {
	for _, tt := range []struct {
		name, input, want string
		valid             bool
	}{
		{"omitted", "license: none", "", true},
		{"empty sequence", "variables: []", "", true},
		{"quoted", "variables: [{name: APP, value: '001'}]", "001", true},
		{"empty string", "variables: [{name: APP, value: \"\"}]", "", true},
		{"string tag", "variables: [{name: APP, value: !!str 123}]", "123", true},
		{"block", "variables:\n  - name: APP\n    value: |\n      one\n      two\n", "one\ntwo\n", true},
		{"whitespace", "variables: [{name: APP, value: '  Keep Me  '}]", "  Keep Me  ", true},
		{"unicode value", "variables: [{name: APP, value: '日本語'}]", "日本語", true},
		{"map", "variables: {APP: text}", "", false},
		{"null section", "variables: null", "", false},
		{"scalar section", "variables: text", "", false},
		{"scalar entry", "variables: [text]", "", false},
		{"null entry", "variables: [null]", "", false},
		{"sequence entry", "variables: [[]]", "", false},
		{"missing value", "variables: [{name: APP}]", "", false},
		{"missing name", "variables: [{value: text}]", "", false},
		{"null value", "variables: [{name: APP, value: null}]", "", false},
		{"implicit null", "variables: [{name: APP, value: }]", "", false},
		{"integer", "variables: [{name: APP, value: 123}]", "", false},
		{"float", "variables: [{name: APP, value: 1.5}]", "", false},
		{"boolean", "variables: [{name: APP, value: true}]", "", false},
		{"timestamp", "variables: [{name: APP, value: 2026-09-07}]", "", false},
		{"map value", "variables: [{name: APP, value: {key: text}}]", "", false},
		{"sequence value", "variables: [{name: APP, value: [text]}]", "", false},
		{"integer name", "variables: [{name: 123, value: text}]", "", false},
		{"boolean name", "variables: [{name: true, value: text}]", "", false},
		{"unknown key", "variables: [{name: APP, value: text, extra: ignored}]", "", false},
		{"duplicate key", "variables: [{name: APP, value: one, value: two}]", "", false},
		{"merge key", "variables: [{name: APP, value: text, <<: {extra: ignored}}]", "", false},
		{"root merge", "<<: {variables: [{name: APP, value: 123}]}", "", false},
		{"duplicate name", "variables: [{name: APP, value: one}, {name: app, value: two}]", "", false},
		{"unknown section", "variables: []\nvariable: []", "", false},
		{"syntax", "variables: [", "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseAndValidate([]byte(tt.input), "test")
			if !tt.valid {
				if err == nil {
					t.Fatal("invalid YAML accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(cfg.Variables) > 0 && (cfg.Variables[0].Value == nil || *cfg.Variables[0].Value != tt.want) {
				t.Fatalf("value = %v, want %q", cfg.Variables[0].Value, tt.want)
			}
		})
	}
}

func TestValidateVariableNames(t *testing.T) {
	for _, tt := range []struct {
		name  string
		valid bool
	}{
		{"A", true},
		{"_", true},
		{"app_123", true},
		{"RUNNER_NAME", true},
		{"GITHUB", true},
		{strings.Repeat("A", 1000), true},
		{"", false},
		{"1APP", false},
		{"APP-NAME", false},
		{"APP NAME", false},
		{"APP\n", false},
		{"é", false},
		{"日本語", false},
		{"GITHUB_NAME", false},
		{"github_name", false},
		{"GitHub_NAME", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Variables: []model.VariableEntry{{Name: tt.name, Value: new("")}}}
			if err := ValidateVariables(cfg); (err == nil) != tt.valid {
				t.Fatalf("ValidateVariables() = %v, valid = %t", err, tt.valid)
			}
		})
	}
}

func TestValidateVariableLimits(t *testing.T) {
	for _, count := range []int{0, 499, 500, 501} {
		t.Run(fmt.Sprintf("count_%d", count), func(t *testing.T) {
			cfg := &Config{}
			for i := range count {
				cfg.Variables = append(cfg.Variables, model.VariableEntry{Name: fmt.Sprintf("VAR_%d", i), Value: new("")})
			}
			if err := ValidateVariables(cfg); (err == nil) != (count <= 500) {
				t.Fatalf("ValidateVariables() = %v", err)
			}
		})
	}
	for _, tt := range []struct {
		name, value string
		valid       bool
	}{
		{"empty", "", true},
		{"below limit", strings.Repeat("a", 48*1024-1), true},
		{"at limit", strings.Repeat("a", 48*1024), true},
		{"above limit", strings.Repeat("a", 48*1024+1), false},
		{"unicode at limit", strings.Repeat("é", 24*1024), true},
		{"unicode above limit", strings.Repeat("é", 24*1024) + "a", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Variables: []model.VariableEntry{{Name: "APP", Value: &tt.value}}}
			if err := ValidateVariables(cfg); (err == nil) != tt.valid {
				t.Fatalf("ValidateVariables() = %v", err)
			}
		})
	}
	if err := ValidateVariables(&Config{Variables: []model.VariableEntry{{Name: "APP"}}}); err == nil {
		t.Fatal("missing value accepted")
	}
}
