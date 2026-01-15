package slack

import (
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/nakamasato/cloud-run-slack-bot/pkg/cloudrun"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/config"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/debug"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/monitoring"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/visualize"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

const (
	ActionIdDescribeResource = "select-resource-for-describe"
	ActionIdMetricsResource  = "select-resource-for-metrics"
	ActionIdCurrentResource  = "select-current-resource"
	ActionIdDebugResource    = "select-resource-for-debug"
	ActionIdMetrics          = "metrics"
	defaultDuration          = 24 * time.Hour
	defaultAggregationPeriod = 5 * time.Minute
	defaultMetricsType       = "count"
)

var durationAggregationPeriodMap = map[string]time.Duration{
	"1h":   1 * time.Minute,          // 60 points
	"24h":  defaultAggregationPeriod, // 288 points
	"168h": 1 * time.Hour,            // 168 points
}

type Memory struct {
	mu sync.Mutex
	// memory for storing target cloud run service or job (slack user id -> service/job id)
	data map[string]string
	// Stores the resource type ("service" or "job")
	resourceType map[string]string
}

func (m *Memory) Get(key string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	val, ok := m.data[key]
	return val, ok
}

func (m *Memory) GetResourceType(key string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	resourceType, ok := m.resourceType[key]
	if !ok {
		return "service" // Default to service for backward compatibility
	}
	return resourceType
}

func (m *Memory) IsJob(key string) bool {
	// Keep for backward compatibility
	return m.GetResourceType(key) == "job"
}

func (m *Memory) Set(key, val string, resourceType string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = val
	m.resourceType[key] = resourceType
}

func NewMemory() *Memory {
	return &Memory{
		data:         make(map[string]string),
		resourceType: make(map[string]string),
	}
}

// ParseResourceValue parses and validates resource value format
func ParseResourceValue(value string) (resourceType, resourceName string, err error) {
	if value == "" {
		return "", "", fmt.Errorf("resource value cannot be empty")
	}

	// Check if value contains the new format with type:name
	if strings.Contains(value, ":") {
		parts := strings.SplitN(value, ":", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("invalid resource format: expected 'type:name', got '%s'", value)
		}
		resourceType = parts[0]
		resourceName = parts[1]
	} else {
		// Legacy format without type prefix
		resourceType = "service" // Default
		resourceName = value
	}

	// Validate resource type
	if resourceType != "service" && resourceType != "job" {
		return "", "", fmt.Errorf("invalid resource type: '%s', must be 'service' or 'job'", resourceType)
	}

	// Validate resource name
	if resourceName == "" {
		return "", "", fmt.Errorf("resource name cannot be empty")
	}

	return resourceType, resourceName, nil
}

// ParseMultiProjectResourceValue parses and validates multi-project resource value format
func ParseMultiProjectResourceValue(value string) (projectID, resourceType, resourceName string, err error) {
	if value == "" {
		return "", "", "", fmt.Errorf("resource value cannot be empty")
	}

	// Parse project:resourceType:resourceName format
	parts := strings.SplitN(value, ":", 3)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid resource format: expected 'project:type:name', got '%s'", value)
	}

	projectID = parts[0]
	resourceType = parts[1]
	resourceName = parts[2]

	// Validate components
	if projectID == "" {
		return "", "", "", fmt.Errorf("project ID cannot be empty")
	}
	if resourceType != "service" && resourceType != "job" {
		return "", "", "", fmt.Errorf("invalid resource type: '%s', must be 'service' or 'job'", resourceType)
	}
	if resourceName == "" {
		return "", "", "", fmt.Errorf("resource name cannot be empty")
	}

	return projectID, resourceType, resourceName, nil
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
		currentItem, ok := h.memory.Get(e.User)

		// Check if we're dealing with services or jobs
		switch command {
		case "describe", "d":
			if !ok {
				return h.list(ctx, e.Channel, ActionIdDescribeResource)
			}
			resourceType := h.memory.GetResourceType(e.User)
			if resourceType == "job" {
				return h.describeJob(ctx, e.Channel, currentItem)
			}
			return h.describeService(ctx, e.Channel, currentItem)
		case "metrics", "m":
			if !ok {
				return h.list(ctx, e.Channel, ActionIdMetricsResource)
			}
			resourceType := h.memory.GetResourceType(e.User)
			if resourceType == "job" {
				// Jobs don't have metrics like services, so show description instead
				return h.describeJob(ctx, e.Channel, currentItem)
			}
			return h.getServiceMetrics(ctx, e.Channel, currentItem, "count", defaultDuration, defaultAggregationPeriod)
		case "set", "s":
			return h.list(ctx, e.Channel, ActionIdCurrentResource)
		case "help", "h":
			return h.help(ctx, e.Channel, e.User)
		case "sample":
			return h.sample(ctx, e.Channel)
		default:
			return h.help(ctx, e.Channel, e.User)
		}
	}
	return fmt.Errorf("unsupported event %v", innerEvent.Type)
}

