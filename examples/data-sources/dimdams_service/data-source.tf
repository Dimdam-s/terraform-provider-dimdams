data "dimdams_service" "vps" {
  name = "mon-vps"
}

output "vps_id" {
  value = data.dimdams_service.vps.id
}
