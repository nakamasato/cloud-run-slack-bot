# Set up Cloud Run with Terraform

## Create secret manager

```
echo -n "xoxb-xxxx" | gcloud secrets create slack-bot-token-cloud-run-slack-bot --replication-policy automatic --project "$PROJECT" --data-file=-
```

## Create GCP resources

`variables.tf`:

```hcl
variable "project" {
  description = "GCP Project ID"
}

variable "region" {
  description = "GCP Region"
}

variable "channel" {
  description = "Slack Channel ID"
}

variable "service_channel_mapping" {
  description = "Mapping of service names to Slack channel IDs (format: service1:channel1,service2:channel2)"
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
