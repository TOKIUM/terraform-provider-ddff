resource "ddff_feature_flag_environment" "new_checkout_prod" {
  feature_flag_id = ddff_feature_flag.new_checkout.id
  environment_id  = data.ddff_environment.production.id

  enabled = false
}
