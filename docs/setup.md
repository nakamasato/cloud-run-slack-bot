# Setup Guide

This guide provides step-by-step instructions for setting up the Cloud Run Slack Bot.

## Prerequisites

- GCP Project(s) to monitor
- Slack Workspace with admin access
- `gcloud` CLI installed and configured

## Cloud Run Setup

### Required Roles

Grant the following IAM roles to the service account that will run the bot:

1. `roles/run.viewer`: To get information about Cloud Run services
2. `roles/monitoring.viewer`: To get metrics of Cloud Run services
3. `roles/logging.viewer`: To read Cloud Logging entries (required for debug feature). Grant this role in each target project when using multi-project configuration.
4. `roles/aiplatform.user`: To access Vertex AI Gemini API (required for debug feature). Grant this role on the project specified in `GCP_PROJECT_ID`. This role includes the `aiplatform.endpoints.predict` permission.

### Environment Variables

#### Configuration

Configure your projects using the `PROJECTS_CONFIG` environment variable:

```json
[
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
    "defaultChannel": "project2-alerts"
  }
]
```

For detailed configuration options, see the [Multi-Project Setup Guide](multi-project-setup.md).

#### Common Configuration

1. `SLACK_BOT_TOKEN`: Slack Bot Token
2. `SLACK_SIGNING_SECRET`: Slack bot signing secret
3. `SLACK_APP_TOKEN` (optional): Slack oauth token (required for `SLACK_APP_MODE=socket`)
4. `SLACK_APP_MODE`: Slack App Mode (`http` or `socket`)
5. `SLACK_CHANNEL`: Default Slack Channel ID to receive notifications (used as fallback for all configurations)
6. `TMP_DIR` (optional): Temporary directory for storing images (default: `/tmp`)

#### Debug Feature Configuration (Optional)

The debug feature uses Gemini AI via Vertex AI to analyze error logs. To enable:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DEBUG_ENABLED` | No | `false` | Set to `true` to enable the debug feature |
| `GCP_PROJECT_ID` | When enabled | - | GCP project ID for Vertex AI API access |
| `VERTEX_LOCATION` | When enabled | - | GCP region for Vertex AI (e.g., `us-central1`) |
| `MODEL_NAME` | No | `gemini-2.5-flash-lite` | Gemini model to use for analysis |
| `DEBUG_TIME_WINDOW` | No | `30` | Time window for error analysis (in minutes) |

**Required APIs** (must be enabled on the Vertex AI project):
- Vertex AI API (`aiplatform.googleapis.com`)

> **Note**: When using `PROJECTS_CONFIG`, the bot automatically generates channel-to-project mappings for intelligent project detection.

### Initial Setup

Set your project and region:

```bash
PROJECT=your-project
REGION=asia-northeast1
```

Create secrets for Slack credentials:

```bash
echo -n "xoxb-xxxx" | gcloud secrets create slack-bot-token --replication-policy automatic --project "$PROJECT" --data-file=-
echo -n "your-signing-secret" | gcloud secrets create slack-signing-secret --replication-policy automatic --project "$PROJECT" --data-file=-
```

Create a service account:

```bash
gcloud iam service-accounts create cloud-run-slack-bot --project $PROJECT
```

Grant the service account access to secrets:

```bash
gcloud secrets add-iam-policy-binding slack-bot-token \
    --member="serviceAccount:cloud-run-slack-bot@${PROJECT}.iam.gserviceaccount.com" \
    --role="roles/secretmanager.secretAccessor" --project ${PROJECT}

gcloud secrets add-iam-policy-binding slack-signing-secret \
    --member="serviceAccount:cloud-run-slack-bot@${PROJECT}.iam.gserviceaccount.com" \
    --role="roles/secretmanager.secretAccessor" --project ${PROJECT}
```

Grant the service account access to Cloud Run and Monitoring:

```bash
# Allow app to get information about Cloud Run services
gcloud projects add-iam-policy-binding $PROJECT \
    --member=serviceAccount:cloud-run-slack-bot@${PROJECT}.iam.gserviceaccount.com --role=roles/run.viewer

# Allow app to get metrics of Cloud Run services
gcloud projects add-iam-policy-binding $PROJECT \
    --member=serviceAccount:cloud-run-slack-bot@${PROJECT}.iam.gserviceaccount.com --role=roles/monitoring.viewer
```

### Deployment

Create your `PROJECTS_CONFIG` environment variable:

```bash
PROJECTS_CONFIG='[
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

Deploy to Cloud Run:

```bash
gcloud run deploy cloud-run-slack-bot \
    --set-secrets "SLACK_BOT_TOKEN=slack-bot-token:latest,SLACK_SIGNING_SECRET=slack-signing-secret:latest" \
    --set-env-vars "PROJECTS_CONFIG=$PROJECTS_CONFIG,SLACK_APP_MODE=http,TMP_DIR=/tmp,SLACK_CHANNEL=general" \
    --image nakamasato/cloud-run-slack-bot:0.5.1 \
    --service-account cloud-run-slack-bot@${PROJECT}.iam.gserviceaccount.com \
    --project "$PROJECT" --region "$REGION"
```

> **Note**: For comprehensive multi-project setup including IAM permissions and Pub/Sub configuration, see the [Multi-Project Setup Guide](multi-project-setup.md).

## Slack App Setup

### Create and Configure Slack App

1. Create a new Slack App at [https://api.slack.com/apps](https://api.slack.com/apps)

2. Add the following OAuth scopes under **OAuth & Permissions**:
   - [app_mentions:read](https://api.slack.com/scopes/app_mentions:read)
   - [chat:write](https://api.slack.com/scopes/chat:write)
   - [files:write](https://api.slack.com/scopes/files:write)
   - [connections:write](https://api.slack.com/scopes/connections:write) (required only when using Socket Mode with `SLACK_APP_MODE=socket`)

3. Install the app to your workspace

4. Configure **Event Subscriptions**:
   - Enable Events
   - Request URL: `https://your-cloud-run-url/slack/events`
   - Subscribe to bot events: `app_mention`
   - Save Changes

5. Configure **Interactivity & Shortcuts**:
   - Enable Interactivity
   - Request URL: `https://your-cloud-run-url/slack/interaction`
   - Save Changes

### Slack Channel Settings

To improve the experience in Slack channels, disable link previews for Google Cloud Console URLs:

1. Go to your Slack channel settings
2. Navigate to **Preferences** > **Link Previews**
3. Add `console.cloud.google.com` to the list of domains to exclude from previews

<img src="slack-channel-preview.png" alt="Slack channel preview settings" width="400"/>

## Advanced Setup

- [Multi-Project Setup Guide](multi-project-setup.md)
- [Terraform Deployment](terraform.md)
- [Auditing Notification](auditing.md)
