data "ddff_feature_flag" "new_checkout" {
  key = "new_checkout_flow"
}

output "new_checkout_id" {
  value = data.ddff_feature_flag.new_checkout.id
}
