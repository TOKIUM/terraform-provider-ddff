## Summary

<!-- One or two sentences on what this PR changes and why. -->

## Type of change

<!-- Use a Conventional Commit prefix in the PR title so release-please can
classify it (feat:, fix:, docs:, refactor:, perf:, deps:, chore:, ci:,
build:, test:). Prefix with feat!: or include "BREAKING CHANGE:" in the
body to trigger a major version bump. -->

## Test plan

<!-- How did you verify the change? Include commands you ran. -->

- [ ] `go build ./...`
- [ ] `go vet ./...`
- [ ] `go test ./...`
- [ ] Manual e2e run against a Datadog org (if behavior-affecting)

## Checklist

- [ ] Conventional Commit title.
- [ ] Documentation updated (`make docs` if schema changed).
- [ ] CHANGELOG updates are left to release-please.
