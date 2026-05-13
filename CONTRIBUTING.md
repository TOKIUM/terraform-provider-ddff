# Contributing

Thanks for your interest in contributing.

## Prerequisites

- Go 1.23+
- Terraform 1.8+
- A Datadog account with API and application keys for end-to-end testing

## Getting started

```bash
git clone https://github.com/TOKIUM/terraform-provider-ddff.git
cd terraform-provider-ddff
go build ./...
go test ./...
```

## Running the provider locally

The `e2e/` directory contains a development scratch project that uses
`dev_overrides` to load the locally built binary. See
[`e2e/Makefile`](./e2e/Makefile) for available targets:

```bash
cd e2e
export DD_API_KEY=...
export DD_APP_KEY=...
make apply       # build + apply against the configured Datadog org
make lifecycle   # apply -> drift check -> destroy -> apply -> destroy
make destroy
```

## Coding conventions

- Run `gofmt -s -w .` and `go vet ./...` before submitting.
- Each resource lives in its own file under `internal/provider/`.
- Resource type names are prefixed with `ddff_`.
- Prefer the official `datadog-api-client-go` over hand-rolled HTTP calls.

## Commit messages

This repository uses [Conventional Commits](https://www.conventionalcommits.org/),
and version bumps + CHANGELOG entries are managed by
[release-please](https://github.com/googleapis/release-please). Use one of:

- `feat:` for new resources, attributes, or behaviors (minor bump)
- `fix:` for bug fixes (patch bump)
- `feat!:` or a `BREAKING CHANGE:` footer for breaking changes (major bump)
- `docs:`, `refactor:`, `perf:`, `deps:`, `chore:`, `ci:`, `build:`,
  `test:`, `style:` for non-release-impacting changes

Example:

```
feat(allocation): add ddff_allocation resource

Adds CRUD for targeting rules and variant weights per (flag, environment).
```

## Documentation

Schema documentation is generated from the resource schemas using
[`tfplugindocs`](https://github.com/hashicorp/terraform-plugin-docs).
After changing a schema, run:

```bash
make docs
```

and commit the regenerated `docs/` tree.

## Releasing

Releases are driven by `release-please`. Merge a Conventional Commit to
`main`; the bot opens or updates a release PR. When the release PR is
merged, the tag triggers `.github/workflows/release.yml`, which builds
and signs the artifacts via GoReleaser and attaches them to the GitHub
Release. The Terraform Registry indexes the release automatically.
