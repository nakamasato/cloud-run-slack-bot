package pubsub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	slackinternal "github.com/nakamasato/cloud-run-slack-bot/pkg/slack"
	"go.uber.org/zap"
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
				logger:         zap.NewNop(), // Use no-op logger for tests
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



// Test revision formatting logic directly
func TestRevisionFormatting(t *testing.T) {
	tests := []struct {
		name                string
		traffic             []struct {
			LatestRevision bool
			Percent        int
			RevisionName   string
			Tag            string
		}
		expectedContains    []string
		expectedNotContains []string
	}{
		{
			name: "traffic with tags",
			traffic: []struct {
				LatestRevision bool
				Percent        int
				RevisionName   string
				Tag            string
			}{
				{LatestRevision: true, Percent: 100, RevisionName: "my-service-00001", Tag: "production"},
				{LatestRevision: false, Percent: 0, RevisionName: "my-service-00002", Tag: "canary"},
			},
			expectedContains: []string{
				"`my-service-00001` (100%) [production] ✅",
				"`my-service-00002` (0%) [canary]",
			},
			expectedNotContains: []string{
				"👀",
			},
		},
		{
			name: "traffic without tags",
			traffic: []struct {
				LatestRevision bool
				Percent        int
				RevisionName   string
				Tag            string
			}{
				{LatestRevision: true, Percent: 100, RevisionName: "my-service-00001", Tag: ""},
				{LatestRevision: false, Percent: 0, RevisionName: "my-service-00002", Tag: ""},
			},
			expectedContains: []string{
				"`my-service-00001` (100%) ✅",
				"`my-service-00002` (0%)",
			},
			expectedNotContains: []string{
				"👀",
				"[]",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the formatting logic from the handler
			revisions := []string{}
			for _, traffic := range tt.traffic {
				revision := fmt.Sprintf("- `%s` (%d%%)", traffic.RevisionName, traffic.Percent)
				if traffic.Tag != "" {
					revision = fmt.Sprintf("%s [%s]", revision, traffic.Tag)
				}
				if traffic.LatestRevision {
					revision = fmt.Sprintf("%s ✅", revision)
				}
				revisions = append(revisions, revision)
			}

			output := fmt.Sprintf("%v", revisions)

			// Check expected content
			for _, expected := range tt.expectedContains {
				found := false
				for _, rev := range revisions {
					if containsString(rev, expected) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected to find %q in revisions, but got: %v", expected, revisions)
				}
			}

			// Check that unwanted content is not present
			for _, notExpected := range tt.expectedNotContains {
				for _, rev := range revisions {
					if containsString(rev, notExpected) {
						t.Errorf("Should not find %q in revision %q, output: %s", notExpected, rev, output)
					}
				}
			}
		})
	}
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if i+len(substr) <= len(s) && s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
