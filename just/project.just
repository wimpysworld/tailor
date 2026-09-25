# Build snapshot release packages
snapshot:
    @goreleaser release --snapshot --clean --skip=sign

# Alter Tailor swatches from source
alter-source:
    @go run ./cmd/tailor alter

# Preview changes from source, then inspect local project health
measure-source:
    @go run ./cmd/tailor baste
    @go run ./cmd/tailor measure
