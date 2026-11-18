data "dimdams_services" "vps" {
  type = "vps"
}

output "vps_names" {
  value = [for s in data.dimdams_services.vps.services : s.name]
}
