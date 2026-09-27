package main

import (
	"fmt"
	"io"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
)

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
	updated, err := alter.ApplyLocal(cfg, dir, a.Path)
	stdout := a.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	for _, name := range updated {
		fmt.Fprintln(stdout, "updated", name)
	}
	if err == nil && len(updated) == 0 {
		fmt.Fprintln(stdout, "no local changes")
	}
	return err
}
