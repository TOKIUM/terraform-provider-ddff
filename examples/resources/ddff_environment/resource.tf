resource "ddff_environment" "production" {
  name    = "Production"
  queries = ["env:production"]

  is_production                 = true
  require_feature_flag_approval = true
}
