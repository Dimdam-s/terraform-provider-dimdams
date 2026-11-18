resource "dimdams_scheduled_tasks" "night" {
  host_id = data.dimdams_service.vps.id

  task {
    name   = "Sauvegarde"
    action = "backup"
    time   = "03:00"
  }

  task {
    name   = "Redémarrage"
    action = "restart"
    time   = "04:00"
  }
}
