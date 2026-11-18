resource "dimdams_firewall_rule" "ssh_office" {
  host_id     = data.dimdams_service.vps.id
  port_range  = "22"
  source_cidr = "203.0.113.10/32"
  description = "SSH depuis le bureau"
}

resource "dimdams_firewall_rule" "deny_rest" {
  host_id  = data.dimdams_service.vps.id
  action   = "DROP"
  protocol = "ANY"
  priority = 900
}
