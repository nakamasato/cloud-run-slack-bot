# Multi-Project Setup Guide

This guide explains how to configure the Cloud Run Slack Bot to monitor multiple GCP projects with project-specific Slack channel routing.

## Overview

The multi-project feature allows you to:
- Monitor Cloud Run services and jobs across multiple GCP projects
- Route notifications to different Slack channels based on project and service
- Maintain unified bot interactions across all projects

## Configuration

### 1. Environment Variables

**Multi-Project Configuration (New)**

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

### 2. Service Account Permissions

For each GCP project you want to monitor, the service account `cloud-run-slack-bot` needs these IAM roles:

```bash
# For each project
gcloud projects add-iam-policy-binding TARGET_PROJECT_ID \
    --member="serviceAccount:cloud-run-slack-bot@<cloud-run-slack-bot project>.iamserviceaccount.com" \
    --role="roles/run.viewer"

gcloud projects add-iam-policy-binding TARGET_PROJECT_ID \
    --member="serviceAccount:cloud-run-slack-bot@<cloud-run-slack-bot project>.iamserviceaccount.com" \
    --role="roles/monitoring.viewer"
```

### 3. Event Subscription Setup

#### Prerequisite

In `cloud-run-slack-bot` project, PubSub topic is already set.

#### Grant Pub/Sub Permissions

```hcl
resource "google_logging_project_sink" "cloud_run_audit_log" {
  name                   = "cloud_run_audit_log"
  destination            = "pubsub.googleapis.com/projects/<cloud-run-slack-botのproject>/topics/cloud-run-audit-log"
  filter                 = "(resource.type = cloud_run_revision OR resource.type = cloud_run_job) AND (logName =
projects/${var.project}/logs/cloudaudit.googleapis.com%2Factivity OR logName =
projects/${var.project}/logs/cloudaudit.googleapis.com%2Fsystem_event)"
  unique_writer_identity = true
}
```


### Centralized vs Distributed Setup

#### Centralized Setup (Recommended)
- Deploy one bot instance
- Configure it to monitor all projects
- All audit logs push to the same bot endpoint

#### Distributed Setup
- Deploy separate bot instances per project
- Each bot monitors its own project
- Requires separate Slack apps or shared bot token


### Common Issues

1. **No resources found**: Check IAM permissions for all projects
2. **Audit logs not delivered**: Verify Pub/Sub subscription and push endpoint
3. **Wrong channel routing**: Check `PROJECTS_CONFIG` JSON syntax and channel mappings
4. **Cross-project errors**: Ensure service account has permissions in all projects

### Health Checks

The bot logs configuration on startup. Look for:
```
Configuration loaded:
  Default Channel: general
  Slack App Mode: http
  Projects:
    - ID: project1, Region: us-central1, Default Channel: project1-alerts
```

## Migration from Single-Project

1. **Backup existing configuration**
2. **Update environment variables**:
   ```bash
   # Before
   export PROJECT=my-project
   export REGION=us-central1
   export SLACK_CHANNEL=alerts
   export SERVICE_CHANNEL_MAPPING=service1:team1,service2:team2

   # After
   export PROJECTS_CONFIG='[{
     "id": "my-project",
     "region": "us-central1",
     "defaultChannel": "alerts",
     "serviceChannels": {
       "service1": "team1",
       "service2": "team2"
     }
   }]'
   ```
3. **Deploy updated bot**
4. **Test functionality**
5. **Remove old environment variables**

## Security Considerations

- **Principle of least privilege**: Grant minimal required permissions
- **Service account key management**: Use workload identity when possible
- **Slack token security**: Store tokens in secret managers
- **Network security**: Restrict ingress to Pub/Sub push endpoints
- **Audit logging**: Monitor bot access patterns

## Scaling Considerations

- **API quotas**: Monitor GCP API usage across projects
- **Slack rate limits**: Consider message frequency across all projects
- **Resource limits**: Scale bot compute resources based on project count
- **Regional deployment**: Consider deploying bots closer to monitored regions

## Terraform Deployment

### Overview

A comprehensive Terraform configuration is provided for automated deployment of the multi-project Cloud Run Slack Bot. This configuration handles all the complexity of setting up cross-project permissions, Pub/Sub subscriptions, and the bot service itself.

### Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Project 1     │    │   Project 2     │    │   Project 3     │
│   (Monitored)   │    │   (Monitored)   │    │   (Monitored)   │
├─────────────────┤    ├─────────────────┤    ├─────────────────┤
│ Cloud Run       │    │ Cloud Run       │    │ Cloud Run       │
│ Cloud Monitoring│    │ Cloud Monitoring│    │ Cloud Monitoring│
│ Pub/Sub Topic   │    │ Pub/Sub Topic   │    │ Pub/Sub Topic   │
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
                    │ │   Secret Manager    │ │
                    │ │   (Slack Secrets)   │ │
                    │ └─────────────────────┘ │
                    │                         │
                    │ ┌─────────────────────┐ │
                    │ │   Service Account   │ │
                    │ │   (Cross-Project)   │ │
                    │ └─────────────────────┘ │
                    └─────────────────────────┘
                                 │
                                 ▼
                         ┌─────────────┐
                         │   Slack     │
                         │   Channels  │
                         └─────────────┘
```

### Quick Start

1. **Navigate to Terraform directory**:
   ```bash
   cd terraform/multi-project
   ```

2. **Copy and configure settings**:
   ```bash
   cp terraform.tfvars.example terraform.tfvars
   vi terraform.tfvars
   ```

3. **Deploy with automated script**:
   ```bash
   ./scripts/deploy.sh
   ```

### Configuration Example

```hcl
# Host project (where bot will be deployed)
host_project_id = "my-bot-host-project"

# Projects to monitor
monitored_projects = [
  {
    project_id      = "web-frontend-project"
    region          = "us-central1"
    default_channel = "web-alerts"
    service_channels = {
      "web-app"     = "web-team"
      "api-gateway" = "api-team"
    }
  },
  {
    project_id      = "data-pipeline-project"
    region          = "us-east1"
    default_channel = "data-alerts"
    service_channels = {
      "etl-job"      = "data-team"
      "ml-inference" = "ml-team"
    }
  },
  {
    project_id      = "mobile-backend-project"
    region          = "europe-west1"
    default_channel = "mobile-alerts"
    service_channels = {
      "user-service"    = "mobile-team"
      "push-service"    = "mobile-team"
      "payment-service" = "payments-team"
    }
  }
]

# Slack configuration
slack_bot_token      = "xoxb-your-bot-token"
slack_signing_secret = "your-signing-secret"
slack_app_mode       = "http"
default_channel      = "general"
```

### Generated Channel Mappings

The above configuration automatically generates these channel-to-project mappings:

- `web-alerts` → `web-frontend-project` (auto-detect enabled)
- `web-team` → `web-frontend-project` (auto-detect enabled)
- `api-team` → `web-frontend-project` (auto-detect enabled)
- `data-alerts` → `data-pipeline-project` (auto-detect enabled)
- `data-team` → `data-pipeline-project` (auto-detect enabled)
- `ml-team` → `data-pipeline-project` (auto-detect enabled)
- `mobile-alerts` → `mobile-backend-project` (auto-detect enabled)
- `mobile-team` → `mobile-backend-project` (auto-detect enabled)
- `payments-team` → `mobile-backend-project` (auto-detect enabled)

### Resources Created

#### Host Project
- **Cloud Run Service**: Multi-project Slack Bot
- **Service Account**: Cross-project permissions
- **Secret Manager**: Slack authentication secrets
- **IAM Bindings**: Required permissions

#### Monitored Projects (Each)
- **Pub/Sub Topic**: Audit log events
- **Pub/Sub Subscription**: Webhook push to bot
- **Logging Sink**: Cloud Run audit logs
- **IAM Bindings**: Bot access permissions

### Deployment Commands

```bash
# Plan deployment
./scripts/deploy.sh plan

# Deploy infrastructure
./scripts/deploy.sh

# Validate configuration
./scripts/deploy.sh validate

# Destroy infrastructure
./scripts/deploy.sh destroy
```

### Manual Deployment

If you prefer manual deployment:

```bash
# Initialize Terraform
terraform init

# Plan deployment
terraform plan

# Apply configuration
terraform apply

# View outputs
terraform output
```

### Post-Deployment Configuration

After successful deployment, configure your Slack App:

1. **Events API URL**: `https://your-bot-url/slack/events`
2. **Interactive Components URL**: `https://your-bot-url/slack/interaction`
3. **Slash Commands**: Configure if needed

### IAM Permissions

#### Host Project Permissions
サービスアカウントに付与される権限（Terraformコード：`google_project_iam_member.host_permissions`）：

```hcl
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
```

- `roles/run.invoker`: Cloud Run service invocation
- `roles/secretmanager.secretAccessor`: Slack secrets access
- `roles/pubsub.subscriber`: Pub/Sub message reception
- `roles/pubsub.publisher`: Pub/Sub message publishing
- `roles/logging.logWriter`: Log writing

#### Monitored Project Permissions
各監視対象プロジェクトに付与される権限（Terraformコード：`google_project_iam_member.monitored_permissions`）：

```hcl
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
```

- `roles/run.viewer`: Cloud Run resource access
- `roles/monitoring.viewer`: Metrics access
- `roles/logging.viewer`: Log access

