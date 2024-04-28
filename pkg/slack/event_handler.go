package slack

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
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
	selectCurrentServiceAction     = "select-current-service"
)

type Memory struct {
	mu sync.Mutex
	// memory for storing target cloud run service (slack user id -> service id)
	data map[string]string
}

func (m *Memory) Get(key string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	val, ok := m.data[key]
	return val, ok
}

func (m *Memory) Set(key, val string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = val
}

func NewMemory() *Memory {
	return &Memory{data: make(map[string]string)}
}

// SlackEventHandler handles slack events this is used by SlackEventService and SlackSocketService
type SlackEventHandler struct {
	// Slack Client
	client *slack.Client
	// Cloud Monitoring Client
	mClient *monitoring.Client
	// Cloud Run Client
	rClient *cloudrun.Client
	// Memory for storing target cloud run service
	memory *Memory
	// Temporary directory for storing images
	tmpDir string
}

func NewSlackEventHandler(client *slack.Client, rClient *cloudrun.Client, mClient *monitoring.Client, tmpDir string) *SlackEventHandler {
	return &SlackEventHandler{client: client, rClient: rClient, mClient: mClient, memory: NewMemory(), tmpDir: tmpDir}
}

// NewSlackEventHandler handles AppMention events
func (h *SlackEventHandler) HandleEvent(event *slackevents.EventsAPIEvent) error {
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
		currentService, ok := h.memory.Get(e.User)
		switch command {
		case "describe", "d":
			if !ok {
				return h.list(ctx, e.Channel, selectServiceActionForDescribe)
			}
			return h.describeService(ctx, e.Channel, currentService)
		case "metrics", "m":
			if !ok {
				return h.list(ctx, e.Channel, selectServiceActionForMetrics)
			}
			return h.getServiceMetrics(ctx, e.Channel, currentService)
		case "set", "s":
			return h.list(ctx, e.Channel, selectCurrentServiceAction)
		case "help", "h":
			return h.help(ctx, e.Channel, e.User)
		default:
			return h.help(ctx, e.Channel, e.User)
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
			h.memory.Set(interaction.User.ID, action.SelectedOption.Value)
			return h.describeService(ctx, interaction.Channel.ID, action.SelectedOption.Value)
		case selectServiceActionForMetrics:
			h.memory.Set(interaction.User.ID, action.SelectedOption.Value)
			return h.getServiceMetrics(ctx, interaction.Channel.ID, action.SelectedOption.Value)
		case selectCurrentServiceAction:
			return h.setCurrentService(ctx, interaction.Channel.ID, interaction.User.ID, action.SelectedOption.Value)
		}
	}
	return errors.New(fmt.Sprintf("unsupported interaction %v", interaction.Type))
}

func (h *SlackEventHandler) help(ctx context.Context, channelId, userId string) error {
	attachment := slack.Attachment{
		Text: "Available commands:",
		Fields: []slack.AttachmentField{
			{
				Title: "`describe` or `d`",
				Value: "describe the target Cloud Run service.\n you can check the latest revision, last modifier, update time, etc.",
			},
			{
				Title: "`metrics` or `m`",
				Value: "show the request count of the target Cloud Run service.\n you can check the request count per revision.",
			},
			{
				Title: "`set` or `s`",
				Value: "set the target Cloud Run service.\n This is set for each Slack user.",
			},
		},
	}
	_, err := h.client.PostEphemeralContext(
		ctx, channelId, userId,
		slack.MsgOptionText("Usage: @<slack app> <command> e.g. `@cloud-run-bot describe`", false),
		slack.MsgOptionAttachments(attachment),
	)
	return err
}

func (h *SlackEventHandler) setCurrentService(ctx context.Context, channelId, userId, svcName string) error {
	h.memory.Set(userId, svcName)
	_, err := h.client.PostEphemeralContext(ctx, channelId, userId, slack.MsgOptionText(fmt.Sprintf("current service is set to %s", svcName), false))
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

	_, _, err = h.client.PostMessageContext(ctx, channel, slack.MsgOptionBlocks(
		slack.SectionBlock{
			Type: slack.MBTSection,
			Text: &slack.TextBlockObject{
				Type: slack.PlainTextType,
				Text: "Please select a Cloud Run service.",
			},
			Accessory: &slack.Accessory{
				SelectElement: &slack.SelectBlockElement{
					ActionID: actionId,
					Type:     slack.OptTypeStatic,
					Placeholder: &slack.TextBlockObject{
						Type: slack.PlainTextType,
						Text: "Select a service",
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
		_, _, err := h.client.PostMessageContext(ctx, channelId, slack.MsgOptionText("Failed to get request: "+err.Error(), false))
		return err
	}
	if len(*seriesMap) == 0 {
		svc, err := h.rClient.GetService(ctx, svcName)
		if err != nil {
			return err
		}
		_, _, err = h.client.PostMessageContext(ctx, channelId,
			slack.MsgOptionText(fmt.Sprintf("No requests found for last %s. Please check <%s|%s>\n", duration, svc.GetMetricsUrl(), "Cloud Run metrics (GCP Console)"), false),
			// slack.MsgOptionBlocks(slack.SectionBlock{
			// 	Type: slack.MBTSection,
			// 	Text: &slack.TextBlockObject{
			// 		Type: slack.MarkdownType,
			// 		Text: fmt.Sprintf("No requests found for last %s.\n", duration),
			// 	},
			// 	Accessory: &slack.Accessory{
			// 		ButtonElement: &slack.ButtonBlockElement{
			// 			Text: &slack.TextBlockObject{
			// 				Type: slack.PlainTextType,
			// 				Text: "metrics",
			// 			},
			// 			URL: svc.GetMetricsUrl(),
			// 		},
			// 	},
			// }),
		)
		return err
	}
	_, _, err = h.client.PostMessageContext(ctx, channelId, slack.MsgOptionText(fmt.Sprintf("requests (last %s) for service:%s\nrequests:\n%s", duration, svcName, seriesMap), false))
	if err != nil {
		return err
	}
	log.Println("visualizing")
	imgName := fmt.Sprintf("%s/%s-metrics.png", h.tmpDir, svcName)
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
		fSummary, err := h.client.UploadFileV2Context(ctx, slack.UploadFileV2Parameters{
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
	msgOptions := []slack.MsgOption{}
	svc, err := h.rClient.GetService(ctx, svcName)
	if err != nil {
		msgOptions = append(msgOptions, slack.MsgOptionText("Failed to get service: "+err.Error(), false))
	} else {
		msgOptions = append(msgOptions, slack.MsgOptionAttachments(slack.Attachment{
			Fields: []slack.AttachmentField{
				{
					Title: "Name",
					Value: svc.Name,
					Short: true,
				},
				{
					Title: "LatestRevision",
					Value: svc.LatestRevision,
					Short: true,
				},
				{
					Title: "Image",
					Value: svc.Image,
					Short: true,
				},
				{
					Title: "LastModifier",
					Value: svc.LastModifier,
					Short: true,
				},
				{
					Title: "UpdateTime",
					Value: svc.UpdateTime.Format("2006/01/02 15:04:05"),
					Short: true,
				},
				{
					Title: "Resource Limit",
					Value: fmt.Sprintf("- cpu:%s\n- memory:%s", svc.ResourceLimits["cpu"], svc.ResourceLimits["memory"]),
					Short: true,
				},
			},
		}))
	}
	_, _, err = h.client.PostMessageContext(ctx, channelId, msgOptions...)
	return err
}
