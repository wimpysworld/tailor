package main

import (
	"fmt"
	"os"
	"time"

	"github.com/alecthomas/kong"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/measure"
	"github.com/wimpysworld/tailor/internal/swatch"
)

var version = "dev"

var cli struct {
	Version kong.VersionFlag `help:"Show version."`
	Fit     FitCmd           `cmd:"" help:"Create a new project with default configuration."`
	Measure MeasureCmd       `cmd:"" help:"Assess project health files and configuration alignment."`
	Alter   AlterCmd         `cmd:"" help:"Apply swatch templates to the current project."`
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
	Apply      bool `help:"Apply changes." xor:"mode"`
	ForceApply bool `help:"Apply and overwrite regardless of mode or existence." name:"force-apply" xor:"mode"`
}

// Run executes the alter command.
func (a *AlterCmd) Run() error {
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

	mode := alter.DryRun
	if a.ForceApply {
		mode = alter.ForceApply
	} else if a.Apply {
		mode = alter.Apply
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
