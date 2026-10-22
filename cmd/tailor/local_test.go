package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestLocalOnlySummaryCounts(t *testing.T) {
	original := applyLocal
	t.Cleanup(func() { applyLocal = original })
	t.Cleanup(gh.SetTokenForHostFunc(func(string) (string, string) {
		t.Fatal("local-only resolved authentication")
		return "", ""
	}))
	t.Cleanup(gh.SetCurrentRepoFunc(func(string) (repository.Repository, error) {
		t.Fatal("local-only resolved repository context")
		return repository.Repository{}, nil
	}))
	t.Cleanup(gh.SetNewRESTClientFunc(func(string) (*api.RESTClient, error) {
		t.Fatal("local-only created a GitHub client")
		return nil, nil
	}))
	t.Cleanup(gh.SetInspectWikiFunc(func(string, string, string) error {
		t.Fatal("local-only inspected a remote wiki")
		return nil
	}))
	dir := t.TempDir()
	testutil.WriteFile(t, dir, ".tailor.yml", "license: none\n")
	failure := errors.New("local write failed")
	for _, test := range []struct {
		name    string
		updated []string
		err     error
		quiet   string
		plain   string
	}{
		{"mixed", []string{"just/tailor.just"}, nil, "1 applied · 1 already match\n", "updated just/tailor.just\n"},
		{"unchanged", nil, nil, "2 already match\n", "no local changes\n"},
		{"partial failure", []string{"just/tailor.just"}, failure, "1 applied\n", "updated just/tailor.just\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			applyLocal = func(_ *config.Config, _ string, paths []string) ([]string, error) {
				if len(paths) != 2 || paths[0] != "just/tailor.just" || paths[1] != "nix/hooks/go.nix" {
					t.Fatalf("unexpected paths: %v", paths)
				}
				return test.updated, test.err
			}
			for _, format := range []string{"plain", "auto"} {
				for _, quiet := range []bool{false, true} {
					args := []string{"alter", "--local-only", "--directory", dir, "--path", "just/tailor.just", "--path", "nix/hooks/go.nix", "--format=" + format}
					want := test.plain
					if quiet {
						args = append(args, "--quiet")
						want = test.quiet
					}
					var stdout, stderr bytes.Buffer
					code := run(args, &stdout, &stderr)
					if stdout.String() != want {
						t.Fatalf("format=%s quiet=%t: stdout = %q, want %q", format, quiet, &stdout, want)
					}
					if test.err == nil {
						if code != 0 || stderr.Len() != 0 {
							t.Fatalf("success: exit %d, stderr = %q", code, &stderr)
						}
					} else if code == 0 || !strings.Contains(stderr.String(), failure.Error()) {
						t.Fatalf("failure: exit %d, stderr = %q", code, &stderr)
					}
				}
			}
		})
	}
}

func TestLocalOnlyCLIHasNoRemoteAccess(t *testing.T) {
	t.Cleanup(gh.SetTokenForHostFunc(func(string) (string, string) {
		t.Fatal("local-only resolved authentication")
		return "", ""
	}))
	t.Cleanup(gh.SetCurrentRepoFunc(func(string) (repository.Repository, error) {
		t.Fatal("local-only resolved repository context")
		return repository.Repository{}, nil
	}))
	t.Cleanup(gh.SetNewRESTClientFunc(func(string) (*api.RESTClient, error) {
		t.Fatal("local-only created a GitHub client")
		return nil, nil
	}))
	t.Cleanup(gh.SetInspectWikiFunc(func(string, string, string) error {
		t.Fatal("local-only inspected a remote wiki")
		return nil
	}))
	dir := t.TempDir()
	testutil.WriteFile(t, dir, ".tailor.yml", "license: none\nrepository:\n  has_wiki: true\nlanguages:\n  go: true\nswatches:\n  - path: .github/dependabot.yml\n    alteration: first-fit\n")
	paths := []string{".github/dependabot.yml", "nix/hooks/go.nix", "just/tailor.just"}
	args := []string{"alter", "--local-only", "--directory", dir}
	for _, name := range paths {
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(name)), 0o755); err != nil {
			t.Fatal(err)
		}
		testutil.WriteFile(t, dir, name, "# Managed by Tailor: "+name+"\nold\n")
		args = append(args, "--path", name)
	}
	var output bytes.Buffer
	if code := run(args, &output, &output); code != 0 {
		t.Fatalf("local-only exited %d: %s", code, &output)
	}
	if bytes.Count(output.Bytes(), []byte("updated ")) != 3 {
		t.Fatalf("unexpected output: %s", &output)
	}
	for _, format := range []string{"plain", "auto"} {
		for _, name := range paths {
			testutil.WriteFile(t, dir, name, "# Managed by Tailor: "+name+"\nold\n")
		}
		quietArgs := append(append([]string{}, args...), "--quiet", "--format="+format)
		for _, want := range []string{"3 applied\n", "3 already match\n"} {
			var stdout, stderr bytes.Buffer
			if code := run(quietArgs, &stdout, &stderr); code != 0 {
				t.Fatalf("quiet local-only exited %d: %s", code, &stderr)
			}
			if stdout.String() != want || stderr.Len() != 0 {
				t.Fatalf("quiet %s output = %q, stderr = %q; want %q", format, &stdout, &stderr, want)
			}
		}
		badArgs := append(append([]string{}, quietArgs...), "--path", "unsupported")
		var stdout, stderr bytes.Buffer
		if code := run(badArgs, &stdout, &stderr); code == 0 || !strings.Contains(stderr.String(), "not supported") || stdout.Len() != 0 {
			t.Fatalf("quiet error: exit %d, stdout = %q, stderr = %q", code, &stdout, &stderr)
		}
	}
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{"path requires local-only", []string{"alter", "--path", "just/tailor.just"}, "--path and --directory require --local-only"},
		{"directory requires local-only", []string{"alter", "--directory", dir}, "--path and --directory require --local-only"},
		{"local-only requires path", []string{"alter", "--local-only"}, "local-only alteration requires at least one --path"},
		{"local-only rejects recut", []string{"alter", "--local-only", "--recut", "--path", "just/tailor.just"}, "local-only alteration does not support --recut"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(test.args, &stdout, &stderr)
			if code == 0 || !strings.Contains(stderr.String(), test.want) || stdout.Len() != 0 {
				t.Fatalf("invalid flags %v: exit %d, stdout = %q, stderr = %q; want %q", test.args, code, &stdout, &stderr, test.want)
			}
		})
	}
}
