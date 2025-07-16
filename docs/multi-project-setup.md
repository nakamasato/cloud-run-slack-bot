# Multi-Project Setup Guide

This guide explains how to configure the Cloud Run Slack Bot to monitor multiple GCP projects with project-specific Slack channel routing.

## Overview

The multi-project feature allows you to:
- Monitor Cloud Run services and jobs across multiple GCP projects
- Route notifications to different Slack channels based on project and service
- Maintain unified bot interactions across all projects

## Configuration

### Environment Variables

#### Multi-Project Configuration (New)

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

#### Channel-to-Project Mapping

Based on the above configuration, the bot automatically creates these mappings:

- `project1-alerts` → `project1` (auto-detect enabled)
- `web-team` → `project1` (auto-detect enabled)
- `api-team` → `project1` (auto-detect enabled)
- `project2-alerts` → `project2` (auto-detect enabled)
- `batch-team` → `project2` (auto-detect enabled)

If multiple projects share the same channel, manual project selection is required:

```bash
# Example with shared channel
export PROJECTS_CONFIG='[
  {
    "id": "project1",
    "region": "us-central1",
    "defaultChannel": "shared-alerts"
  },
  {
    "id": "project2", 
    "region": "us-east1",
    "defaultChannel": "shared-alerts"
  }
]'
```

Result: `shared-alerts` → `[project1, project2]` (manual selection required)

#### Backward Compatibility (Legacy)

If `PROJECTS_CONFIG` is not set, the bot falls back to single-project mode using these variables:
- `PROJECT`: GCP project ID
- `REGION`: GCP region
- `SLACK_CHANNEL`: Default Slack channel
- `SERVICE_CHANNEL_MAPPING`: Service-to-channel mapping (format: `service1:channel1,service2:channel2`)

#### Required Variables

These variables are required regardless of configuration mode:
- `SLACK_BOT_TOKEN`: Slack bot token
- `SLACK_SIGNING_SECRET`: Slack signing secret (for HTTP mode)
- `SLACK_APP_TOKEN`: Slack app token (for socket mode)
- `SLACK_APP_MODE`: `socket` or `http`
- `TMP_DIR`: Directory for temporary files

### Configuration Structure

Each project configuration supports:

- **`id`**: GCP project ID (required)
- **`region`**: GCP region (required)  
- **`defaultChannel`**: Default Slack channel for this project (optional)
- **`serviceChannels`**: Service/job-specific channel mappings (optional)

### Channel Resolution Priority

The bot resolves Slack channels in this order:
1. Service-specific channel in project configuration
2. Project default channel
3. Global default channel (`SLACK_CHANNEL`)

## IAM Permissions Setup

### Service Account Permissions

For each GCP project you want to monitor, the service account needs these IAM roles:

#### Required Roles

```bash
# For each project
gcloud projects add-iam-policy-binding PROJECT_ID \
    --member="serviceAccount:SERVICE_ACCOUNT_EMAIL" \
    --role="roles/run.viewer"

gcloud projects add-iam-policy-binding PROJECT_ID \
    --member="serviceAccount:SERVICE_ACCOUNT_EMAIL" \
    --role="roles/monitoring.viewer"
```

#### Alternative: Custom Role

Create a custom role with minimal permissions:

```bash
# Create custom role
gcloud iam roles create cloudRunSlackBotRole \
    --project=PROJECT_ID \
    --title="Cloud Run Slack Bot Role" \
    --description="Minimal permissions for Cloud Run Slack Bot" \
    --permissions="\
run.services.list,\
run.services.get,\
run.jobs.list,\
run.jobs.get,\
monitoring.timeSeries.list"

# Assign custom role to service account
gcloud projects add-iam-policy-binding PROJECT_ID \
    --member="serviceAccount:SERVICE_ACCOUNT_EMAIL" \
    --role="projects/PROJECT_ID/roles/cloudRunSlackBotRole"
```

### Cross-Project Setup

If your service account is in a different project than the ones you're monitoring:

