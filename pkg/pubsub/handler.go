package pubsub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/nakamasato/cloud-run-slack-bot/pkg/config"
	internalslack "github.com/nakamasato/cloud-run-slack-bot/pkg/slack"
	"github.com/slack-go/slack"
	"go.uber.org/zap"
)

var boolEmoji = map[bool]string{
	true:  "✅",
	false: "👀",
}

// Color can be good, warning, danger, or any hex color code (eg. #439FE0).
func getColor(severity string) string {
	if color, ok := severityColor[severity]; ok {
		return color
	}
	return "#D3D3D3" // light gray
}

var severityColor = map[string]string{
	"NOTICE": "good",
	"INFO":   "good",
	"ERROR":  "danger",
}

// PubSubMessage is the payload of a Pub/Sub event.
// See the documentation for more details:
// https://cloud.google.com/pubsub/docs/reference/rest/v1/PubsubMessage
type PubSubMessage struct {
	Message struct {
		Data []byte `json:"data,omitempty"`
		ID   string `json:"id"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

// LogEntry https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry
type CloudRunAuditLog struct {
	Resource struct {
		Labels map[string]string `json:"labels"`
		Type   string            `json:"type"`
	} `json:"resource"`
	Severity     string `json:"severity"`
	LogName      string `json:"logName"`
	ProtoPayload struct {
		Status struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"status"`
		ResourceName string `json:"resourceName"`
		MethodName   string `json:"methodName"`
		Request      struct {
			Name string `json:"name"`
		} `json:"request"`
		Response struct {
			Status struct {
				LatestCreatedRevisionName string `json:"latestCreatedRevisionName"`
				LatestReadyRevisionName   string `json:"latestReadyRevisionName"`
				Traffic                   []struct {
					LatestRevision bool   `json:"latestRevision"`
					Percent        int    `json:"percent"`
					RevisionName   string `json:"revisionName"`
					Tag            string `json:"tag"`
				} `json:"traffic"`
				// For Jobs
				LatestCreatedExecutionName string `json:"latestCreatedExecutionName"`
				Conditions                 []struct {
					Type    string `json:"type"`
					Status  string `json:"status"`
					Reason  string `json:"reason"`
					Message string `json:"message"`
				} `json:"conditions"`
			} `json:"status"`
			Metadata struct {
				Generation  int `json:"generation"`
				Annotations struct {
					LastModifier string `json:"serving.knative.dev/lastModifier"`
				} `json:"annotations"`
			} `json:"metadata"`
		} `json:"response"`
	} `json:"protoPayload"`
}

type CloudRunAuditLogHandler struct {
	// Slack Client
	client         internalslack.Client
	channels       map[string]string // Maps service/job names to Slack channel names
	defaultChannel string            // Default channel for services/jobs not in the mapping
	logger         *zap.Logger
}

func NewCloudRunAuditLogHandler(channels map[string]string, defaultChannel string, client internalslack.Client, logger *zap.Logger) *CloudRunAuditLogHandler {
	return &CloudRunAuditLogHandler{
		client:         client,
		channels:       channels,
		defaultChannel: defaultChannel,
		logger:         logger,
	}
}

