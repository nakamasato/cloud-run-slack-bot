# Multi-Project Setup Guide

This guide explains how to configure the Cloud Run Slack Bot to monitor multiple GCP projects with project-specific Slack channel routing.

## Overview

The multi-project feature allows you to:
- Monitor Cloud Run services and jobs across multiple GCP projects
- Route notifications to different Slack channels based on project and service
- Maintain unified bot interactions across all projects

## Configuration

### 1. Environment Variables in Cloud Run service

**Multi-Project Configuration**

Set the `PROJECTS_CONFIG` environment variable with a JSON array of project configurations:

```bash
export PROJECTS_CONFIG='[
  {
    "id": "project1",
    "region": "us-central1",
    "defaultChannel": "project1-alerts",
    "serviceChannels": {
      "web-service": "web-team",
      "api-service": "api-team"
    }
  },
  {
    "id": "project2",
    "region": "us-east1",
    "defaultChannel": "project2-alerts",
    "serviceChannels": {
      "batch-job": "batch-team"
    }
  }
]'
```

**Channel-to-Project Mapping**

Based on the above configuration, the bot automatically creates these mappings:

- `project1-alerts` → `project1` (auto-detect enabled)
- `web-team` → `project1` (auto-detect enabled)
- `api-team` → `project1` (auto-detect enabled)
- `project2-alerts` → `project2` (auto-detect enabled)
- `batch-team` → `project2` (auto-detect enabled)

Result: `shared-alerts` → `[project1, project2]` (manual selection required)

**Configuration Structure**

Each project configuration supports:

- **`id`**: GCP project ID (required)
- **`region`**: GCP region (required)
- **`defaultChannel`**: Default Slack channel for this project (optional)
- **`serviceChannels`**: Service/job-specific channel mappings (optional)

**Channel Resolution Priority**

The bot resolves Slack channels in this order:
1. Service-specific channel in project configuration
2. Project default channel
3. Global default channel (`SLACK_CHANNEL`)


**Cloud Run yaml**:

```yaml
# cloud-run-service.yaml
apiVersion: serving.knative.dev/v1
kind: Service
metadata:
  name: cloud-run-slack-bot
  annotations:
    run.googleapis.com/ingress: all
spec:
  template:
    metadata:
      annotations:
        run.googleapis.com/service-account: SERVICE_ACCOUNT_EMAIL
    spec:
      containers:
      - image: gcr.io/PROJECT_ID/cloud-run-slack-bot
        env:
        - name: PROJECTS_CONFIG
          value: |
            [
              {
                "id": "project1",
                "region": "us-central1",
                "defaultChannel": "project1-alerts",
                "serviceChannels": {
                  "web-service": "web-team"
                }
              },
              {
                "id": "project2",
                "region": "us-east1",
                "defaultChannel": "project2-alerts"
              }
            ]
        - name: SLACK_BOT_TOKEN
          valueFrom:
            secretKeyRef:
              name: slack-secrets
              key: bot-token
        - name: SLACK_SIGNING_SECRET
          valueFrom:
            secretKeyRef:
              name: slack-secrets
              key: signing-secret
        - name: SLACK_APP_MODE
          value: "http"
        - name: TMP_DIR
          value: "/tmp"
```

### 2. Target Project Configuration

> [!NOTE]
> 1. Allow service account `cloud-run-slack-bot` to get Cloud Run service inforamtion and monitoring information.
> 1. Publish Cloud Run audit log from the target project to PubSub topic (in the project where cloud-run-slack-bot is running) (This is for Cloud Run audit log notification)

With terraform:

```hcl
locals {
 cloud_run_slack_bot_project = "cloud-run-slack-bot-project"
}
resource "google_project_iam_member" "di_sandbox_cloud_run_slack_bot" {
  for_each = toset([
    "roles/monitoring.viewer",
    "roles/run.viewer",
  ])
  project = var.project
  role    = each.value
  member  = "serviceAccount:cloud-run-slack-bot@${local.cloud_run_slack_bot_project}.iam.gserviceaccount.com"
}

# Log Router Sink - publishes to cloud-run-audit-log pubsub topic
resource "google_logging_project_sink" "cloud_run_audit_log" {
  name                   = "cloud_run_audit_log"
  destination            = "pubsub.googleapis.com/projects/${local.cloud_run_slack_bot_project}/topics/cloud-run-audit-log"
  filter                 = "(resource.type = cloud_run_revision OR resource.type = cloud_run_job) AND (logName = projects/${var.project}/logs/cloudaudit.googleapis.com%2Factivity OR logName = projects/${var.project}/logs/cloudaudit.googleapis.com%2Fsystem_event)"
  unique_writer_identity = true
}
```

With gcloud:

```bash
# For each project
gcloud projects add-iam-policy-binding TARGET_PROJECT_ID \
    --member="serviceAccount:cloud-run-slack-bot@<cloud-run-slack-bot project>.iamserviceaccount.com" \
    --role="roles/run.viewer"

gcloud projects add-iam-policy-binding TARGET_PROJECT_ID \
    --member="serviceAccount:cloud-run-slack-bot@<cloud-run-slack-bot project>.iamserviceaccount.com" \
    --role="roles/monitoring.viewer"
```

The following commands are necessary for Cloud Run audit log notification. Also see [terraform.md](terraform.md#optional-set-up-cloud-run-audit-log-notification)

```bash
# Set variables
CLOUD_RUN_SLACK_BOT_PROJECT="your-cloud-run-slack-bot-project"
TARGET_PROJECT="your-target-project"
# Create the logging sink
gcloud logging sinks create cloud_run_audit_log \
    "pubsub.googleapis.com/projects/${CLOUD_RUN_SLACK_BOT_PROJECT}/topics/cloud-run-audit-log" \
    --log-filter="(resource.type=cloud_run_revision OR resource.type=cloud_run_job) AND
(logName=projects/${TARGET_PROJECT}/logs/cloudaudit.googleapis.com%2Factivity OR
logName=projects/${TARGET_PROJECT}/logs/cloudaudit.googleapis.com%2Fsystem_event)" \
    --project="${TARGET_PROJECT}"
```

### 3. cloud-run-slack-bot Project Configuration

> [!NOTE]
> Allow gcp-sa-logging service account in the target project to publish pubsub message to `cloud-run-slack-bot` pubsub topic

> [!WARNING]
> This configuration is necessary for Cloud Run audit log notification. Also see [terraform.md](terraform.md#optional-set-up-cloud-run-audit-log-notification)

```hcl
locals {
  cloud_run_slack_bot_target_project_numbers = [123456789]
}
## Allow logging project sink in proj-everbrew to publish pubsub message to the cloud_run_audit_log topic
resource "google_pubsub_topic_iam_member" "log_writer" {
  for_each = toset(
    local.cloud_run_slack_bot_target_project_numbers
  )
  project = google_pubsub_topic.cloud_run_audit_log.project
  topic   = google_pubsub_topic.cloud_run_audit_log.name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:service-${each.value}@gcp-sa-logging.iam.gserviceaccount.com"
}
```

gcloud:

```bash
# Get the sink's service account (equivalent to unique_writer_identity)
SINK_SERVICE_ACCOUNT=$(gcloud logging sinks describe cloud_run_audit_log --project="${TARGET_PROJECT}" --format="value(writerIdentity)")

# Grant the sink service account permission to publish to the Pub/Sub topic
gcloud pubsub topics add-iam-policy-binding cloud-run-audit-log \
    --member="${SINK_SERVICE_ACCOUNT}" \
    --role="roles/pubsub.publisher" \
    --project="${CLOUD_RUN_SLACK_BOT_PROJECT}"
```
