# Multi-Project Cloud Run Slack Bot Terraform Configuration

terraform {
  required_version = ">= 1.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

# Variables
variable "host_project_id" {
  description = "Project ID where the bot will be deployed"
  type        = string
}

variable "monitored_projects" {
  description = "List of projects to monitor"
  type = list(object({
    project_id      = string
    region          = string
    default_channel = string
    service_channels = map(string)
  }))
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

variable "slack_app_mode" {
  description = "Slack app mode (http or socket)"
  type        = string
  default     = "http"
}

variable "default_channel" {
  description = "Default Slack channel"
  type        = string
}

variable "service_account_name" {
  description = "Name of the service account"
  type        = string
  default     = "cloud-run-slack-bot"
}

variable "cloud_run_service_name" {
  description = "Name of the Cloud Run service"
  type        = string
  default     = "cloud-run-slack-bot"
}

variable "container_image" {
  description = "Container image for the bot"
  type        = string
  default     = "gcr.io/PROJECT_ID/cloud-run-slack-bot:latest"
}

# Provider configuration for host project
provider "google" {
  project = var.host_project_id
  region  = "us-central1"
}

# Provider aliases for monitored projects
provider "google" {
  alias   = "project1"
  project = var.monitored_projects[0].project_id
  region  = var.monitored_projects[0].region
}

# Add more provider aliases as needed for additional projects
# provider "google" {
#   alias   = "project2"
#   project = var.monitored_projects[1].project_id
#   region  = var.monitored_projects[1].region
# }

# Data sources
data "google_project" "host" {
  project_id = var.host_project_id
}

data "google_project" "monitored" {
  for_each   = { for p in var.monitored_projects : p.project_id => p }
  project_id = each.value.project_id
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

# Enable required APIs in monitored projects
resource "google_project_service" "monitored_apis" {
  for_each = {
    for pair in flatten([
      for project in var.monitored_projects : [
        for api in ["run.googleapis.com", "monitoring.googleapis.com", "logging.googleapis.com", "pubsub.googleapis.com"] : {
          project_id = project.project_id
          api        = api
          key        = "${project.project_id}-${api}"
        }
      ]
    ]) : pair.key => pair
  }

  project = each.value.project_id
  service = each.value.api

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
    "roles/pubsub.publisher",
    "roles/logging.logWriter"
  ])

  project = var.host_project_id
  role    = each.value
  member  = "serviceAccount:${google_service_account.bot_sa.email}"
}

# IAM roles for the service account in monitored projects
resource "google_project_iam_member" "monitored_permissions" {
  for_each = {
    for pair in flatten([
      for project in var.monitored_projects : [
        for role in ["roles/run.viewer", "roles/monitoring.viewer", "roles/logging.viewer"] : {
          project_id = project.project_id
          role       = role
          key        = "${project.project_id}-${role}"
        }
      ]
    ]) : pair.key => pair
  }

  project = each.value.project_id
  role    = each.value.role
  member  = "serviceAccount:${google_service_account.bot_sa.email}"
}

# Secret Manager secrets
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

# Pub/Sub topics and subscriptions for each monitored project
resource "google_pubsub_topic" "audit_logs" {
  for_each = { for p in var.monitored_projects : p.project_id => p }

  project = each.value.project_id
  name    = "cloud-run-audit-logs"

  depends_on = [google_project_service.monitored_apis]
}

# Log sinks for each monitored project
resource "google_logging_project_sink" "audit_sink" {
  for_each = { for p in var.monitored_projects : p.project_id => p }

  project     = each.value.project_id
  name        = "cloud-run-audit-sink"
  destination = "pubsub.googleapis.com/${google_pubsub_topic.audit_logs[each.key].id}"

  filter = "protoPayload.serviceName=\"run.googleapis.com\""

  unique_writer_identity = true

  depends_on = [google_pubsub_topic.audit_logs]
}

# IAM binding for log sink service account
resource "google_pubsub_topic_iam_binding" "sink_publisher" {
  for_each = { for p in var.monitored_projects : p.project_id => p }

  project = each.value.project_id
  topic   = google_pubsub_topic.audit_logs[each.key].name
  role    = "roles/pubsub.publisher"

  members = [
    google_logging_project_sink.audit_sink[each.key].writer_identity
  ]
}

# Pub/Sub push subscriptions
resource "google_pubsub_subscription" "audit_subscription" {
  for_each = { for p in var.monitored_projects : p.project_id => p }

  project = each.value.project_id
  name    = "cloud-run-audit-subscription"
  topic   = google_pubsub_topic.audit_logs[each.key].name

  push_config {
    push_endpoint = "${google_cloud_run_v2_service.bot.uri}/cloudrun/events"

    attributes = {
      x-goog-version = "v1"
    }

    oidc_token {
      service_account_email = google_service_account.bot_sa.email
    }
  }

  ack_deadline_seconds = 20
  retain_acked_messages = false
  message_retention_duration = "604800s" # 7 days

  depends_on = [google_cloud_run_v2_service.bot]
}

# Cloud Run service
resource "google_cloud_run_v2_service" "bot" {
  project  = var.host_project_id
  name     = var.cloud_run_service_name
  location = "us-central1"

  template {
    service_account = google_service_account.bot_sa.email

    containers {
      image = replace(var.container_image, "PROJECT_ID", var.host_project_id)

      env {
        name = "PROJECTS_CONFIG"
        value = jsonencode([
          for project in var.monitored_projects : {
            id              = project.project_id
            region          = project.region
            defaultChannel  = project.default_channel
            serviceChannels = project.service_channels
          }
        ])
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
        value = var.slack_app_mode
      }

      env {
        name  = "SLACK_CHANNEL"
        value = var.default_channel
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

output "audit_topics" {
  description = "Pub/Sub topics for audit logs"
  value = {
    for project_id, topic in google_pubsub_topic.audit_logs : project_id => topic.id
  }
}

output "projects_config" {
  description = "Generated projects configuration"
  value = jsonencode([
    for project in var.monitored_projects : {
      id              = project.project_id
      region          = project.region
      defaultChannel  = project.default_channel
      serviceChannels = project.service_channels
    }
  ])
  sensitive = false
}
