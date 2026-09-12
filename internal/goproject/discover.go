// Package goproject discovers release executables from local Go syntax without running project code or resolving dependencies.
package goproject

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
)

// Build describes a GoReleaser executable with unique build and binary names.
type Build struct {
	ID     string
	Binary string
	Main   string // Main is the package directory relative to the project root.
}

const maxFileSize = 1 << 20

// Discover finds executables in the root module for Linux and Darwin on amd64 and arm64 without CGO.
// It rejects missing or incompatible entry points and bounds filesystem reads.
func Discover(directory string) ([]Build, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, fmt.Errorf("opening Go project: %w", err)
	}
	defer root.Close()
	project := root.FS()
	moduleName, err := readModuleName(project)
	if err != nil {
		return nil, err
	}
	packages, err := readSources(project)
	if err != nil {
		return nil, err
	}
	return discoverBuilds(moduleName, packages)
}

func readModuleName(project fs.FS) (string, error) {
	module, err := readRegular(project, "go.mod")
	if err != nil {
		return "", fmt.Errorf("go release discovery requires a readable root go.mod: %w", err)
	}
	moduleName := ""
	for line := range strings.SplitSeq(string(module), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "module" {
			moduleName = fields[1]
			if strings.HasPrefix(moduleName, `"`) {
				moduleName, err = strconv.Unquote(moduleName)
				if err != nil {
					return "", fmt.Errorf("invalid module name: %w", err)
				}
			}
			break
		}
	}
	if moduleName == "" {
		return "", fmt.Errorf("go release discovery requires a module declaration in go.mod")
	}
	// A semantic import version suffix is not part of the root binary name.
	if version := path.Base(moduleName); len(version) > 1 && version[0] == 'v' {
		if number, err := strconv.Atoi(version[1:]); err == nil && number >= 2 {
			moduleName = path.Dir(moduleName)
		}
	}
	return moduleName, nil
}

func readSources(project fs.FS) (map[string][]source, error) {
	packages := map[string][]source{}
	count, total := 0, 0
	err := fs.WalkDir(project, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		count++
		if count > 100000 {
			return fmt.Errorf("go release discovery exceeds 100000 filesystem entries")
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			if name == "." {
				return nil
			}
			base := entry.Name()
			if strings.HasPrefix(base, ".") || strings.HasPrefix(base, "_") || base == "vendor" || base == "testdata" {
				return fs.SkipDir
			}
			if _, err := fs.Lstat(project, path.Join(name, "go.mod")); err == nil {
				return fs.SkipDir
			} else if !os.IsNotExist(err) {
				return err
			}
			return nil
		}
		base := entry.Name()
		if !strings.HasSuffix(base, ".go") || strings.HasSuffix(base, "_test.go") || strings.HasPrefix(base, ".") || strings.HasPrefix(base, "_") {
			return nil
		}
		data, err := readRegular(project, name)
		if err != nil {
			return err
		}
		total += len(data)
		if total > 64<<20 {
			return fmt.Errorf("go release discovery exceeds 64 MiB of source")
		}
		item, err := parseSource(name, data)
		if err != nil {
			return err
		}
		dir := path.Dir(name)
		packages[dir] = append(packages[dir], item)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discovering Go main packages: %w", err)
	}
	return packages, nil
}

func parseSource(name string, data []byte) (source, error) {
	parsed, err := parser.ParseFile(token.NewFileSet(), name, data, 0)
	if err != nil {
		return source{}, fmt.Errorf("parsing Go source %q: %w", name, err)
	}
	item := source{name: name, data: data, pkg: parsed.Name.Name}
	for _, imported := range parsed.Imports {
		if imported.Path.Value == `"C"` {
			item.cgo = true
		}
	}
	for _, declaration := range parsed.Decls {
		if fn, ok := declaration.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == "main" {
			if fn.Type.Params.NumFields() != 0 || fn.Type.Results.NumFields() != 0 || fn.Type.TypeParams.NumFields() != 0 {
				item.invalidMain = true
			}
			item.mains++
		}
	}
	return item, nil
}

