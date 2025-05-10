package cloudrunslackbot

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/nakamasato/cloud-run-slack-bot/pkg/logger"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/pubsub"
	slackinternal "github.com/nakamasato/cloud-run-slack-bot/pkg/slack"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/trace"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"go.uber.org/zap"
)

type CloudRunSlackBotHttp struct {
	client        *slack.Client
	slackHandler  *slackinternal.SlackEventHandler
	auditHandler  *pubsub.CloudRunAuditLogHandler
	signingSecret string
}

func NewCloudRunSlackBotHttp(channels map[string]string, defaultChannel string, sClient *slack.Client, handler *slackinternal.SlackEventHandler, signingSecret string) *CloudRunSlackBotHttp {
	return &CloudRunSlackBotHttp{
		client:        sClient,
		slackHandler:  handler,
		auditHandler:  pubsub.NewCloudRunAuditLogHandler(channels, defaultChannel, sClient),
		signingSecret: signingSecret,
	}
}

// Run starts the HTTP server with instrumentation
func (svc *CloudRunSlackBotHttp) Run() {
	// Create a logger
	l, err := logger.NewLogger()
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}

	// Wrap handlers with OpenTelemetry instrumentation
	http.Handle("/slack/events", trace.WrapHandlerFunc(svc.SlackEventsHandler(), "slack_events"))
	http.Handle("/slack/interaction", trace.WrapHandlerFunc(svc.SlackInteractionHandler(), "slack_interaction"))
	http.Handle("/cloudrun/events", trace.WrapHandlerFunc(svc.auditHandler.HandleCloudRunAuditLogs, "cloudrun_events"))

	l.Info("Server listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		l.Fatal("Failed to start server", zap.Error(err))
	}
}

// SlackEventsHandler is http.HandlerFunc for Slack Events API
func (svc *CloudRunSlackBotHttp) SlackEventsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get logger from context
		ctx := r.Context()
		l := logger.FromContext(ctx)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			l.Error("Failed to read request body", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Verify the request signature
		sv, err := slack.NewSecretsVerifier(r.Header, svc.signingSecret)
		if err != nil {
			l.Error("Failed to create secrets verifier", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, err := sv.Write(body); err != nil {
			l.Error("Failed to write body to verifier", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := sv.Ensure(); err != nil {
			l.Error("Failed to verify request signature", zap.Error(err))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		eventsAPIEvent, err := slackevents.ParseEvent(json.RawMessage(body), slackevents.OptionNoVerifyToken())
		if err != nil {
			l.Error("Failed to parse Slack event", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		l.Info("Received Slack event",
			zap.String("event_type", string(eventsAPIEvent.Type)))

		switch eventsAPIEvent.Type {
		case slackevents.URLVerification:
			var res *slackevents.ChallengeResponse
			if err := json.Unmarshal(body, &res); err != nil {
				l.Error("Failed to unmarshal challenge response", zap.Error(err))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			if _, err := w.Write([]byte(res.Challenge)); err != nil {
				l.Error("Failed to write challenge response", zap.Error(err))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			l.Info("Responded to URL verification challenge")
		case slackevents.CallbackEvent:
			// Pass the context to maintain trace information
			err := svc.slackHandler.HandleEvent(ctx, &eventsAPIEvent)
			if err != nil {
				l.Error("Failed to handle callback event", zap.Error(err))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			l.Info("Successfully handled callback event")
		default:
			l.Warn("Received unknown event type", zap.String("type", string(eventsAPIEvent.Type)))
		}
	}
}

func (svc *CloudRunSlackBotHttp) SlackInteractionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get logger from context
		ctx := r.Context()
		l := logger.FromContext(ctx)

		payload := r.FormValue("payload")
		var interaction slack.InteractionCallback
		if err := json.Unmarshal([]byte(payload), &interaction); err != nil {
			l.Error("Failed to unmarshal interaction payload", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		l.Info("Received Slack interaction",
			zap.String("action_id", interaction.ActionID),
			zap.String("callback_id", interaction.CallbackID),
			zap.String("trigger_id", interaction.TriggerID),
			zap.String("user_id", interaction.User.ID))

		// Pass the context to maintain trace information
		if err := svc.slackHandler.HandleInteraction(ctx, &interaction); err != nil {
			l.Error("Failed to handle interaction", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		l.Info("Successfully handled interaction")
	}
}