```bash
# For each monitored project
for PROJECT_ID in project1 project2; do
    gcloud projects add-iam-policy-binding $PROJECT_ID \
        --member="serviceAccount:SERVICE_ACCOUNT_EMAIL" \
        --role="roles/run.viewer"
    
    gcloud projects add-iam-policy-binding $PROJECT_ID \
        --member="serviceAccount:SERVICE_ACCOUNT_EMAIL" \
        --role="roles/monitoring.viewer"
done
```

## Event Subscription Setup

### Pub/Sub Configuration

For audit log notifications, set up Pub/Sub subscriptions for each project:

#### 1. Create Pub/Sub Topic

```bash
# For each project
gcloud pubsub topics create cloud-run-audit-logs --project=PROJECT_ID
```

#### 2. Create Log Sink

```bash
# For each project
gcloud logging sinks create cloud-run-audit-sink \
    pubsub.googleapis.com/projects/PROJECT_ID/topics/cloud-run-audit-logs \
    --log-filter='protoPayload.serviceName="run.googleapis.com"' \
    --project=PROJECT_ID
```

#### 3. Grant Pub/Sub Permissions

```bash
# Get the service account for the log sink
SERVICE_ACCOUNT=$(gcloud logging sinks describe cloud-run-audit-sink \
    --project=PROJECT_ID \
    --format="value(writerIdentity)")

# Grant publish permissions
gcloud pubsub topics add-iam-policy-binding cloud-run-audit-logs \
    --member="$SERVICE_ACCOUNT" \
    --role="roles/pubsub.publisher" \
    --project=PROJECT_ID
```

#### 4. Create Push Subscription

```bash
# For each project
gcloud pubsub subscriptions create cloud-run-audit-subscription \
    --topic=cloud-run-audit-logs \
    --push-endpoint=https://YOUR_BOT_URL/cloudrun/events \
    --project=PROJECT_ID
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

## Deployment Examples

### Cloud Run Deployment

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

### Docker Compose Example

```yaml
# docker-compose.yml
version: '3.8'
services:
  cloud-run-slack-bot:
    image: cloud-run-slack-bot:latest
    ports:
      - "8080:8080"
    environment:
      - PROJECTS_CONFIG=[{"id":"project1","region":"us-central1","defaultChannel":"alerts"}]
      - SLACK_BOT_TOKEN=${SLACK_BOT_TOKEN}
      - SLACK_SIGNING_SECRET=${SLACK_SIGNING_SECRET}
      - SLACK_APP_MODE=http
      - TMP_DIR=/tmp
    volumes:
      - /tmp:/tmp
```

## Bot Usage

### Channel-Based Project Detection

The bot automatically detects projects based on the Slack channel where commands are issued:

#### Single Project Channel (Auto-Detection)
When a channel is associated with exactly one project:
```
@cloud-run-bot describe
```
Shows resources in simplified format: `[SVC/JOB] resource-name` (no project ID needed)

#### Multi-Project Channel (Manual Selection)
When a channel is associated with multiple projects:
```
@cloud-run-bot describe
```
Shows resources in format: `[project-id] [SVC/JOB] resource-name` (project selection required)

#### Unconfigured Channel
When a channel has no project association:
```
@cloud-run-bot describe
```
Shows resources from all configured projects with full project information.

### Project-Specific Resource Format

Selected resources are stored in format: `project:type:name`
- `project1:service:web-service`
- `project2:job:batch-job`

### Commands Work Across Projects

All existing commands work seamlessly across projects:
- `describe` - Shows resources from channel-associated projects or all projects
- `metrics` - Displays metrics for services across projects
- `set` - Sets current resource from channel-associated projects or all projects

### Channel Configuration Benefits

- **Reduced Cognitive Load**: Users don't need to specify projects in channels dedicated to specific projects
- **Context Awareness**: Bot behavior adapts to the channel context
- **Flexibility**: Supports both single-project and multi-project channels

## Monitoring and Troubleshooting

### Logs to Monitor

```bash
# Check bot logs
gcloud logs read "resource.type=cloud_run_revision" \
    --project=BOT_PROJECT_ID \
    --filter="resource.labels.service_name=cloud-run-slack-bot"

# Check audit log delivery
gcloud logs read "resource.type=pubsub_subscription" \
    --project=MONITORED_PROJECT_ID \
    --filter="resource.labels.subscription_name=cloud-run-audit-subscription"
```

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