// HandleInteraction handles Slack interaction events e.g. selectbox, etc.
func (h *SlackEventHandler) HandleInteraction(interaction *slack.InteractionCallback) error {
	ctx := context.Background()
	switch interaction.Type {
	case slack.InteractionTypeBlockActions:
		action := interaction.ActionCallback.BlockActions[0]

		// Parse resource type and name from the selected option value
		value := action.SelectedOption.Value
		resourceType, resourceName, err := ParseResourceValue(value)
		if err != nil {
			return fmt.Errorf("failed to parse resource value: %v", err)
		}

		switch action.ActionID {
		case ActionIdDescribeResource:
			// Handle all describe actions
			h.memory.Set(interaction.User.ID, resourceName, resourceType)
			if resourceType == "job" {
				return h.describeJob(ctx, interaction.Channel.ID, resourceName)
			}
			return h.describeService(ctx, interaction.Channel.ID, resourceName)

		case ActionIdMetricsResource:
			// Handle all metrics actions
			h.memory.Set(interaction.User.ID, resourceName, resourceType)
			if resourceType == "job" {
				// Jobs don't have metrics, show job description instead
				return h.describeJob(ctx, interaction.Channel.ID, resourceName)
			}
			return h.getServiceMetrics(ctx, interaction.Channel.ID, resourceName, "count", defaultDuration, defaultAggregationPeriod)

		case ActionIdCurrentResource:
			// Handle all set current resource actions
			return h.setCurrentResource(ctx, interaction.Channel.ID, interaction.User.ID, resourceName, resourceType)
		}
	case slack.InteractionTypeInteractionMessage:
		callbackId := interaction.CallbackID
		switch callbackId {
		case ActionIdMetrics:
			durationVal := defaultDuration.String()
			metricsTypeVal := defaultMetricsType
			for _, action := range interaction.ActionCallback.AttachmentActions {
				switch action.Name {
				case "duration":
					durationVal = action.SelectedOptions[0].Value
				case "metrics":
					metricsTypeVal = action.SelectedOptions[0].Value
				}
			}

			log.Printf("test: %d\n", len(interaction.ActionCallback.AttachmentActions))
			// metricsTypeVal := interaction.ActionCallback.AttachmentActions[1].SelectedOptions[0].Value
			svc, ok := h.memory.Get(interaction.User.ID)
			if !ok {
				return h.list(ctx, interaction.Channel.ID, ActionIdMetricsResource)
			}
			duration, err := time.ParseDuration(durationVal)
			if err != nil {
				return err
			}
			aggregationPeriod, ok := durationAggregationPeriodMap[durationVal]
			if !ok {
				aggregationPeriod = defaultAggregationPeriod
			}
			return h.getServiceMetrics(ctx, interaction.Channel.ID, svc, metricsTypeVal, duration, aggregationPeriod)
		}

	}
	return fmt.Errorf("unsupported interaction %v", interaction.Type)
}

