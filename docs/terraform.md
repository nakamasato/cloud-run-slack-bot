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
resource "google_service_account" "cloud_run_slack_bot" {
  account_id   = "cloud-run-slack-bot"
  display_name = "Cloud Run Slack Bot"
}

resource "google_project_iam_member" "cloud_run_slack_bot" {
  for_each = toset([
    "roles/run.viewer",
    "roles/monitoring.viewer",
  ])
  project = var.project
  role    = each.value
  member  = google_service_account.cloud_run_slack_bot.member
}

resource "google_secret_manager_secret_iam_member" "cloud_run_slack_bot_is_secret_accessor" {
  for_each = {
    for secret in [
      google_secret_manager_secret.slack_bot_token_cloud_run_slack_bot,
    ] : secret.secret_id => secret.project
  }
  project   = each.value
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

// Import manually created secret manager
import {
  id = "projects/${project}/secrets/slack-bot-token-cloud-run-slack-bot"
  to = google_secret_manager_secret.slack_bot_token_cloud_run_slack_bot
}

resource "google_cloud_run_v2_service" "cloud-run-slack-bot" {
  name     = "cloud-run-slack-bot"
  location = var.region # asia-northeast1

  template {
    containers {
      image = "nakamasato/cloud-run-slack-bot:0.0.5"
      env {
        name  = "PROJECT"
        value = var.project
      }
      env {
        name  = "REGION"
        value = var.region
      }

      env {
        name  = "SLACK_APP_MODE"
        value = "http"
      }

      env {
        name  = "TMP_DIR"
        value = "/tmp"
      }

      env {
        name = "SLACK_BOT_TOKEN"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.slack_bot_token_cloud_run_slack_bot.secret_id
            version = "latest"
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
}

resource "google_cloud_run_service_iam_binding" "cloud_run_slack_bot" {
  location = google_cloud_run_v2_service.cloud_run_slack_bot.location
  service  = google_cloud_run_v2_service.cloud_run_slack_bot.name
  role     = "roles/run.invoker"
  members = [
    "allUsers"
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
  filter                 = "resource.type = cloud_run_revision AND logName = projects/${var.project}/logs/cloudaudit.googleapis.com%2Factivity"
  unique_writer_identity = true
}

# Pubsub -> Cloud Run https://cloud.google.com/run/docs/triggering/pubsub-push?hl=ja
# https://cloud.google.com/run/docs/tutorials/pubsub#terraform_2
resource "google_service_account" "sa" {
  account_id   = "cloud-run-pubsub-invoker"
  display_name = "Cloud Run Pub/Sub Invoker"
}

resource "google_cloud_run_service_iam_binding" "cloud_run_slack_bot" {
  location = google_cloud_run_v2_service.cloud_run_slack_bot.location
  service  = google_cloud_run_v2_service.cloud_run_slack_bot.name
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

resource "google_project_iam_binding" "project_token_creator" {
  project = var.project
  role    = "roles/iam.serviceAccountTokenCreator"
  members = ["serviceAccount:${google_project_service_identity.pubsub_agent.email}"]
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
