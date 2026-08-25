package gh

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/cli/go-gh/v2/pkg/auth"
	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/cli/go-gh/v2/pkg/ssh"
)

var currentRepoAt = repositoryFromRemotes

// Repo identifies a GitHub repository and the host that serves it.
type Repo struct {
	Host  string
	Owner string
	Name  string
}

// RepoContext detects the GitHub repository for the current directory.
// It returns the host, owner, and name if a GitHub remote is found.
// When no remote is configured, it returns ok=false.
func RepoContext() (repo Repo, ok bool) {
	repo, ok, _ = RepoContextAt(".")
	return repo, ok
}

// RepoContextAt detects the GitHub repository for the given directory.
// It returns the host, owner, and name if a GitHub remote is found;
// ok=false otherwise.
func RepoContextAt(dir string) (repo Repo, ok bool, err error) {
	info, err := os.Stat(dir)
	if err != nil {
		return Repo{}, false, fmt.Errorf("accessing directory %q: %w", dir, err)
	}
	if !info.IsDir() {
		return Repo{}, false, fmt.Errorf("accessing directory %q: not a directory", dir)
	}

	found, repoErr := currentRepoAt(dir)
	if repoErr != nil {
		return Repo{}, false, nil
	}
	return Repo{Host: found.Host, Owner: found.Owner, Name: found.Name}, true, nil
}

func repositoryFromRemotes(dir string) (repository.Repository, error) {
	output, err := exec.CommandContext(context.Background(), "git", "-C", dir, "remote", "-v").Output()
	if err != nil {
		return repository.Repository{}, err
	}

	knownHosts := auth.KnownHosts()
	translator := ssh.NewTranslator()
	bestPriority := -1
	var best repository.Repository
	for line := range strings.Lines(string(output)) {
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[2] != "(fetch)" {
			continue
		}
		rawURL := fields[1]
		if authority, path, found := strings.Cut(rawURL, ":"); found &&
			!strings.Contains(authority, "/") && path != "" && !strings.HasPrefix(path, "//") {
			rawURL = "ssh://" + authority + "/" + path
		}
		remoteURL, parseErr := url.Parse(rawURL)
		if parseErr != nil {
			continue
		}
		if remoteURL.Scheme == "git+ssh" {
			remoteURL.Scheme = "ssh"
		}
		repo, parseErr := repository.Parse(translator.Translate(remoteURL).String())
		if parseErr != nil || !isKnownHost(repo.Host, knownHosts) {
			continue
		}
		priority := remotePriority(fields[0])
		if priority > bestPriority {
			best = repo
			bestPriority = priority
		}
	}
	if bestPriority < 0 {
		return repository.Repository{}, errors.New("unable to determine current repository")
	}
	return best, nil
}

func isKnownHost(host string, knownHosts []string) bool {
	for _, knownHost := range knownHosts {
		if strings.EqualFold(host, knownHost) {
			return true
		}
	}
	return false
}

func remotePriority(name string) int {
	switch strings.ToLower(name) {
	case "upstream":
		return 3
	case "github":
		return 2
	case "origin":
		return 1
	default:
		return 0
	}
}
