package pubsub

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
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

// HandleCloudRunAuditLogs receives and processes a Pub/Sub push message.
func HandleCloudRunAuditLogs(w http.ResponseWriter, r *http.Request) {
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
	requestName := logEntry.ProtoPayload.Request.Name

	log.Printf("Method Name: %s, Request Name: %s", methodName, requestName)
}
