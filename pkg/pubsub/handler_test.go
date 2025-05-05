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
		resourceName   string
		resourceType   string // "service" or "job"
		methodName     string
		channels       map[string]string
		defaultChannel string
		wantStatus     int
	}{
		{
			name:           "service with specific channel",
			resourceName:   "test-service",
			resourceType:   "service",
			methodName:     "google.cloud.run.v1.Services.ReplaceService",
			channels:       map[string]string{"test-service": "test-channel"},
			defaultChannel: "default-channel",
			wantStatus:     http.StatusOK,
		},
		{
			name:           "service using default channel",
			resourceName:   "other-service",
			resourceType:   "service",
			methodName:     "google.cloud.run.v1.Services.ReplaceService",
			channels:       map[string]string{"test-service": "test-channel"},
			defaultChannel: "default-channel",
			wantStatus:     http.StatusOK,
		},
		{
			name:           "service with no channel and no default",
			resourceName:   "other-service",
			resourceType:   "service",
			methodName:     "google.cloud.run.v1.Services.ReplaceService",
			channels:       map[string]string{"test-service": "test-channel"},
			defaultChannel: "",
			wantStatus:     http.StatusOK, // no error but no message is sent to slack
		},
		{
			name:           "job with specific channel",
			resourceName:   "test-job",
			resourceType:   "job",
			methodName:     "google.cloud.run.v1.Jobs.ReplaceJob",
			channels:       map[string]string{"test-job": "test-channel"},
			defaultChannel: "default-channel",
			wantStatus:     http.StatusOK,
		},
		{
			name:           "job using default channel",
			resourceName:   "other-job",
			resourceType:   "job",
			methodName:     "google.cloud.run.v1.Jobs.ReplaceJob",
			channels:       map[string]string{"test-job": "test-channel"},
			defaultChannel: "default-channel",
			wantStatus:     http.StatusOK,
		},
		{
			name:           "job with no channel and no default",
			resourceName:   "other-job",
			resourceType:   "job",
			methodName:     "google.cloud.run.v1.Jobs.ReplaceJob",
			channels:       map[string]string{"test-job": "test-channel"},
			defaultChannel: "",
			wantStatus:     http.StatusOK, // no error but no message is sent to slack
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
								"%s_name": "%s"
							},
							"type": "%s"
						},
						"severity": "NOTICE",
						"protoPayload": {
							"methodName": "%s",
							"request": {
								"name": "projects/test-project/locations/asia-northeast1/%ss/%s"
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
					}`,
					tt.resourceType, tt.resourceName,
					func() string {
						if tt.resourceType == "job" {
							return "cloud_run_job"
						}
						return "cloud_run_revision"
					}(),
					tt.methodName, tt.resourceType, tt.resourceName)),
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
