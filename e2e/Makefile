# E2E manual test harness.
#
# Prereq: DATADOG_API_KEY and DATADOG_APP_KEY must already be exported in the
# shell (the Makefile intentionally does not load any .env file).

export TF_CLI_CONFIG_FILE := $(CURDIR)/.terraformrc

PROVIDER_DIR := $(CURDIR)/..

.PHONY: help build apply plan drift destroy lifecycle clean

help:
	@echo "Available targets:"
	@echo "  build      Rebuild the provider binary."
	@echo "  apply      Build + terraform apply."
	@echo "  plan       Build + terraform plan."
	@echo "  drift      Build + plan with detailed-exitcode (non-zero on drift)."
	@echo "  destroy    terraform destroy."
	@echo "  lifecycle  apply -> drift check -> destroy -> apply -> destroy."
	@echo "  clean      Remove local state and .terraform/ caches."

build:
	cd $(PROVIDER_DIR) && go build .

apply: build
	terraform apply -auto-approve

plan: build
	terraform plan

drift: build
	terraform plan -detailed-exitcode

destroy:
	terraform destroy -auto-approve

lifecycle: build
	terraform apply -auto-approve
	terraform plan -detailed-exitcode
	terraform destroy -auto-approve
	terraform apply -auto-approve
	terraform destroy -auto-approve

clean:
	rm -rf .terraform .terraform.lock.hcl terraform.tfstate terraform.tfstate.backup
