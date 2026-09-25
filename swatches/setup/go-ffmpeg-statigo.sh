set -e
if grep -Eq '^module[[:space:]]+"?github\.com/linuxmatters/ffmpeg-statigo"?([[:space:]]|$)' go.mod; then
    echo "Error: setup supports consumer repositories only, not the ffmpeg-statigo source root."
    exit 1
fi
echo "Configuring git for submodule-friendly pulls..."
git config pull.ff only
git config submodule.recurse true

# Get latest stable release tag
TAG=$(
    if command -v jq &> /dev/null; then
        curl -s https://api.github.com/repos/linuxmatters/ffmpeg-statigo/releases | \
            jq -r '[.[] | select(.prerelease == false and .draft == false and (.tag_name | startswith("v")))][0].tag_name'
    else
        curl -s https://api.github.com/repos/linuxmatters/ffmpeg-statigo/releases | \
            grep -B5 '"prerelease": false' | grep '"tag_name"' | grep -v 'lib-' | head -1 | cut -d'"' -f4
    fi
)
if [ -z "$TAG" ] || [ "$TAG" = "null" ]; then
    echo "Error: Could not fetch latest release tag"
    exit 1
fi

# Initialise submodule if not already present
if [ ! -f "third_party/ffmpeg-statigo/go.mod" ]; then
    echo "Initialising ffmpeg-statigo submodule..."
    git submodule update --init --recursive
fi

# Check current version
cd third_party/ffmpeg-statigo
git fetch --no-tags origin "refs/tags/$TAG:refs/tags/$TAG"
CURRENT=$(git describe --tags --exact-match 2>/dev/null || echo "")

if [ "$CURRENT" = "$TAG" ]; then
    echo "ffmpeg-statigo already at latest version ($TAG)"
    cd ../..
else
    if [ -n "$CURRENT" ]; then
        echo "Updating ffmpeg-statigo from $CURRENT to $TAG..."
    else
        echo "Setting up ffmpeg-statigo $TAG..."
    fi
    git checkout "$TAG"
    cd ../..

    # Remove old library to force re-download
    rm -f third_party/ffmpeg-statigo/lib/*/libffmpeg.a

    # Stage the submodule change
    git add third_party/ffmpeg-statigo
fi

# Download libraries (will skip if correct version already exists)
echo "Checking ffmpeg-statigo libraries..."
cd third_party/ffmpeg-statigo && env CGO_ENABLED=1 go run ./cmd/download-lib
cd ../..

# Check if there are staged changes to commit
if git diff --cached --quiet third_party/ffmpeg-statigo; then
    echo "Setup complete!"
else
    echo ""
    echo "Setup complete! Submodule updated to $TAG"
    echo "Don't forget to commit: git commit -m 'chore: update ffmpeg-statigo to $TAG'"
fi
