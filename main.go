package main

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/nakamasato/cloud-run-slack-bot/pkg/cloudrun"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/cloudrunslackbot"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/logger"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/monitoring"
	slackinternal "github.com/nakamasato/cloud-run-slack-bot/pkg/slack"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/trace"
	"github.com/slack-go/slack"
	"go.uber.org/zap"
)

func main() {
	// Create a root context
	ctx := context.Background()

	// Initialize logger
	l, err := logger.NewLogger()
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer func() {
		if err := l.Sync(); err != nil {
			log.Printf("Failed to sync logger: %v", err)
		}
	}()

	// Create a context with logger
	ctx = logger.WithContext(ctx, l)

	// Get required environment variables
	project := os.Getenv("PROJECT")
	if project == "" {
		l.Fatal("PROJECT env var is required")
	}
	region := os.Getenv("REGION")
	if region == "" {
		l.Fatal("REGION env var is required")
	}

	// Set service name environment variable for logger
	serviceName := "cloud-run-slack-bot"
	if err := os.Setenv("SERVICE_NAME", serviceName); err != nil {
		l.Warn("Failed to set SERVICE_NAME environment variable", zap.Error(err))
	}

	// Initialize OpenTelemetry
	shutdown, err := trace.Initialize(ctx, serviceName)
	if err != nil {
		l.Fatal("Failed to initialize OpenTelemetry", zap.Error(err))
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			l.Error("Failed to shutdown OpenTelemetry", zap.Error(err))
		}
	}()

	// Start a root span for the main function
	ctx, span := trace.GetTracer().Start(ctx, "main")
	defer span.End()

	// Extract trace ID for logging
	traceID := trace.ExtractTraceID(ctx)

	// Update logger with trace information and store in context
	l = l.WithTraceID(traceID)
	ctx = logger.WithContext(ctx, l)

	// Get environment
	env := os.Getenv("ENV")
	if env == "" {
		env = "dev" // Default to dev if not specified
	}

	l.Info("Starting cloud-run-slack-bot",
		zap.String("project", project),
		zap.String("region", region),
		zap.String("environment", env),
		zap.String("trace_id", traceID))

	// Initialize monitoring client
	mClient, err := monitoring.NewMonitoringClient(project)
	if err != nil {
		l.Fatal("Failed to create monitoring client", zap.Error(err))
	}
	defer func() {
		if err := mClient.Close(); err != nil {
			l.Error("Failed to close monitoring client", zap.Error(err))
		}
	}()

	// Initialize Cloud Run client
	rClient, err := cloudrun.NewClient(ctx, project, region)
	if err != nil {
		l.Fatal("Failed to create run service", zap.Error(err))
	}

	// Initialize Slack client
	ops := []slack.Option{}
	if appToken := os.Getenv("SLACK_APP_TOKEN"); appToken != "" {
		ops = append(ops, slack.OptionAppLevelToken(appToken))
	}
	sClient := slack.New(os.Getenv("SLACK_BOT_TOKEN"), ops...)

	// Create Slack event handler
	handler := slackinternal.NewSlackEventHandler(sClient, rClient, mClient, os.Getenv("TMP_DIR"))

	// Check signing secret for HTTP mode
	signingSecret := os.Getenv("SLACK_SIGNING_SECRET")
	if signingSecret == "" && os.Getenv("SLACK_APP_MODE") != "socket" {
		l.Fatal("SLACK_SIGNING_SECRET env var is required for HTTP mode")
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
	l.Info("Channel configuration",
		zap.Any("service_channel_mapping", serviceChannelMapping),
		zap.String("default_channel", defaultChannel))

	// Create and run the service
	svc := cloudrunslackbot.NewCloudRunSlackBotService(
		sClient,
		serviceChannelMapping,
		defaultChannel,
		os.Getenv("SLACK_APP_MODE"),
		handler,
		signingSecret,
	)

	l.Info("Starting service")
	svc.Run()
}
