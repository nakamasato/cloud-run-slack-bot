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
	"go.uber.org/zap"
)

func main() {
	ctx := context.Background()

	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := logger.Sync(); err != nil {
			log.Printf("Error syncing logger: %v", err)
		}
	}()

	project := os.Getenv("PROJECT")
	if project == "" {
		logger.Fatal("PROJECT env var is required")
	}
	region := os.Getenv("REGION")
	if region == "" {
		logger.Fatal("REGION env var is required")
	}

	mClient, err := monitoring.NewMonitoringClient(project, monitoring.WithLogger(logger))
	if err != nil {
		logger.Fatal("failed to initialize monitoring client", zap.Error(err))
	}
	defer mClient.Close()

	rClient, err := cloudrun.NewClient(ctx, cloudrun.WithLogger(logger), cloudrun.WithProject(project), cloudrun.WithRegion(region))
	if err != nil {
		logger.Fatal("Failed to create run service", zap.Error(err))
	}

	ops := []slack.Option{}

	if appToken := os.Getenv("SLACK_APP_TOKEN"); appToken != "" {
		ops = append(ops, slack.OptionAppLevelToken(appToken))
	}

	sClient := slack.New(os.Getenv("SLACK_BOT_TOKEN"), ops...)
	handler := slackinternal.NewSlackEventHandler(sClient, rClient, mClient, logger, os.Getenv("TMP_DIR"))
	svc := cloudrunslackbot.NewCloudRunSlackBotService(
		sClient,
		os.Getenv("SLACK_CHANNEL"),
		os.Getenv("SLACK_APP_MODE"),
		handler,
	)
	svc.Run()
}
