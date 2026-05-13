terraform {
  required_providers {
    ddff = {
      source  = "TOKIUM/datadog-feature-flags"
      version = "~> 0.1"
    }
  }
}

provider "ddff" {
  # api_key, app_key, and site fall back to DD_API_KEY / DD_APP_KEY / DD_SITE
  # (or DATADOG_API_KEY / DATADOG_APP_KEY / DATADOG_SITE) environment
  # variables when not set here.
  #
  # api_key = var.dd_api_key
  # app_key = var.dd_app_key
  # site    = "datadoghq.com"
}
