# terraform-provider-datadog-feature-flags

> **Status:** Private development. Will be open-sourced when stable.

An unofficial Terraform provider for [Datadog Feature Flags](https://docs.datadoghq.com/feature_flags/).
Manages feature flags, environments, allocations (targeting rules), and exposure schedules
via the Datadog REST API.

**Disclaimer:** This project is not affiliated with, endorsed by, or sponsored by
Datadog, Inc. "Datadog" is a registered trademark of Datadog, Inc. This provider
is maintained independently and uses the publicly documented Datadog REST API.

## Status

Pre-1.0. Breaking changes are possible until v1.0.0.

## Usage (preview)

```hcl
terraform {
  required_providers {
    ddff = {
      source  = "TOKIUM/datadog-feature-flags"
      version = "~> 0.1"
    }
  }
}

provider "ddff" {
  # Credentials read from DD_API_KEY / DD_APP_KEY environment variables by default
  # (DATADOG_API_KEY / DATADOG_APP_KEY are honored as a secondary fallback, matching
  # the official DataDog/datadog provider's convention).
  # api_key = var.dd_api_key
  # app_key = var.dd_app_key
  # api_url = "https://api.datadoghq.com"  # or DD_HOST / DATADOG_HOST
}

resource "ddff_feature_flag" "new_checkout" {
  key         = "new_checkout_flow"
  name        = "New checkout flow"
  description = "Enables the redesigned checkout flow."
}
```

## Installation (private phase)

While the provider is private, install via `scripts/install.sh` which downloads
the binary from a GitHub Release and places it in the local filesystem mirror.
Configure `~/.terraformrc`:

```hcl
provider_installation {
  filesystem_mirror {
    path    = "/home/USER/.terraform.d/plugins-mirror"
    include = ["registry.terraform.io/TOKIUM/*"]
  }
  direct {
    exclude = ["registry.terraform.io/TOKIUM/*"]
  }
}
```

## Development

- Go 1.23+
- Terraform 1.8+

```bash
go build .
```

## License

[Apache License 2.0](./LICENSE)
