if [[ "${mode}" == correct ]]; then
    go mod tidy
    golangci-lint fmt --config .golangci.yml
fi
go mod tidy -diff
golangci-lint fmt --config .golangci.yml --diff
golangci-lint run --config .golangci.yml
govulncheck -show verbose ./...
gocyclo -top 20 -avg -ignore '_test\.go$' .
