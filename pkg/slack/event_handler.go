package slack

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	monitoringineternal "github.com/nakamasato/go-cloud-run-alert-bot/pkg/monitoring"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
	"google.golang.org/api/run/v2"
)

type SlackEventHandler struct {
	client     *slack.Client
	mClient    *monitoringineternal.MonitoringClient
	runService *run.ProjectsLocationsServicesService
}

// Implement SocketmodeHanlderFunc
func (h *SlackEventHandler) SocketmodeHandlerFuncEventsAPI(socketEvent *socketmode.Event, client *socketmode.Client) {
	event, ok := socketEvent.Data.(slackevents.EventsAPIEvent)
	if !ok {
		return
	}
	client.Ack(*socketEvent.Request)
	h.HandleEvents(&event)
}

func (h *SlackEventHandler) HandleEvents(event *slackevents.EventsAPIEvent) error {
	innerEvent := event.InnerEvent
	switch e := innerEvent.Data.(type) {
	case *slackevents.AppMentionEvent:
		message := strings.Split(e.Text, " ")
		if len(message) < 2 {
			return errors.New("no command found")
		}
		command := message[1] // e.Text is "<@bot_id> command"
		switch command {
		case "ping":
			return h.ping(e.Channel)
		case "deploy":
			return h.deploy(e.Channel)
		case "list":
			return h.list(e.Channel)
		case "metrics":
			return h.metrics(e.Channel)
		default:
			_, _, err := h.client.PostMessage(e.Channel, slack.MsgOptionText("I don't understand the command", false))
			return err
		}
	}
	return errors.New(fmt.Sprintf("unsupported event %v", innerEvent.Type))
}

func (h *SlackEventHandler) ping(channel string) error {
	_, _, err := h.client.PostMessage(channel, slack.MsgOptionText("pong", false))
	return err
}

func (h *SlackEventHandler) deploy(channel string) error {
	text := slack.NewTextBlockObject(slack.MarkdownType, "Please select *version*.", false, false)
	textSection := slack.NewSectionBlock(text, nil, nil)

	versions := []string{"v1.0.0", "v1.1.0", "v1.1.1"}
	options := make([]*slack.OptionBlockObject, 0, len(versions))
	for _, v := range versions {
		optionText := slack.NewTextBlockObject(slack.PlainTextType, v, false, false)
		description := slack.NewTextBlockObject(slack.PlainTextType, "This is the version you want to deploy.", false, false)
		options = append(options, slack.NewOptionBlockObject(v, optionText, description))
	}

	placeholder := slack.NewTextBlockObject(slack.PlainTextType, "Select version", false, false)
	selectMenu := slack.NewOptionsSelectBlockElement(slack.OptTypeStatic, placeholder, "", options...)

	actionBlock := slack.NewActionBlock(selectVersionAction, selectMenu)

	fallbackText := slack.MsgOptionText("This client is not supported.", false)
	blocks := slack.MsgOptionBlocks(textSection, actionBlock)

	_, _, err := h.client.PostMessage(channel, fallbackText, blocks)
	return err
}

func (h *SlackEventHandler) list(channel string) error {
	projLoc := "projects/" + os.Getenv("PROJECT") + "/locations/" + os.Getenv("REGION")
	res, err := h.runService.List(projLoc).Do()
	if err != nil {
		return err
	}
	svcNames := []string{}
	for _, s := range res.Services {
		svcNames = append(svcNames, s.Name)
	}
	svcNamesStr := strings.Join(svcNames, "\n")
	_, _, err = h.client.PostMessage(channel, slack.MsgOptionText(svcNamesStr, false))
	return err
}

func (h *SlackEventHandler) metrics(channel string) error {
	ctx := context.Background()
	rc, err := h.mClient.GetRequestCount(ctx, monitoringineternal.MonitorCondition{
		Project: os.Getenv("PROJECT"),
		Filters: []monitoringineternal.MonitorFilter{
			{"metric.type": "run.googleapis.com/request_count"},
			{"resource.labels.service_name": "go-cloud-run-alert-bot"}, // TODO: enable to specify service name
		},
	}, 24*time.Hour)
	msg := fmt.Sprintf("Request count: %d", rc)
	if err != nil {
		msg = "Failed to get metrics: " + err.Error()
	}
	_, _, err = h.client.PostMessage(channel, slack.MsgOptionText(msg, false))
	return err
}
