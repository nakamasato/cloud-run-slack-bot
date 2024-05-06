package pubsub

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	internalslack "github.com/nakamasato/cloud-run-slack-bot/pkg/slack"
	"github.com/slack-go/slack"
	"google.golang.org/protobuf/types/known/timestamppb"
)

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
	} `json:"protoPayload"`
	Timestamp timestamppb.Timestamp `json:"timestamp"`
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

	log.Printf("Method Name: %s, Request Name: %s", methodName, serviceName)

	if h.channel == "" {
		log.Println("Slack channel not set")
		return
	}

	attachment := slack.Attachment{
		Text: "Cloud Run audit event",
		Fields: []slack.AttachmentField{
			{
				Title: "Timestamp",
				Value: fmt.Sprintf("<!date^%d^{date} {time}|%d>", logEntry.Timestamp.Seconds, logEntry.Timestamp.Seconds),
			},
			{
				Title: "Method",
				Value: methodName,
				Short: true,
			},
			{
				Title: "Service",
				Value: serviceName,
				Short: true,
			},
			{
				Title: "message.Data",
				Value: fmt.Sprintf("```\n%s\n```", m.Message.Data),
				Short: false,
			},
		},
		Color: "good",
	}
	_, _, err = h.client.PostMessage(h.channel, slack.MsgOptionAttachments(attachment))
	if err != nil {
		log.Printf("slack.PostMessage: %v", err)
		http.Error(w, "Failed to post Slack message", http.StatusInternalServerError)
		return
	}
}
