package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/nakamasato/cloud-run-slack-bot/pkg/adk"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/cloudrun"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/cloudrunslackbot"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/config"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/debug"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/logger"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/logging"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/monitoring"
	slackinternal "github.com/nakamasato/cloud-run-slack-bot/pkg/slack"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/trace"
	"github.com/slack-go/slack"
	"go.uber.org/zap"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Configuration validation failed: %v", err)
	}

	ctx := context.Background()

	// Get GCP project ID for Cloud Trace and other GCP services
	projectID := os.Getenv("GCP_PROJECT_ID")
	if projectID == "" && len(cfg.Projects) > 0 {
		// Fallback to first project ID if GCP_PROJECT_ID env var not set
		projectID = cfg.Projects[0].ID
	}

	// Initialize structured logger
	zapLogger, err := logger.NewLogger(projectID)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer func() {
		if err := zapLogger.Sync(); err != nil {
			zapLogger.Error("Failed to sync logger", zap.Error(err))
		}
	}()
	zapLogger.Info("Structured logger initialized successfully")

	// Log configuration
	cfg.LogConfiguration(zapLogger.Logger)

	// Initialize tracing if TRACING_ENABLED is set
	var traceProvider *trace.Provider
	tracingEnabled := os.Getenv("TRACING_ENABLED") == "true"
	if tracingEnabled && projectID != "" {
		samplingRate := 1.0 // Default to always sample; adjust for production
		traceProvider, err = trace.NewProvider(ctx, trace.Config{
			ProjectID:    projectID,
			ServiceName:  "cloud-run-slack-bot",
			SamplingRate: samplingRate,
		}, zapLogger.Logger)
		if err != nil {
			zapLogger.Warn("Failed to initialize tracing", zap.Error(err))
		} else {
			zapLogger.Info("Tracing initialized successfully")
			defer func() {
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := traceProvider.Shutdown(shutdownCtx); err != nil {
					zapLogger.Error("Failed to shutdown trace provider", zap.Error(err))
				}
			}()
		}
	} else if !tracingEnabled {
		zapLogger.Info("Tracing disabled (TRACING_ENABLED not set to true)")
	} else {
		zapLogger.Warn("GCP_PROJECT_ID not set, tracing disabled")
	}

	// Initialize clients for all projects
	rClients := make(map[string]*cloudrun.Client)
	mClients := make(map[string]*monitoring.Client)

	for _, project := range cfg.Projects {
		// Create monitoring client for this project
		mClient, err := monitoring.NewMonitoringClient(project.ID, zapLogger.Logger)
		if err != nil {
			zapLogger.Fatal("Failed to create monitoring client for project", zap.String("projectID", project.ID), zap.Error(err))
		}
		mClients[project.ID] = mClient

		// Create Cloud Run client for this project
		rClient, err := cloudrun.NewClient(ctx, project.ID, project.Region, zapLogger.Logger)
		if err != nil {
			zapLogger.Fatal("Failed to create Cloud Run client for project", zap.String("projectID", project.ID), zap.Error(err))
		}
		rClients[project.ID] = rClient
	}

	// Initialize debug feature if enabled
	var lClients map[string]*logging.Client
	var debugger *debug.Debugger

	if cfg.DebugEnabled {
		zapLogger.Info("Debug feature enabled, initializing logging clients and ADK agent")

		// Initialize logging clients per project
		lClients = make(map[string]*logging.Client)
		for _, project := range cfg.Projects {
			lClient, err := logging.NewLoggingClient(ctx, project.ID, zapLogger.Logger)
			if err != nil {
				zapLogger.Fatal("Failed to create logging client for project", zap.String("projectID", project.ID), zap.Error(err))
			}
			lClients[project.ID] = lClient
		}

		// Initialize ADK agent (singleton)
		adkAgent, err := adk.NewDebugAgent(ctx, adk.Config{
			Project:   cfg.GCPProjectID,
			Location:  cfg.VertexLocation,
			ModelName: cfg.ModelName,
		}, zapLogger.Logger)
		if err != nil {
			zapLogger.Fatal("Failed to create ADK agent", zap.Error(err))
		}

		// Initialize debugger
		debugger = debug.NewDebugger(lClients, adkAgent, debug.Config{
			LookbackDuration: time.Duration(cfg.DebugTimeWindow) * time.Minute,
		}, zapLogger.Logger)
	}

	// Ensure proper cleanup
	defer func() {
		for projectID, mClient := range mClients {
			if err := mClient.Close(); err != nil {
				zapLogger.Error("Failed to close monitoring client for project", zap.String("projectID", projectID), zap.Error(err))
			}
		}
		for projectID, rClient := range rClients {
			if err := rClient.Close(); err != nil {
				zapLogger.Error("Failed to close Cloud Run client for project", zap.String("projectID", projectID), zap.Error(err))
			}
		}
		for projectID, lClient := range lClients {
			if err := lClient.Close(); err != nil {
				zapLogger.Error("Failed to close logging client for project", zap.String("projectID", projectID), zap.Error(err))
			}
		}
	}()

	// Setup Slack client
	ops := []slack.Option{}
	if cfg.SlackAppToken != "" {
		ops = append(ops, slack.OptionAppLevelToken(cfg.SlackAppToken))
	}
	sClient := slack.New(cfg.SlackBotToken, ops...)

	// Create multi-project handler
	handler := slackinternal.NewMultiProjectSlackEventHandler(sClient, rClients, mClients, debugger, cfg.TmpDir, cfg, zapLogger.Logger)

	// Create service with multi-project support
	svc := cloudrunslackbot.NewMultiProjectCloudRunSlackBotService(
		sClient,
		cfg,
		handler,
		zapLogger.Logger,
	)
	svc.Run()
}
