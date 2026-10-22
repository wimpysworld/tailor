package config

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestBlenderStrictParsing(t *testing.T) {
	for _, value := range []string{"null", "", "'true'", "1", "yes", "[]", "{enabled: true}"} {
		t.Run(value, func(t *testing.T) {
			_, err := parseAndValidate([]byte("mcp: {blender: "+value+"}\n"), "config")
			if err == nil || !strings.Contains(err.Error(), "mcp.blender must be a bool") {
				t.Fatalf("error = %v", err)
			}
		})
	}
	_, err := parseAndValidate([]byte("mcp: {blender: true, blender: false}\n"), "config")
	if err == nil || !strings.Contains(err.Error(), `mapping key "blender" already defined`) {
		t.Fatalf("duplicate error = %v", err)
	}
}

func TestMCPMixedStatesPersist(t *testing.T) {
	states := []*bool{nil, new(false), new(true)}
	for _, blender := range states {
		for _, playwright := range states {
			t.Run(fmt.Sprintf("%v/%v", blender, playwright), func(t *testing.T) {
				cfg := &Config{License: "none", MCP: &MCPSettings{Blender: blender, Playwright: playwright}}
				want := *cfg.MCP
				dir := t.TempDir()
				for range 2 {
					if _, err := MergeDefaults(cfg); err != nil {
						t.Fatal(err)
					}
					if err := Write(dir, cfg, "2026-10-21", "Altered"); err != nil {
						t.Fatal(err)
					}
					var err error
					cfg, err = Load(dir)
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(*cfg.MCP, want) {
						t.Fatalf("MCP = %+v, want %+v", *cfg.MCP, want)
					}
					if cfg.BlenderDeclared() != (blender != nil) || cfg.BlenderEnabled() != (blender != nil && *blender) {
						t.Fatal("incorrect Blender state")
					}
				}
			})
		}
	}
}
