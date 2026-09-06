package config

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"reflect"
	"slices"
	"strings"
	"text/template"
	"unicode"

	"github.com/wimpysworld/tailor/internal/model"
	"gopkg.in/yaml.v3"
)

// yamlDoubleQuoted preserves the established style. yaml.v3 handles all
// quoting that YAML syntax requires.
const yamlDoubleQuoted = "{}[]#&*!|>'\"%@`"

// yamlVal encodes v as one YAML string scalar. Newlines use double quotes so
// the result stays on one line when the template adds its indentation.
func yamlVal(v string) (string, error) {
	var node yaml.Node
	node.SetString(v)
	if strings.ContainsAny(v, yamlDoubleQuoted) || strings.Contains(v, "\n") {
		node.Style = yaml.DoubleQuotedStyle
	}

	encoded, err := yaml.Marshal(&node)
	if err != nil {
		return "", fmt.Errorf("encoding YAML scalar: %w", err)
	}
	result := strings.TrimSuffix(string(encoded), "\n")
	// yaml.v3 escapes all non-BMP characters, although YAML permits them.
	// Match escaped backslashes first to preserve literal escape-looking text.
	replacements := []string{`\\`, `\\`}
	for _, r := range v {
		if r > '\uFFFF' && unicode.IsPrint(r) {
			replacements = append(replacements, fmt.Sprintf(`\U%08X`, r), string(r))
		}
	}
	if len(replacements) > 2 {
		result = strings.NewReplacer(replacements...).Replace(result)
	}
	return result, nil
}

// settingLines renders one "  key: value" line per set field. Scalar fields
// keep struct order; list fields follow them, preserving the output order
// that places topics last.
func settingLines(fields []model.SettingField) ([]string, error) {
	var lines, lists []string
	for _, field := range fields {
		if !field.Set {
			continue
		}
		v := field.Value.Elem()
		switch v.Kind() {
		case reflect.String:
			value, err := yamlVal(v.String())
			if err != nil {
				return nil, err
			}
			lines = append(lines, fmt.Sprintf("  %s: %s", field.YAMLKey, value))
		case reflect.Bool:
			lines = append(lines, fmt.Sprintf("  %s: %t", field.YAMLKey, v.Bool()))
		case reflect.Slice:
			list, err := listLines(field.YAMLKey, v)
			if err != nil {
				return nil, err
			}
			lists = append(lists, list)
		}
	}
	return append(lines, lists...), nil
}

// listLines renders a string-list field as a block sequence, or [] when empty.
func listLines(key string, v reflect.Value) (string, error) {
	values := make([]string, v.Len())
	for i := range values {
		values[i] = v.Index(i).String()
	}
	w := &rulesetWriter{}
	w.list(2, key, values)
	return strings.Join(w.lines, "\n"), w.err
}

// templateFuncs provides helpers for the config template.
var templateFuncs = template.FuncMap{
	"yamlVal": yamlVal,
	"repositoryLines": func(r *model.RepositorySettings) ([]string, error) {
		return settingLines(model.RepositorySettingFields(r))
	},
	"actionsLines": func(a *model.ActionsSettings) ([]string, error) {
		return settingLines(model.ActionsSettingFields(a))
	},
	"codeScanningLines": func(c *model.CodeScanningSettings) ([]string, error) {
		lines, err := settingLines(model.CodeScanningSettingFields(c))
		return withLanguagesComment(lines, model.CodeScanningLanguages), err
	},
	"codeQualityLines": func(c *model.CodeQualitySettings) ([]string, error) {
		lines, err := settingLines(model.CodeQualitySettingFields(c))
		return withLanguagesComment(lines, model.CodeQualityLanguages), err
	},
	"rulesetLines": rulesetLines,
}

// rulesetWriter collects the indented lines of the ruleset section. The
// guidance comments come from the model slices so the documented values
// cannot drift from the values validation accepts.
type rulesetWriter struct {
	lines []string
	err   error
}

