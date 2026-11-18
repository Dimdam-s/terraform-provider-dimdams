resource "dimdams_ssh_keys" "team" {
  host_id = data.dimdams_service.vps.id

  key {
    name       = "Portable"
    public_key = file("~/.ssh/id_ed25519.pub")
  }
}
