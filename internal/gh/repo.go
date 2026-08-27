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

	// Accept the default host and github.com even when no host is
	// authenticated, so repository detection works without credentials.
	defaultHost, _ := auth.DefaultHost()
	acceptedHosts := append(auth.KnownHosts(), "github.com", defaultHost)
	translator := ssh.NewTranslator()
	remotes := map[string]repository.Repository{}
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
		if parseErr != nil || !isKnownHost(repo.Host, acceptedHosts) {
			continue
		}
		remotes[fields[0]] = repo
		priority := remotePriority(fields[0])
		if priority > bestPriority {
			best = repo
			bestPriority = priority
		}
	}
	if bestPriority < 0 {
		return repository.Repository{}, errors.New("unable to determine current repository")
	}
	if resolved, ok := resolvedRepository(dir, remotes, acceptedHosts); ok {
		return resolved, nil
	}
	return best, nil
}

// resolvedRepository honours the remote resolution that `gh repo set-default`
// stores in git config as remote.<name>.gh-resolved. A value of "base" selects
// the remote's own repository; any other value names a repository directly.
func resolvedRepository(dir string, remotes map[string]repository.Repository, acceptedHosts []string) (repository.Repository, bool) {
	output, err := exec.CommandContext(context.Background(), "git", "-C", dir,
		"config", "--get-regexp", `^remote\..+\.gh-resolved$`).Output()
	if err != nil {
		return repository.Repository{}, false
	}
	bestPriority := -1
	var best repository.Repository
	for line := range strings.Lines(string(output)) {
		key, value, found := strings.Cut(strings.TrimSpace(line), " ")
		if !found {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(key, "remote."), ".gh-resolved")
		var repo repository.Repository
		if value == "base" {
			remote, known := remotes[name]
			if !known {
				continue
			}
			repo = remote
		} else {
			parsed, parseErr := repository.Parse(value)
			if parseErr != nil || !isKnownHost(parsed.Host, acceptedHosts) {
				continue
			}
			repo = parsed
		}
		if priority := remotePriority(name); priority > bestPriority {
			best = repo
			bestPriority = priority
		}
	}
	return best, bestPriority >= 0
}

func isKnownHost(host string, knownHosts []string) bool {
	for _, knownHost := range knownHosts {
		if strings.EqualFold(host, knownHost) {
			return true
		}
	}
	return false
}

// remotePriority ranks origin above upstream: tailor manages the repository
// the user administers and pushes to, so in a fork clone the fork wins over
// the parent repository. This inverts the gh CLI ordering deliberately.
func remotePriority(name string) int {
	switch strings.ToLower(name) {
	case "origin":
		return 3
	case "github":
		return 2
	case "upstream":
		return 1
	default:
		return 0
	}
}
