#!/usr/bin/env bash
set -euo pipefail

VERSION=$(git describe --tags --match 'v*.*.*' --always --dirty 2>/dev/null || echo "dev")
VERSION="${VERSION#v}"
go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o tailor ./cmd/tailor