// HandleCloudRunAuditLogs receives and processes a Pub/Sub push message.
func (h *CloudRunAuditLogHandler) HandleCloudRunAuditLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := h.logger.With(zap.String("handler", "CloudRunAuditLogHandler"))

	var m PubSubMessage
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("Failed to read request body", zap.Error(err))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	// byte slice unmarshalling handles base64 decoding.
	if err := json.Unmarshal(body, &m); err != nil {
		logger.Error("Failed to unmarshal PubSub message", zap.Error(err))
		http.Error(w, "Failed to parse PubSub message", http.StatusBadRequest)
		return
	}

	logger.Debug("Received Cloud Run audit log", zap.String("message_data", string(m.Message.Data)))

	var logEntry CloudRunAuditLog
	if err := json.Unmarshal(m.Message.Data, &logEntry); err != nil {
		logger.Error("Failed to unmarshal log entry", zap.Error(err))
		http.Error(w, "Failed to parse logEntry", http.StatusBadRequest)
		return
	}

	methodName := logEntry.ProtoPayload.MethodName

	var jobOrSvcName string // job_name or service_name
	var resourceType string // job or service
	jobName := logEntry.Resource.Labels["job_name"]
	serviceName := logEntry.Resource.Labels["service_name"]
	if jobName != "" {
		jobOrSvcName = jobName
		resourceType = "job"
	} else if serviceName != "" {
		jobOrSvcName = serviceName
		resourceType = "service"
	} else {
		logger.Warn("No job or service name found in the log entry")
	}

	lastModifier := logEntry.ProtoPayload.Response.Metadata.Annotations.LastModifier
	generation := logEntry.ProtoPayload.Response.Metadata.Generation

	// Service specific fields
	latestReadyRevision := logEntry.ProtoPayload.Response.Status.LatestReadyRevisionName
	latestCreatedRevision := logEntry.ProtoPayload.Response.Status.LatestCreatedRevisionName

	// Job specific fields
	latestCreatedExecution := logEntry.ProtoPayload.Response.Status.LatestCreatedExecutionName

	logger.Info("Processing Cloud Run audit log",
		zap.String("method", methodName),
		zap.String("resource_name", jobOrSvcName),
		zap.String("resource_type", resourceType),
	)

	// Get the channel for this service/job, or use the default channel
	channel, ok := h.channels[jobOrSvcName]
	if !ok {
		channel = h.defaultChannel
	}
	if channel == "" {
		logger.Warn("No channel found for resource",
			zap.String("resource_name", jobOrSvcName),
			zap.String("resource_type", resourceType),
		)
		return
	}
	logger.Info("Set Slack channel for resource",
		zap.String("channel", channel),
		zap.String("resource_name", jobOrSvcName),
		zap.String("resource_type", resourceType),
	)

	fields := []slack.AttachmentField{
		{
			Title: resourceType,
			Value: jobOrSvcName,
			Short: true,
		},
	}
	if resourceName := logEntry.ProtoPayload.ResourceName; resourceName != "" {
		parts := strings.Split(resourceName, "/")
		shortName := parts[len(parts)-1]

		if shortName != jobOrSvcName { // only when short name is different from jobOrSvcName e.g. revision name, execution name
			fields = append(fields, slack.AttachmentField{
				Title: "ResourceName",
				Value: shortName,
				Short: true,
			})
		}
	}
	if methodName != "" {
		fields = append(fields, slack.AttachmentField{
			Title: "Method",
			Value: methodName,
			Short: true,
		})
	}

	if resourceType == "job" {
		// Job-specific fields
		if latestCreatedExecution != "" {
			fields = append(fields, slack.AttachmentField{
				Title: "Latest Created Execution",
				Value: fmt.Sprintf("`%s`", latestCreatedExecution),
				Short: true,
			})
		}

		// Add job conditions if available
		conditions := []string{}
		for _, condition := range logEntry.ProtoPayload.Response.Status.Conditions {
			conditions = append(conditions, fmt.Sprintf("- `%s`: %s (%s)", condition.Type, condition.Status, condition.Reason))
		}
		if len(conditions) > 0 {
			fields = append(fields, slack.AttachmentField{
				Title: "Conditions",
				Value: strings.Join(conditions, "\n"),
			})
		}
	} else {
		// Service-specific fields
		if latestCreatedRevision != "" {
			fields = append(fields, slack.AttachmentField{
				Title: "Latest Created Revision",
				Value: fmt.Sprintf("`%s` (%s)", latestCreatedRevision, boolEmoji[latestReadyRevision == latestCreatedRevision]),
				Short: true,
			})
		}

	revisions := []string{}
	for _, traffic := range logEntry.ProtoPayload.Response.Status.Traffic {
		revision := fmt.Sprintf("- `%s` (%d%%)", traffic.RevisionName, traffic.Percent)
		if traffic.Tag != "" {
			revision = fmt.Sprintf("%s [%s]", revision, traffic.Tag)
		}
		if traffic.LatestRevision {
			revision = fmt.Sprintf("%s ✅", revision)
		}
		revisions = append(revisions, revision)
		}
		if len(revisions) > 0 {
			fields = append(fields, slack.AttachmentField{
				Title: "Traffic Revisions",
				Value: strings.Join(revisions, "\n"),
			})
		}
	}
	if logEntry.Severity == "ERROR" {
		fields = append(fields, slack.AttachmentField{
			Title: "Error",
			Value: fmt.Sprintf("Code: %d\nMessage: %s", logEntry.ProtoPayload.Status.Code, logEntry.ProtoPayload.Status.Message),
		})
	}

	fields = append(fields, slack.AttachmentField{
		Title: "Severity",
		Value: logEntry.Severity,
		Short: true,
	})

	text := ""
	if logEntry.ProtoPayload.Status.Message != "" {
		text = logEntry.ProtoPayload.Status.Message
	} else if lastModifier != "" {
		text = fmt.Sprintf("Cloud Run %s `%s` has been modified by `%s` (generation: %d).", resourceType, jobOrSvcName, lastModifier, generation)
	} else {
		text = fmt.Sprintf("Cloud Run %s `%s` has been updated (generation: %d).", resourceType, jobOrSvcName, generation)
	}

	attachment := slack.Attachment{
		Text:   text,
		Fields: fields,
		Color:  getColor(logEntry.Severity),
	}

	_, _, err = h.client.PostMessage(channel,
		slack.MsgOptionAttachments(attachment),
	)
	if err != nil {
		logger.Error("Failed to post Slack message", zap.Error(err))
		http.Error(w, "Failed to post Slack message", http.StatusInternalServerError)
		return
	}

	_ = ctx // Suppress unused variable warning if not using trace context
}

