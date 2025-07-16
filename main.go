package main

import (
	"context"
	"log"

	"github.com/nakamasato/cloud-run-slack-bot/pkg/cloudrun"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/cloudrunslackbot"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/config"
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

	// Ensure proper cleanup
	defer func() {
		for projectID, mClient := range mClients {
			if err := mClient.Close(); err != nil {
				log.Printf("Failed to close monitoring client for project %s: %v", projectID, err)
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
	handler := slackinternal.NewMultiProjectSlackEventHandler(sClient, rClients, mClients, cfg.TmpDir, cfg)

	// Create service with multi-project support
	svc := cloudrunslackbot.NewMultiProjectCloudRunSlackBotService(
		sClient,
		cfg,
		handler,
	)
	svc.Run()
}
