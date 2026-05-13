# Allow-list pattern: only customers in the enterprise / professional
# tiers see the `on` variant; everyone else falls back to the flag's
# default_variant_key.
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
