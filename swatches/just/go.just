# Managed by Tailor: just/go.just

# Build Go packages
build:
    @go build ./...

# Run Go linters
lint-go:
    @golangci-lint run

# Run Go tests
test:
    @go test ./...