// discoverBuilds excludes main packages that match no release target, including ignored generators.
// Each remaining executable must support every target.
func discoverBuilds(moduleName string, packages map[string][]source) ([]Build, error) {
	var dirs []string
	for dir, sources := range packages {
		candidate := false
		for _, file := range sources {
			if file.pkg == "main" && file.mains > 0 {
				for _, goos := range []string{"linux", "darwin"} {
					for _, goarch := range []string{"amd64", "arm64"} {
						matches, err := file.matchesTarget(goos, goarch)
						if err != nil {
							return nil, err
						}
						candidate = candidate || matches
					}
				}
			}
		}
		if !candidate {
			continue
		}
		if err := validateTargets(dir, sources); err != nil {
			return nil, err
		}
		dirs = append(dirs, dir)
	}
	// Stable directory order keeps numeric name suffixes reproducible.
	slices.Sort(dirs)
	if len(dirs) == 0 {
		return nil, fmt.Errorf("go release discovery found no main packages; provide a custom .goreleaser.yaml or set its alteration to never")
	}
	var builds []Build
	used := map[string]bool{}
	for _, dir := range dirs {
		name := path.Base(dir)
		if dir == "." {
			name = path.Base(moduleName)
		}
		name = strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
				return r
			}
			return '-'
		}, name)
		if strings.Trim(name, "-_") == "" {
			return nil, fmt.Errorf("cannot derive a binary name for Go main package %q", dir)
		}
		unique := name
		for index := 2; used[unique]; index++ {
			unique = fmt.Sprintf("%s-%d", name, index)
		}
		used[unique] = true
		main := "."
		if dir != "." {
			main = "./" + dir
		}
		builds = append(builds, Build{ID: unique, Binary: unique, Main: main})
	}
	return builds, nil
}

type source struct {
	name        string
	data        []byte
	pkg         string
	mains       int
	cgo         bool
	invalidMain bool
}

func (file source) matchesTarget(goos, goarch string) (bool, error) {
	context := build.Default
	context.GOOS, context.GOARCH, context.CgoEnabled = goos, goarch, false
	// Match the release baseline rather than the host's architecture features or custom tags.
	context.BuildTags = nil
	context.ToolTags = []string{goarch + ".v1"}
	if goarch == "arm64" {
		context.ToolTags = []string{"arm64.v8.0"}
	}
	context.OpenFile = func(string) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(file.data)), nil }
	matches, err := context.MatchFile(".", path.Base(file.name))
	if err != nil {
		return false, fmt.Errorf("checking build constraints in %q: %w", file.name, err)
	}
	return matches, nil
}

func validateTargets(directory string, sources []source) error {
	for _, goos := range []string{"linux", "darwin"} {
		for _, goarch := range []string{"amd64", "arm64"} {
			mains := 0
			for _, file := range sources {
				matches, err := file.matchesTarget(goos, goarch)
				if err != nil {
					return err
				}
				if !matches {
					continue
				}
				if file.cgo {
					return fmt.Errorf("go main package %q uses cgo; generated releases require CGO_ENABLED=0", directory)
				}
				if file.pkg != "main" {
					return fmt.Errorf("go main package %q has mixed package names for %s/%s", directory, goos, goarch)
				}
				if file.invalidMain {
					return fmt.Errorf("unsupported main signature in %q", file.name)
				}
				mains += file.mains
			}
			if mains != 1 {
				return fmt.Errorf("go main package %q has %d main functions for %s/%s; generated releases require exactly one for every target", directory, mains, goos, goarch)
			}
		}
	}
	return nil
}

func readRegular(project fs.FS, name string) ([]byte, error) {
	info, err := fs.Lstat(project, name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%q is not a regular file", name)
	}
	if info.Size() > maxFileSize {
		return nil, fmt.Errorf("%q exceeds 1 MiB", name)
	}
	file, err := project.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFileSize {
		return nil, fmt.Errorf("%q exceeds 1 MiB", name)
	}
	return data, nil
}
