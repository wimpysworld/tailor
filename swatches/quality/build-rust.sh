version="$(git describe --tags --match 'v*.*.*' --always --dirty 2>/dev/null || echo dev)"
version="${version#v}"
BUILD_VERSION="${version}" cargo build --workspace --locked
