variable "app_name" {
  description = "The name of the Heroku app"
  type        = string
}

variable "region" {
  description = "The region where the Heroku app will be deployed"
  type        = string
  default     = "us"
}

variable "apple_client_id" {
  description = "The Apple client ID for the Heroku app"
  type        = string
}

variable "apple_team_id" {
  description = "The Apple team ID for the Heroku app"
  type        = string
}

variable "apple_key_id" {
  description = "The Apple key ID for the Heroku app"
  type        = string
}

variable "apple_private_key" {
  description = "The Apple private key for the Heroku app"
  type        = string
  sensitive   = true
}
