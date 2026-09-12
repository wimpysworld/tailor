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
	"github.com/wimpysworld/tailor/internal/output"
	"github.com/wimpysworld/tailor/internal/swatch"
	"github.com/wimpysworld/tailor/internal/termtext"
)

// version is set at release time by GoReleaser via -X main.version.
var version = "dev"

// CLI is the root command structure parsed by Kong.
type CLI struct {
	Version    kong.VersionFlag `help:"Show version."`
	Format     output.Format    `help:"Output format." default:"auto" enum:"auto,plain"`
	Color      output.Color     `help:"Colour policy." default:"auto" enum:"auto,always,never"`
	Verbose    bool             `help:"Show every result and stage."`
	Quiet      bool             `help:"Print only the final summary."`
	NoProgress bool             `help:"Disable live progress."`
	Fit        FitCmd           `cmd:"" help:"Create a new project with default configuration."`
	Alter      AlterCmd         `cmd:"" help:"Apply swatch templates to the current project."`
	Baste      BasteCmd         `cmd:"" help:"Preview what alter would do without making any changes."`
	Measure    MeasureCmd       `cmd:"" help:"Assess project health files and configuration alignment."`
	Docket     DocketCmd        `cmd:"" help:"Display GitHub authentication state and repository context."`
}

// FitCmd creates a new project directory with a default .tailor.yml.
type FitCmd struct {
	Path        string  `arg:"" help:"Project directory to create."`
	License     string  `help:"Licence identifier." default:"BlueOak-1.0.0"`
	Description *string `help:"Repository description."`

	stdout io.Writer
	stderr io.Writer
	output *output.Policy
}

// Run executes the fit command.
func (f *FitCmd) Run() (runErr error) {
	policy := f.output
	if policy == nil {
		policy = output.New(f.stdout, f.stderr, output.Auto)
	}
	progress := policy.StartProgress()
	defer progress.Stop()
	stageID, stageLabel := "repository", "Finding repository"
	observe := func(event output.StageEvent) {
		stageID, stageLabel = event.ID, event.Label
		progress.Observe(event)
	}
	defer func() {
		if runErr != nil {
			progress.Observe(output.StageEvent{ID: stageID, Label: stageLabel, Phase: "error", Err: runErr})
		}
	}()
	f.stderr = progress.WarningWriter(policy.Stderr())
	observe(output.StageEvent{ID: stageID, Label: stageLabel, Phase: "start"})
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

	observe(output.StageEvent{ID: "repository", Label: "Repository found", Phase: "complete"})
	// Verify the token against the API before creating the project
	// directory, so an invalid token cannot leave a partial fit behind.
	observe(output.StageEvent{ID: "auth", Label: "Verifying GitHub authentication", Phase: "start"})
	client, _, err := gh.VerifyAuth(repo.Host)
	if err != nil {
		return err
	}
	observe(output.StageEvent{ID: "auth", Label: "GitHub authentication verified", Phase: "complete"})

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
		live, err := gh.ReadRepoMetadata(client, repo.Owner, repo.Name)
		if err != nil {
			return err
		}
		config.MergeRepoMetadata(cfg, live, f.Description)
	} else {
		if f.Description != nil {
			if cfg.Repository == nil {
				cfg.Repository = &model.RepositorySettings{}
			}
			cfg.Repository.Description = f.Description
		}
		config.ApplyRepoDefaults(cfg, projectName(f.Path), "")
	}

	today := time.Now().Format("2006-01-02")
	observe(output.StageEvent{ID: "write", Label: "Creating configuration", Phase: "start"})
	if err := config.Write(f.Path, cfg, today, "Initially fitted"); err != nil {
		return err
	}

	observe(output.StageEvent{ID: "write", Label: "Configuration created", Phase: "complete"})
	progress.Stop()
	renderedPath := f.Path
	if policy.Rich() {
		renderedPath = termtext.EscapeControlText(renderedPath)
	}
	doc := output.Document{Command: "fit", Context: renderedPath, Items: []output.Item{{Domain: "Configuration", Outcome: output.Created, Action: "create", Name: ".tailor.yml", Provenance: renderedPath}}, Guidance: []output.Guidance{{Order: 1, Text: "Change into the project directory, then run `tailor alter`."}}}
	policy.Print(doc, fmt.Sprintf("Fitted %s with .tailor.yml\n", f.Path))
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
	stdout io.Writer
	stderr io.Writer
	output *output.Policy
}

// Run executes the alter command.
func (a *AlterCmd) Run() error {
	mode := alter.Apply
	if a.Recut {
		mode = alter.Recut
	}
	return runAlterWithPolicy(mode, a.output, a.stdout, a.stderr)
}

// BasteCmd previews what alter would do without making any changes.
type BasteCmd struct {
	stdout io.Writer
	stderr io.Writer
	output *output.Policy
}

// Run executes the baste command.
func (b *BasteCmd) Run() error {
	return runAlterWithPolicy(alter.DryRun, b.output, b.stdout, b.stderr)
}

// runAlter performs auth check, resolves the working directory, loads the
// tailor config, and runs alter with the given mode.
func runAlter(mode alter.ApplyMode, stdout, stderr io.Writer) error {
	return runAlterWithPolicy(mode, output.New(stdout, stderr, output.Plain), stdout, stderr)
}

