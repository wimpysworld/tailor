# Match the pure-Go release build so lint and test check the same code.
# Disabling CGO also prevents false positives from a broken local C toolchain.
export CGO_ENABLED := "0"

# Build the stripped Tailor binary
build-tailor:
    @go build -ldflags "-s -w" -o tailor ./cmd/tailor

# Build snapshot release packages
snapshot:
    @goreleaser release --snapshot --clean --skip=sign

# Check the GoReleaser config and Nix flake
check:
    @goreleaser check
    @nix flake check

# Run all linters
lint-all: lint-go lint

# Alter Tailor swatches from source
alter-source:
    @go run ./cmd/tailor alter

# Preview changes from source, then inspect local project health
measure-source:
    @go run ./cmd/tailor baste
    @go run ./cmd/tailor measure
