# Cloud Run Slack Bot

This is a simple bot that sends a message to a Slack channel when a new revision is deployed to a Cloud Run service.

1. trigger (github, cloud run deploy event, slo alert, slack event subscription, etc.)
1. check status (logs, traces, dependencies, etc.)
1. send notification (slack message, email, etc)

## Deploy

```
PROJECT=your-project
REGION=asia-northeast1
```

```
echo -n "xoxb-xxxx" | gcloud secrets create slack-bot-token --replication-policy automatic --project "$PROJECT" --data-file=-
```

```
gcloud iam service-accounts create go-cloud-run-alert-bot --project $PROJECT
gcloud secrets add-iam-policy-binding slack-bot-token \
    --member="serviceAccount:go-cloud-run-alert-bot@${PROJECT}.iam.gserviceaccount.com" \
    --role="roles/secretmanager.secretAccessor" --project ${PROJECT}
gcloud builds submit . --pack "image=asia-northeast1-docker.pkg.dev/$PROJECT/cloud-run-source-deploy/go-cloud-run-alert-bot" --project ${PROJECT}
gcloud run deploy go-cloud-run-alert-bot --set-secrets SLACK_BOT_TOKEN=slack-bot-token:latest \
    --image asia-northeast1-docker.pkg.dev/$PROJECT/cloud-run-source-deploy/go-cloud-run-alert-bot \
    --service-account go-cloud-run-alert-bot@${PROJECT}.iam.gserviceaccount.com \
    --project "$PROJECT" --region "$REGION"
```

## References
1. https://pkg.go.dev/google.golang.org/api@v0.172.0/run/v2
1. https://qiita.com/frozenbonito/items/cf75dadce12ef9a048e9
1. https://medium.com/google-cloud/querying-metrics-from-google-cloud-monitoring-in-golang-2631ee3d33c1
>>>>>>> bc7f8fb (feat: create slack bot)