func runAlterWithPolicy(mode alter.ApplyMode, policy *output.Policy, stdout, stderr io.Writer) (runErr error) {
	if policy == nil {
		policy = output.New(stdout, stderr, output.Auto)
	}
	stderr = policy.Stderr()
	progress := policy.StartProgress()
	defer progress.Stop()
	stageID, stageLabel, stagePhase := "preflight", "Preparing command", "start"
	observe := func(event output.StageEvent) {
		stageID, stageLabel, stagePhase = event.ID, event.Label, event.Phase
		progress.Observe(event)
	}
	defer func() {
		if runErr != nil && stagePhase != "error" {
			progress.Observe(output.StageEvent{ID: stageID, Label: stageLabel, Phase: "error", Err: runErr})
		}
	}()
	observe(output.StageEvent{ID: stageID, Label: stageLabel, Phase: "start"})

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

	observe(output.StageEvent{ID: "preflight", Label: "Command prepared", Phase: "complete"})
	stderr = progress.WarningWriter(stderr)
	report, err := alter.Execute(cfg, dir, mode, nil, stderr, alter.Options{Observer: observe})
	progress.Stop()
	policy.Print(report.Document, report.Plain)
	return err
}

// MeasureCmd checks community health files and, when a config is present,
// compares it against the built-in default swatch set.
type MeasureCmd struct {
	stdout io.Writer
	stderr io.Writer
	output *output.Policy
}

// Run executes the measure command.
func (m *MeasureCmd) Run() (runErr error) {
	policy := m.output
	if policy == nil {
		policy = output.New(m.stdout, m.stderr, output.Auto)
	}
	progress := policy.StartProgress()
	defer progress.Stop()
	defer func() {
		if runErr != nil {
			progress.Observe(output.StageEvent{ID: "measure", Label: "Measuring local files", Phase: "error", Err: runErr})
		}
	}()
	progress.Observe(output.StageEvent{ID: "measure", Label: "Measuring local files", Phase: "start"})
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

	progress.Observe(output.StageEvent{ID: "measure", Label: "Local files measured", Phase: "complete"})
	progress.Stop()
	doc := measureDocument(filepath.Base(dir), health, diff, hasConfig)
	policy.Print(doc, measure.FormatOutput(health, diff, hasConfig))
	return nil
}

func measureDocument(context string, health []measure.HealthResult, diff []measure.DiffResult, hasConfig bool) output.Document {
	doc := output.Document{Command: "measure", Context: context}
	for _, result := range health {
		outcome := output.Unchanged
		if result.Status != measure.Present {
			outcome = output.Attention
		}
		doc.Items = append(doc.Items, output.Item{Domain: "Files", Category: "Community health", Outcome: outcome, Action: string(result.Status), Name: result.Path, Reason: result.Detail, Provenance: result.Path})
	}
	for _, result := range diff {
		doc.Items = append(doc.Items, output.Item{Domain: "Configuration", Category: "Swatches", Outcome: output.Attention, Action: string(result.Category), Name: result.Path, Reason: result.Detail, Provenance: result.Path})
	}
	if !hasConfig {
		doc.Notices = append(doc.Notices, output.Notice{Level: "notice", Text: "No .tailor.yml found."})
		doc.Guidance = append(doc.Guidance, output.Guidance{Order: 1, Text: "Run `tailor fit <path>` to initialise."})
	}
	doc.Summary = output.Count(doc.Items)
	return doc
}

// DocketCmd displays GitHub authentication state and repository context.
type DocketCmd struct {
	client *api.RESTClient
	stdout io.Writer
	stderr io.Writer
	output *output.Policy
}

// Run executes the docket command.
func (d *DocketCmd) Run() (runErr error) {
	policy := d.output
	if policy == nil {
		policy = output.New(d.stdout, d.stderr, output.Auto)
	}
	progress := policy.StartProgress()
	defer progress.Stop()
	defer func() {
		if runErr != nil {
			progress.Observe(output.StageEvent{ID: "identity", Label: "Verifying GitHub identity", Phase: "error", Err: runErr})
		}
	}()
	progress.Observe(output.StageEvent{ID: "identity", Label: "Verifying GitHub identity", Phase: "start"})
	result, err := docket.Run(d.client)
	if err != nil {
		return err
	}
	progress.Observe(output.StageEvent{ID: "identity", Label: "GitHub identity checked", Phase: "complete"})
	progress.Stop()
	context := result.Repository
	if context == "(none)" {
		context = ""
	}
	doc := output.Document{Command: "docket", Context: context, Items: []output.Item{
		{Domain: "Identity", Outcome: output.Unchanged, Name: "User", After: result.User},
		{Domain: "Identity", Outcome: output.Unchanged, Name: "Repository", After: result.Repository},
		{Domain: "Identity", Outcome: output.Unchanged, Name: "Authentication", After: result.Auth},
	}}
	policy.Print(doc, docket.FormatOutput(result))
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

	ctx, err := parser.Parse(args)
	parser.FatalIfErrorf(err)

	policy := output.New(stdout, stderr, cli.Format, output.WithColor(cli.Color), output.WithVerbose(cli.Verbose), output.WithQuiet(cli.Quiet), output.WithNoProgress(cli.NoProgress))
	cli.Fit.stdout = stdout
	cli.Fit.stderr = stderr
	cli.Fit.output = policy
	cli.Alter.stdout = stdout
	cli.Alter.stderr = stderr
	cli.Alter.output = policy
	cli.Baste.stdout = stdout
	cli.Baste.stderr = stderr
	cli.Baste.output = policy
	cli.Measure.stdout = stdout
	cli.Measure.stderr = stderr
	cli.Measure.output = policy
	cli.Docket.stdout = stdout
	cli.Docket.stderr = stderr
	cli.Docket.output = policy

	err = ctx.Run()
	ctx.FatalIfErrorf(err)
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
