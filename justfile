# List available recipes
default:
    @just --list

# Alter tailor swatches
alter:
    @tailor alter

# Build tailor binary
build:
    @go build -ldflags "-s -w" -o tailor ./cmd/tailor

# Run linters
lint:
    @go vet ./...
    @golangci-lint run

# Run tests
test:
    @go test ./...

# Check what tailor would change and measure
measure:
    @tailor baste
    @tailor measure

# Create a new release tag (requires VERSION=x.y.z)
release VERSION:
    #!/usr/bin/env bash
    set -e

    # Validate version format
    if ! echo "{{VERSION}}" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$'; then
        echo "Error: VERSION must be in format x.y.z (e.g., 0.1.0)"
        exit 1
    fi

    # Check for uncommitted or untracked changes
    if [ -n "$(git status --porcelain)" ]; then
        echo "Error: Working directory is not clean"
        exit 1
    fi

    # Check if tag already exists
    if git show-ref --tags --verify --quiet "refs/tags/{{VERSION}}"; then
        echo "Error: Tag {{VERSION}} already exists"
        exit 1
    fi

    echo "Creating release {{VERSION}}..."
    git tag -a "{{VERSION}}" -m "{{VERSION}}"
    echo "Tag {{VERSION}} created"
    echo ""
    echo "To publish the release:"
    echo "  git push origin {{VERSION}}"
    echo ""
    echo "This will trigger the GitHub Actions release workflow which will:"
    echo "  - Build binaries for all platforms"
    echo "  - Generate changelog from commits"
    echo "  - Create GitHub release with downloadable assets"
