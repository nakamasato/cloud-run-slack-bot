# Cloud Run Slack Bot

This is a simple bot that sends a message to a Slack channel when a new revision is deployed to a Cloud Run service.

1. trigger (github, cloud run deploy event, slo alert, slack event subscription, etc.)
1. check status (logs, traces, dependencies, etc.)
1. send notification (slack message, email, etc)

## Features

1. Interact with Cloud Run service on Slack.
    1. Get metrics of the service. (`roles/monitoring.viewer` is required.)

## Environment Variables

1. `SLACK_BOT_TOKEN`: Slack Bot Token
1. `SLACK_OAUTH_TOKEN`: Slack oauth token
1. `SLACK_APP_MODE`: Slack App Mode (`events` or `socket`)


## Local Run

```
go run main.go
```

TODO:
```
curl -H 'Content-Type: application/json' -X POST -d '{"type": "event_callback", "event": {"type": "app_mention", "user": "xx", "reaction": "memo", "item_user": "xx", "item": {"type": "message", "channel": "CHANNEL", "ts": "1701919197.246629"}, "event_ts": "1704502151.000000"}}' localhost:8080/slack/events
```

## Deploy

```
PROJECT=your-project
REGION=asia-northeast1
```

### Initial Setup

```shell
echo -n "xoxb-xxxx" | gcloud secrets create slack-bot-token --replication-policy automatic --project "$PROJECT" --data-file=-
gcloud iam service-accounts create go-cloud-run-alert-bot --project $PROJECT
# allow app to access the secret
gcloud secrets add-iam-policy-binding slack-bot-token \
    --member="serviceAccount:go-cloud-run-alert-bot@${PROJECT}.iam.gserviceaccount.com" \
    --role="roles/secretmanager.secretAccessor" --project ${PROJECT}
# allow app to get information about Cloud Run services
gcloud projects add-iam-policy-binding $PROJECT \
    --member=serviceAccount:go-cloud-run-alert-bot@${PROJECT}.iam.gserviceaccount.com --role=roles/run.viewer
# allow app to get metrics of Cloud Run services
gcloud projects add-iam-policy-binding $PROJECT \
    --member=serviceAccount:go-cloud-run-alert-bot@${PROJECT}.iam.gserviceaccount.com --role=roles/monitoring.viewer
```

Build a container image

```
gcloud builds submit . --pack "image=$REGION-docker.pkg.dev/$PROJECT/cloud-run-source-deploy/go-cloud-run-alert-bot" --project ${PROJECT}
```

Deploy the image to Cloud Run

```
gcloud run deploy go-cloud-run-alert-bot \
    --set-secrets SLACK_BOT_TOKEN=slack-bot-token:latest \
    --set-env-vars "PROJECT=$PROJECT,REGION=$REGION,SLACK_APP_MODE=events" \
    --image $REGION-docker.pkg.dev/$PROJECT/cloud-run-source-deploy/go-cloud-run-alert-bot \
    --service-account go-cloud-run-alert-bot@${PROJECT}.iam.gserviceaccount.com \
    --project "$PROJECT" --region "$REGION"
```

### Deploy new version

Build a container image

```
gcloud builds submit . --pack "image=$REGION-docker.pkg.dev/$PROJECT/cloud-run-source-deploy/go-cloud-run-alert-bot" --project ${PROJECT}
```

Deploy the image to Cloud Run

```
gcloud run deploy go-cloud-run-alert-bot --image $REGION-docker.pkg.dev/$PROJECT/cloud-run-source-deploy/go-cloud-run-alert-bot --project "$PROJECT" --region "$REGION"
```

## References
1. https://pkg.go.dev/google.golang.org/api/run/v2
1. https://qiita.com/frozenbonito/items/cf75dadce12ef9a048e9
1. https://qiita.com/frozenbonito/items/1df9bb685e6173160991
1. https://medium.com/google-cloud/querying-metrics-from-google-cloud-monitoring-in-golang-2631ee3d33c1
>>>>>>> bc7f8fb (feat: create slack bot)
