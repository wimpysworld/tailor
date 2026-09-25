# Build snapshot release packages
snapshot:
    @goreleaser release --snapshot --clean --skip=sign
