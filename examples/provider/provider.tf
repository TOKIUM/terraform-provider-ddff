terraform {
  required_providers {
    ddff = {
      source  = "TOKIUM/datadog-feature-flags"
      version = "~> 0.1"
    }
  }
}

provider "ddff" {
  # api_key, app_key, and api_url fall back to DD_API_KEY / DD_APP_KEY /
  # DD_HOST (or DATADOG_API_KEY / DATADOG_APP_KEY / DATADOG_HOST as a
  # secondary fallback) environment variables when not set here. The
  # precedence matches the official DataDog/datadog Terraform provider.
  #
  # api_key = var.dd_api_key
  # app_key = var.dd_app_key
  # api_url = "https://api.datadoghq.com"
}
