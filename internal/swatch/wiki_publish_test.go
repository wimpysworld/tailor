package swatch_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/swatch"
	"gopkg.in/yaml.v3"
)

func TestWikiWorkflowContract(t *testing.T) {
	content, err := swatch.WikiContent("release/docs+v2")
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		On struct {
			Push struct {
				Branches []string `yaml:"branches"`
				Paths    []string `yaml:"paths"`
			} `yaml:"push"`
		} `yaml:"on"`
		Permissions map[string]string `yaml:"permissions"`
		Jobs        map[string]struct {
			If          string            `yaml:"if"`
			Permissions map[string]string `yaml:"permissions"`
			Steps       []struct {
				Uses string         `yaml:"uses"`
				With map[string]any `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(content, &workflow); err != nil {
		t.Fatal(err)
	}
	if branches := workflow.On.Push.Branches; len(branches) != 1 || branches[0] != `release/docs\+v2` {
		t.Fatalf("incorrect literal branch filter: %q", branches)
	}
	if paths := workflow.On.Push.Paths; len(paths) != 2 || paths[0] != "wiki/**" || paths[1] != swatch.WikiDestination {
		t.Fatalf("incorrect publication paths: %q", paths)
	}
	job := workflow.Jobs["publish"]
	if len(workflow.Permissions) != 0 || len(workflow.Jobs) != 1 || len(job.Permissions) != 1 || job.Permissions["contents"] != "write" {
		t.Fatal("publisher permissions exceed contents: write")
	}
	if !strings.Contains(job.If, "github.event.repository.has_wiki") || !strings.Contains(job.If, "github.event.repository.visibility == 'public'") || !strings.Contains(job.If, "github.ref == format('refs/heads/{0}', github.event.repository.default_branch)") {
		t.Fatal("publisher lacks its wiki, public visibility or default branch guard")
	}
	for _, step := range job.Steps {
		if step.Uses != "" {
			if !regexp.MustCompile(`^actions/checkout@[0-9a-f]{40}$`).MatchString(step.Uses) {
				t.Fatalf("unexpected or unpinned action: %s", step.Uses)
			}
			if step.With["persist-credentials"] != false || step.With["fetch-depth"] != 0 {
				t.Fatal("checkout must omit credentials and fetch full history")
			}
		}
	}
	if actionlint, err := exec.LookPath("actionlint"); err == nil {
		file := filepath.Join(t.TempDir(), "wiki.yml")
		if err := os.WriteFile(file, content, 0o600); err != nil {
			t.Fatal(err)
		}
		if output, err := exec.CommandContext(t.Context(), actionlint, "-shellcheck=", file).CombinedOutput(); err != nil {
			t.Fatalf("actionlint: %v\n%s", err, output)
		}
	}
}

type wikiPublisherFixture struct {
	t            *testing.T
	root, source string
	remote       string
	baseline     string
	env          []string
	script       string
}

func newWikiPublisherFixture(t *testing.T) *wikiPublisherFixture {
	t.Helper()
	for _, program := range []string{"git", "python3"} {
		if _, err := exec.LookPath(program); err != nil {
			t.Skip(program + " is required for offline publisher tests")
		}
	}
	root := t.TempDir()
	f := &wikiPublisherFixture{
		t: t, root: root, source: filepath.Join(root, "source"), remote: filepath.Join(root, "project.wiki.git"),
		env: append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
			"GIT_AUTHOR_NAME=Wiki Test", "GIT_AUTHOR_EMAIL=wiki@example.invalid",
			"GIT_COMMITTER_NAME=Wiki Test", "GIT_COMMITTER_EMAIL=wiki@example.invalid"),
	}
	content, err := os.ReadFile(filepath.Join("..", "..", "swatches", ".github", "workflows", "tailor-wiki.yml"))
	if err != nil {
		t.Fatal(err)
	}
	content = []byte(strings.ReplaceAll(string(content), "[[quote .Branch]]", `"main"`))
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Run string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(content, &workflow); err != nil {
		t.Fatal(err)
	}
	for _, step := range workflow.Jobs["publish"].Steps {
		if step.Run != "" {
			f.script = step.Run
		}
	}
	if f.script == "" {
		t.Fatal("publisher script is missing")
	}
	seed := filepath.Join(root, "seed")
	f.git(root, "init", "-b", "handbook", seed)
	f.write(filepath.Join(seed, "Home.md"), "Existing wiki\n")
	f.write(filepath.Join(seed, "Old.md"), "Old page\n")
	f.git(seed, "add", ".")
	f.git(seed, "commit", "-m", "Initial wiki")
	f.baseline = f.git(seed, "rev-parse", "HEAD")
	f.git(root, "clone", "--bare", seed, f.remote)
	f.git(root, "init", "-b", "main", f.source)
	f.write(filepath.Join(f.source, "wiki", "Home.md"), "Imported and revised wiki\n")
	f.write(filepath.Join(f.source, "wiki", "_Sidebar.md"), "[Home](Home)\n")
	f.write(filepath.Join(f.source, "wiki", ".tailor-wiki-base"), f.baseline+"\n")
	f.git(f.source, "add", ".")
	f.git(f.source, "commit", "-m", "Import wiki")
	f.git(root, "clone", "--bare", f.source, filepath.Join(root, "project.git"))
	f.git(f.source, "remote", "add", "origin", filepath.Join(root, "project.git"))
	return f
}

func (f *wikiPublisherFixture) git(directory string, args ...string) string {
	f.t.Helper()
	command := exec.CommandContext(f.t.Context(), "git", append([]string{"-C", directory}, args...)...) // #nosec G204 -- Isolated local Git fixtures.
	command.Env = f.env
	output, err := command.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func (f *wikiPublisherFixture) write(path, content string) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *wikiPublisherFixture) commit() {
	f.t.Helper()
	f.git(f.source, "add", "-A")
	f.git(f.source, "commit", "-m", "Update source")
	f.git(f.source, "push", "origin", "main")
}

func (f *wikiPublisherFixture) publish(sha string, extraEnv ...string) (string, error) {
	f.t.Helper()
	if sha == "" {
		sha = f.git(f.source, "rev-parse", "HEAD")
	}
	python := strings.TrimSuffix(strings.TrimPrefix(f.script, "python3 - <<'PY'\n"), "PY\n")
	command := exec.CommandContext(f.t.Context(), "python3", "-c", python)
	command.Dir = f.source
	command.Env = append(append([]string{}, f.env...),
		"GITHUB_SHA="+sha, "GITHUB_SERVER_URL=file://"+f.root,
		"GITHUB_REPOSITORY=project", "WIKI_DEFAULT_BRANCH=main", "WIKI_TOKEN=not-a-real-token")
	command.Env = append(command.Env, extraEnv...)
	output, err := command.CombinedOutput()
	if strings.Contains(string(output), "not-a-real-token") {
		f.t.Fatal("publisher exposed its token")
	}
	return string(output), err
}

func (f *wikiPublisherFixture) requirePublish() {
	f.t.Helper()
	if output, err := f.publish(""); err != nil {
		f.t.Fatalf("publisher: %v\n%s", err, output)
	}
}

func TestWikiPublisherAdoptionAndUpdates(t *testing.T) {
	f := newWikiPublisherFixture(t)
	f.requirePublish()
	first := f.git(f.remote, "rev-parse", "HEAD")
	if parent := f.git(f.remote, "rev-parse", "HEAD^"); parent != f.baseline {
		t.Fatal("publisher discarded wiki history")
	}
	if branch := f.git(f.remote, "symbolic-ref", "HEAD"); branch != "refs/heads/handbook" {
		t.Fatalf("wiki branch changed: %s", branch)
	}
	if names := f.git(f.remote, "ls-tree", "--name-only", "HEAD"); names != "Home.md\n_Sidebar.md" {
		t.Fatalf("unexpected published files: %s", names)
	}
	if body := f.git(f.remote, "show", "HEAD:Home.md"); body != "Imported and revised wiki" {
		t.Fatalf("wrong published body: %s", body)
	}
	f.requirePublish()
	if next := f.git(f.remote, "rev-parse", "HEAD"); next != first {
		t.Fatal("unchanged publication created a commit")
	}
	f.write(filepath.Join(f.source, "wiki", "Home.md"), "Second revision\n")
	f.write(filepath.Join(f.source, "wiki", "guides", "Install.md"), "Install guide\n")
	if err := os.Remove(filepath.Join(f.source, "wiki", "_Sidebar.md")); err != nil {
		t.Fatal(err)
	}
	f.commit()
	f.requirePublish()
	if names := f.git(f.remote, "ls-tree", "-r", "--name-only", "HEAD"); names != "Home.md\nguides/Install.md" {
		t.Fatalf("updates and deletions did not mirror: %s", names)
	}
	if parent := f.git(f.remote, "rev-parse", "HEAD^"); parent != first {
		t.Fatal("update discarded the prior publisher commit")
	}
}

func TestWikiPublisherUnchangedAdoptionRecordsOwnership(t *testing.T) {
	f := newWikiPublisherFixture(t)
	f.write(filepath.Join(f.source, "wiki", "Home.md"), "Existing wiki\n")
	f.write(filepath.Join(f.source, "wiki", "Old.md"), "Old page\n")
	if err := os.Remove(filepath.Join(f.source, "wiki", "_Sidebar.md")); err != nil {
		t.Fatal(err)
	}
	f.commit()
	f.requirePublish()
	if tree := f.git(f.remote, "rev-parse", "HEAD^{tree}"); tree != f.git(f.remote, "rev-parse", f.baseline+"^{tree}") {
		t.Fatal("unchanged adoption changed content")
	}
	if message := f.git(f.remote, "log", "-1", "--format=%B"); !strings.Contains(message, "Tailor-Wiki-Source: ") {
		t.Fatal("unchanged adoption did not record ownership")
	}
}

func TestWikiPublisherRefusesUnsafeStates(t *testing.T) {
	for _, name := range []string{"missing baseline", "invalid baseline", "wrong baseline", "unavailable remote", "empty remote", "symlink", "committed symlink", "special file", "git directory", "stale source", "missing source branch", "unavailable source"} {
		t.Run(name, func(t *testing.T) {
			f := newWikiPublisherFixture(t)
			before := f.git(f.remote, "rev-parse", "HEAD")
			sha := ""
			switch name {
			case "missing baseline":
				if err := os.Remove(filepath.Join(f.source, "wiki", ".tailor-wiki-base")); err != nil {
					t.Fatal(err)
				}
				f.commit()
			case "invalid baseline":
				f.write(filepath.Join(f.source, "wiki", ".tailor-wiki-base"), "invalid\n")
				f.commit()
			case "wrong baseline":
				f.write(filepath.Join(f.source, "wiki", ".tailor-wiki-base"), strings.Repeat("0", 40)+"\n")
				f.commit()
			case "unavailable remote":
				if err := os.Rename(f.remote, f.remote+".saved"); err != nil {
					t.Fatal(err)
				}
			case "empty remote":
				f.git(f.remote, "update-ref", "-d", "refs/heads/handbook")
			case "symlink":
				if err := os.Symlink("../outside", filepath.Join(f.source, "wiki", "linked")); err != nil {
					t.Fatal(err)
				}
			case "committed symlink":
				linked := filepath.Join(f.source, "wiki", "linked")
				if err := os.Symlink("../outside", linked); err != nil {
					t.Fatal(err)
				}
				f.commit()
				if err := os.Remove(linked); err != nil {
					t.Fatal(err)
				}
				f.write(linked, "Regular worktree file must not hide an unsafe committed file\n")
			case "special file":
				mkfifo, err := exec.LookPath("mkfifo")
				if err != nil {
					t.Skip("mkfifo is required for the special-file test")
				}
				if output, err := exec.CommandContext(t.Context(), mkfifo, filepath.Join(f.source, "wiki", "pipe")).CombinedOutput(); err != nil {
					t.Fatalf("mkfifo: %v\n%s", err, output)
				}
			case "git directory":
				if err := os.Mkdir(filepath.Join(f.source, "wiki", ".git"), 0o755); err != nil {
					t.Fatal(err)
				}
			case "stale source":
				sha = f.git(f.source, "rev-parse", "HEAD")
				f.write(filepath.Join(f.source, "README.md"), "New source commit\n")
				f.commit()
				f.git(f.source, "checkout", "--detach", sha)
			case "missing source branch":
				f.git(filepath.Join(f.root, "project.git"), "update-ref", "-d", "refs/heads/main")
			case "unavailable source":
				f.git(f.source, "remote", "set-url", "origin", filepath.Join(f.root, "unavailable.git"))
			}
			if output, err := f.publish(sha); err == nil {
				t.Fatalf("unsafe publication succeeded: %s", output)
			}
			if name != "unavailable remote" && name != "empty remote" && f.git(f.remote, "rev-parse", "HEAD") != before {
				t.Fatal("rejected publication changed the wiki")
			}
		})
	}
}

func TestWikiPublisherReadoptsManualEdits(t *testing.T) {
	f := newWikiPublisherFixture(t)
	f.requirePublish()
	manual := filepath.Join(f.root, "manual")
	f.git(f.root, "clone", f.remote, manual)
	f.write(filepath.Join(manual, "Home.md"), "Manual change\n")
	f.write(filepath.Join(manual, "New.md"), "New remote page\n")
	f.git(manual, "add", ".")
	f.git(manual, "commit", "-m", "Manual edit without ownership trailer")
	f.git(manual, "push", "origin", "handbook")
	baseline := f.git(f.remote, "rev-parse", "HEAD")
	f.write(filepath.Join(f.source, "wiki", "Home.md"), "Manual change with reviewed revision\n")
	f.write(filepath.Join(f.source, "wiki", "New.md"), "New remote page\n")
	f.write(filepath.Join(f.source, "wiki", ".tailor-wiki-base"), baseline+"\n")
	f.commit()
	f.requirePublish()
	if parent := f.git(f.remote, "rev-parse", "HEAD^"); parent != baseline {
		t.Fatal("re-adoption discarded the manual edit")
	}
	if page := f.git(f.remote, "show", "HEAD:New.md"); page != "New remote page" {
		t.Fatal("re-adoption lost the imported page")
	}
	f.requirePublish()
}

func TestWikiPublisherRefusesRemoteEdits(t *testing.T) {
	for _, forgedTrailer := range []bool{false, true} {
		t.Run(map[bool]string{false: "manual commit", true: "copied trailer"}[forgedTrailer], func(t *testing.T) {
			f := newWikiPublisherFixture(t)
			f.requirePublish()
			previousSource := f.git(f.source, "rev-parse", "HEAD")
			manual := filepath.Join(f.root, "manual")
			f.git(f.root, "clone", f.remote, manual)
			f.write(filepath.Join(manual, "Home.md"), "Manual change\n")
			f.git(manual, "add", ".")
			message := "Edit wiki"
			if forgedTrailer {
				message += "\n\nTailor-Wiki-Source: " + previousSource
			}
			f.git(manual, "commit", "-m", message)
			f.git(manual, "push", "origin", "handbook")
			before := f.git(f.remote, "rev-parse", "HEAD")
			f.write(filepath.Join(f.source, "wiki", "Home.md"), "Next source\n")
			f.commit()
			if output, err := f.publish(""); err == nil {
				t.Fatalf("overwrote a remote edit: %s", output)
			}
			if after := f.git(f.remote, "rev-parse", "HEAD"); after != before {
				t.Fatal("remote edit was lost")
			}
		})
	}
}

func TestWikiPublisherRefusesConcurrentPush(t *testing.T) {
	f := newWikiPublisherFixture(t)
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	tree := f.git(f.remote, "rev-parse", "HEAD^{tree}")
	race := f.git(f.remote, "commit-tree", tree, "-p", f.baseline, "-m", "Concurrent edit")
	wrapper := filepath.Join(f.root, "bin", "git")
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
	f.write(wrapper, "#!/bin/sh\nfor argument do\n"+
		"  if [ \"$argument\" = push ]; then\n    "+quote(gitPath)+" -C "+quote(f.remote)+
		" update-ref refs/heads/handbook "+quote(race)+"\n  fi\ndone\nexec "+quote(gitPath)+" \"$@\"\n")
	if err := os.Chmod(wrapper, 0o755); err != nil {
		t.Fatal(err)
	}
	if output, err := f.publish("", "PATH="+filepath.Dir(wrapper)+string(os.PathListSeparator)+os.Getenv("PATH")); err == nil {
		t.Fatalf("concurrent push was accepted: %s", output)
	}
	if after := f.git(f.remote, "rev-parse", "HEAD"); after != race {
		t.Fatal("concurrent edit was lost")
	}
}

func TestWikiPublisherRefusesSourceHistoryRewind(t *testing.T) {
	f := newWikiPublisherFixture(t)
	oldSource := f.git(f.source, "rev-parse", "HEAD")
	f.requirePublish()
	f.write(filepath.Join(f.source, "wiki", "Home.md"), "Latest published source\n")
	f.commit()
	f.requirePublish()
	before := f.git(f.remote, "rev-parse", "HEAD")
	f.git(f.source, "checkout", "--detach", oldSource)
	f.git(filepath.Join(f.root, "project.git"), "update-ref", "refs/heads/main", oldSource)
	if output, err := f.publish(oldSource); err == nil {
		t.Fatalf("publisher accepted a source history rewind: %s", output)
	}
	if after := f.git(f.remote, "rev-parse", "HEAD"); after != before {
		t.Fatal("source history rewind changed the wiki")
	}
}
