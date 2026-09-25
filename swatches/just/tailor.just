# Managed by Tailor: just/tailor.just
[[TAILOR_MANAGED_CGO]]

# Run explicitly requested setup for enabled components
setup: [[TAILOR_MANAGED_SETUP_DEPENDENCIES]]
    [[TAILOR_MANAGED_SETUP_COMMAND]]

# Build all enabled ecosystems
build:
    #!/usr/bin/env bash
    set -euo pipefail
    [[TAILOR_MANAGED_BUILD_BODY]]

# Run full vet and tests with console coverage
test:
    #!/usr/bin/env bash
    set -euo pipefail
    [[TAILOR_MANAGED_TEST_BODY]]

# Check source without changes, or tidy and format with correct
lint $mode="check":
    #!/usr/bin/env bash
    set -euo pipefail
    case "${mode}" in
        check|correct) ;;
        *) echo "Error: lint mode must be check or correct" >&2; exit 1 ;;
    esac
    [[TAILOR_MANAGED_LINT_BODY]]

# Create a local release tag with just release x.y.z
release $VERSION:
    #!/usr/bin/env bash
    set -e

    if ! [[ "${VERSION}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        echo "Error: VERSION must be in format x.y.z (e.g., 0.1.0)"
        exit 1
    fi

    # Require a clean checkout so that the tag matches the local source files.
    if [ -n "$(git status --porcelain)" ]; then
        echo "Error: Working directory is not clean"
        exit 1
    fi

    if git show-ref --tags --verify --quiet "refs/tags/v${VERSION}"; then
        echo "Error: Tag v${VERSION} already exists"
        exit 1
    fi

    echo "Creating release v${VERSION}..."
    git tag -a "v${VERSION}" -m "v${VERSION}"
    echo "Tag v${VERSION} created"
    echo ""
    echo "To publish the release:"
    echo "  git push origin v${VERSION}"
    echo ""
