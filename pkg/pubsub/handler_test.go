package pubsub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	slackinternal "github.com/nakamasato/cloud-run-slack-bot/pkg/slack"
)

func TestCloudRunAuditLogHandler(t *testing.T) {
	tests := []struct {
		name           string
		serviceName    string
		channels       map[string]string
		defaultChannel string
		wantStatus     int
	}{
		{
			name:           "service with specific channel",
			serviceName:    "test-service",
			channels:       map[string]string{"test-service": "test-channel"},
			defaultChannel: "default-channel",
			wantStatus:     http.StatusOK,
		},
		{
			name:           "service using default channel",
			serviceName:    "other-service",
			channels:       map[string]string{"test-service": "test-channel"},
			defaultChannel: "default-channel",
			wantStatus:     http.StatusOK,
		},
		{
			name:           "service with no channel and no default",
			serviceName:    "other-service",
			channels:       map[string]string{"test-service": "test-channel"},
			defaultChannel: "",
			wantStatus:     http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Prepare a sample Pub/Sub message payload
			payload := PubSubMessage{
				Message: struct {
					Data []byte `json:"data,omitempty"`
					ID   string `json:"id"`
				}{
					Data: []byte(fmt.Sprintf(`{
						"resource": {
							"labels": {
								"service_name": "%s"
							},
							"type": "cloud_run_revision"
						},
						"severity": "NOTICE",
						"protoPayload": {
							"methodName": "google.cloud.run.v1.Services.ReplaceService",
							"request": {
								"name": "projects/test-project/locations/asia-northeast1/services/%s"
							},
							"response": {
								"metadata": {
									"generation": 1,
									"annotations": {
										"serving.knative.dev/lastModifier": "test@example.com"
									}
								}
							}
						}
					}`, tt.serviceName, tt.serviceName)),
					ID: "1",
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
				client:         &dummy,
				channels:       tt.channels,
				defaultChannel: tt.defaultChannel,
			}
			handler := http.HandlerFunc(auditHandler.HandleCloudRunAuditLogs)

			handler.ServeHTTP(rr, req)
			if status := rr.Code; status != tt.wantStatus {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.wantStatus)
			}
		})
	}
}
