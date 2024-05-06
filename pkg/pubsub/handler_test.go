package pubsub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	slackinternal "github.com/nakamasato/cloud-run-slack-bot/pkg/slack"
)

func TestCloudRunAuditLogHandler(t *testing.T) {

	// Prepare a sample Pub/Sub message payload
	payload := PubSubMessage{
		Message: struct {
			Data []byte `json:"data,omitempty"`
			ID   string `json:"id"`
		}{
			Data: []byte(`{"protoPayload":{"methodName":"google.cloud.run.v1.Services.ReplaceService","request":{"name":"projects/test-project/locations/asia-northeast1/services/test-service"}}}`),
			ID:   "1",
		},
		Subscription: "test-subscription",
	}
	payloadBytes, _ := json.Marshal(payload)

	// Create a new HTTP request
	req, err := http.NewRequest("POST", "/cloudrun/events", bytes.NewBuffer(payloadBytes))
	if err != nil {
		t.Fatal(err)
	}
	// We create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
	rr := httptest.NewRecorder()
	dummy := slackinternal.DummySlackClient{}
	auditHandler := &CloudRunAuditLogHandler{
		client:  &dummy,
		channel: "test",
	}
	handler := http.HandlerFunc(auditHandler.HandleCloudRunAuditLogs)

	handler.ServeHTTP(rr, req)
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	expected := ``
	if rr.Body.String() != expected {
		t.Errorf("handler returned unexpected body: got %v want %v",
			rr.Body.String(), expected)
	}
}
