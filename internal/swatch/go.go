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
		var dockers strings.Builder
		var packageIDs []string
		usedImages := map[string]bool{}
		builds.WriteString("builds:\n")
		dockers.WriteString("dockers_v2:\n")
		for _, build := range options.Builds {
			fmt.Fprintf(&builds, "  - id: %s\n    main: %s\n    binary: %s\n    env: [CGO_ENABLED=0]\n    goos: [linux, darwin]\n    goarch: [amd64, arm64]\n    ldflags: [-s -w]\n", strconv.Quote(build.ID), strconv.Quote(build.Main), strconv.Quote(build.Binary))
			packageIDs = append(packageIDs, strconv.Quote(build.ID))
			image := `ghcr.io/{{ tolower .Env.GITHUB_REPOSITORY_OWNER }}/{{ $name := replace (replace (tolower (index (split .Env.GITHUB_REPOSITORY "/") 1)) "_" "-") "." "-" }}{{ if not (filter $name "^[a-z0-9]") }}image{{ end }}{{ $name }}{{ if not (filter $name "[a-z0-9]$") }}image{{ end }}`
			if len(options.Builds) > 1 {
				parts := strings.FieldsFunc(strings.ToLower(build.Binary), func(r rune) bool {
					return (r < 'a' || r > 'z') && (r < '0' || r > '9')
				})
				name := strings.Join(parts, "-")
				if name == "" {
					return nil, fmt.Errorf("cannot derive a container image name for Go binary %q", build.Binary)
				}
				unique := name
				for index := 2; usedImages[unique]; index++ {
					unique = fmt.Sprintf("%s-%d", name, index)
				}
				usedImages[unique] = true
				image += "-" + unique
			}
			fmt.Fprintf(&dockers, "  - id: %s\n    ids: [%s]\n    dockerfile: Dockerfile\n    images: [%s]\n    tags:\n      - '{{ .Version }}'\n      - '{{ if and (not .Prerelease) (not .IsSnapshot) }}latest{{ end }}'\n    platforms: [linux/amd64, linux/arm64]\n    labels:\n      org.opencontainers.image.source: 'https://github.com/{{ .Env.GITHUB_REPOSITORY }}'\n    build_args:\n      BINARY: %s\n", strconv.Quote(build.ID), strconv.Quote(build.ID), strconv.Quote(image), strconv.Quote(build.Binary))
		}
		data = bytes.ReplaceAll(data, []byte("[[TAILOR_GO_BUILDS]]"), []byte(strings.TrimSuffix(builds.String(), "\n")))
		data = bytes.ReplaceAll(data, []byte("[[TAILOR_GO_PACKAGE_IDS]]"), []byte("["+strings.Join(packageIDs, ", ")+"]"))
		data = bytes.ReplaceAll(data, []byte("[[TAILOR_GO_DOCKERS]]"), []byte(strings.TrimSuffix(dockers.String(), "\n")))
	}
	return data, nil
}
