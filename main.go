package main

import (
	"context"
	"log"
	"os"

	monitoringineternal "github.com/nakamasato/go-cloud-run-alert-bot/pkg/monitoring"
	slackinternal "github.com/nakamasato/go-cloud-run-alert-bot/pkg/slack"
	"google.golang.org/api/run/v2"
)

func main() {
	var err error
	mClient, err := monitoringineternal.NewMonitoringClient()
	if err != nil {
		log.Fatal(err)
	}
	defer mClient.Close()

	ctx := context.Background()
	runService, err := run.NewService(ctx)
	plSvc := run.NewProjectsLocationsServicesService(runService)
	if err != nil {
		log.Fatalf("Failed to create run service: %v", err)
	}

	var svc slackinternal.SlackService
	if os.Getenv("SLACK_APP_MODE") == "events" {
		svc, err = slackinternal.NewSlackEventService(
			os.Getenv("SLACK_BOT_TOKEN"),
			plSvc,
			mClient,
		)
	} else if os.Getenv("SLACK_APP_MODE") == "socket" {
		svc, err = slackinternal.NewSlackSocketService(
			os.Getenv("SLACK_BOT_TOKEN"),
			os.Getenv("SLACK_APP_TOKEN"),
			plSvc,
			mClient,
		)
	}
	if err != nil {
		log.Fatal(err)
	}
	svc.Run()
}