// MultiProjectCloudRunAuditLogHandler handles audit logs for multiple projects
type MultiProjectCloudRunAuditLogHandler struct {
	client internalslack.Client
	config *config.Config
	logger *zap.Logger
}

func NewMultiProjectCloudRunAuditLogHandler(cfg *config.Config, client internalslack.Client, logger *zap.Logger) *MultiProjectCloudRunAuditLogHandler {
	return &MultiProjectCloudRunAuditLogHandler{
		client: client,
		config: cfg,
		logger: logger,
	}
}

func (h *MultiProjectCloudRunAuditLogHandler) HandleCloudRunAuditLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := h.logger.With(zap.String("handler", "MultiProjectCloudRunAuditLogHandler"))

	var m PubSubMessage
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("Failed to read request body", zap.Error(err))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if err := json.Unmarshal(body, &m); err != nil {
		logger.Error("Failed to unmarshal PubSub message", zap.Error(err))
		http.Error(w, "Failed to parse PubSub message", http.StatusBadRequest)
		return
	}

	logger.Debug("Received Cloud Run audit log", zap.String("message_data", string(m.Message.Data)))

	var logEntry CloudRunAuditLog
	if err := json.Unmarshal(m.Message.Data, &logEntry); err != nil {
		logger.Error("Failed to unmarshal log entry", zap.Error(err))
		http.Error(w, "Failed to parse logEntry", http.StatusBadRequest)
		return
	}

	methodName := logEntry.ProtoPayload.MethodName
	projectID := logEntry.Resource.Labels["project_id"]
	if projectID == "" {
		logger.Warn("No project_id found in the log entry")
		return
	}

	var jobOrSvcName string
	var resourceType string
	jobName := logEntry.Resource.Labels["job_name"]
	serviceName := logEntry.Resource.Labels["service_name"]
	if jobName != "" {
		jobOrSvcName = jobName
		resourceType = "job"
	} else if serviceName != "" {
		jobOrSvcName = serviceName
		resourceType = "service"
	} else {
		logger.Warn("No job or service name found in the log entry")
		return
	}

	lastModifier := logEntry.ProtoPayload.Response.Metadata.Annotations.LastModifier
	generation := logEntry.ProtoPayload.Response.Metadata.Generation

	// Service specific fields
	latestReadyRevision := logEntry.ProtoPayload.Response.Status.LatestReadyRevisionName
	latestCreatedRevision := logEntry.ProtoPayload.Response.Status.LatestCreatedRevisionName

	// Job specific fields
	latestCreatedExecution := logEntry.ProtoPayload.Response.Status.LatestCreatedExecutionName

	logger.Info("Processing Cloud Run audit log",
		zap.String("method", methodName),
		zap.String("project_id", projectID),
		zap.String("resource_name", jobOrSvcName),
		zap.String("resource_type", resourceType),
	)

	// Get the channel for this service/job using the multi-project configuration
	channel := h.config.GetChannelForService(projectID, jobOrSvcName)
	if channel == "" {
		logger.Warn("No channel found for resource",
			zap.String("resource_name", jobOrSvcName),
			zap.String("resource_type", resourceType),
			zap.String("project_id", projectID),
		)
		return
	}
	logger.Info("Set Slack channel for resource",
		zap.String("channel", channel),
		zap.String("resource_name", jobOrSvcName),
		zap.String("resource_type", resourceType),
		zap.String("project_id", projectID),
	)

	fields := []slack.AttachmentField{
		{
			Title: "Project",
			Value: projectID,
			Short: true,
		},
		{
			Title: resourceType,
			Value: jobOrSvcName,
			Short: true,
		},
	}

	if resourceName := logEntry.ProtoPayload.ResourceName; resourceName != "" {
		parts := strings.Split(resourceName, "/")
		shortName := parts[len(parts)-1]

		if shortName != jobOrSvcName {
			fields = append(fields, slack.AttachmentField{
				Title: "ResourceName",
				Value: shortName,
				Short: true,
			})
		}
	}

	if methodName != "" {
		fields = append(fields, slack.AttachmentField{
			Title: "Method",
			Value: methodName,
			Short: true,
		})
	}

	if resourceType == "job" {
		// Job-specific fields
		if latestCreatedExecution != "" {
			fields = append(fields, slack.AttachmentField{
				Title: "Latest Created Execution",
				Value: fmt.Sprintf("`%s`", latestCreatedExecution),
				Short: true,
			})
		}

		// Add job conditions if available
		conditions := []string{}
		for _, condition := range logEntry.ProtoPayload.Response.Status.Conditions {
			conditions = append(conditions, fmt.Sprintf("- `%s`: %s (%s)", condition.Type, condition.Status, condition.Reason))
		}
		if len(conditions) > 0 {
			fields = append(fields, slack.AttachmentField{
				Title: "Conditions",
				Value: strings.Join(conditions, "\n"),
			})
		}
	} else {
		// Service-specific fields
		if latestCreatedRevision != "" {
			fields = append(fields, slack.AttachmentField{
				Title: "Latest Created Revision",
				Value: fmt.Sprintf("`%s` (%s)", latestCreatedRevision, boolEmoji[latestReadyRevision == latestCreatedRevision]),
				Short: true,
			})
		}

	revisions := []string{}
	for _, traffic := range logEntry.ProtoPayload.Response.Status.Traffic {
		revision := fmt.Sprintf("- `%s` (%d%%)", traffic.RevisionName, traffic.Percent)
		if traffic.Tag != "" {
			revision = fmt.Sprintf("%s [%s]", revision, traffic.Tag)
		}
		if traffic.LatestRevision {
			revision = fmt.Sprintf("%s ✅", revision)
		}
		revisions = append(revisions, revision)
		}
		if len(revisions) > 0 {
			fields = append(fields, slack.AttachmentField{
				Title: "Traffic Revisions",
				Value: strings.Join(revisions, "\n"),
			})
		}
	}

	if logEntry.Severity == "ERROR" {
		fields = append(fields, slack.AttachmentField{
			Title: "Error",
			Value: fmt.Sprintf("Code: %d\nMessage: %s", logEntry.ProtoPayload.Status.Code, logEntry.ProtoPayload.Status.Message),
		})
	}

	fields = append(fields, slack.AttachmentField{
		Title: "Severity",
		Value: logEntry.Severity,
		Short: true,
	})

	text := ""
	if logEntry.ProtoPayload.Status.Message != "" {
		text = logEntry.ProtoPayload.Status.Message
	} else if lastModifier != "" {
		text = fmt.Sprintf("Cloud Run %s `%s` in project `%s` has been modified by `%s` (generation: %d).", resourceType, jobOrSvcName, projectID, lastModifier, generation)
	} else {
		text = fmt.Sprintf("Cloud Run %s `%s` in project `%s` has been updated (generation: %d).", resourceType, jobOrSvcName, projectID, generation)
	}

	attachment := slack.Attachment{
		Text:   text,
		Fields: fields,
		Color:  getColor(logEntry.Severity),
	}

	_, _, err = h.client.PostMessage(channel,
		slack.MsgOptionAttachments(attachment),
	)
	if err != nil {
		logger.Error("Failed to post Slack message", zap.Error(err))
		http.Error(w, "Failed to post Slack message", http.StatusInternalServerError)
		return
	}

	_ = ctx // Suppress unused variable warning if not using trace context
}
