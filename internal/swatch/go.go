package swatch

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/wimpysworld/tailor"
	"github.com/wimpysworld/tailor/internal/goproject"
)

const (
	GoLintDestination     = ".golangci.yml"
	GoReleaseDestination  = ".goreleaser.yaml"
	GoWorkflowDestination = ".github/workflows/build-go.yml"
)

type Options struct {
	GoDeclared    bool
	GoEnabled     bool
	DefaultBranch string
	Builds        []goproject.Build
}

func Render(path string, options Options) ([]byte, error) {
	if path == "justfile" && options.GoEnabled {
		return tailor.SwatchFS.ReadFile("swatches/go/justfile")
	}
	if path == ".github/dependabot.yml" && options.GoDeclared && !options.GoEnabled {
		return tailor.SwatchFS.ReadFile("swatches/go/dependabot-disabled.yml")
	}
	data, err := Content(path)
	if err != nil {
		return nil, err
	}
	switch path {
	case GoWorkflowDestination:
		branch, guard := "**", ""
		if options.DefaultBranch == "" {
			guard = "    if: github.event_name != 'push' || github.ref == format('refs/heads/{0}', github.event.repository.default_branch) || startsWith(github.ref, 'refs/tags/')"
		} else {
			branch, err = workflowBranchFilter(options.DefaultBranch)
			if err != nil {
				return nil, err
			}
		}
		data = bytes.ReplaceAll(data, []byte("[[TAILOR_DEFAULT_BRANCH]]"), []byte(strconv.Quote(branch)))
		data = bytes.ReplaceAll(data, []byte("[[TAILOR_GO_JOB_GUARD]]"), []byte(guard))
	case GoReleaseDestination:
		if len(options.Builds) == 0 {
			return nil, fmt.Errorf("rendering Go release configuration requires at least one discovered main package")
		}
		var builds strings.Builder
		builds.WriteString("builds:\n")
		for _, build := range options.Builds {
			fmt.Fprintf(&builds, "  - id: %s\n    main: %s\n    binary: %s\n    env: [CGO_ENABLED=0]\n    goos: [linux, darwin]\n    goarch: [amd64, arm64]\n    ldflags: [-s -w]\n", strconv.Quote(build.ID), strconv.Quote(build.Main), strconv.Quote(build.Binary))
		}
		data = bytes.ReplaceAll(data, []byte("[[TAILOR_GO_BUILDS]]"), []byte(strings.TrimSuffix(builds.String(), "\n")))
	}
	return data, nil
}
