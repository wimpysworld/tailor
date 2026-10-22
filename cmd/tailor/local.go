package main

import (
	"fmt"
	"strings"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/output"
)

var applyLocal = alter.ApplyLocal

func (a *AlterCmd) runLocal() error {
	if len(a.Path) == 0 {
		return fmt.Errorf("local-only alteration requires at least one --path")
	}
	dir := a.Directory
	if dir == "" {
		dir = "."
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return err
	}
	updated, err := applyLocal(cfg, dir, a.Path)
	policy := a.output
	if policy == nil {
		policy = output.New(a.stdout, a.stderr, output.Auto)
	}
	var plain strings.Builder
	doc := output.Document{Command: "alter", Context: dir}
	for _, name := range updated {
		fmt.Fprintln(&plain, "updated", name)
		doc.Items = append(doc.Items, output.Item{Domain: "Files", Outcome: output.Applied, Action: "updated", Name: name})
	}
	if err == nil && len(updated) == 0 {
		fmt.Fprintln(&plain, "no local changes")
	}
	if err == nil {
		doc.Summary = output.Summary{Applied: len(updated), Unchanged: len(a.Path) - len(updated)}
	}
	if err == nil || len(updated) > 0 {
		policy.Print(doc, plain.String())
	}
	return err
}
