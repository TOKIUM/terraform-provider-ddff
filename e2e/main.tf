terraform {
  required_version = ">= 1.7"

  required_providers {
    ddff = {
      source  = "TOKIUM/ddff"
      version = "~> 0.1"
    }
  }
}

provider "ddff" {
  # Credentials and the API URL fall back to DD_API_KEY / DD_APP_KEY /
  # DD_HOST (and DATADOG_API_KEY / DATADOG_APP_KEY / DATADOG_HOST as a
  # secondary fallback) when not set here.
  api_url = "https://api.datadoghq.com"
}

# -----------------------------------------------------------------------------
# Feature flags - one per supported value_type to cover the full schema.
# -----------------------------------------------------------------------------

resource "ddff_feature_flag" "boolean" {
  key                 = "ddff_e2e_boolean"
  name                = "ddff e2e boolean flag"
  description         = "End-to-end coverage for BOOLEAN value_type with two variants and an explicit default."
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

resource "ddff_feature_flag" "string" {
  key                 = "ddff_e2e_string"
  name                = "ddff e2e string flag"
  description         = "End-to-end coverage for STRING value_type with three variants."
  value_type          = "STRING"
  default_variant_key = "control"

  variants {
    key   = "control"
    name  = "Control"
    value = "control"
  }
  variants {
    key   = "treatment_a"
    name  = "Treatment A"
    value = "treatment_a"
  }
  variants {
    key   = "treatment_b"
    name  = "Treatment B"
    value = "treatment_b"
  }
}

resource "ddff_feature_flag" "integer" {
  key                 = "ddff_e2e_integer"
  name                = "ddff e2e integer flag"
  description         = "End-to-end coverage for INTEGER value_type."
  value_type          = "INTEGER"
  default_variant_key = "small"

  variants {
    key   = "small"
    name  = "Small"
    value = "10"
  }
  variants {
    key   = "medium"
    name  = "Medium"
    value = "100"
  }
  variants {
    key   = "large"
    name  = "Large"
    value = "1000"
  }
}

resource "ddff_feature_flag" "numeric" {
  key                 = "ddff_e2e_numeric"
  name                = "ddff e2e numeric flag"
  description         = "End-to-end coverage for NUMERIC value_type."
  value_type          = "NUMERIC"
  default_variant_key = "baseline"

  variants {
    key   = "baseline"
    name  = "Baseline"
    value = "0.05"
  }
  variants {
    key   = "aggressive"
    name  = "Aggressive"
    value = "0.25"
  }
}

resource "ddff_feature_flag" "json" {
  key                 = "ddff_e2e_json"
  name                = "ddff e2e json flag"
  description         = "End-to-end coverage for JSON value_type, including json_schema validation."
  value_type          = "JSON"
  default_variant_key = "disabled"

  json_schema = jsonencode({
    type = "object"
    properties = {
      enabled  = { type = "boolean" }
      max_rows = { type = "integer" }
    }
    required = ["enabled"]
  })

  variants {
    key   = "disabled"
    name  = "Disabled"
    value = jsonencode({ enabled = false, max_rows = 0 })
  }
  variants {
    key   = "enabled_small"
    name  = "Enabled (small)"
    value = jsonencode({ enabled = true, max_rows = 100 })
  }
  variants {
    key   = "enabled_large"
    name  = "Enabled (large)"
    value = jsonencode({ enabled = true, max_rows = 10000 })
  }
}

# -----------------------------------------------------------------------------
# Environment + per-environment binding.
# -----------------------------------------------------------------------------

resource "ddff_environment" "e2e" {
  name                          = "ddff e2e"
  queries                       = ["env:ddff-e2e"]
  is_production                 = false
  require_feature_flag_approval = false
}

resource "ddff_feature_flag_environment" "boolean_e2e" {
  feature_flag_id = ddff_feature_flag.boolean.id
  environment_id  = ddff_environment.e2e.id
  enabled         = true
}

# -----------------------------------------------------------------------------
# Allocation set: serve `on` to listed customer tiers, fall back to the flag's
# default_variant_key ("off") for everyone else.
# -----------------------------------------------------------------------------

resource "ddff_allocation_set" "boolean_e2e" {
  feature_flag_id = ddff_feature_flag.boolean.id
  environment_id  = ddff_environment.e2e.id

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

# -----------------------------------------------------------------------------
# Data sources - verify lookup works for both flag (by key) and environment
# (by name; the Datadog API does not expose a per-environment key).
# -----------------------------------------------------------------------------

data "ddff_feature_flag" "boolean_lookup" {
  key = ddff_feature_flag.boolean.key
}

data "ddff_environment" "e2e_lookup" {
  name = ddff_environment.e2e.name
}

# -----------------------------------------------------------------------------
# Outputs - surface every computed attribute for visual inspection.
# -----------------------------------------------------------------------------

output "boolean_flag" {
  value = {
    id         = ddff_feature_flag.boolean.id
    created_at = ddff_feature_flag.boolean.created_at
  }
}

output "string_flag_id" { value = ddff_feature_flag.string.id }
output "integer_flag_id" { value = ddff_feature_flag.integer.id }
output "numeric_flag_id" { value = ddff_feature_flag.numeric.id }
output "json_flag_id" { value = ddff_feature_flag.json.id }

output "environment" {
  value = {
    id            = ddff_environment.e2e.id
    name          = ddff_environment.e2e.name
    is_production = ddff_environment.e2e.is_production
  }
}

output "boolean_in_e2e" {
  value = {
    id                 = ddff_feature_flag_environment.boolean_e2e.id
    status             = ddff_feature_flag_environment.boolean_e2e.status
    default_variant_id = ddff_feature_flag_environment.boolean_e2e.default_variant_id
  }
}

output "boolean_e2e_allocation" {
  value = {
    id    = ddff_allocation_set.boolean_e2e.id
    rules = [for a in ddff_allocation_set.boolean_e2e.allocation : { key = a.key, id = a.id, order_position = a.order_position }]
  }
}

output "boolean_lookup_id" { value = data.ddff_feature_flag.boolean_lookup.id }
output "e2e_lookup_id" { value = data.ddff_environment.e2e_lookup.id }
