package main

import (
	"context"
	"log"
	"time"

	"github.com/nakamasato/cloud-run-slack-bot/pkg/adk"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/cloudrun"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/cloudrunslackbot"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/config"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/debug"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/logging"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/monitoring"
	slackinternal "github.com/nakamasato/cloud-run-slack-bot/pkg/slack"
	"github.com/slack-go/slack"
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

	// Log configuration
	cfg.LogConfiguration()

	ctx := context.Background()


	// Initialize clients for all projects
	rClients := make(map[string]*cloudrun.Client)
	mClients := make(map[string]*monitoring.Client)

	for _, project := range cfg.Projects {
		// Create monitoring client for this project
		mClient, err := monitoring.NewMonitoringClient(project.ID)
		if err != nil {
			log.Fatalf("Failed to create monitoring client for project %s: %v", project.ID, err)
		}
		mClients[project.ID] = mClient

		// Create Cloud Run client for this project
		rClient, err := cloudrun.NewClient(ctx, project.ID, project.Region)
		if err != nil {
			log.Fatalf("Failed to create Cloud Run client for project %s: %v", project.ID, err)
		}
		rClients[project.ID] = rClient
	}

	// Initialize debug feature if enabled
	var lClients map[string]*logging.Client
	var debugger *debug.Debugger

	if cfg.DebugEnabled {
		log.Println("Debug feature enabled, initializing logging clients and ADK agent...")

		// Initialize logging clients per project
		lClients = make(map[string]*logging.Client)
		for _, project := range cfg.Projects {
			lClient, err := logging.NewLoggingClient(ctx, project.ID)
			if err != nil {
				log.Fatalf("Failed to create logging client for project %s: %v", project.ID, err)
			}
			lClients[project.ID] = lClient
		}

		// Initialize ADK agent (singleton)
		adkAgent, err := adk.NewAgent(ctx, adk.Config{
			Project:   cfg.GCPProjectID,
			Location:  cfg.VertexLocation,
			ModelName: cfg.ModelName,
		})
		if err != nil {
			log.Fatalf("Failed to create ADK agent: %v", err)
		}

		// Initialize debugger
		debugger = debug.NewDebugger(lClients, adkAgent, debug.Config{
			LookbackDuration: time.Duration(cfg.DebugTimeWindow) * time.Minute,
		})
	}

	// Ensure proper cleanup
	defer func() {
		for projectID, mClient := range mClients {
			if err := mClient.Close(); err != nil {
				log.Printf("Failed to close monitoring client for project %s: %v", projectID, err)
			}
		}
		for projectID, rClient := range rClients {
			if err := rClient.Close(); err != nil {
				log.Printf("Failed to close Cloud Run client for project %s: %v", projectID, err)
			}
		}
		for projectID, lClient := range lClients {
			if err := lClient.Close(); err != nil {
				log.Printf("Failed to close logging client for project %s: %v", projectID, err)
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
	handler := slackinternal.NewMultiProjectSlackEventHandler(sClient, rClients, mClients, debugger, cfg.TmpDir, cfg)

	// Create service with multi-project support
	svc := cloudrunslackbot.NewMultiProjectCloudRunSlackBotService(
		sClient,
		cfg,
		handler,
	)
	svc.Run()
}
