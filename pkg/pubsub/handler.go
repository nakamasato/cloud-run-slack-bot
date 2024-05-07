package pubsub

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	internalslack "github.com/nakamasato/cloud-run-slack-bot/pkg/slack"
	"github.com/slack-go/slack"
)

var boolEmoji = map[bool]string{
	true:  "✅",
	false: "👀",
}

// Color can be good, warning, danger, or any hex color code (eg. #439FE0).
var boolColor = map[bool]string{
	true:  "good",
	false: "warning",
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
	} `json:"resource"`
	ProtoPayload struct {
		MethodName string `json:"methodName"`
		Request    struct {
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
			} `json:"status"`
		} `json:"response"`
	} `json:"protoPayload"`
}

type CloudRunAuditLogHandler struct {
	// Slack Client
	client  internalslack.Client
	channel string
}

func NewCloudRunAuditLogHandler(channel string, client internalslack.Client) *CloudRunAuditLogHandler {
	return &CloudRunAuditLogHandler{
		client:  client,
		channel: channel,
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
	serviceName := logEntry.Resource.Labels["service_name"]
	latestReadyRevision := logEntry.ProtoPayload.Response.Status.LatestReadyRevisionName
	latestCreatedRevision := logEntry.ProtoPayload.Response.Status.LatestCreatedRevisionName

	log.Printf("Method Name: %s, Request Name: %s", methodName, serviceName)

	if h.channel == "" {
		log.Println("Slack channel not set")
		return
	}

	fields := []slack.AttachmentField{
		{
			Title: "Method",
			Value: methodName,
			Short: true,
		},
	}

	fields = append(fields, slack.AttachmentField{
		Title: "Latest Revision Status",
		Value: fmt.Sprintf("latestCreatedRevision: %s (%s)", latestCreatedRevision, boolEmoji[latestReadyRevision == latestCreatedRevision]),
		Short: true,
	})

	for _, traffic := range logEntry.ProtoPayload.Response.Status.Traffic {
		fields = append(fields, slack.AttachmentField{
			Title: "Traffic Revision",
			Value: fmt.Sprintf("%s (%d%%) (latest: %s)\n", traffic.RevisionName, traffic.Percent, boolEmoji[traffic.LatestRevision]),
		})
	}

	attachment := slack.Attachment{
		Text:   serviceName,
		Fields: fields,
		Color:  boolColor[latestReadyRevision == latestCreatedRevision],
	}
	_, _, err = h.client.PostMessage(h.channel,
		slack.MsgOptionText(fmt.Sprintf("Cloud Run service '%s' has been updated.\n", serviceName), false),
		slack.MsgOptionAttachments(attachment),
	)
	if err != nil {
		log.Printf("slack.PostMessage: %v", err)
		http.Error(w, "Failed to post Slack message", http.StatusInternalServerError)
		return
	}
}