func (w *rulesetWriter) line(indent int, format string, args ...any) {
	w.lines = append(w.lines, strings.Repeat(" ", indent)+fmt.Sprintf(format, args...))
}

// scalar renders one string scalar. prefix is "- " for the first line of a
// sequence entry, "  " for a later line of that entry, and "" otherwise.
func (w *rulesetWriter) scalar(indent int, prefix, key, value string) {
	encoded, err := yamlVal(value)
	if err != nil && w.err == nil {
		w.err = err
	}
	w.line(indent, "%s%s: %s", prefix, key, encoded)
}

func (w *rulesetWriter) boolean(indent int, key string, value *bool) {
	if value != nil {
		w.line(indent, "%s: %t", key, *value)
	}
}

// list renders a string list as a block sequence, or [] when empty.
func (w *rulesetWriter) list(indent int, key string, values []string) {
	if len(values) == 0 {
		w.line(indent, "%s: []", key)
		return
	}
	w.line(indent, "%s:", key)
	for _, value := range values {
		encoded, err := yamlVal(value)
		if err != nil && w.err == nil {
			w.err = err
		}
		w.line(indent+2, "- %s", encoded)
	}
}

// rulesetLines renders the ruleset section, comments included, for the
// config template.
func rulesetLines(r *model.RulesetSettings) ([]string, error) {
	w := &rulesetWriter{}
	w.line(2, "# Tailor manages one ruleset named %q and owns it entirely.", model.RulesetName)
	w.line(2, "# %s enforces the rules. %s keeps the ruleset on GitHub but", model.RulesetEnforcements[0], model.RulesetEnforcements[1])
	w.line(2, "# GitHub ignores it, so a hand-made ruleset can govern instead.")
	if r.Enforcement != nil {
		w.scalar(2, "", "enforcement", *r.Enforcement)
	}
	if r.BypassActors != nil {
		w.bypassActors(*r.BypassActors)
	}
	if r.Conditions != nil && r.Conditions.RefName != nil {
		w.refName(r.Conditions.RefName)
	}
	if r.Rules != nil {
		w.rules(r.Rules)
	}
	return w.lines, w.err
}

func (w *rulesetWriter) bypassActors(actors []model.RulesetBypassActor) {
	roles := make([]string, 0, len(model.RulesetRepositoryRoles))
	for _, role := range model.RulesetRepositoryRoles {
		roles = append(roles, fmt.Sprintf("%d %s", role.ID, role.Name))
	}
	comments := []string{
		"# actor_type: " + strings.Join(model.RulesetActorTypes, ", "),
		"# RepositoryRole actor_id: " + strings.Join(roles, ", "),
		"# bypass_mode: " + strings.Join(model.RulesetBypassModes, ", "),
	}
	if len(actors) == 0 {
		for _, comment := range comments {
			w.line(2, "%s", comment)
		}
		w.line(2, "bypass_actors: []")
		return
	}
	w.line(2, "bypass_actors:")
	for _, comment := range comments {
		w.line(4, "%s", comment)
	}
	for _, actor := range actors {
		prefix := "- "
		if actor.ActorID != nil {
			w.line(4, "%sactor_id: %d", prefix, *actor.ActorID)
			prefix = "  "
		}
		if actor.ActorType != nil {
			w.scalar(4, prefix, "actor_type", *actor.ActorType)
			prefix = "  "
		}
		if actor.BypassMode != nil {
			w.scalar(4, prefix, "bypass_mode", *actor.BypassMode)
		}
	}
}

func (w *rulesetWriter) refName(refName *model.RulesetRefName) {
	w.line(2, "conditions:")
	w.line(4, "ref_name:")
	w.line(6, "# Branch names or fnmatch patterns in refs/heads/<name> form.")
	w.line(6, "# include also accepts ~DEFAULT_BRANCH and ~ALL.")
	if refName.Include != nil {
		w.list(6, "include", *refName.Include)
	}
	if refName.Exclude != nil {
		w.list(6, "exclude", *refName.Exclude)
	}
}

