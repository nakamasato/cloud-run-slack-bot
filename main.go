package main

import (
	"context"
	"log"
	"os"

	"github.com/nakamasato/cloud-run-slack-bot/pkg/cloudrun"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/monitoring"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/slack"
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

	var svc slack.SlackService
	if os.Getenv("SLACK_APP_MODE") == "events" {
		svc, err = slack.NewSlackEventService(
			os.Getenv("SLACK_BOT_TOKEN"),
			rClient,
			mClient,
		)
	} else if os.Getenv("SLACK_APP_MODE") == "socket" {
		svc, err = slack.NewSlackSocketService(
			os.Getenv("SLACK_BOT_TOKEN"),
			os.Getenv("SLACK_APP_TOKEN"),
			rClient,
			mClient,
		)
	}
	if err != nil {
		log.Fatal(err)
	}
	svc.Run()
}
