terraform {
  required_providers {
    heroku = {
      source  = "heroku/heroku"
      version = "~> 5.4.1"
    }
  }
}

resource "random_password" "session" {
  length  = 64
  special = false
}

resource "heroku_app" "app" {
  name   = var.app_name
  region = var.region
  config_vars = {
    APP_NAME       = var.app_name
    APPLE_TEAM_ID  = var.apple_team_id
    APPLE_KEY_ID   = var.apple_key_id
    SESSION_SECRET = random_password.session.result
  }
  sensitive_config_vars = {
    APPLE_PRIVATE_KEY = var.apple_private_key
  }
  buildpacks = ["heroku/go"]
}

resource "heroku_addon" "db" {
  app_id = heroku_app.app.id
  plan   = "heroku-postgresql:essential-0"
}

resource "heroku_addon" "logs" {
  app_id = heroku_app.app.id
  plan   = "logtail:free"
}

resource "heroku_formation" "web" {
  app_id   = heroku_app.app.id
  type     = "web"
  quantity = 1
  size     = "basic"
}
