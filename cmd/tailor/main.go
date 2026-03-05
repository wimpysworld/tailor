package main

import (
	"fmt"
	"os"
	"time"

	"github.com/alecthomas/kong"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/docket"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/measure"
	"github.com/wimpysworld/tailor/internal/swatch"
)

var version = "dev"

var cli struct {
	Version kong.VersionFlag `help:"Show version."`
	Fit     FitCmd           `cmd:"" help:"Create a new project with default configuration."`
	Alter   AlterCmd         `cmd:"" help:"Apply swatch templates to the current project."`
	Baste   BasteCmd         `cmd:"" help:"Preview what alter would do without making any changes."`
	Measure MeasureCmd       `cmd:"" help:"Assess project health files and configuration alignment."`
	Docket  DocketCmd        `cmd:"" help:"Display GitHub authentication state and repository context."`
}

// FitCmd creates a new project directory with a default .tailor/config.yml.
type FitCmd struct {
	Path        string `arg:"" help:"Project directory to create."`
	License     string `help:"Licence identifier." default:"MIT"`
	Description string `help:"Repository description."`
}

// Run executes the fit command.
func (f *FitCmd) Run() error {
	if err := gh.CheckAuth(); err != nil {
		return err
	}

	if err := os.MkdirAll(f.Path, 0o755); err != nil {
		return err
	}

	if config.Exists(f.Path) {
		return fmt.Errorf(".tailor/config.yml already exists at %s; edit it directly to change swatch configuration", f.Path)
	}

	cfg, err := config.DefaultConfig(f.License)
	if err != nil {
		return err
	}

	owner, name, ok := gh.RepoContext()

	if ok {
		client, err := api.DefaultRESTClient()
		if err != nil {
			return err
		}
		live, err := gh.ReadRepoSettings(client, owner, name)
		if err != nil {
			return err
		}
		config.MergeRepoSettings(cfg, live, f.Description)
	} else if f.Description != "" {
		if cfg.Repository == nil {
			cfg.Repository = &config.RepositorySettings{}
		}
		cfg.Repository.Description = &f.Description
	}

	today := time.Now().Format("2006-01-02")
	if err := config.Write(f.Path, cfg, today); err != nil {
		return err
	}

	fmt.Printf("Fitted %s with .tailor/config.yml\n", f.Path)
	return nil
}

// AlterCmd applies swatch templates to the current project.
type AlterCmd struct {
	Recut bool `help:"Overwrite all files regardless of mode or existence." name:"recut"`
}

// Run executes the alter command.
func (a *AlterCmd) Run() error {
	mode := alter.Apply
	if a.Recut {
		mode = alter.Recut
	}
	return runAlter(mode)
}

// BasteCmd previews what alter would do without making any changes.
type BasteCmd struct{}

// Run executes the baste command.
func (b *BasteCmd) Run() error {
	return runAlter(alter.DryRun)
}

// runAlter performs auth check, resolves the working directory, loads the
// tailor config, and runs alter with the given mode.
func runAlter(mode alter.ApplyMode) error {
	if err := gh.CheckAuth(); err != nil {
		return err
	}

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		return fmt.Errorf(".tailor/config.yml is missing or malformed. Run 'tailor fit <path>' to create a valid configuration, or edit .tailor/config.yml directly to correct it")
	}

	return alter.Run(cfg, dir, mode, nil)
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
		diff = measure.CheckConfigDiff(cfg, swatch.All())
	}

	fmt.Print(measure.FormatOutput(health, diff, hasConfig))
	return nil
}

// DocketCmd displays GitHub authentication state and repository context.
type DocketCmd struct{}

// Run executes the docket command.
func (d *DocketCmd) Run() error {
	result := docket.Run(nil)
	fmt.Print(docket.FormatOutput(result))
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
