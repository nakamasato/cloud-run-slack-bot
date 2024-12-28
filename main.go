package main

import (
	"context"
	"log"
	"os"

	"github.com/nakamasato/cloud-run-slack-bot/pkg/cloudrun"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/cloudrunslackbot"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/logging"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/monitoring"
	slackinternal "github.com/nakamasato/cloud-run-slack-bot/pkg/slack"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/tracing"
	"github.com/slack-go/slack"
	"go.uber.org/zap"
)

func main() {
	ctx := context.Background()

	logger, err := logging.New(ctx)
	if err != nil {
		log.Fatalf("Failed to create logging client: %v", err)
	}
	defer func() {
		if err := logger.Sync(); err != nil {
			log.Fatalf("Error syncing logger: %v", err)
		}
	}()

	// Setup metrics, tracing, and context propagation
	shutdown, err := tracing.SetupOpenTelemetry(ctx)
	if err != nil {
		logger.Fatal("error setting up OpenTelemetry", zap.Error(err))
		os.Exit(1)
	}
	defer func(ctx context.Context) {
		if err := shutdown(ctx); err != nil {
			logger.Fatal("error shutting down OpenTelemetry", zap.Error(err))
		}
	}(ctx)

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
	handler := slackinternal.NewSlackEventHandler(sClient, rClient, mClient, slackinternal.WithLogger(logger), slackinternal.WithTmpDir(os.Getenv("TMP_DIR")))

	signingSecret := os.Getenv("SLACK_SIGNING_SECRET")
	if signingSecret == "" && os.Getenv("SLACK_APP_MODE") != "socket" {
		logger.Fatal("SLACK_SIGNING_SECRET env var is required for HTTP mode")
	}

	svc := cloudrunslackbot.NewCloudRunSlackBotService(
		sClient,
		os.Getenv("SLACK_CHANNEL"),
		os.Getenv("SLACK_APP_MODE"),
		handler,
		signingSecret,
	)
	svc.Run()
}