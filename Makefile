# The binary suffix (datadog-feature-flags) is the Registry slug, but the
# provider's HCL type / resource prefix is `ddff`. tfplugindocs needs the
# resource prefix to discover schemas correctly.
PROVIDER_NAME ?= ddff

.PHONY: build test vet fmt docs

build:
	go build ./...

test:
	go test -race -count=1 ./...

vet:
	go vet ./...

fmt:
	gofmt -s -w .

# Regenerates docs/ from the provider schema and the examples/ directory.
# Uses the Go tools mechanism so the contributor does not have to install
# tfplugindocs separately. Requires a local Terraform CLI on $PATH; the
# tool spawns Terraform to introspect the schema.
docs:
	go tool tfplugindocs generate \
		--provider-name $(PROVIDER_NAME) \
		--rendered-provider-name "Datadog Feature Flags"

# Validates that docs/, examples/, and templates/ are consistent with each
# other (used in CI to detect uncommitted regeneration).
docs-validate:
	go tool tfplugindocs validate --provider-name $(PROVIDER_NAME)
