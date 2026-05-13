terraform {
  required_version = ">= 1.7"

  required_providers {
    ddff = {
      source  = "TOKIUM/datadog-feature-flags"
      version = "~> 0.1"
    }
  }
}

provider "ddff" {
  # All three attributes are optional. When unset they fall back to the
  # DD_API_KEY / DD_APP_KEY / DD_SITE environment variables (and
  # DATADOG_API_KEY / DATADOG_APP_KEY / DATADOG_SITE as a secondary
  # fallback). Setting them inline here is supported but discouraged for
  # secrets.
  #
  # api_key = var.dd_api_key
  # app_key = var.dd_app_key
  site = "datadoghq.com"
}

# -----------------------------------------------------------------------------
# BOOLEAN flag — the canonical use case.
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

# -----------------------------------------------------------------------------
# STRING flag with three variants (A/B/C experiment).
# -----------------------------------------------------------------------------

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

# -----------------------------------------------------------------------------
# INTEGER flag — values are still encoded as strings at the API layer.
# -----------------------------------------------------------------------------

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

# -----------------------------------------------------------------------------
# NUMERIC flag — floating-point variants.
# -----------------------------------------------------------------------------

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

# -----------------------------------------------------------------------------
# JSON flag — exercises both json_schema and structured variant values.
# -----------------------------------------------------------------------------

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
# Outputs — surface every computed attribute for visual inspection.
# -----------------------------------------------------------------------------

output "boolean_flag" {
  value = {
    id         = ddff_feature_flag.boolean.id
    created_at = ddff_feature_flag.boolean.created_at
    updated_at = ddff_feature_flag.boolean.updated_at
  }
}

output "string_flag" {
  value = {
    id         = ddff_feature_flag.string.id
    created_at = ddff_feature_flag.string.created_at
    updated_at = ddff_feature_flag.string.updated_at
  }
}

output "integer_flag" {
  value = {
    id         = ddff_feature_flag.integer.id
    created_at = ddff_feature_flag.integer.created_at
    updated_at = ddff_feature_flag.integer.updated_at
  }
}

output "numeric_flag" {
  value = {
    id         = ddff_feature_flag.numeric.id
    created_at = ddff_feature_flag.numeric.created_at
    updated_at = ddff_feature_flag.numeric.updated_at
  }
}

output "json_flag" {
  value = {
    id         = ddff_feature_flag.json.id
    created_at = ddff_feature_flag.json.created_at
    updated_at = ddff_feature_flag.json.updated_at
  }
}
