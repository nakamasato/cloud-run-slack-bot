# Cloud Run Slack Bot

This is a simple Slack bot running on Cloud Run with which you can interact with Cloud Run services.

<img src="docs/preview.gif" alt="preview" width="400"/>

## Architecture

![](docs/diagram.drawio.svg)

## Features

1. Interact with Cloud Run service on Slack.
    1. Get metrics of Cloud Run service.
    1. Describe Cloud Run service.
1. Receive notification for Cloud Run audit logs on Slack.

## Cloud Run

### Roles

1. `roles/run.viewer`: To get information of Cloud Run services
1. `roles/monitoring.viewer`: To get metrics of Cloud Run services

### Environment Variables

1. `PROJECT`: GCP Project ID to monitor
1. `REGION`: GCP Region to monitor
1. `SLACK_BOT_TOKEN`: Slack Bot Token
1. `SLACK_SIGNING_SECRET`: Slack bot signing secret
1. `SLACK_APP_TOKEN` (optional): Slack oauth token (required for `SLACK_APP_MODE=socket`)
1. `SLACK_APP_MODE`: Slack App Mode (`http` or `socket`)
1. `SLACK_CHANNEL` (optional): Slack Channel ID to receive notification for Cloud Run audit logs
1. `TMP_DIR` (optional): Temporary directory for storing images (default: `/tmp`)

### Deploy

```
PROJECT=your-project
REGION=asia-northeast1
```

### Initial Setup

```shell
echo -n "xoxb-xxxx" | gcloud secrets create slack-bot-token --replication-policy automatic --project "$PROJECT" --data-file=-
echo -n "your-signing-secret" | gcloud secrets create slack-signing-secret --replication-policy automatic --project "$PROJECT" --data-file=-
gcloud iam service-accounts create cloud-run-slack-bot --project $PROJECT
# allow app to access the secret
gcloud secrets add-iam-policy-binding slack-bot-token \
    --member="serviceAccount:cloud-run-slack-bot@${PROJECT}.iam.gserviceaccount.com" \
    --role="roles/secretmanager.secretAccessor" --project ${PROJECT}
gcloud secrets add-iam-policy-binding slack-signing-secret \
    --member="serviceAccount:cloud-run-slack-bot@${PROJECT}.iam.gserviceaccount.com" \
    --role="roles/secretmanager.secretAccessor" --project ${PROJECT}
# allow app to get information about Cloud Run services
gcloud projects add-iam-policy-binding $PROJECT \
    --member=serviceAccount:cloud-run-slack-bot@${PROJECT}.iam.gserviceaccount.com --role=roles/run.viewer
# allow app to get metrics of Cloud Run services
gcloud projects add-iam-policy-binding $PROJECT \
    --member=serviceAccount:cloud-run-slack-bot@${PROJECT}.iam.gserviceaccount.com --role=roles/monitoring.viewer
```

Deploy to Cloud Run

```
gcloud run deploy cloud-run-slack-bot \
    --set-secrets "SLACK_BOT_TOKEN=slack-bot-token:latest,SLACK_SIGNING_SECRET=slack-signing-secret:latest" \
    --set-env-vars "PROJECT=$PROJECT,REGION=$REGION,SLACK_APP_MODE=http,TMP_DIR=/tmp" \
    --image nakamasato/cloud-run-slack-bot:0.0.2 \
    --service-account cloud-run-slack-bot@${PROJECT}.iam.gserviceaccount.com \
    --project "$PROJECT" --region "$REGION"
```

## Slack App

1. Create a new Slack App
    - [https://api.slack.com/apps](https://api.slack.com/apps)
1. Add the following scopes:
    - [mention:read](https://api.slack.com/scopes/app_mentions:read)
    - [chat:write](https://api.slack.com/scopes/chat:write)
    - [files:write](https://api.slack.com/scopes/files:write)
1. Install the app to your workspace
1. Event Subscriptions
    - Request URL: `https://your-cloud-run-url/slack/events`
    - Subscribe to bot events: `app_mention`
1. Interactivity & Shortcuts
    - Request URL: `https://your-cloud-run-url/slack/interaction`

## Slack Channel Settings

1. Remove preview for console.cloud.google.com

<img src="docs/slack-channel-preview.png" alt="preview" width="400"/>


## More

1. [Terraform](docs/terraform.md)
1. [Auditing Notification](docs/auditing.md)
