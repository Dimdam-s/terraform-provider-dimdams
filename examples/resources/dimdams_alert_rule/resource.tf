resource "dimdams_alert_rule" "disk" {
  host_id         = data.dimdams_service.vps.id
  metric_type     = "DISK"
  threshold_value = 85
  channel_type    = "EMAIL"
  channel_target  = "ops@example.com"
}
