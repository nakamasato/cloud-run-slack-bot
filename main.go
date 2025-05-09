package main

import (
	"context"
	"log"
	"os"
	"strings"

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
	defer func() {
		if err := mClient.Close(); err != nil {
			log.Printf("Failed to close monitoring client: %v", err)
		}
	}()

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
	signingSecret := os.Getenv("SLACK_SIGNING_SECRET")
	if signingSecret == "" && os.Getenv("SLACK_APP_MODE") != "socket" {
		log.Fatal("SLACK_SIGNING_SECRET env var is required for HTTP mode")
	}

	// Parse service-channel mapping from environment variable
	// Format: service1:channel1,service2:channel2
	serviceChannelMapping := make(map[string]string)
	serviceChannelStr := os.Getenv("SERVICE_CHANNEL_MAPPING")
	if serviceChannelStr != "" {
		pairs := strings.Split(serviceChannelStr, ",")
		for _, pair := range pairs {
			parts := strings.Split(pair, ":")
			if len(parts) == 2 {
				serviceChannelMapping[parts[0]] = parts[1]
			}
		}
	}

	defaultChannel := os.Getenv("SLACK_CHANNEL")

	svc := cloudrunslackbot.NewCloudRunSlackBotService(
		sClient,
		serviceChannelMapping,
		defaultChannel,
		os.Getenv("SLACK_APP_MODE"),
		handler,
		signingSecret,
	)
	svc.Run()
}
