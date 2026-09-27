package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/testutil"
)

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
	dir := t.TempDir()
	testutil.WriteFile(t, dir, ".tailor.yml", "license: none\nlanguages:\n  go: true\nswatches:\n  - path: .github/dependabot.yml\n    alteration: first-fit\n")
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
	for _, args := range [][]string{
		{"alter", "--path", "just/tailor.just"},
		{"alter", "--directory", dir},
		{"alter", "--local-only"},
		{"alter", "--local-only", "--recut", "--path", "just/tailor.just"},
	} {
		if code := run(args, io.Discard, io.Discard); code == 0 {
			t.Fatalf("accepted invalid flags: %v", args)
		}
	}
}
