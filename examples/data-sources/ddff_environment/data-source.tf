data "ddff_environment" "production" {
  key = "production"
}

output "production_id" {
  value = data.ddff_environment.production.id
}
