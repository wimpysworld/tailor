package main

import (
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/measure"
	"github.com/wimpysworld/tailor/internal/swatch"
)

var version = "dev"

var cli struct {
	Version kong.VersionFlag `help:"Show version."`
	Measure MeasureCmd       `cmd:"" help:"Assess project health files and configuration alignment."`
}

// MeasureCmd checks community health files and, when a config is present,
// compares it against the built-in default swatch set.
type MeasureCmd struct{}

// Run executes the measure command.
func (m *MeasureCmd) Run() error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	health := measure.CheckHealth(dir)

	hasConfig := config.Exists(dir)
	var diff []measure.DiffResult
	if hasConfig {
		cfg, err := config.Load(dir)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		diff = measure.CheckConfigDiff(cfg, swatch.DefaultSwatchSet())
	}

	fmt.Print(measure.FormatOutput(health, diff, hasConfig))
	return nil
}

func main() {
	ctx := kong.Parse(&cli,
		kong.Name("tailor"),
		kong.Description("Bespoke project templates for GitHub repositories."),
		kong.UsageOnError(),
		kong.Vars{"version": version},
	)

	err := ctx.Run()
	ctx.FatalIfErrorf(err)
}
