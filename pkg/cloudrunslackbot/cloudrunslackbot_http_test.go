package cloudrunslackbot

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	slackinternal "github.com/nakamasato/cloud-run-slack-bot/pkg/slack"
	"github.com/slack-go/slack"
)

func TestSlackEventsVerification(t *testing.T) {
	signingSecret := "test_secret"
	handler := &slackinternal.SlackEventHandler{}
	channels := map[string]string{"test-service": "test-channel"}
	defaultChannel := "default-channel"
	svc := NewCloudRunSlackBotHttp(channels, defaultChannel, &slack.Client{}, handler, signingSecret)

	tests := []struct {
		name           string
		body           string
		validSignature bool
		wantStatus     int
	}{
		{
			name:           "valid signature events",
			body:           `{"type":"url_verification","challenge":"test"}`,
			validSignature: true,
			wantStatus:     http.StatusOK,
		},
		{
			name:           "invalid signature events",
			body:           `{"type":"url_verification","challenge":"test"}`,
			validSignature: false,
			wantStatus:     http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/slack/events", bytes.NewBufferString(tt.body))
			timestamp := fmt.Sprintf("%d", time.Now().Unix())
			req.Header.Set("X-Slack-Request-Timestamp", timestamp)

			if tt.validSignature {
				// Generate valid signature
				hash := hmac.New(sha256.New, []byte(signingSecret))
				hash.Write([]byte(fmt.Sprintf("v0:%s:%s", timestamp, tt.body)))
				sig := hex.EncodeToString(hash.Sum(nil))
				req.Header.Set("X-Slack-Signature", "v0="+sig)
			} else {
				req.Header.Set("X-Slack-Signature", "v0=0000000000000000000000000000000000000000")
			}

			w := httptest.NewRecorder()
			handler := svc.SlackEventsHandler()
			handler(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}
