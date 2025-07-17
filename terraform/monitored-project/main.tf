# Monitored Project Terraform Configuration
# 監視対象プロジェクト用の設定（各プロジェクトで実行）

terraform {
  required_version = ">= 1.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

variable "project_id" {
  description = "Monitored project ID"
  type        = string
}

variable "region" {
  description = "Region for resources"
  type        = string
  default     = "us-central1"
}

variable "bot_service_account_email" {
  description = "Email of the Cloud Run Slack Bot service account from host project"
  type        = string
}

variable "audit_webhook_url" {
  description = "Webhook URL for audit logs from host project"
  type        = string
}

provider "google" {
  project = var.project_id
  region  = var.region
}

# Enable required APIs in monitored project
resource "google_project_service" "monitored_apis" {
  for_each = toset([
    "run.googleapis.com",
    "monitoring.googleapis.com",
    "logging.googleapis.com",
    "pubsub.googleapis.com"
  ])

  project = var.project_id
  service = each.value

  disable_on_destroy = false
}

# 1. Grant permissions to bot service account for reading project resources
# Use custom role with specific permissions to avoid region-specific issues
resource "google_project_iam_custom_role" "cloud_run_slack_bot_role" {
  project     = var.project_id
  role_id     = "cloudRunSlackBotRole"
  title       = "Cloud Run Slack Bot Role"
  description = "Custom role for Cloud Run Slack Bot with minimal required permissions"

  permissions = [
    "run.services.list",
    "run.services.get",
    "run.jobs.list",
    "run.jobs.get",
    "run.locations.list",
    "run.revisions.get",
    "run.revisions.list",
    "monitoring.timeSeries.list",
    "monitoring.metricDescriptors.list",
    "monitoring.metricDescriptors.get",
    "logging.entries.list",
    "logging.logEntries.list",
    "resourcemanager.projects.get"
  ]

  depends_on = [google_project_service.monitored_apis]
}

resource "google_project_iam_member" "bot_custom_permissions" {
  project = var.project_id
  role    = google_project_iam_custom_role.cloud_run_slack_bot_role.name
  member  = "serviceAccount:${var.bot_service_account_email}"

  depends_on = [google_project_iam_custom_role.cloud_run_slack_bot_role]
}

# Additional permissions for cross-project access
resource "google_project_iam_member" "bot_service_usage" {
  project = var.project_id
  role    = "roles/serviceusage.serviceUsageConsumer"
  member  = "serviceAccount:${var.bot_service_account_email}"

  depends_on = [google_project_service.monitored_apis]
}

# Browser role for API discovery (sometimes needed for cross-project calls)
resource "google_project_iam_member" "bot_browser" {
  project = var.project_id
  role    = "roles/browser"
  member  = "serviceAccount:${var.bot_service_account_email}"

  depends_on = [google_project_service.monitored_apis]
}

# 2. Create Pub/Sub topic for audit logs
resource "google_pubsub_topic" "audit_logs" {
  project = var.project_id
  name    = "cloud-run-audit-logs"

  depends_on = [google_project_service.monitored_apis]
}

# 3. Create log sink for Cloud Run audit logs
resource "google_logging_project_sink" "audit_sink" {
  project     = var.project_id
  name        = "cloud-run-audit-sink"
  destination = "pubsub.googleapis.com/${google_pubsub_topic.audit_logs.id}"

  # Filter for Cloud Run audit logs
  filter = "protoPayload.serviceName=\"run.googleapis.com\""

  unique_writer_identity = true

  depends_on = [google_pubsub_topic.audit_logs]
}

# 4. Grant log sink service account permission to publish to topic
resource "google_pubsub_topic_iam_binding" "sink_publisher" {
  project = var.project_id
  topic   = google_pubsub_topic.audit_logs.name
  role    = "roles/pubsub.publisher"

  members = [
    google_logging_project_sink.audit_sink.writer_identity
  ]
}

# 5. Create push subscription to send audit logs to bot webhook
resource "google_pubsub_subscription" "audit_subscription" {
  project = var.project_id
  name    = "cloud-run-audit-subscription"
  topic   = google_pubsub_topic.audit_logs.name

  push_config {
    push_endpoint = var.audit_webhook_url

    attributes = {
      x-goog-version = "v1"
    }

    oidc_token {
      service_account_email = var.bot_service_account_email
    }
  }

  ack_deadline_seconds = 20
  retain_acked_messages = false
  message_retention_duration = "604800s" # 7 days

  depends_on = [google_pubsub_topic.audit_logs]
}

# Outputs
output "audit_topic_id" {
  description = "ID of the audit logs Pub/Sub topic"
  value       = google_pubsub_topic.audit_logs.id
}

output "audit_subscription_id" {
  description = "ID of the audit logs Pub/Sub subscription"
  value       = google_pubsub_subscription.audit_subscription.id
}

output "log_sink_writer_identity" {
  description = "Writer identity of the log sink"
  value       = google_logging_project_sink.audit_sink.writer_identity
}
