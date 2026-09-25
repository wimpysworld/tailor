#!/usr/bin/env bash
set -euo pipefail
go build -ldflags "-s -w" -o tailor ./cmd/tailor
