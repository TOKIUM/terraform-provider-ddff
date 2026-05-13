# terraform-provider-ddff

[![Release](https://img.shields.io/github/v/release/TOKIUM/terraform-provider-ddff?include_prereleases&sort=semver)](https://github.com/TOKIUM/terraform-provider-ddff/releases)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](./LICENSE)

`ddff` is a community Terraform provider for managing feature flags,
environments, and per-environment enablement through the
[Datadog Feature Flags](https://docs.datadoghq.com/feature_flags/) v2 REST API.

> **Disclaimer**
> This project is not affiliated with, endorsed by, or sponsored by Datadog, Inc.
> "Datadog" is a registered trademark of Datadog, Inc. The provider is
> maintained independently and consumes only the publicly documented REST API.

## Installation

```hcl
terraform {
  required_version = ">= 1.8"
  required_providers {
    ddff = {
      source  = "TOKIUM/ddff"
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

Declare a flag, bind it to a production environment, and serve `on` only
to customers in the `enterprise` / `professional` tiers. Everyone else
gets the `default_variant_key` (`off`).

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

data "ddff_environment" "production" {
  name = "Production"
}

resource "ddff_feature_flag_environment" "new_checkout_prod" {
  feature_flag_id = ddff_feature_flag.new_checkout.id
  environment_id  = data.ddff_environment.production.id
  enabled         = true
}

resource "ddff_allocation_set" "new_checkout_prod" {
  feature_flag_id = ddff_feature_flag.new_checkout.id
  environment_id  = data.ddff_environment.production.id

  allocation {
    key  = "tier-allowlist"
    name = "Allowed customer tiers"
    type = "FEATURE_GATE"

    targeting_rule {
      condition {
        attribute = "customer_tier"
        operator  = "ONE_OF"
        value     = ["enterprise", "professional"]
      }
    }

    variant_weight {
      variant_key = "on"
      value       = 100
    }
    variant_weight {
      variant_key = "off"
      value       = 0
    }
  }
}
```

Full examples live under [`examples/`](./examples), and the generated
reference docs are served on the
[Terraform Registry](https://registry.terraform.io/providers/TOKIUM/ddff/latest).

## Resources

| Resource | Purpose |
| --- | --- |
| `ddff_feature_flag` | Define a feature flag, its value type, and its variants. |
| `ddff_environment` | Manage environments (dev / staging / prod, custom scopes). |
| `ddff_feature_flag_environment` | Enable or disable a flag in a specific environment. |
| `ddff_allocation_set` | Manage the full set of targeting rules + variant weight distributions for one feature flag in one environment. |

## Data sources

| Data source | Purpose |
| --- | --- |
| `data.ddff_feature_flag` | Look up an existing feature flag by key. |
| `data.ddff_environment` | Look up an existing environment by name. |

## Provider configuration

| Attribute | Environment variables (in order) | Description |
| --- | --- | --- |
| `api_key` | `DD_API_KEY`, `DATADOG_API_KEY` | API key. |
| `app_key` | `DD_APP_KEY`, `DATADOG_APP_KEY` | Application key. |
| `api_url` | `DD_HOST`, `DATADOG_HOST` | Full API URL. Defaults to `https://api.datadoghq.com`. |

The provider does not persist credentials; they are read at apply time only.

## Known behavior

- **Archive on destroy** — The upstream API has no physical delete for
  feature flags, so `terraform destroy` archives the flag. Re-applying
  a previously destroyed configuration creates a brand-new flag with a
  fresh UUID; the archived flag remains visible in the Datadog UI.
- **`default_variant_key` is write-only at the flag level** — The
  effective default is stored per environment, not on the flag itself.
  The provider preserves whatever value you set in HCL; UI-side changes
  to the per-environment default are not detected as drift on
  `ddff_feature_flag`.
- **`json_schema` uses semantic JSON equality** — The API reformats the
  schema before returning it. The provider parses both sides as JSON
  before comparing, so cosmetic differences (whitespace, key order) are
  ignored while structural drift is still detected.
- **`ddff_allocation_set` does not detect UI-side drift** — The upstream
  API exposes no dedicated read endpoint for allocations. The resource
  trusts state and reconciles by overwriting on the next apply; edits
  made in the Datadog UI will be silently replaced.

## Compatibility

| Component | Minimum |
| --- | --- |
| Terraform | 1.8 |
| Go (for building from source) | 1.23 |
| Datadog Feature Flags API | v2 (`/api/v2/feature-flags/*`) |

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md). Bug reports and pull requests
are welcome.

## License

[Apache License 2.0](./LICENSE).

## Trademarks

"Datadog" and the Datadog logo are trademarks of Datadog, Inc. and are
used here for descriptive purposes only.
