# Cloud Run Slack Bot

This is a simple Slack bot running on Cloud Run with which you can interact with Cloud Run services.

![](docs/diagram.drawio.svg)

## Features

Interact with Cloud Run service on Slack.
1. Get metrics of Cloud Run service.
1. Describe Cloud Run service.

## Cloud Run

### Roles

1. `roles/run.viewer`: To get information of Cloud Run services
1. `roles/monitoring.viewer`: To get metrics of Cloud Run services

### Environment Variables

1. `PROJECT`: GCP Project ID to monitor
1. `REGION`: GCP Region to monitor
1. `SLACK_BOT_TOKEN`: Slack Bot Token
1. `SLACK_APP_TOKEN` (optional): Slack oauth token (required for `SLACK_APP_MODE=socket`)
1. `SLACK_APP_MODE`: Slack App Mode (`http` or `socket`)
1. `TMP_DIR` (optional): Temporary directory for storing images (default: `/tmp`)

### Deploy

```
PROJECT=your-project
REGION=asia-northeast1
```

### Initial Setup

```shell
echo -n "xoxb-xxxx" | gcloud secrets create slack-bot-token --replication-policy automatic --project "$PROJECT" --data-file=-
gcloud iam service-accounts create cloud-run-slack-bot --project $PROJECT
# allow app to access the secret
gcloud secrets add-iam-policy-binding slack-bot-token \
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
    --set-secrets SLACK_BOT_TOKEN=slack-bot-token:latest \
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
    - Request URL: `https://your-cloud-run-url/slack/interactivity`

## Local Run

```
go run main.go
```

TODO:
```
curl -H 'Content-Type: application/json' -X POST -d '{"type": "event_callback", "event": {"type": "app_mention", "user": "xx", "reaction": "memo", "item_user": "xx", "item": {"type": "message", "channel": "CHANNEL", "ts": "1701919197.246629"}, "event_ts": "1704502151.000000"}}' localhost:8080/slack/events
```

## Slack Channel Settings

1. Remove preview for console.cloud.google.com

![](docs/slack-channel-preview.png)

