# Multi-Project Cloud Run Slack Bot with Terraform

This guide shows how to set up the Cloud Run Slack Bot with multi-project support using Terraform. The bot will be deployed in a host project and monitor multiple target projects.

## Architecture Overview

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Project 1     │    │   Project 2     │    │   Project 3     │
│   (Monitored)   │    │   (Monitored)   │    │   (Monitored)   │
├─────────────────┤    ├─────────────────┤    ├─────────────────┤
│ Cloud Run       │    │ Cloud Run       │    │ Cloud Run       │
│ Cloud Monitoring│    │ Cloud Monitoring│    │ Cloud Monitoring│
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
                                 ▼
                    ┌─────────────────────────┐
                    │     Host Project        │
                    │                         │
                    │ ┌─────────────────────┐ │
                    │ │   Cloud Run Bot     │ │
                    │ │   (Multi-Project)   │ │
                    │ └─────────────────────┘ │
                    │                         │
                    │ ┌─────────────────────┐ │
                    │ │   Secret Manager    │ │
                    │ │   (Slack Secrets)   │ │
                    │ └─────────────────────┘ │
                    └─────────────────────────┘
                                 │
                                 ▼
                         ┌─────────────┐
                         │   Slack     │
                         │   Channels  │
                         └─────────────┘
```

## Prerequisites

1. **Host Project**: Where the Cloud Run Slack Bot will be deployed
2. **Target Projects**: Projects to monitor (can be the same as host project)
3. **Slack App**: With necessary permissions and tokens
4. **Terraform**: Version 1.0+

## Setup Steps

### 1. Create Secret Manager

```bash
# Set your host project
HOST_PROJECT="your-host-project"

# Create secrets in host project
echo -n "xoxb-xxxx" | gcloud secrets create slack-bot-token-cloud-run-slack-bot --replication-policy automatic --project "$HOST_PROJECT" --data-file=-
echo -n "your-signing-secret" | gcloud secrets create slack-signing-secret-cloud-run-slack-bot --replication-policy automatic --project "$HOST_PROJECT" --data-file=-
```

### 2. Create GCP Resources

`variables.tf`:

```hcl
variable "host_project_id" {
  description = "GCP Project ID where the bot will be deployed"
  type        = string
}

