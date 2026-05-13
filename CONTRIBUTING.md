# Contributing

Thanks for your interest in contributing.

## Development

Requirements:

- Go 1.23+
- Terraform 1.8+

```bash
go build .
go test ./...
```

## Code style

- `gofmt -s -w .`
- `go vet ./...`

## Resource conventions

- Resource type names use the `ddff_` prefix.
- New resources go under `internal/provider/`.
- Each resource lives in its own file (`<resource>_resource.go`).

## Releasing

Releases are tagged with `vX.Y.Z` (semver). CI builds and signs artifacts.
