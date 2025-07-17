# Host Project Terraform Configuration
# Cloud Run Slack Botをデプロイするプロジェクト用の設定

terraform {
  required_version = ">= 1.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

variable "host_project_id" {
  description = "Host project ID where the bot will be deployed"
  type        = string
}

variable "service_account_name" {
  description = "Name of the service account for the bot"
  type        = string
  default     = "cloud-run-slack-bot"
}

variable "slack_bot_token" {
  description = "Slack bot token"
  type        = string
  sensitive   = true
}

variable "slack_signing_secret" {
  description = "Slack signing secret"
  type        = string
  sensitive   = true
}

variable "slack_app_token" {
  description = "Slack app token (for socket mode)"
  type        = string
  sensitive   = true
  default     = ""
}

variable "projects_config" {
  description = "JSON configuration for monitored projects"
  type        = string
}

provider "google" {
  project = var.host_project_id
  region  = "us-central1"
}

# Enable required APIs in host project
resource "google_project_service" "host_apis" {
  for_each = toset([
    "run.googleapis.com",
    "cloudbuild.googleapis.com",
    "secretmanager.googleapis.com",
    "pubsub.googleapis.com",
    "logging.googleapis.com",
    "monitoring.googleapis.com",
    "iam.googleapis.com"
  ])

  project = var.host_project_id
  service = each.value

  disable_on_destroy = false
}

# Service account for the bot
resource "google_service_account" "bot_sa" {
  project      = var.host_project_id
  account_id   = var.service_account_name
  display_name = "Cloud Run Slack Bot Service Account"
  description  = "Service account for Cloud Run Slack Bot with multi-project access"

  depends_on = [google_project_service.host_apis]
}

# IAM roles for the service account in host project
resource "google_project_iam_member" "host_permissions" {
  for_each = toset([
    "roles/run.invoker",
    "roles/secretmanager.secretAccessor",
    "roles/pubsub.subscriber",
    "roles/logging.logWriter"
  ])

  project = var.host_project_id
  role    = each.value
  member  = "serviceAccount:${google_service_account.bot_sa.email}"
}

# Secret Manager secrets for Slack credentials
resource "google_secret_manager_secret" "slack_secrets" {
  for_each = toset(["slack-bot-token", "slack-signing-secret", "slack-app-token"])

  project   = var.host_project_id
  secret_id = each.value

  replication {
    automatic = true
  }

  depends_on = [google_project_service.host_apis]
}

resource "google_secret_manager_secret_version" "slack_bot_token" {
  secret      = google_secret_manager_secret.slack_secrets["slack-bot-token"].id
  secret_data = var.slack_bot_token
}

resource "google_secret_manager_secret_version" "slack_signing_secret" {
  secret      = google_secret_manager_secret.slack_secrets["slack-signing-secret"].id
  secret_data = var.slack_signing_secret
}

resource "google_secret_manager_secret_version" "slack_app_token" {
  secret      = google_secret_manager_secret.slack_secrets["slack-app-token"].id
  secret_data = var.slack_app_token
}

# Cloud Run service
resource "google_cloud_run_v2_service" "bot" {
  project  = var.host_project_id
  name     = "cloud-run-slack-bot"
  location = "us-central1"

  template {
    service_account = google_service_account.bot_sa.email

    containers {
      image = "gcr.io/${var.host_project_id}/cloud-run-slack-bot:latest"

      env {
        name  = "PROJECTS_CONFIG"
        value = var.projects_config
      }

      env {
        name = "SLACK_BOT_TOKEN"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.slack_secrets["slack-bot-token"].secret_id
            version = "latest"
          }
        }
      }

      env {
        name = "SLACK_SIGNING_SECRET"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.slack_secrets["slack-signing-secret"].secret_id
            version = "latest"
          }
        }
      }

      env {
        name = "SLACK_APP_TOKEN"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.slack_secrets["slack-app-token"].secret_id
            version = "latest"
          }
        }
      }

      env {
        name  = "SLACK_APP_MODE"
        value = "http"
      }

      env {
        name  = "SLACK_CHANNEL"
        value = "general"
      }

      env {
        name  = "TMP_DIR"
        value = "/tmp"
      }

      resources {
        limits = {
          cpu    = "1000m"
          memory = "1Gi"
        }
      }

      ports {
        container_port = 8080
      }
    }

    scaling {
      min_instance_count = 0
      max_instance_count = 10
    }
  }

  depends_on = [
    google_project_service.host_apis,
    google_secret_manager_secret_version.slack_bot_token,
    google_secret_manager_secret_version.slack_signing_secret,
    google_secret_manager_secret_version.slack_app_token
  ]
}

# IAM policy for Cloud Run service (allow unauthenticated access for webhooks)
resource "google_cloud_run_v2_service_iam_binding" "public_access" {
  project  = var.host_project_id
  location = google_cloud_run_v2_service.bot.location
  name     = google_cloud_run_v2_service.bot.name
  role     = "roles/run.invoker"

  members = [
    "allUsers"
  ]
}

# Outputs
output "service_url" {
  description = "URL of the Cloud Run service"
  value       = google_cloud_run_v2_service.bot.uri
}

output "service_account_email" {
  description = "Email of the service account"
  value       = google_service_account.bot_sa.email
}

output "webhook_url" {
  description = "Webhook URL for Slack Events API"
  value       = "${google_cloud_run_v2_service.bot.uri}/slack/events"
}

output "interaction_url" {
  description = "Interaction URL for Slack interactive components"
  value       = "${google_cloud_run_v2_service.bot.uri}/slack/interaction"
}

output "audit_webhook_url" {
  description = "Webhook URL for Cloud Run audit logs"
  value       = "${google_cloud_run_v2_service.bot.uri}/cloudrun/events"
}
