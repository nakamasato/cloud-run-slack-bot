package cloudrunslackbot

import (
	"context"
	"log"

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
}

func NewCloudRunSlackBotSocket(channels map[string]string, defaultChannel string, sClient *slack.Client, handler *slackinternal.SlackEventHandler) *CloudRunSlackBotSocket {
	// https://pkg.go.dev/github.com/slack-go/slack/socketmode#New
	socketClient := socketmode.New(sClient)
	return &CloudRunSlackBotSocket{
		sClient: socketClient,
		handler: handler,
	}
}

// Run starts socket mode
// https://pkg.go.dev/github.com/slack-go/slack/socketmode
// https://github.com/slack-go/slack/blob/master/examples/socketmode/socketmode.go
func (svc *CloudRunSlackBotSocket) Run() {
	// Create logger
	l, err := logger.NewLogger()
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}

	l.Info("Starting Slack Socket mode")

	go svc.SlackEventsHandler()

	err = svc.sClient.Run()
	if err != nil {
		l.Fatal("Failed to run socket client", zap.Error(err))
	}
}

// SlackEventsHandler receives events from Slack socket mode channel and handles each event
func (svc *CloudRunSlackBotSocket) SlackEventsHandler() {
	// Create logger
	l, err := logger.NewLogger()
	if err != nil {
		log.Fatalf("Failed to create logger for socket mode handler: %v", err)
	}

	// Create a background context for handler calls
	ctx := context.Background()
	ctx = logger.WithContext(ctx, l)

	for socketEvent := range svc.sClient.Events {
		switch socketEvent.Type {
		case socketmode.EventTypeConnecting:
			l.Info("Connecting to Slack with Socket Mode...")
		case socketmode.EventTypeConnectionError:
			l.Error("Connection failed. Retrying later...")
		case socketmode.EventTypeConnected:
			l.Info("Connected to Slack with Socket Mode.")
		case socketmode.EventTypeEventsAPI:
			event, ok := socketEvent.Data.(slackevents.EventsAPIEvent)
			if !ok {
				l.Warn("Received invalid EventsAPI event", zap.Any("data", socketEvent.Data))
				continue
			}

			// Create a new context for this specific event
			eventCtx := ctx

			// Acknowledge receipt of the event
			svc.sClient.Ack(*socketEvent.Request)

			l.Info("Handling Slack events API event",
				zap.String("event_type", string(event.Type)))

			err := svc.handler.HandleEvent(eventCtx, &event)
			if err != nil {
				l.Error("Failed to handle event", zap.Error(err))
			}
		case socketmode.EventTypeInteractive:
			interaction, ok := socketEvent.Data.(slack.InteractionCallback)
			if !ok {
				l.Warn("Received invalid Interactive event", zap.Any("data", socketEvent.Data))
				continue
			}

			// Create a new context for this specific interaction
			interactionCtx := ctx

			l.Info("Handling Slack interactive event",
				zap.String("callback_id", interaction.CallbackID),
				zap.String("action_id", interaction.ActionID))

			err := svc.handler.HandleInteraction(interactionCtx, &interaction)
			if err != nil {
				l.Error("Failed to handle interaction", zap.Error(err))
			}
		default:
			l.Debug("Ignoring unsupported event type", zap.String("type", string(socketEvent.Type)))
		}
	}
}