func (h *SlackEventHandler) help(ctx context.Context, channelId, userId string) error {
	attachment := slack.Attachment{
		Text: "Available commands:",
		Fields: []slack.AttachmentField{
			{
				Title: "`describe` or `d`",
				Value: "describe the target Cloud Run service or job.\n you can check the latest revision, last modifier, update time, etc.",
			},
			{
				Title: "`metrics` or `m`",
				Value: "show the request count of the target Cloud Run service or job description.\n for services, you can check the request count per revision.",
			},
			{
				Title: "`set` or `s`",
				Value: "set the target Cloud Run service or job.\n this displays a list of both services and jobs to select from.",
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

func (h *SlackEventHandler) setCurrentResource(ctx context.Context, channelId, userId, name string, resourceType string) error {
	h.memory.Set(userId, name, resourceType)
	_, err := h.client.PostEphemeralContext(ctx, channelId, userId, slack.MsgOptionText(fmt.Sprintf("current %s is set to %s", resourceType, name), false))
	return err
}

func (h *SlackEventHandler) list(ctx context.Context, channel, actionId string) error {
	// Get both services and jobs
	svcNames, err := h.rClient.ListServices(ctx)
	if err != nil {
		return err
	}

	jobNames, err := h.rClient.ListJobs(ctx)
	if err != nil {
		return err
	}

	options := []*slack.OptionBlockObject{}

	// Add services with [SVC] prefix
	for _, svcName := range svcNames {
		displayName := fmt.Sprintf("[SVC] %s", svcName)
		value := fmt.Sprintf("service:%s", svcName)
		options = append(options, &slack.OptionBlockObject{
			Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: displayName},
			Value: value,
		})
	}

	// Add jobs with [JOB] prefix
	for _, jobName := range jobNames {
		displayName := fmt.Sprintf("[JOB] %s", jobName)
		value := fmt.Sprintf("job:%s", jobName)
		options = append(options, &slack.OptionBlockObject{
			Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: displayName},
			Value: value,
		})
	}

	// If no resources found, inform the user
	if len(options) == 0 {
		_, _, err = h.client.PostMessageContext(ctx, channel,
			slack.MsgOptionText("No Cloud Run services or jobs found in this project/region.", false))
		return err
	}

	_, _, err = h.client.PostMessageContext(ctx, channel, slack.MsgOptionBlocks(
		slack.SectionBlock{
			Type: slack.MBTSection,
			Text: &slack.TextBlockObject{
				Type: slack.PlainTextType,
				Text: "Please select a Cloud Run service or job.",
			},
			Accessory: &slack.Accessory{
				SelectElement: &slack.SelectBlockElement{
					ActionID: actionId,
					Type:     slack.OptTypeStatic,
					Placeholder: &slack.TextBlockObject{
						Type: slack.PlainTextType,
						Text: "Select a resource",
					},
					Options: options,
				},
			},
		},
	))
	return err
}


func (h *SlackEventHandler) getServiceMetrics(ctx context.Context, channelId, svcName, metricsType string, duration, aggregationPeriod time.Duration) error {
	now := time.Now().UTC()
	endTime := now.Truncate(aggregationPeriod).Add(aggregationPeriod)

	startTime := endTime.Add(-1 * duration).UTC()
	var seriesMap *monitoring.TimeSeriesMap
	var err error
	var title string
	if metricsType == "latency" {
		title = "Request Latency"
		seriesMap, err = h.mClient.GetCloudRunServiceRequestLatencies(ctx, svcName, aggregationPeriod, startTime, endTime)
	} else {
		title = "Request Count"
		seriesMap, err = h.mClient.GetCloudRunServiceRequestCount(ctx, svcName, aggregationPeriod, startTime, endTime)
	}

	if err != nil {
		_, _, err := h.client.PostMessageContext(ctx, channelId, slack.MsgOptionText("Failed to get request: "+err.Error(), false))
		return err
	}
	if len(*seriesMap) == 0 {
		log.Printf("DEBUG: Getting service '%s' for metrics URL (legacy handler)", svcName)
		svc, err := h.rClient.GetService(ctx, svcName)
		if err != nil {
			log.Printf("ERROR: Failed to get service '%s' for metrics URL (legacy handler): %v", svcName, err)
			return err
		}
		_, _, err = h.client.PostMessageContext(ctx, channelId,
			slack.MsgOptionText(fmt.Sprintf("No requests found for last %s. Please check <%s|%s>\n", duration, svc.GetMetricsUrl(), "Cloud Run metrics (GCP Console)"), false),
		)
		return err
	}

	log.Println("visualizing")
	imgName := path.Join(h.tmpDir, fmt.Sprintf("%s-metrics.png", svcName))
	log.Printf("imgName: %s\n", imgName)

	size, err := visualize.Visualize(title, imgName, startTime, endTime, aggregationPeriod, seriesMap)
	if err != nil {
		log.Println(err)
		return nil
	}
	file, err := os.Open(imgName)
	if err != nil {
		return err
	}

	// UploadFileV2Context does the followings:
	// 1. https://api.slack.com/methods/files.getUploadURLExternal
	// 2. https://api.slack.com/methods/files.upload
	// 3. https://api.slack.com/methods/files.completeUploadExternal
	// but there are two problems:
	// 1. The file is sent to channel, although channel id is optional parameter of completeUploadExternal.
	// 2. The link to the file is not available from the response (FileSummary{Id, Title})
	_, err = h.client.UploadFileV2Context(ctx, slack.UploadFileV2Parameters{
		Reader:   file,
		FileSize: int(size),
		Filename: imgName,
		Channel:  channelId,
	})
	if err != nil {
		log.Println(err)
		return err
	}

	// f, err := h.client.UploadFileContext(ctx, slack.FileUploadParameters{
	// 	Reader:   file,
	// 	Filename: imgName,
	// 	Filetype: "png",
	// })
	// if err != nil {
	// 	return err
	// }

	fields := []slack.AttachmentField{}
	for k, v := range *seriesMap {
		var total int64
		for _, p := range v {
			total += int64(p.Val)
		}
		fields = append(fields, slack.AttachmentField{
			Title: k,
			Value: fmt.Sprint(total),
			Short: true,
		})
	}

	attachment := slack.Attachment{
		Text:       title,
		Fields:     fields,
		Color:      "good", // good, warning, danger
		CallbackID: ActionIdMetrics,
		Actions: []slack.AttachmentAction{
			{
				Name: "duration",
				Text: "Duration",
				Type: "select",
				Options: []slack.AttachmentActionOption{
					{
						Text:  "1h",
						Value: "1h",
					},
					{
						Text:  "1d",
						Value: "24h",
					},
					{
						Text:  "1w",
						Value: "168h",
					},
				},
			},
			{
				Name: "metrics",
				Text: "Metrics",
				Type: "select",
				Options: []slack.AttachmentActionOption{
					{
						Text:  "latency",
						Value: "latency",
					},
					{
						Text:  "count",
						Value: "count",
					},
				},
			},
		},
	}
	_, _, err = h.client.PostMessageContext(
		ctx, channelId,
		slack.MsgOptionText(fmt.Sprintf("`%s`", svcName), false),
		slack.MsgOptionAttachments(attachment),
	)
	if err != nil {
		return err
	}
	return err
}

func (h *SlackEventHandler) describeService(ctx context.Context, channelId, svcName string) error {
	msgOptions := []slack.MsgOption{}
	log.Printf("DEBUG: Getting service '%s' using legacy single-project handler", svcName)
	svc, err := h.rClient.GetService(ctx, svcName)
	if err != nil {
		log.Printf("ERROR: Failed to get service '%s' (legacy handler): %v", svcName, err)
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

func (h *SlackEventHandler) describeJob(ctx context.Context, channelId, jobName string) error {
	msgOptions := []slack.MsgOption{}
	job, err := h.rClient.GetJob(ctx, jobName)
	if err != nil {
		msgOptions = append(msgOptions, slack.MsgOptionText("Failed to get job: "+err.Error(), false))
	} else {
		msgOptions = append(msgOptions, slack.MsgOptionAttachments(slack.Attachment{
			Fields: []slack.AttachmentField{
				{
					Title: "Name",
					Value: job.Name,
					Short: true,
				},
				{
					Title: "Image",
					Value: job.Image,
					Short: true,
				},
				{
					Title: "LastModifier",
					Value: job.LastModifier,
					Short: true,
				},
				{
					Title: "UpdateTime",
					Value: job.UpdateTime.Format("2006/01/02 15:04:05"),
					Short: true,
				},
				{
					Title: "Resource Limit",
					Value: fmt.Sprintf("- cpu:%s\n- memory:%s", job.ResourceLimits["cpu"], job.ResourceLimits["memory"]),
					Short: true,
				},
				{
					Title: "Console URL",
					Value: fmt.Sprintf("<%s|Cloud Run Job>", job.GetYamlUrl()),
					Short: true,
				},
			},
		}))
	}
	_, _, err = h.client.PostMessageContext(ctx, channelId, msgOptions...)
	return err
}

func (h *SlackEventHandler) sample(ctx context.Context, channelId string) error {
	imgName := path.Join(h.tmpDir, "sample.png")
	err := visualize.VisualizeSample(imgName)
	if err != nil {
		return err
	}
	file, err := os.Open(imgName)
	if err != nil {
		return err
	}
	stat, err := file.Stat()
	if err != nil {
		return err
	}
	fSummary, err := h.client.UploadFileV2Context(ctx, slack.UploadFileV2Parameters{
		Reader:   file,
		FileSize: int(stat.Size()), // random value
		Filename: imgName,
		Channel:  channelId,
	})
	log.Println(fSummary)
	return err
}

// MultiProjectSlackEventHandler handles slack events for multiple projects
type MultiProjectSlackEventHandler struct {
	client   *slack.Client
	mClients map[string]*monitoring.Client
	rClients map[string]*cloudrun.Client
	debugger *debug.Debugger // nil if debug feature is disabled
	memory   *Memory
	tmpDir   string
	config   *config.Config
}

func NewMultiProjectSlackEventHandler(client *slack.Client, rClients map[string]*cloudrun.Client, mClients map[string]*monitoring.Client, debugger *debug.Debugger, tmpDir string, cfg *config.Config) *MultiProjectSlackEventHandler {
	return &MultiProjectSlackEventHandler{
		client:   client,
		rClients: rClients,
		mClients: mClients,
		debugger: debugger,
		memory:   NewMemory(),
		tmpDir:   tmpDir,
		config:   cfg,
	}
}

func (h *MultiProjectSlackEventHandler) HandleEvent(event *slackevents.EventsAPIEvent) error {
	ctx := context.Background()
	innerEvent := event.InnerEvent
	switch e := innerEvent.Data.(type) {
	case *slackevents.AppMentionEvent:
		message := strings.Split(e.Text, " ")
		command := "describe"
		if len(message) > 1 {
			command = message[1]
		}
		log.Printf("command: %s\n", command)
		currentItem, ok := h.memory.Get(e.User)

		// Check if we can auto-detect projects from channel
		channelProjects := h.config.GetProjectsForChannel(e.Channel)
		log.Printf("Channel %s is associated with projects: %v", e.Channel, channelProjects)

		switch command {
		case "describe", "d":
			if !ok {
				return h.listResourcesForChannel(ctx, e.Channel, ActionIdDescribeResource, channelProjects)
			}
			return h.describeResource(ctx, e.Channel, currentItem)
		case "metrics", "m":
			if !ok {
				return h.listResourcesForChannel(ctx, e.Channel, ActionIdMetricsResource, channelProjects)
			}
			return h.getResourceMetrics(ctx, e.Channel, currentItem, "count", defaultDuration, defaultAggregationPeriod)
		case "debug", "dbg":
			if h.debugger == nil {
				_, err := h.client.PostEphemeralContext(ctx, e.Channel, e.User,
					slack.MsgOptionText("Debug feature is not enabled. Set DEBUG_ENABLED=true to enable.", false))
				return err
			}
			if !ok {
				return h.listResourcesForChannel(ctx, e.Channel, ActionIdDebugResource, channelProjects)
			}
			return h.debugResource(ctx, e.Channel, e.User, currentItem)
		case "set", "s":
			return h.listResourcesForChannel(ctx, e.Channel, ActionIdCurrentResource, channelProjects)
		case "help", "h":
			return h.help(ctx, e.Channel, e.User)
		case "sample":
			return h.sample(ctx, e.Channel)
		default:
			return h.help(ctx, e.Channel, e.User)
		}
	}
	return fmt.Errorf("unsupported event %v", innerEvent.Type)
}

func (h *MultiProjectSlackEventHandler) HandleInteraction(interaction *slack.InteractionCallback) error {
	ctx := context.Background()
	switch interaction.Type {
	case slack.InteractionTypeBlockActions:
		action := interaction.ActionCallback.BlockActions[0]
		value := action.SelectedOption.Value

		// Parse project:resourceType:resourceName format
		projectID, resourceType, resourceName, err := ParseMultiProjectResourceValue(value)
		if err != nil {
			return fmt.Errorf("failed to parse multi-project resource value: %v", err)
		}
		_ = projectID // Used in action handlers below
		_ = resourceName // Used in action handlers below

		switch action.ActionID {
		case ActionIdDescribeResource:
			h.memory.Set(interaction.User.ID, value, resourceType)
			return h.describeResource(ctx, interaction.Channel.ID, value)
		case ActionIdMetricsResource:
			h.memory.Set(interaction.User.ID, value, resourceType)
			return h.getResourceMetrics(ctx, interaction.Channel.ID, value, "count", defaultDuration, defaultAggregationPeriod)
		case ActionIdDebugResource:
			if h.debugger == nil {
				_, err := h.client.PostEphemeralContext(ctx, interaction.Channel.ID, interaction.User.ID,
					slack.MsgOptionText("Debug feature is not enabled.", false))
				return err
			}
			h.memory.Set(interaction.User.ID, value, resourceType)
			return h.debugResource(ctx, interaction.Channel.ID, interaction.User.ID, value)
		case ActionIdCurrentResource:
			return h.setCurrentResource(ctx, interaction.Channel.ID, interaction.User.ID, value, resourceType)
		}
	case slack.InteractionTypeInteractionMessage:
		callbackId := interaction.CallbackID
		switch callbackId {
		case ActionIdMetrics:
			durationVal := defaultDuration.String()
			metricsTypeVal := defaultMetricsType
			for _, action := range interaction.ActionCallback.AttachmentActions {
				switch action.Name {
				case "duration":
					durationVal = action.SelectedOptions[0].Value
				case "metrics":
					metricsTypeVal = action.SelectedOptions[0].Value
				}
			}

			svc, ok := h.memory.Get(interaction.User.ID)
			if !ok {
				channelProjects := h.config.GetProjectsForChannel(interaction.Channel.ID)
				return h.listResourcesForChannel(ctx, interaction.Channel.ID, ActionIdMetricsResource, channelProjects)
			}
			duration, err := time.ParseDuration(durationVal)
			if err != nil {
				return err
			}
			aggregationPeriod, ok := durationAggregationPeriodMap[durationVal]
			if !ok {
				aggregationPeriod = defaultAggregationPeriod
			}
			return h.getResourceMetrics(ctx, interaction.Channel.ID, svc, metricsTypeVal, duration, aggregationPeriod)
		}
	}
	return fmt.Errorf("unsupported interaction %v", interaction.Type)
}

func (h *MultiProjectSlackEventHandler) listResourcesForChannel(ctx context.Context, channel, actionId string, channelProjects []string) error {
	// If channel has exactly one project, list only that project's resources
	if len(channelProjects) == 1 {
		return h.listSingleProjectResources(ctx, channel, actionId, channelProjects[0])
	}
	// If channel has multiple projects or no specific projects, list all or filtered resources
	return h.listAllProjects(ctx, channel, actionId)
}

func (h *MultiProjectSlackEventHandler) listSingleProjectResources(ctx context.Context, channel, actionId, projectID string) error {
	rClient, ok := h.rClients[projectID]
	if !ok {
		log.Printf("Warning: No client found for project %s", projectID)
		return h.listAllProjects(ctx, channel, actionId)
	}

	options := []*slack.OptionBlockObject{}

	// Get services for this project
	svcNames, err := rClient.ListServices(ctx)
	if err != nil {
		log.Printf("Error listing services for project %s: %v", projectID, err)
		return h.listAllProjects(ctx, channel, actionId)
	}

	for _, svcName := range svcNames {
		displayName := fmt.Sprintf("[SVC] %s", svcName) // No need to show project ID
		value := fmt.Sprintf("%s:service:%s", projectID, svcName)
		options = append(options, &slack.OptionBlockObject{
			Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: displayName},
			Value: value,
		})
	}

	// Get jobs for this project
	jobNames, err := rClient.ListJobs(ctx)
	if err != nil {
		log.Printf("Error listing jobs for project %s: %v", projectID, err)
		return h.listAllProjects(ctx, channel, actionId)
	}

	for _, jobName := range jobNames {
		displayName := fmt.Sprintf("[JOB] %s", jobName) // No need to show project ID
		value := fmt.Sprintf("%s:job:%s", projectID, jobName)
		options = append(options, &slack.OptionBlockObject{
			Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: displayName},
			Value: value,
		})
	}

	if len(options) == 0 {
		_, _, err := h.client.PostMessageContext(ctx, channel,
			slack.MsgOptionText(fmt.Sprintf("No Cloud Run services or jobs found in project %s.", projectID), false))
		return err
	}

	_, _, err = h.client.PostMessageContext(ctx, channel, slack.MsgOptionBlocks(
		slack.SectionBlock{
			Type: slack.MBTSection,
			Text: &slack.TextBlockObject{
				Type: slack.PlainTextType,
				Text: fmt.Sprintf("Please select a Cloud Run service or job (Project: %s).", projectID),
			},
			Accessory: &slack.Accessory{
				SelectElement: &slack.SelectBlockElement{
					ActionID: actionId,
					Type:     slack.OptTypeStatic,
					Placeholder: &slack.TextBlockObject{
						Type: slack.PlainTextType,
						Text: "Select a resource",
					},
					Options: options,
				},
			},
		},
	))
	return err
}

func (h *MultiProjectSlackEventHandler) listAllProjects(ctx context.Context, channel, actionId string) error {
	options := []*slack.OptionBlockObject{}

	for _, project := range h.config.Projects {
		rClient, ok := h.rClients[project.ID]
		if !ok {
			log.Printf("Warning: No client found for project %s", project.ID)
			continue
		}

		// Get services for this project
		svcNames, err := rClient.ListServices(ctx)
		if err != nil {
			log.Printf("Error listing services for project %s: %v", project.ID, err)
			continue
		}

		for _, svcName := range svcNames {
			displayName := fmt.Sprintf("[%s] [SVC] %s", project.ID, svcName)
			value := fmt.Sprintf("%s:service:%s", project.ID, svcName)
			options = append(options, &slack.OptionBlockObject{
				Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: displayName},
				Value: value,
			})
		}

		// Get jobs for this project
		jobNames, err := rClient.ListJobs(ctx)
		if err != nil {
			log.Printf("Error listing jobs for project %s: %v", project.ID, err)
			continue
		}

		for _, jobName := range jobNames {
			displayName := fmt.Sprintf("[%s] [JOB] %s", project.ID, jobName)
			value := fmt.Sprintf("%s:job:%s", project.ID, jobName)
			options = append(options, &slack.OptionBlockObject{
				Text: &slack.TextBlockObject{Type: slack.PlainTextType, Text: displayName},
				Value: value,
			})
		}
	}

	if len(options) == 0 {
		_, _, err := h.client.PostMessageContext(ctx, channel,
			slack.MsgOptionText("No Cloud Run services or jobs found in any configured project.", false))
		return err
	}

	_, _, err := h.client.PostMessageContext(ctx, channel, slack.MsgOptionBlocks(
		slack.SectionBlock{
			Type: slack.MBTSection,
			Text: &slack.TextBlockObject{
				Type: slack.PlainTextType,
				Text: "Please select a Cloud Run service or job.",
			},
			Accessory: &slack.Accessory{
				SelectElement: &slack.SelectBlockElement{
					ActionID: actionId,
					Type:     slack.OptTypeStatic,
					Placeholder: &slack.TextBlockObject{
						Type: slack.PlainTextType,
						Text: "Select a resource",
					},
					Options: options,
				},
			},
		},
	))
	return err
}

func (h *MultiProjectSlackEventHandler) describeResource(ctx context.Context, channelId, resourceValue string) error {
	projectID, resourceType, resourceName, err := ParseMultiProjectResourceValue(resourceValue)
	if err != nil {
		return fmt.Errorf("failed to parse resource value: %v", err)
	}

	rClient, ok := h.rClients[projectID]
	if !ok {
		return fmt.Errorf("no client found for project %s", projectID)
	}

	if resourceType == "job" {
		return h.describeJobForProject(ctx, channelId, resourceName, rClient)
	}
	return h.describeServiceForProject(ctx, channelId, resourceName, rClient)
}

func (h *MultiProjectSlackEventHandler) getResourceMetrics(ctx context.Context, channelId, resourceValue, metricsType string, duration, aggregationPeriod time.Duration) error {
	projectID, resourceType, resourceName, err := ParseMultiProjectResourceValue(resourceValue)
	if err != nil {
		return fmt.Errorf("failed to parse resource value: %v", err)
	}

	if resourceType == "job" {
		// Jobs don't have metrics like services, show job description instead
		rClient, ok := h.rClients[projectID]
		if !ok {
			return fmt.Errorf("no client found for project %s", projectID)
		}
		return h.describeJobForProject(ctx, channelId, resourceName, rClient)
	}

	mClient, ok := h.mClients[projectID]
	if !ok {
		return fmt.Errorf("no monitoring client found for project %s", projectID)
	}

	rClient, ok := h.rClients[projectID]
	if !ok {
		return fmt.Errorf("no cloud run client found for project %s", projectID)
	}

	return h.getServiceMetricsForProject(ctx, channelId, resourceName, metricsType, duration, aggregationPeriod, mClient, rClient)
}

func (h *MultiProjectSlackEventHandler) setCurrentResource(ctx context.Context, channelId, userId, resourceValue, resourceType string) error {
	h.memory.Set(userId, resourceValue, resourceType)
	projectID, _, resourceName, err := ParseMultiProjectResourceValue(resourceValue)
	if err != nil {
		// Fallback to legacy format
		_, err := h.client.PostEphemeralContext(ctx, channelId, userId,
			slack.MsgOptionText(fmt.Sprintf("current %s is set to %s", resourceType, resourceValue), false))
		return err
	}
	_, err = h.client.PostEphemeralContext(ctx, channelId, userId,
		slack.MsgOptionText(fmt.Sprintf("current %s is set to %s in project %s", resourceType, resourceName, projectID), false))
	return err
}

func (h *MultiProjectSlackEventHandler) describeServiceForProject(ctx context.Context, channelId, svcName string, rClient *cloudrun.Client) error {
	msgOptions := []slack.MsgOption{}
	log.Printf("DEBUG: Getting service '%s' for project (rClient initialized for specific project)", svcName)
	svc, err := rClient.GetService(ctx, svcName)
	if err != nil {
		log.Printf("ERROR: Failed to get service '%s': %v", svcName, err)
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

func (h *MultiProjectSlackEventHandler) describeJobForProject(ctx context.Context, channelId, jobName string, rClient *cloudrun.Client) error {
	msgOptions := []slack.MsgOption{}
	job, err := rClient.GetJob(ctx, jobName)
	if err != nil {
		msgOptions = append(msgOptions, slack.MsgOptionText("Failed to get job: "+err.Error(), false))
	} else {
		msgOptions = append(msgOptions, slack.MsgOptionAttachments(slack.Attachment{
			Fields: []slack.AttachmentField{
				{
					Title: "Name",
					Value: job.Name,
					Short: true,
				},
				{
					Title: "Image",
					Value: job.Image,
					Short: true,
				},
				{
					Title: "LastModifier",
					Value: job.LastModifier,
					Short: true,
				},
				{
					Title: "UpdateTime",
					Value: job.UpdateTime.Format("2006/01/02 15:04:05"),
					Short: true,
				},
				{
					Title: "Resource Limit",
					Value: fmt.Sprintf("- cpu:%s\n- memory:%s", job.ResourceLimits["cpu"], job.ResourceLimits["memory"]),
					Short: true,
				},
				{
					Title: "Console URL",
					Value: fmt.Sprintf("<%s|Cloud Run Job>", job.GetYamlUrl()),
					Short: true,
				},
			},
		}))
	}
	_, _, err = h.client.PostMessageContext(ctx, channelId, msgOptions...)
	return err
}

func (h *MultiProjectSlackEventHandler) getServiceMetricsForProject(ctx context.Context, channelId, svcName, metricsType string, duration, aggregationPeriod time.Duration, mClient *monitoring.Client, rClient *cloudrun.Client) error {
	now := time.Now().UTC()
	endTime := now.Truncate(aggregationPeriod).Add(aggregationPeriod)
	startTime := endTime.Add(-1 * duration).UTC()

	var seriesMap *monitoring.TimeSeriesMap
	var err error
	var title string

	if metricsType == "latency" {
		title = "Request Latency"
		seriesMap, err = mClient.GetCloudRunServiceRequestLatencies(ctx, svcName, aggregationPeriod, startTime, endTime)
	} else {
		title = "Request Count"
		seriesMap, err = mClient.GetCloudRunServiceRequestCount(ctx, svcName, aggregationPeriod, startTime, endTime)
	}

	if err != nil {
		_, _, err := h.client.PostMessageContext(ctx, channelId, slack.MsgOptionText("Failed to get request: "+err.Error(), false))
		return err
	}

	if len(*seriesMap) == 0 {
		log.Printf("DEBUG: Getting service '%s' for metrics URL (multi-project handler)", svcName)
		svc, err := rClient.GetService(ctx, svcName)
		if err != nil {
			log.Printf("ERROR: Failed to get service '%s' for metrics URL (multi-project handler): %v", svcName, err)
			return err
		}
		_, _, err = h.client.PostMessageContext(ctx, channelId,
			slack.MsgOptionText(fmt.Sprintf("No requests found for last %s. Please check <%s|%s>\n", duration, svc.GetMetricsUrl(), "Cloud Run metrics (GCP Console)"), false),
		)
		return err
	}

	log.Println("visualizing")
	imgName := path.Join(h.tmpDir, fmt.Sprintf("%s-metrics.png", svcName))
	log.Printf("imgName: %s\n", imgName)

	size, err := visualize.Visualize(title, imgName, startTime, endTime, aggregationPeriod, seriesMap)
	if err != nil {
		log.Println(err)
		return nil
	}

	file, err := os.Open(imgName)
	if err != nil {
		return err
	}

	_, err = h.client.UploadFileV2Context(ctx, slack.UploadFileV2Parameters{
		Reader:   file,
		FileSize: int(size),
		Filename: imgName,
		Channel:  channelId,
	})
	if err != nil {
		log.Println(err)
		return err
	}

	fields := []slack.AttachmentField{}
	for k, v := range *seriesMap {
		var total int64
		for _, p := range v {
			total += int64(p.Val)
		}
		fields = append(fields, slack.AttachmentField{
			Title: k,
			Value: fmt.Sprint(total),
			Short: true,
		})
	}

	attachment := slack.Attachment{
		Text:       title,
		Fields:     fields,
		Color:      "good",
		CallbackID: ActionIdMetrics,
		Actions: []slack.AttachmentAction{
			{
				Name: "duration",
				Text: "Duration",
				Type: "select",
				Options: []slack.AttachmentActionOption{
					{
						Text:  "1h",
						Value: "1h",
					},
					{
						Text:  "1d",
						Value: "24h",
					},
					{
						Text:  "1w",
						Value: "168h",
					},
				},
			},
			{
				Name: "metrics",
				Text: "Metrics",
				Type: "select",
				Options: []slack.AttachmentActionOption{
					{
						Text:  "latency",
						Value: "latency",
					},
					{
						Text:  "count",
						Value: "count",
					},
				},
			},
		},
	}
	_, _, err = h.client.PostMessageContext(
		ctx, channelId,
		slack.MsgOptionText(fmt.Sprintf("`%s`", svcName), false),
		slack.MsgOptionAttachments(attachment),
	)
	return err
}

func (h *MultiProjectSlackEventHandler) sample(ctx context.Context, channelId string) error {
	imgName := path.Join(h.tmpDir, "sample.png")
	err := visualize.VisualizeSample(imgName)
	if err != nil {
		return err
	}
	file, err := os.Open(imgName)
	if err != nil {
		return err
	}
	stat, err := file.Stat()
	if err != nil {
		return err
	}
	fSummary, err := h.client.UploadFileV2Context(ctx, slack.UploadFileV2Parameters{
		Reader:   file,
		FileSize: int(stat.Size()),
		Filename: imgName,
		Channel:  channelId,
	})
	log.Println(fSummary)
	return err
}

func (h *MultiProjectSlackEventHandler) help(ctx context.Context, channelId, userId string) error {
	fields := []slack.AttachmentField{
		{
			Title: "`describe` or `d`",
			Value: "describe the target Cloud Run service or job from any configured project.\n you can check the latest revision, last modifier, update time, etc.",
		},
		{
			Title: "`metrics` or `m`",
			Value: "show the request count of the target Cloud Run service or job description.\n for services, you can check the request count per revision across all projects.",
		},
		{
			Title: "`set` or `s`",
			Value: "set the target Cloud Run service or job from any configured project.\n this displays a list of both services and jobs from all projects to select from.",
		},
	}

	// Add debug command if enabled
	if h.debugger != nil {
		fields = append(fields, slack.AttachmentField{
			Title: "`debug` or `dbg`",
			Value: "analyze recent error logs for the target Cloud Run service or job using AI.\n groups similar errors and provides root cause analysis and suggestions.",
		})
	}

	attachment := slack.Attachment{
		Text:   "Available commands (Multi-Project Mode):",
		Fields: fields,
	}
	_, err := h.client.PostEphemeralContext(
		ctx, channelId, userId,
		slack.MsgOptionText("Usage: @<slack app> <command> e.g. `@cloud-run-bot describe`", false),
		slack.MsgOptionAttachments(attachment),
	)
	return err
}

func (h *MultiProjectSlackEventHandler) debugResource(ctx context.Context, channelId, userId, resourceValue string) error {
	projectID, resourceType, resourceName, err := ParseMultiProjectResourceValue(resourceValue)
	if err != nil {
		return fmt.Errorf("failed to parse resource value: %v", err)
	}

	// Send initial "analyzing" message
	_, err = h.client.PostEphemeralContext(ctx, channelId, userId,
		slack.MsgOptionText(fmt.Sprintf("Analyzing errors for %s `%s` in project `%s`... This may take a moment.", resourceType, resourceName, projectID), false))
	if err != nil {
		log.Printf("Warning: failed to send analyzing message: %v", err)
	}

	// Run debug analysis
	result, err := h.debugger.DebugResource(ctx, projectID, resourceType, resourceName)
	if err != nil {
		_, _, postErr := h.client.PostMessageContext(ctx, channelId,
			slack.MsgOptionText(fmt.Sprintf("Debug analysis failed: %s", err.Error()), false))
		if postErr != nil {
			return postErr
		}
		return err
	}

	return h.postDebugResult(ctx, channelId, result)
}

func (h *MultiProjectSlackEventHandler) postDebugResult(ctx context.Context, channelId string, result *debug.DebugResult) error {
	if result.TotalErrors == 0 {
		_, _, err := h.client.PostMessageContext(ctx, channelId,
			slack.MsgOptionText(fmt.Sprintf("No errors found for %s `%s` in project `%s` (last %d minutes).",
				result.ResourceType, result.ResourceName, result.ProjectID, result.LookbackMin), false))
		return err
	}

	// Build header text
	headerText := fmt.Sprintf("*Debug Analysis: %s `%s`* (Project: `%s`)\nTime Range: Last %d minutes | Total Errors: %d | Error Groups: %d",
		result.ResourceType, result.ResourceName, result.ProjectID, result.LookbackMin, result.TotalErrors, len(result.ErrorGroups))

	// Build fields for each error group
	var fields []slack.AttachmentField
	for i, group := range result.ErrorGroups {
		// Summary field
		groupTitle := fmt.Sprintf("Group %d: %s (%d errors)", i+1, group.Pattern, group.ErrorCount)

		// Build group details
		var details strings.Builder
		details.WriteString(fmt.Sprintf("*Summary*: %s\n", group.Analysis.Summary))

		if len(group.Analysis.PossibleCauses) > 0 {
			details.WriteString("*Possible Causes*:\n")
			for _, cause := range group.Analysis.PossibleCauses {
				details.WriteString(fmt.Sprintf("  - %s\n", cause))
			}
		}

		if len(group.Analysis.Suggestions) > 0 {
			details.WriteString("*Suggestions*:\n")
			for _, suggestion := range group.Analysis.Suggestions {
				details.WriteString(fmt.Sprintf("  - %s\n", suggestion))
			}
		}

		if group.TraceID != "" {
			details.WriteString(fmt.Sprintf("*Sample Trace*: `%s`", group.TraceID))
		}

		fields = append(fields, slack.AttachmentField{
			Title: groupTitle,
			Value: details.String(),
			Short: false,
		})
	}

	attachment := slack.Attachment{
		Color:  "danger",
		Fields: fields,
		Footer: fmt.Sprintf("Generated at %s", result.GeneratedAt.Format("2006/01/02 15:04:05")),
	}

	_, _, err := h.client.PostMessageContext(ctx, channelId,
		slack.MsgOptionText(headerText, false),
		slack.MsgOptionAttachments(attachment))
	return err
}