#### 追加権限（Pub/Sub用）
監査ログ用のPub/Sub権限（Terraformコード：`google_pubsub_topic_iam_binding.sink_publisher`）：

```hcl
resource "google_pubsub_topic_iam_binding" "sink_publisher" {
  for_each = { for p in var.monitored_projects : p.project_id => p }

  project = each.value.project_id
  topic   = google_pubsub_topic.audit_logs[each.key].name
  role    = "roles/pubsub.publisher"

  members = [
    google_logging_project_sink.audit_sink[each.key].writer_identity
  ]
}
```

#### Cloud Run Public Access
Slackからのwebhook受信用の公開アクセス権限（Terraformコード：`google_cloud_run_v2_service_iam_binding.public_access`）：

```hcl
resource "google_cloud_run_v2_service_iam_binding" "public_access" {
  project  = var.host_project_id
  location = google_cloud_run_v2_service.bot.location
  name     = google_cloud_run_v2_service.bot.name
  role     = "roles/run.invoker"

  members = [
    "allUsers"
  ]
}
```

### Terraform Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `host_project_id` | Project where bot is deployed | Required |
| `monitored_projects` | List of projects to monitor | Required |
| `slack_bot_token` | Slack bot token | Required |
| `slack_signing_secret` | Slack signing secret | Required |
| `slack_app_token` | Slack app token (socket mode) | "" |
| `slack_app_mode` | "http" or "socket" | "http" |
| `default_channel` | Default Slack channel | "general" |
| `service_account_name` | Service account name | "cloud-run-slack-bot" |
| `cloud_run_service_name` | Cloud Run service name | "cloud-run-slack-bot" |
| `container_image` | Container image URI | "gcr.io/PROJECT_ID/cloud-run-slack-bot:latest" |

### Terraform Outputs

| Output | Description |
|--------|-------------|
| `service_url` | Cloud Run service URL |
| `webhook_url` | Slack webhook URL |
| `interaction_url` | Slack interaction URL |
| `service_account_email` | Service account email |
| `projects_config` | Generated projects configuration |
| `channel_to_project_mapping` | Channel mappings |

### Monitoring and Troubleshooting

#### View Terraform State
```bash
terraform state list
terraform state show google_cloud_run_v2_service.bot
```

#### Check Deployment Status
```bash
terraform output service_url
terraform output projects_config
```

#### Debug Issues
```bash
# Check service logs
gcloud run services logs read cloud-run-slack-bot --project=HOST_PROJECT_ID

# Check Pub/Sub subscriptions
gcloud pubsub subscriptions list --project=MONITORED_PROJECT_ID

# Verify IAM permissions
gcloud projects get-iam-policy PROJECT_ID --flatten="bindings[].members" --format="table(bindings.role)" --filter="bindings.members:SERVICE_ACCOUNT_EMAIL"
```

### Updating Configuration

To add new projects or modify existing ones:

1. Update `terraform.tfvars`
2. Run `terraform plan` to review changes
3. Run `terraform apply` to apply changes
4. The bot will automatically pick up new configuration

### Best Practices

1. **State Management**: Use remote state storage
   ```hcl
   terraform {
     backend "gcs" {
       bucket = "your-terraform-state-bucket"
       prefix = "cloud-run-slack-bot"
     }
   }
   ```

2. **Environment Separation**: Use workspaces for different environments
   ```bash
   terraform workspace new production
   terraform workspace new staging
   ```

3. **Security**: Store sensitive variables securely
   ```bash
   export TF_VAR_slack_bot_token="xoxb-your-token"
   ```

4. **Validation**: Always run plan before apply
   ```bash
   terraform plan -out=tfplan
   terraform apply tfplan
   ```

### Cost Optimization

- **Cloud Run**: Configured for 0 minimum instances
- **Pub/Sub**: Pay-per-use pricing
- **Secret Manager**: Minimal cost for secrets
- **Monitoring**: Included in GCP free tier

### Advanced Configuration

#### Custom Service Account
```hcl
# Use existing service account
variable "existing_service_account" {
  description = "Email of existing service account"
  type        = string
  default     = ""
}
```

#### Regional Deployment
```hcl
# Deploy to specific region
variable "service_region" {
  description = "Region for Cloud Run service"
  type        = string
  default     = "us-central1"
}
```

#### Resource Limits
```hcl
# Custom resource limits
variable "cpu_limit" {
  description = "CPU limit for Cloud Run"
  type        = string
  default     = "1000m"
}

variable "memory_limit" {
  description = "Memory limit for Cloud Run"
  type        = string
  default     = "1Gi"
}
```

This Terraform configuration provides a production-ready, scalable deployment of the multi-project Cloud Run Slack Bot with automated channel-based project detection.
