package pubsub

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	internalslack "github.com/nakamasato/cloud-run-slack-bot/pkg/slack"
	"github.com/slack-go/slack"
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
}

func NewCloudRunAuditLogHandler(channels map[string]string, defaultChannel string, client internalslack.Client) *CloudRunAuditLogHandler {
	return &CloudRunAuditLogHandler{
		client:         client,
		channels:       channels,
		defaultChannel: defaultChannel,
	}
}

// HandleCloudRunAuditLogs receives and processes a Pub/Sub push message.
func (h *CloudRunAuditLogHandler) HandleCloudRunAuditLogs(w http.ResponseWriter, r *http.Request) {
	var m PubSubMessage
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("ioutil.ReadAll: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	// byte slice unmarshalling handles base64 decoding.
	if err := json.Unmarshal(body, &m); err != nil {
		log.Printf("json.Unmarshal: %v", err)
		http.Error(w, "Failed to parse PubSub message", http.StatusBadRequest)
		return
	}

	log.Printf("Cloud Run audit log message.Data: %s\n", string(m.Message.Data))

	var logEntry CloudRunAuditLog
	if err := json.Unmarshal(m.Message.Data, &logEntry); err != nil {
		log.Printf("json.Unmarshal: %v", err)
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
	} else {
		jobOrSvcName = serviceName
		resourceType = "service"
	}

	lastModifier := logEntry.ProtoPayload.Response.Metadata.Annotations.LastModifier
	generation := logEntry.ProtoPayload.Response.Metadata.Generation

	// Service specific fields
	latestReadyRevision := logEntry.ProtoPayload.Response.Status.LatestReadyRevisionName
	latestCreatedRevision := logEntry.ProtoPayload.Response.Status.LatestCreatedRevisionName

	// Job specific fields
	latestCreatedExecution := logEntry.ProtoPayload.Response.Status.LatestCreatedExecutionName

	log.Printf("Method Name: %s, Resource Name: %s, Resource Type: %s", methodName, jobOrSvcName, resourceType)

	// Get the channel for this service/job, or use the default channel
	channel, ok := h.channels[jobOrSvcName]
	if !ok {
		channel = h.defaultChannel
	}
	log.Printf("Set Channel to '%s' for '%s'(%s)", channel, jobOrSvcName, resourceType)

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
			revisions = append(revisions, fmt.Sprintf("- `%s` (%d%%) (latest: %s)", traffic.RevisionName, traffic.Percent, boolEmoji[traffic.LatestRevision]))
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
		log.Printf("slack.PostMessage: %v", err)
		http.Error(w, "Failed to post Slack message", http.StatusInternalServerError)
		return
	}
}
