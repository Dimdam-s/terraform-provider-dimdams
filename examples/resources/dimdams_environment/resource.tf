variable "discord_token" {
  type      = string
  sensitive = true
}

resource "dimdams_environment" "bot" {
  host_id = "bot-1793755197793-aecp57"

  variables = {
    NODE_ENV = "production"
  }

  secrets = {
    DISCORD_TOKEN = var.discord_token
  }
}
