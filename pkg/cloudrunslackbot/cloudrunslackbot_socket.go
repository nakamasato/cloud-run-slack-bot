package cloudrunslackbot

import (
	"github.com/nakamasato/cloud-run-slack-bot/pkg/config"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/logger"
	slackinternal "github.com/nakamasato/cloud-run-slack-bot/pkg/slack"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
	"go.uber.org/zap"
)

type CloudRunSlackBotSocket struct {
	// https://pkg.go.dev/github.com/slack-go/slack/socketmode#Client
	sClient *socketmode.Client
	handler *slackinternal.SlackEventHandler
	logger  *logger.Logger
}

func NewCloudRunSlackBotSocket(channels map[string]string, defaultChannel string, sClient *slack.Client, handler *slackinternal.SlackEventHandler, log *logger.Logger) *CloudRunSlackBotSocket {
	// https://pkg.go.dev/github.com/slack-go/slack/socketmode#New
	socketClient := socketmode.New(sClient)
	return &CloudRunSlackBotSocket{
		sClient: socketClient,
		handler: handler,
		logger:  log,
	}
}

// runSocket start socket mode
// https://pkg.go.dev/github.com/slack-go/slack/socketmode
// https://github.com/slack-go/slack/blob/master/examples/socketmode/socketmode.go
func (svc *CloudRunSlackBotSocket) Run() {
	go svc.SlackEventsHandler()

	err := svc.sClient.Run()
	if err != nil {
		svc.logger.Fatal("Socket mode client failed", zap.Error(err))
	}
}

// SlackEventsHandler receives events from Slack socket mode channel and handles each event
func (svc *CloudRunSlackBotSocket) SlackEventsHandler() {
	for socketEvent := range svc.sClient.Events {
		switch socketEvent.Type {
		case socketmode.EventTypeConnecting:
			svc.logger.Info("Connecting to Slack with Socket Mode")
		case socketmode.EventTypeConnectionError:
			svc.logger.Warn("Connection failed. Retrying later")
		case socketmode.EventTypeConnected:
			svc.logger.Info("Connected to Slack with Socket Mode")
		case socketmode.EventTypeEventsAPI:
			event, ok := socketEvent.Data.(slackevents.EventsAPIEvent)
			if !ok {
				continue
			}
			svc.sClient.Ack(*socketEvent.Request)
			err := svc.handler.HandleEvent(&event)
			if err != nil {
				svc.logger.Error("Failed to handle EventsAPI event", zap.Error(err))
			}
		case socketmode.EventTypeInteractive:
			interaction, ok := socketEvent.Data.(slack.InteractionCallback)
			if !ok {
				continue
			}
			err := svc.handler.HandleInteraction(&interaction)
			if err != nil {
				svc.logger.Error("Failed to handle interactive event", zap.Error(err))
			}
		}
	}
}

// MultiProjectCloudRunSlackBotSocket handles multi-project socket mode
type MultiProjectCloudRunSlackBotSocket struct {
	sClient *socketmode.Client
	handler *slackinternal.MultiProjectSlackEventHandler
	logger  *logger.Logger
}

func NewMultiProjectCloudRunSlackBotSocket(cfg *config.Config, sClient *slack.Client, handler *slackinternal.MultiProjectSlackEventHandler, log *logger.Logger) *MultiProjectCloudRunSlackBotSocket {
	socketClient := socketmode.New(sClient)
	return &MultiProjectCloudRunSlackBotSocket{
		sClient: socketClient,
		handler: handler,
		logger:  log,
	}
}

func (svc *MultiProjectCloudRunSlackBotSocket) Run() {
	go svc.SlackEventsHandler()

	err := svc.sClient.Run()
	if err != nil {
		svc.logger.Fatal("Socket mode client failed", zap.Error(err))
	}
}

func (svc *MultiProjectCloudRunSlackBotSocket) SlackEventsHandler() {
	for socketEvent := range svc.sClient.Events {
		switch socketEvent.Type {
		case socketmode.EventTypeConnecting:
			svc.logger.Info("Connecting to Slack with Socket Mode")
		case socketmode.EventTypeConnectionError:
			svc.logger.Warn("Connection failed. Retrying later")
		case socketmode.EventTypeConnected:
			svc.logger.Info("Connected to Slack with Socket Mode")
		case socketmode.EventTypeEventsAPI:
			event, ok := socketEvent.Data.(slackevents.EventsAPIEvent)
			if !ok {
				continue
			}
			svc.sClient.Ack(*socketEvent.Request)
			err := svc.handler.HandleEvent(&event)
			if err != nil {
				svc.logger.Error("Failed to handle EventsAPI event", zap.Error(err))
			}
		case socketmode.EventTypeInteractive:
			interaction, ok := socketEvent.Data.(slack.InteractionCallback)
			if !ok {
				continue
			}
			err := svc.handler.HandleInteraction(&interaction)
			if err != nil {
				svc.logger.Error("Failed to handle interactive event", zap.Error(err))
			}
		}
	}
}
