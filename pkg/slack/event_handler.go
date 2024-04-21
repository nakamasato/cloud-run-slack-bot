package slack

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nakamasato/cloud-run-slack-bot/pkg/cloudrun"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/monitoring"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

const (
	selectVersionAction           = "select-version"
	selectServiceAction           = "select-service"
	selectServiceForMetricsAction = "select-service-for-metrics"
)

// SlackEventHandler handles slack events this is used by SlackEventService and SlackSocketService
type SlackEventHandler struct {
	client  *slack.Client
	mClient *monitoring.Client
	rClient *cloudrun.Client
}

func (h *SlackEventHandler) HandleEvents(event *slackevents.EventsAPIEvent) error {
	ctx := context.Background()
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
			return h.list(ctx, e.Channel)
		case "metrics":
			return h.metrics(ctx, e.Channel)
		default:
			_, _, err := h.client.PostMessage(e.Channel, slack.MsgOptionText("I don't understand the command", false))
			return err
		}
	}
	return errors.New(fmt.Sprintf("unsupported event %v", innerEvent.Type))
}

func (h *SlackEventHandler) HandleInteraction(interaction *slack.InteractionCallback) error {
	ctx := context.Background()
	switch interaction.Type {
	case slack.InteractionTypeBlockActions:
		action := interaction.ActionCallback.BlockActions[0]
		switch action.ActionID {
		case selectVersionAction:
			selected := action.SelectedOption.Value
			_, _, err := h.client.PostMessage(interaction.Channel.ID, slack.MsgOptionText("Deploying "+selected, false))
			return err
		case selectServiceAction:
			svcName := action.SelectedOption.Value
			res, err := h.rClient.GetService(ctx, svcName)
			if err != nil {
				_, _, err := h.client.PostMessage(interaction.Channel.ID, slack.MsgOptionText("Failed to get service: "+err.Error(), false))
				return err
			}
			_, _, err = h.client.PostMessage(interaction.Channel.ID, slack.MsgOptionText(res.String(), false))
			return err
		case selectServiceForMetricsAction:
			svcName := action.SelectedOption.Value
			duration := 24 * time.Hour
			rc, err := h.mClient.GetCloudRunServiceRequestCount(ctx, svcName, duration)
			if err != nil {
				_, _, err := h.client.PostMessage(interaction.Channel.ID, slack.MsgOptionText("Failed to get metrics: "+err.Error(), false))
				return err
			}
			_, _, err = h.client.PostMessage(interaction.Channel.ID, slack.MsgOptionText(fmt.Sprintf("Requests count for '%s' is %d (last %s)", svcName, rc, duration), false))
			return err
		}
	}
	return errors.New(fmt.Sprintf("unsupported interaction %v", interaction.Type))
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

func (h *SlackEventHandler) list(ctx context.Context, channel string) error {

	svcNames, err := h.rClient.ListServices(ctx)
	if err != nil {
		return err
	}
	options := []*slack.OptionBlockObject{}
	for _, svcName := range svcNames {
		fmt.Println(svcName)
		options = append(options, &slack.OptionBlockObject{
			Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: svcName}, Value: svcName,
		})
	}

	_, _, err = h.client.PostMessage(channel, slack.MsgOptionBlocks(
		slack.SectionBlock{
			Type: slack.MBTSection,
			Text: &slack.TextBlockObject{
				Type: slack.PlainTextType,
				Text: "which service do you want to check?",
			},
			Accessory: &slack.Accessory{
				SelectElement: &slack.SelectBlockElement{
					ActionID: selectServiceAction,
					Type:     slack.OptTypeStatic,
					Placeholder: &slack.TextBlockObject{
						Type: slack.PlainTextType,
						Text: "Select service",
					},
					Options: options,
				},
			},
		},
	))
	return err
}

func (h *SlackEventHandler) metrics(ctx context.Context, channel string) error {
	svcNames, err := h.rClient.ListServices(ctx)
	if err != nil {
		return err
	}
	options := []*slack.OptionBlockObject{}
	for _, svcName := range svcNames {
		fmt.Println(svcName)
		options = append(options, &slack.OptionBlockObject{
			Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: svcName}, Value: svcName,
		})
	}

	_, _, err = h.client.PostMessage(channel, slack.MsgOptionBlocks(
		slack.SectionBlock{
			Type: slack.MBTSection,
			Text: &slack.TextBlockObject{
				Type: slack.PlainTextType,
				Text: "which service do you want to metrics?",
			},
			Accessory: &slack.Accessory{
				SelectElement: &slack.SelectBlockElement{
					ActionID: selectServiceForMetricsAction,
					Type:     slack.OptTypeStatic,
					Placeholder: &slack.TextBlockObject{
						Type: slack.PlainTextType,
						Text: "Select service",
					},
					Options: options,
				},
			},
		},
	))
	return err
}