func (w *rulesetWriter) rules(rules *model.RulesetRules) {
	w.line(2, "rules:")
	w.boolean(4, "creation", rules.Creation)
	w.boolean(4, "update", rules.Update)
	w.boolean(4, "deletion", rules.Deletion)
	w.boolean(4, "required_linear_history", rules.RequiredLinearHistory)
	w.boolean(4, "required_signatures", rules.RequiredSignatures)
	w.boolean(4, "non_fast_forward", rules.NonFastForward)
	if rules.PullRequest != nil {
		w.pullRequest(rules.PullRequest)
	}
	if rules.RequiredStatusChecks != nil {
		w.statusChecks(rules.RequiredStatusChecks)
	}
	if rules.CodeScanning != nil {
		w.codeScanning(rules.CodeScanning)
	}
}

func (w *rulesetWriter) pullRequest(rule *model.RulesetPullRequest) {
	w.line(4, "pull_request:")
	w.boolean(6, "enabled", rule.Enabled)
	p := rule.Parameters
	if p == nil {
		return
	}
	w.line(6, "parameters:")
	if p.RequiredApprovingReviewCount != nil {
		w.line(8, "required_approving_review_count: %d", *p.RequiredApprovingReviewCount)
	}
	w.boolean(8, "dismiss_stale_reviews_on_push", p.DismissStaleReviewsOnPush)
	w.boolean(8, "require_code_owner_review", p.RequireCodeOwnerReview)
	w.boolean(8, "require_last_push_approval", p.RequireLastPushApproval)
	w.boolean(8, "required_review_thread_resolution", p.RequiredReviewThreadResolution)
	w.boolean(8, "require_extra_approval_for_unattributed_changes", p.RequireExtraApprovalForUnattributedChanges)
	if p.AllowedMergeMethods != nil {
		w.line(8, "# Any combination of %s. At least one.", strings.Join(model.RulesetMergeMethods, ", "))
		w.list(8, "allowed_merge_methods", *p.AllowedMergeMethods)
	}
}

func (w *rulesetWriter) statusChecks(rule *model.RulesetStatusChecks) {
	w.line(4, "required_status_checks:")
	w.boolean(6, "enabled", rule.Enabled)
	p := rule.Parameters
	if p == nil {
		return
	}
	w.line(6, "parameters:")
	if p.StrictRequiredStatusChecksPolicy != nil {
		w.line(8, "# Require branches to be up to date before merging.")
		w.boolean(8, "strict_required_status_checks_policy", p.StrictRequiredStatusChecksPolicy)
	}
	if p.DoNotEnforceOnCreate != nil {
		w.line(8, "# Do not require status checks on creation.")
		w.boolean(8, "do_not_enforce_on_create", p.DoNotEnforceOnCreate)
	}
	if p.RequiredStatusChecks == nil {
		return
	}
	w.line(8, "# context is the check name as shown on a pull request. For a GitHub")
	w.line(8, "# Actions job that is the job's name. integration_id is optional and")
	w.line(8, "# restricts the check to one app; 15368 is GitHub Actions.")
	if len(*p.RequiredStatusChecks) == 0 {
		w.line(8, "required_status_checks: []")
		return
	}
	w.line(8, "required_status_checks:")
	for _, check := range *p.RequiredStatusChecks {
		w.scalar(10, "- ", "context", check.Context)
		if check.IntegrationID != nil {
			w.line(10, "  integration_id: %d", *check.IntegrationID)
		}
	}
}

