terraform {
  required_providers {
    ddff = {
      source  = "TOKIUM/datadog-feature-flags"
      version = "~> 0.1"
    }
  }
}

provider "ddff" {
  # api_key, app_key, site default to DD_API_KEY / DD_APP_KEY / DD_SITE.
}

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
