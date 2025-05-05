# Development

## Test on local

> [!NOTE]
> It's convenient to use `socketmode` for testing on local.


```
PROJECT=xxxx REGION=xxx SLACK_APP_MODE=socket SLACK_BOT_TOKEN=xxx SLACK_APP_TOKEN=xxx go run main.go
```

<details><summary></summary>

To check `http` mode (TODO):

```
curl -H 'Content-Type: application/json' -X POST -d '{"type": "event_callback", "event": {"type": "app_mention", "user": "xx", "reaction": "memo", "item_user": "xx", "item": {"type": "message", "channel": "CHANNEL", "ts": "1701919197.246629"}, "event_ts": "1704502151.000000"}}' localhost:8080/slack/events
```

</details>


## Deploy new version

Build a container image

```
gcloud builds submit . --pack "image=$REGION-docker.pkg.dev/$PROJECT/cloud-run-source-deploy/cloud-run-slack-bot" --project ${PROJECT}
```

Deploy the image to Cloud Run

```
gcloud run deploy cloud-run-slack-bot --image $REGION-docker.pkg.dev/$PROJECT/cloud-run-source-deploy/cloud-run-slack-bot --project "$PROJECT" --region "$REGION"
```
