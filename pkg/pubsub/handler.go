package pubsub

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/slack-go/slack"
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

type CloudRunAuditLog struct {
	ProtoPayload struct {
		MethodName string `json:"methodName"`
		Request    struct {
			Name string `json:"name"`
		} `json:"request"`
	} `json:"protoPayload"`
}

type CloudRunAuditLogHandler struct {
	// Slack Client
	client  *slack.Client
	channel string
}

func NewCloudRunAuditLogHandler(channel string, client *slack.Client) *CloudRunAuditLogHandler {
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
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	var logEntry CloudRunAuditLog
	if err := json.Unmarshal(m.Message.Data, &logEntry); err != nil {
		log.Printf("json.Unmarshal: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	methodName := logEntry.ProtoPayload.MethodName
	serviceName := logEntry.ProtoPayload.Request.Name

	log.Printf("Method Name: %s, Request Name: %s", methodName, serviceName)

	if h.channel == "" {
		log.Println("Slack channel not set")
		return
	}

	attachment := slack.Attachment{
		Text: "Cloud Run audit event",
		Fields: []slack.AttachmentField{
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
