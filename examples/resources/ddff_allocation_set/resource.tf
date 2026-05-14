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

# Progressive rollout pattern: ramp the `on` variant for matching traffic
# in three steps with a uniform 24 h cadence between each step. Declare
# the `exposure_schedule` block to manage the schedule via Terraform; omit
# it (as in the allow-list example above) to leave any UI-side schedule
# untouched and disable drift detection for it.
resource "ddff_allocation_set" "new_checkout_canary" {
  feature_flag_id = ddff_feature_flag.new_checkout.id
  environment_id  = data.ddff_environment.production.id

  allocation {
    key  = "starter-canary"
    name = "Starter tier progressive rollout"
    type = "CANARY"

    targeting_rule {
      condition {
        attribute = "customer_tier"
        operator  = "EQUALS"
        value     = ["starter"]
      }
    }

    variant_weight {
      variant_key = "on"
      value       = 100
    }

    exposure_schedule {
      rollout_options {
        strategy              = "UNIFORM_INTERVALS"
        autostart             = false
        selection_interval_ms = 86400000 # 24 h between each step
      }
      rollout_step {
        exposure_ratio = 0.10
      }
      rollout_step {
        exposure_ratio = 0.50
      }
      rollout_step {
        exposure_ratio = 1.0
      }
    }
  }
}
