package slack

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/nakamasato/cloud-run-slack-bot/pkg/cloudrun"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/monitoring"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/visualize"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

const (
	selectServiceActionForDescribe = "select-service-for-describe"
	selectServiceActionForMetrics  = "select-service-for-metrics"
)

// SlackEventHandler handles slack events this is used by SlackEventService and SlackSocketService
type SlackEventHandler struct {
	client  *slack.Client
	mClient *monitoring.Client
	rClient *cloudrun.Client
}

// NewSlackEventHandler handles AppMention events
func (h *SlackEventHandler) HandleEvents(event *slackevents.EventsAPIEvent) error {
	ctx := context.Background()
	innerEvent := event.InnerEvent
	switch e := innerEvent.Data.(type) {
	case *slackevents.AppMentionEvent:
		message := strings.Split(e.Text, " ")
		command := "describe" // default command
		if len(message) > 1 {
			command = message[1] // e.Text is "<@bot_id> command"
		}
		log.Printf("command: %s\n", command)
		switch command {
		case "ping":
			return h.ping(e.Channel)
		case "describe":
			return h.list(ctx, e.Channel, selectServiceActionForDescribe)
		case "metrics":
			return h.list(ctx, e.Channel, selectServiceActionForMetrics)
		default:
			_, _, err := h.client.PostMessage(e.Channel, slack.MsgOptionText(fmt.Sprintf("Command '%s' not supported", command), false))
			return err
		}
	}
	return errors.New(fmt.Sprintf("unsupported event %v", innerEvent.Type))
}

// HandleInteraction handles Slack interaction events e.g. selectbox, etc.
func (h *SlackEventHandler) HandleInteraction(interaction *slack.InteractionCallback) error {
	ctx := context.Background()
	switch interaction.Type {
	case slack.InteractionTypeBlockActions:
		action := interaction.ActionCallback.BlockActions[0]
		switch action.ActionID {
		case selectServiceActionForDescribe:
			return h.describeService(ctx, interaction.Channel.ID, action.SelectedOption.Value)
		case selectServiceActionForMetrics:
			return h.getServiceMetrics(ctx, interaction.Channel.ID, action.SelectedOption.Value)
		}
	}
	return errors.New(fmt.Sprintf("unsupported interaction %v", interaction.Type))
}

func (h *SlackEventHandler) ping(channel string) error {
	_, _, err := h.client.PostMessage(channel, slack.MsgOptionText("pong", false))
	return err
}

func (h *SlackEventHandler) list(ctx context.Context, channel, actionId string) error {
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
					ActionID: actionId,
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

func (h *SlackEventHandler) getServiceMetrics(ctx context.Context, channelId, svcName string) error {
	duration := 24 * time.Hour
	aggergationPeriod := 1 * time.Hour
	seriesMap, err := h.mClient.GetCloudRunServiceRequestCount(ctx, svcName, aggergationPeriod, duration)
	if err != nil {
		_, _, err := h.client.PostMessage(channelId, slack.MsgOptionText("Failed to get request: "+err.Error(), false))
		return err
	}
	_, _, err = h.client.PostMessage(channelId, slack.MsgOptionText(fmt.Sprintf("requests (last %s) for service:%s\nrequests:\n%s", duration, svcName, seriesMap), false))
	if err != nil {
		return err
	}
	log.Println("visualizing")
	imgName := fmt.Sprintf("%s-metrics.png", svcName)
	xaxis := []string{}
	for _, val := range *seriesMap {
		for i := 0; i < len(val); i++ {
			xaxis = append(xaxis, fmt.Sprintf("%d", i))
		}
		break
	}
	visualize.Visualize("Request Count", "Cloud Run request counts per revision", imgName, &xaxis, seriesMap)
	file, err := os.Open(imgName)
	if err != nil {
		log.Println(err)
		return err
	}
	if stat, err := file.Stat(); err != nil {
		return err
	} else {
		fSummary, err := h.client.UploadFileV2(slack.UploadFileV2Parameters{
			Reader:   file,
			FileSize: int(stat.Size()),
			Filename: imgName,
			Channel:  channelId,
		})
		log.Println(fSummary)
		return err
	}
}

func (h *SlackEventHandler) describeService(ctx context.Context, channelId, svcName string) error {
	res, err := h.rClient.GetService(ctx, svcName)
	if err != nil {
		_, _, err := h.client.PostMessage(channelId, slack.MsgOptionText("Failed to get service: "+err.Error(), false))
		return err
	}
	_, _, err = h.client.PostMessage(channelId, slack.MsgOptionText(res.String(), false))
	return err
}
