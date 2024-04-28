package main

import (
	"context"
	"log"
	"os"

	"github.com/nakamasato/cloud-run-slack-bot/pkg/cloudrun"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/cloudrunslackbot"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/monitoring"
	slackinternal "github.com/nakamasato/cloud-run-slack-bot/pkg/slack"
	"github.com/slack-go/slack"
)

func main() {
	var err error
	project := os.Getenv("PROJECT")
	if project == "" {
		log.Fatal("PROJECT env var is required")
	}
	region := os.Getenv("REGION")
	if region == "" {
		log.Fatal("REGION env var is required")
	}

	mClient, err := monitoring.NewMonitoringClient(project)
	if err != nil {
		log.Fatal(err)
	}
	defer mClient.Close()

	ctx := context.Background()
	rClient, err := cloudrun.NewClient(ctx, project, region)
	if err != nil {
		log.Fatalf("Failed to create run service: %v", err)
	}

	ops := []slack.Option{}
	if appToken := os.Getenv("SLACK_APP_TOKEN"); appToken != "" {
		ops = append(ops, slack.OptionAppLevelToken(appToken))
	}
	sClient := slack.New(os.Getenv("SLACK_BOT_TOKEN"), ops...)
	handler := slackinternal.NewSlackEventHandler(sClient, rClient, mClient, os.Getenv("TMP_DIR"))
	svc, err := cloudrunslackbot.NewCloudRunSlackBotService(
		sClient,
		os.Getenv("SLACK_APP_MODE"),
		handler,
	)
	if err != nil {
		log.Fatal(err)
	}
	svc.Run()
}
