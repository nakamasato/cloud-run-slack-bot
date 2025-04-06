package cloudrunslackbot

import (
	"log"

	slackinternal "github.com/nakamasato/cloud-run-slack-bot/pkg/slack"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

type CloudRunSlackBotSocket struct {
	// https://pkg.go.dev/github.com/slack-go/slack/socketmode#Client
	sClient *socketmode.Client
	handler *slackinternal.SlackEventHandler
}

func NewCloudRunSlackBotSocket(channels map[string]string, sClient *slack.Client, handler *slackinternal.SlackEventHandler) *CloudRunSlackBotSocket {
	// https://pkg.go.dev/github.com/slack-go/slack/socketmode#New
	socketClient := socketmode.New(sClient)
	return &CloudRunSlackBotSocket{
		sClient: socketClient,
		handler: handler,
	}
}

// runSocket start socket mode
// https://pkg.go.dev/github.com/slack-go/slack/socketmode
// https://github.com/slack-go/slack/blob/master/examples/socketmode/socketmode.go
func (svc *CloudRunSlackBotSocket) Run() {
	go svc.SlackEventsHandler()

	err := svc.sClient.Run()
	if err != nil {
		log.Fatal(err)
	}
}

// SlackEventsHandler receives events from Slack socket mode channel and handles each event
func (svc *CloudRunSlackBotSocket) SlackEventsHandler() {
	for socketEvent := range svc.sClient.Events {
		switch socketEvent.Type {
		case socketmode.EventTypeConnecting:
			log.Println("Connecting to Slack with Socket Mode...")
		case socketmode.EventTypeConnectionError:
			log.Println("Connection failed. Retrying later...")
		case socketmode.EventTypeConnected:
			log.Println("Connected to Slack with Socket Mode.")
		case socketmode.EventTypeEventsAPI:
			event, ok := socketEvent.Data.(slackevents.EventsAPIEvent)
			if !ok {
				continue
			}
			svc.sClient.Ack(*socketEvent.Request)
			err := svc.handler.HandleEvent(&event)
			if err != nil {
				log.Println(err)
			}
		case socketmode.EventTypeInteractive:
			interaction, ok := socketEvent.Data.(slack.InteractionCallback)
			if !ok {
				continue
			}
			err := svc.handler.HandleInteraction(&interaction)
			if err != nil {
				log.Println(err)
			}
		}
	}
}