func (w *rulesetWriter) codeScanning(rule *model.RulesetCodeScanning) {
	w.line(4, "code_scanning:")
	w.boolean(6, "enabled", rule.Enabled)
	p := rule.Parameters
	if p == nil {
		return
	}
	w.line(6, "parameters:")
	if p.CodeScanningTools == nil {
		return
	}
	w.line(8, "# tool is the tool name as GitHub shows it, for example CodeQL.")
	w.line(8, "# alerts_threshold: %s", strings.Join(model.RulesetAlertsThresholds, ", "))
	w.line(8, "# security_alerts_threshold: %s", strings.Join(model.RulesetSecurityAlertsThresholds, ", "))
	if len(*p.CodeScanningTools) == 0 {
		w.line(8, "code_scanning_tools: []")
		return
	}
	w.line(8, "code_scanning_tools:")
	for _, tool := range *p.CodeScanningTools {
		w.scalar(10, "- ", "tool", tool.Tool)
		w.scalar(10, "  ", "alerts_threshold", tool.AlertsThreshold)
		w.scalar(10, "  ", "security_alerts_threshold", tool.SecurityAlertsThreshold)
	}
}

// withLanguagesComment inserts the languages guidance comment before the
// languages line, so the valid values in the config match the model.
func withLanguagesComment(lines []string, valid []string) []string {
	index := slices.IndexFunc(lines, func(line string) bool {
		return strings.HasPrefix(line, "  languages:")
	})
	if index == -1 {
		return lines
	}
	comment := []string{
		"  # An empty list means GitHub detects the languages. Valid values:",
		"  # " + strings.Join(valid, ", "),
	}
	return slices.Insert(lines, index, comment...)
}

// configTemplate renders .tailor.yml in the exact format specified. It uses
// text/template rather than yaml.Marshal to control key order, blank lines
// between swatch entries, and omission of nil pointer fields.
var configTemplate = template.Must(template.New("config").Funcs(templateFuncs).Parse(
	`# {{ .Verb }} by tailor on {{ .Date }}
license: {{ yamlVal .License }}

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
{{- if .CodeScanning }}

code_scanning:
{{- range codeScanningLines .CodeScanning }}
{{ . }}
{{- end }}
{{- end }}
{{- if .CodeQuality }}

code_quality:
{{- range codeQualityLines .CodeQuality }}
{{ . }}
{{- end }}
{{- end }}
{{- if .Ruleset }}

ruleset:
{{- range rulesetLines .Ruleset }}
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
func Write(dir string, cfg *Config, date string, verb string) (retErr error) {
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

	mode := os.FileMode(0o644)
	info, err := root.Lstat(ConfigSwatchPath)
	switch {
	case err == nil:
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("writing config: %s is a symbolic link", ConfigSwatchPath)
		}
		if info.Mode().IsRegular() {
			mode = info.Mode().Perm()
		}
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("inspecting config: %w", err)
	}

	tempPath := ConfigSwatchPath + ".tmp-" + rand.Text()
	temp, err := root.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("creating temporary config: %w", err)
	}
	tempOpen := true
	defer func() {
		if tempOpen {
			if err := temp.Close(); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("closing temporary config: %w", err))
			}
		}
		if tempPath != "" {
			if err := root.Remove(tempPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
				retErr = errors.Join(retErr, fmt.Errorf("removing temporary config: %w", err))
			}
		}
	}()

	if _, err := temp.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("writing temporary config: %w", err)
	}
	if err := temp.Chmod(mode); err != nil {
		return fmt.Errorf("setting temporary config permissions: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("syncing temporary config: %w", err)
	}
	if err := temp.Close(); err != nil {
		tempOpen = false
		return fmt.Errorf("closing temporary config: %w", err)
	}
	tempOpen = false

	if err := root.Rename(tempPath, ConfigSwatchPath); err != nil {
		return fmt.Errorf("replacing config: %w", err)
	}
	tempPath = ""

	rootDir, err := root.Open(".")
	if err != nil {
		return fmt.Errorf("opening config directory: %w", err)
	}
	if err := rootDir.Sync(); err != nil {
		_ = rootDir.Close()
		return fmt.Errorf("syncing config directory: %w", err)
	}
	if err := rootDir.Close(); err != nil {
		return fmt.Errorf("closing config directory: %w", err)
	}

	return nil
}
