package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/alecthomas/kong"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/docket"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/measure"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/swatch"
)

// version is set at release time by GoReleaser via -X main.version.
var version = "dev"

// CLI is the root command structure parsed by Kong.
type CLI struct {
	Version kong.VersionFlag `help:"Show version."`
	Fit     FitCmd           `cmd:"" help:"Create a new project with default configuration."`
	Alter   AlterCmd         `cmd:"" help:"Apply swatch templates to the current project."`
	Baste   BasteCmd         `cmd:"" help:"Preview what alter would do without making any changes."`
	Measure MeasureCmd       `cmd:"" help:"Assess project health files and configuration alignment."`
	Docket  DocketCmd        `cmd:"" help:"Display GitHub authentication state and repository context."`
}

// FitCmd creates a new project directory with a default .tailor.yml.
type FitCmd struct {
	Path        string `arg:"" help:"Project directory to create."`
	License     string `help:"Licence identifier." default:"BlueOak-1.0.0"`
	Description string `help:"Repository description."`

	stderr io.Writer
}

// Run executes the fit command.
func (f *FitCmd) Run() error {
	// Resolve the repository context before the auth check so the check
	// verifies a token for the host that will be written to. The path may
	// not exist yet; a fresh directory has no repository context.
	var repo gh.Repo
	var ok bool
	if info, statErr := os.Stat(f.Path); statErr == nil && info.IsDir() {
		var err error
		repo, ok, err = gh.RepoContextAt(f.Path)
		if err != nil {
			return err
		}
	}

	// Verify the token against the API before creating the project
	// directory, so an invalid token cannot leave a partial fit behind.
	client, _, err := gh.VerifyAuth(repo.Host)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(f.Path, 0o755); err != nil {
		return err
	}

	hasConfig, err := config.Exists(f.Path)
	if err != nil {
		return err
	}
	if hasConfig {
		return fmt.Errorf(".tailor.yml already exists at %s; edit it directly to change swatch configuration", f.Path)
	}

	cfg, err := config.DefaultConfig(f.License)
	if err != nil {
		return err
	}

	if ok {
		live, warnings, err := gh.ReadRepoSettings(client, repo.Owner, repo.Name)
		if err != nil {
			return err
		}
		stderr := f.stderr
		if stderr == nil {
			stderr = os.Stderr
		}
		for _, w := range warnings {
			fmt.Fprintf(stderr, "warning: %v\n", w)
		}
		config.MergeRepoSettings(cfg, live, f.Description)
		homepage := fmt.Sprintf("https://%s/%s/%s", repo.Host, repo.Owner, repo.Name)
		config.ApplyRepoDefaults(cfg, repo.Name, homepage)
	} else {
		if f.Description != "" {
			if cfg.Repository == nil {
				cfg.Repository = &model.RepositorySettings{}
			}
			cfg.Repository.Description = &f.Description
		}
		config.ApplyRepoDefaults(cfg, projectName(f.Path), "")
	}

	today := time.Now().Format("2006-01-02")
	if err := config.Write(f.Path, cfg, today, "Initially fitted"); err != nil {
		return err
	}

	fmt.Printf("Fitted %s with .tailor.yml\n", f.Path)
	return nil
}

// projectName derives the fallback description from the target directory name.
func projectName(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return filepath.Base(abs)
	}
	return filepath.Base(path)
}

// AlterCmd applies swatch templates to the current project.
type AlterCmd struct {
	Recut  bool `help:"Overwrite existing first-fit swatches and merge missing .tailor.yml defaults (never swatches and the licence stay untouched)." name:"recut"`
	run    func(alter.ApplyMode) error
	stdout io.Writer
	stderr io.Writer
}

// Run executes the alter command.
func (a *AlterCmd) Run() error {
	mode := alter.Apply
	if a.Recut {
		mode = alter.Recut
	}
	if a.run != nil {
		return a.run(mode)
	}
	return runAlter(mode, a.stdout, a.stderr)
}

// BasteCmd previews what alter would do without making any changes.
type BasteCmd struct {
	run    func(alter.ApplyMode) error
	stdout io.Writer
	stderr io.Writer
}

// Run executes the baste command.
func (b *BasteCmd) Run() error {
	if b.run != nil {
		return b.run(alter.DryRun)
	}
	return runAlter(alter.DryRun, b.stdout, b.stderr)
}

// runAlter performs auth check, resolves the working directory, loads the
// tailor config, and runs alter with the given mode.
func runAlter(mode alter.ApplyMode, stdout, stderr io.Writer) error {
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Check auth for the host of the detected repository so writes cannot
	// go through a token for a different authenticated host.
	repo, _, err := gh.RepoContextAt(dir)
	if err != nil {
		return err
	}
	if err := gh.CheckAuth(repo.Host); err != nil {
		return err
	}

	cfg, err := config.Load(dir)
	if err != nil {
		return fmt.Errorf(".tailor.yml is missing or malformed: %w. Run 'tailor fit <path>' to create a valid configuration, or edit .tailor.yml directly to correct it", err)
	}

	return alter.Run(cfg, dir, mode, nil, stdout, stderr)
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

	hasConfig, err := config.Exists(dir)
	if err != nil {
		return err
	}
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
type DocketCmd struct {
	client *api.RESTClient
}

// Run executes the docket command.
func (d *DocketCmd) Run() error {
	result, err := docket.Run(d.client)
	if err != nil {
		return err
	}
	fmt.Print(docket.FormatOutput(result))
	return nil
}

// exitStatus carries an exit code through a panic raised by Kong's Exit
// function, so run can unwind mid-parse (help, version, usage errors) and
// return the code instead of terminating the process.
type exitStatus int

// run parses args with Kong, executes the selected command, and returns the
// process exit code. Help, usage, and error output go to the given writers.
func run(args []string, stdout, stderr io.Writer) (code int) {
	defer func() {
		if r := recover(); r != nil {
			status, ok := r.(exitStatus)
			if !ok {
				panic(r)
			}
			code = int(status)
		}
	}()

	var cli CLI
	parser, err := kong.New(&cli,
		kong.Name("tailor"),
		kong.Description("Bespoke project templates for GitHub repositories."),
		kong.UsageOnError(),
		kong.Vars{"version": version},
		kong.Writers(stdout, stderr),
		kong.Exit(func(c int) { panic(exitStatus(c)) }),
	)
	if err != nil {
		// A construction error is a programming error in the CLI grammar,
		// matching kong.Parse behaviour.
		panic(err)
	}

	cli.Fit.stderr = stderr
	cli.Alter.stdout = stdout
	cli.Alter.stderr = stderr
	cli.Baste.stdout = stdout
	cli.Baste.stderr = stderr

	ctx, err := parser.Parse(args)
	parser.FatalIfErrorf(err)

	err = ctx.Run()
	ctx.FatalIfErrorf(err)
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