variable "monitored_projects" {
  description = "List of projects to monitor"
  type = list(object({
    project_id       = string
    region           = string
    default_channel  = optional(string)
    service_channels = optional(map(string))
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
  default     = ""
  sensitive   = true
}

variable "slack_app_mode" {
  description = "Slack app mode (http or socket)"
  type        = string
  default     = "http"
  validation {
    condition     = contains(["http", "socket"], var.slack_app_mode)
    error_message = "Slack app mode must be 'http' or 'socket'."
  }
}

variable "default_channel" {
  description = "Default Slack channel ID"
  type        = string
  default     = "general"
}

variable "service_account_name" {
  description = "Service account name for the bot"
  type        = string
  default     = "cloud-run-slack-bot"
}

variable "cloud_run_service_name" {
  description = "Cloud Run service name"
  type        = string
  default     = "cloud-run-slack-bot"
}

variable "container_image" {
  description = "Container image URI"
  type        = string
  default     = "nakamasato/cloud-run-slack-bot:latest"
}

variable "service_region" {
  description = "Region for Cloud Run service"
  type        = string
  default     = "us-central1"
}
```

`main.tf`

```hcl
locals {
  cloud_run_slack_bot_envs = {
    PROJECT              = var.project
    REGION               = var.region
    SLACK_APP_MODE       = "http"
    SLACK_CHANNEL        = var.channel
    SERVICE_CHANNEL_MAPPING = var.service_channel_mapping
    TMP_DIR             = "/tmp"
  }
  cloud_run_slack_bot_secrets = {
    SLACK_BOT_TOKEN      = google_secret_manager_secret.slack_bot_token_cloud_run_slack_bot.secret_id
    SLACK_SIGNING_SECRET = google_secret_manager_secret.slack_signing_secret_cloud_run_slack_bot.secret_id
  }
}

resource "google_service_account" "cloud_run_slack_bot" {
  account_id   = "cloud-run-slack-bot"
  display_name = "Cloud Run Slack Bot"
}

resource "google_project_iam_member" "cloud_run_slack_bot" {
  for_each = toset([
    "roles/run.viewer",
    "roles/monitoring.viewer",
    "roles/cloudtrace.agent",
  ])
  project = var.project
  role    = each.value
  member  = google_service_account.cloud_run_slack_bot.member
}

resource "google_secret_manager_secret_iam_member" "cloud_run_slack_bot_is_secret_accessor" {
  for_each  = toset([for _, secret_id in local.cloud_run_slack_bot_secrets : secret_id])
  project   = var.project
  secret_id = each.key
  role      = "roles/secretmanager.secretAccessor"
  member    = google_service_account.cloud_run_slack_bot.member
}

// store secret version using gcloud command
resource "google_secret_manager_secret" "slack_bot_token_cloud_run_slack_bot" {
  secret_id = "slack-bot-token-cloud-run-slack-bot"
  replication {
    auto {}
  }
}

resource "google_secret_manager_secret" "slack_signing_secret_cloud_run_slack_bot" {
  secret_id = "slack-signing-secret-cloud-run-slack-bot"
  replication {
    auto {}
  }
}

import {
  id = "projects/${var.project}/secrets/slack-bot-token-cloud-run-slack-bot"
  to = google_secret_manager_secret.slack_bot_token_cloud_run_slack_bot
}

import {
  id = "projects/${var.project}/secrets/slack-signing-secret-cloud-run-slack-bot"
  to = google_secret_manager_secret.slack_signing_secret_cloud_run_slack_bot
}

resource "google_cloud_run_v2_service" "cloud_run_slack_bot" {
  name     = "cloud-run-slack-bot"
  location = var.region

  template {
    containers {
      image = "nakamasato/cloud-run-slack-bot"
      dynamic "env" {
        for_each = local.cloud_run_slack_bot_envs
        content {
          name  = env.key
          value = env.value
        }
      }

      dynamic "env" {
        for_each = local.cloud_run_slack_bot_secrets
        content {
          name = env.key
          value_source {
            secret_key_ref {
              secret  = env.value
              version = "latest"
            }
          }
        }
      }
    }
    service_account = google_service_account.cloud_run_slack_bot.email
  }

  lifecycle {
    ignore_changes = [
      client,
      client_version,
    ]
  }
  depends_on = [google_secret_manager_secret_iam_member.cloud_run_slack_bot_is_secret_accessor]
}

resource "google_cloud_run_service_iam_binding" "cloud_run_slack_bot" {
  location = google_cloud_run_v2_service.cloud_run_slack_bot.location
  service  = google_cloud_run_v2_service.cloud_run_slack_bot.name
  role     = "roles/run.invoker"
  members = [
    "allUsers",
  ]
}
```

```
terraform apply
```

## Set up Slack app

1. Set up Event Subscriptions

    - Request URL: `https://cloud-run-slack-bot-xxxxx.a.run.app/slack/events`
    - Subscribe to bot events: `app_mention`
    - Save Changes
1. Set up Interactivity & Shortcuts

    - Request URL: `https://cloud-run-slack-bot-xxxxx.a.run.app/slack/interaction`
    - Save Changes

Invite the Slack app to the target channel.


## (Optional) Set up Cloud Run audit log notification

To apply the following resources, the following roles are required:
1. `roles/logging.configWriter`
1. `roles/pubsub.admin`
1. `roles/iam.serviceAccountAdmin`

### Audit Log Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Project 1     │    │   Project 2     │    │   Project 3     │
│   (Monitored)   │    │   (Monitored)   │    │   (Monitored)   │
├─────────────────┤    ├─────────────────┤    ├─────────────────┤
│ Cloud Run       │    │ Cloud Run       │    │ Cloud Run       │
│ Cloud Monitoring│    │ Cloud Monitoring│    │ Cloud Monitoring│
│ Logging Sink    │    │ Logging Sink    │    │ Logging Sink    │
│ Audit Logs      │    │ Audit Logs      │    │ Audit Logs      │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
                                 ▼
                    ┌─────────────────────────┐
                    │     Host Project        │
                    │                         │
                    │ ┌─────────────────────┐ │
                    │ │   Cloud Run Bot     │ │
                    │ │   (Multi-Project)   │ │
                    │ └─────────────────────┘ │
                    │                         │
                    │ ┌─────────────────────┐ │
                    │ │   Pub/Sub Topic     │ │
                    │ │   (Audit Logs)     │ │
                    │ └─────────────────────┘ │
                    │                         │
                    │ ┌─────────────────────┐ │
                    │ │   Secret Manager    │ │
                    │ │   (Slack Secrets)   │ │
                    │ └─────────────────────┘ │
                    └─────────────────────────┘
                                 │
                                 ▼
                         ┌─────────────┐
                         │   Slack     │
                         │   Channels  │
                         └─────────────┘
```

Create the following resources:

1. Pub/Sub topic: `cloud-run-audit-log` (in host project)
1. Logging sink: Publish Cloud Run audit logs to Pub/Sub topic (in each monitored project)
1. Pub/Sub subscription: push subscription to deliver Cloud Run audit log to `cloud-run-slack-bot` service via HTTP request
1. Pub/Sub -> Cloud Run service account


```hcl
# Pub/Sub topic in host project
resource "google_pubsub_topic" "cloud_run_audit_log" {
  project = var.host_project_id
  name    = "cloud-run-audit-log"
}

# Logging sink in each monitored project
resource "google_logging_project_sink" "cloud_run_audit_log" {
  for_each = { for p in var.monitored_projects : p.project_id => p }

  project                = each.value.project_id
  name                   = "cloud_run_audit_log"
  destination            = "pubsub.googleapis.com/projects/${var.host_project_id}/topics/${google_pubsub_topic.cloud_run_audit_log.name}"
  filter                 = "(resource.type = cloud_run_revision OR resource.type = cloud_run_job) AND (logName = projects/${each.value.project_id}/logs/cloudaudit.googleapis.com%2Factivity OR logName = projects/${each.value.project_id}/logs/cloudaudit.googleapis.com%2Fsystem_event)"
  unique_writer_identity = true
}

# Grant logging sink permission to publish to Pub/Sub topic
resource "google_pubsub_topic_iam_member" "log_writer" {
  for_each = { for p in var.monitored_projects : p.project_id => p }

  project = var.host_project_id
  topic   = google_pubsub_topic.cloud_run_audit_log.name
  role    = "roles/pubsub.publisher"
  member  = google_logging_project_sink.cloud_run_audit_log[each.key].writer_identity
}

# Pubsub -> Cloud Run https://cloud.google.com/run/docs/triggering/pubsub-push
resource "google_service_account" "pubsub_invoker" {
  project      = var.host_project_id
  account_id   = "cloud-run-pubsub-invoker"
  display_name = "Cloud Run Pub/Sub Invoker"
}

resource "google_cloud_run_v2_service_iam_binding" "pubsub_invoker" {
  project  = var.host_project_id
  name     = google_cloud_run_v2_service.cloud_run_slack_bot.name
  location = google_cloud_run_v2_service.cloud_run_slack_bot.location
  role     = "roles/run.invoker"
  members = [
    "allUsers",
    google_service_account.pubsub_invoker.member,
  ]
}

resource "google_project_service_identity" "pubsub_agent" {
  provider = google-beta
  project  = var.host_project_id
  service  = "pubsub.googleapis.com"
}

resource "google_project_iam_member" "token_creator" {
  project = var.host_project_id
  role    = "roles/iam.serviceAccountTokenCreator"
  member  = "serviceAccount:${google_project_service_identity.pubsub_agent.email}"
}

resource "google_pubsub_subscription" "audit_logs" {
  project = var.host_project_id
  name    = "cloud-run-audit-log-subscription"
  topic   = google_pubsub_topic.cloud_run_audit_log.name

  push_config {
    push_endpoint = "${google_cloud_run_v2_service.cloud_run_slack_bot.uri}/cloudrun/events"
    oidc_token {
      service_account_email = google_service_account.pubsub_invoker.email
    }
    attributes = {
      x-goog-version = "v1"
    }
  }
}
```
