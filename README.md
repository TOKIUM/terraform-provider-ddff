# terraform-provider-datadog-feature-flags

[![Release](https://img.shields.io/github/v/release/TOKIUM/terraform-provider-datadog-feature-flags?include_prereleases&sort=semver)](https://github.com/TOKIUM/terraform-provider-datadog-feature-flags/releases)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](./LICENSE)

A community Terraform provider for [Datadog Feature Flags](https://docs.datadoghq.com/feature_flags/).
It manages feature flags, environments, per-environment enablement, and
targeting allocations through the Datadog Feature Flags v2 REST API.

> **Disclaimer**
> This project is not affiliated with, endorsed by, or sponsored by Datadog, Inc.
> "Datadog" is a registered trademark of Datadog, Inc. The provider is
> maintained independently and uses the publicly documented Datadog REST API.

## Installation

```hcl
terraform {
  required_version = ">= 1.8"
  required_providers {
    ddff = {
      source  = "TOKIUM/datadog-feature-flags"
      version = "~> 0.1"
    }
  }
}

provider "ddff" {
  # api_key, app_key, api_url fall back to DD_API_KEY / DD_APP_KEY / DD_HOST
  # (or DATADOG_API_KEY / DATADOG_APP_KEY / DATADOG_HOST) environment
  # variables. The precedence matches the official DataDog/datadog provider.
}
```

## Quick start

```hcl
resource "ddff_feature_flag" "new_checkout" {
  key                 = "new_checkout_flow"
  name                = "New checkout flow"
  description         = "Enables the redesigned checkout flow."
  value_type          = "BOOLEAN"
  default_variant_key = "off"

  variants {
    key   = "on"
    name  = "On"
    value = "true"
  }

  variants {
    key   = "off"
    name  = "Off"
    value = "false"
  }
}
```

For complete examples, see [`examples/`](./examples) and the documentation
on the [Terraform Registry](https://registry.terraform.io/providers/TOKIUM/datadog-feature-flags/latest).

## Resources

| Resource | Purpose |
| --- | --- |
| `ddff_feature_flag` | Define a feature flag, its value type, and its variants. |
| `ddff_environment` | Manage feature flag environments (dev/staging/prod, custom scopes). |
| `ddff_feature_flag_environment` | Enable or disable a flag in a specific environment. |

> **Targeting rule (allocation) management is not yet supported.**
> The Datadog API exposes only a "create one" POST and a "replace all" PUT
> for allocations, with no dedicated read endpoint, which makes it hard to
> model as an idempotent Terraform resource. A `ddff_allocation` resource
> is planned for a later release; manage allocations through the Datadog
> UI in the meantime.

## Data sources

| Data source | Purpose |
| --- | --- |
| `data.ddff_feature_flag` | Look up an existing feature flag by key. |
| `data.ddff_environment` | Look up an existing environment by key. |

## Provider configuration

| Attribute | Environment variables (in order) | Description |
| --- | --- | --- |
| `api_key` | `DD_API_KEY`, `DATADOG_API_KEY` | Datadog API key. |
| `app_key` | `DD_APP_KEY`, `DATADOG_APP_KEY` | Datadog application key. |
| `api_url` | `DD_HOST`, `DATADOG_HOST` | Full Datadog API URL. Defaults to `https://api.datadoghq.com`. |

The provider does not persist credentials; they are read at apply time only.

## Known behavior

- **Archive on destroy** — The Datadog API has no physical delete for feature
  flags, so `terraform destroy` archives the flag. Re-applying a previously
  destroyed configuration creates a brand-new flag with a fresh UUID; the
  archived flag remains visible in the Datadog UI.
- **`default_variant_key` is write-only at the flag level** — Datadog stores
  the effective default per environment, not on the flag itself. The
  provider preserves whatever value you set in HCL; UI-side changes to the
  per-environment default are not detected as drift on the
  `ddff_feature_flag` resource.
- **`json_schema` uses semantic JSON equality** — The API reformats the
  schema before returning it. The provider parses both sides as JSON
  before comparing, so cosmetic differences (whitespace, key order) are
  ignored while structural drift is still detected.

## Compatibility

| Component | Minimum |
| --- | --- |
| Terraform | 1.8 |
| Go (for building from source) | 1.23 |
| Datadog API | v2 (`/api/v2/feature-flags/*`) |

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md). Bug reports and pull requests
are welcome.

## License

[Apache License 2.0](./LICENSE).

## Trademarks

"Datadog" and the Datadog logo are trademarks of Datadog, Inc. and are
used here for descriptive purposes only.
