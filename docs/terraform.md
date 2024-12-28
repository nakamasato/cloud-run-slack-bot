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
```

`main.tf`

```hcl
locals {
  cloud_run_slack_bot_envs = {
    PROJECT        = var.project
    REGION         = var.region
    SLACK_APP_MODE = "http"
    SLACK_CHANNEL  = var.channel
    TMP_DIR        = "/tmp"
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

Create the following resources:

1. Pub/Sub topic: `cloud-run-audit-log`
1. Logging sink: Publish Cloud Run audit logs to Pub/Sub topic
1. Pub/Sub subscription: push subscription to deliver Cloud Run audit log to `cloud-run-slack-bot` service via HTTP request
1. Pub/Sub -> Cloud Run service account


```hcl
# pub/sub topic
resource "google_pubsub_topic" "cloud_run_audit_log" {
  name = "cloud-run-audit-log"
}

resource "google_pubsub_topic_iam_member" "log_writer" {
  project = google_pubsub_topic.cloud_run_audit_log.project
  topic   = google_pubsub_topic.cloud_run_audit_log.name
  role    = "roles/pubsub.publisher"
  member  = google_logging_project_sink.cloud_run_audit_log.writer_identity
}

# Log Router Sink
resource "google_logging_project_sink" "cloud_run_audit_log" {
  name                   = "cloud_run_audit_log"
  destination            = "pubsub.googleapis.com/projects/${google_pubsub_topic.cloud_run_audit_log.project}/topics/${google_pubsub_topic.cloud_run_audit_log.name}"
  filter                 = "resource.type = cloud_run_revision AND (logName = projects/${var.project}/logs/cloudaudit.googleapis.com%2Factivity OR logName = projects/${var.project}/logs/cloudaudit.googleapis.com%2Fsystem_event)"
  unique_writer_identity = true
}

# Pubsub -> Cloud Run https://cloud.google.com/run/docs/triggering/pubsub-push?hl=ja
# https://cloud.google.com/run/docs/tutorials/pubsub#terraform_2
resource "google_service_account" "sa" {
  account_id   = "cloud-run-pubsub-invoker"
  display_name = "Cloud Run Pub/Sub Invoker"
}

resource "google_cloud_run_v2_service_iam_binding" "cloud_run_slack_bot" {
  name     = google_cloud_run_v2_service.cloud_run_slack_bot.name
  location = google_cloud_run_v2_service.cloud_run_slack_bot.location
  role     = "roles/run.invoker"
  members = [
    "allUsers",
    google_service_account.sa.member,
  ]
}

resource "google_project_service_identity" "pubsub_agent" {
  provider = google-beta
  service  = "pubsub.googleapis.com"
}

resource "google_project_iam_member" "project_token_creator" {
  project = var.project
  role    = "roles/iam.serviceAccountTokenCreator"
  member  = "serviceAccount:${google_project_service_identity.pubsub_agent.email}"
}

resource "google_pubsub_subscription" "subscription" {
  name  = "pubsub_subscription"
  topic = google_pubsub_topic.cloud_run_audit_log.name
  push_config {
    push_endpoint = "${google_cloud_run_v2_service.cloud_run_slack_bot.uri}/cloudrun/events" # defined in the cloud-run-slack-bot app
    oidc_token {
      service_account_email = google_service_account.sa.email
    }
    attributes = {
      x-goog-version = "v1"
    }
  }
}
```
