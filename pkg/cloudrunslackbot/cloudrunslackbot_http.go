package cloudrunslackbot

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/nakamasato/cloud-run-slack-bot/pkg/config"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/pubsub"
	slackinternal "github.com/nakamasato/cloud-run-slack-bot/pkg/slack"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"go.uber.org/zap"
)

type CloudRunSlackBotHttp struct {
	client        *slack.Client
	slackHandler  *slackinternal.SlackEventHandler
	auditHandler  *pubsub.CloudRunAuditLogHandler
	signingSecret string
	logger        *zap.Logger
}

func NewCloudRunSlackBotHttp(channels map[string]string, defaultChannel string, sClient *slack.Client, handler *slackinternal.SlackEventHandler, signingSecret string, logger *zap.Logger) *CloudRunSlackBotHttp {
	return &CloudRunSlackBotHttp{
		client:        sClient,
		slackHandler:  handler,
		auditHandler:  pubsub.NewCloudRunAuditLogHandler(channels, defaultChannel, sClient, logger),
		signingSecret: signingSecret,
		logger:        logger,
	}
}

// SlackEventsHandler starts http server
func (svc *CloudRunSlackBotHttp) Run() {
	http.HandleFunc("/slack/events", svc.SlackEventsHandler())
	http.HandleFunc("/slack/interaction", svc.SlackInteractionHandler())
	http.HandleFunc("/cloudrun/events", svc.auditHandler.HandleCloudRunAuditLogs)
	svc.logger.Info("Server listening", zap.Int("port", 8080))
	if err := http.ListenAndServe(":8080", nil); err != nil {
		svc.logger.Fatal("Server failed to start", zap.Error(err))
	}
}

// SlackEventsHandler is http.HandlerFunc for Slack Events API
func (svc *CloudRunSlackBotHttp) SlackEventsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		logger := svc.logger.With(zap.String("handler", "SlackEventsHandler"))

		body, err := io.ReadAll(r.Body)
		if err != nil {
			logger.Error("Failed to read request body", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Verify the request signature
		sv, err := slack.NewSecretsVerifier(r.Header, svc.signingSecret)
		if err != nil {
			logger.Error("Failed to create secrets verifier", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, err := sv.Write(body); err != nil {
			logger.Error("Failed to write body to verifier", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := sv.Ensure(); err != nil {
			logger.Error("Failed to verify request signature", zap.Error(err))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		eventsAPIEvent, err := slackevents.ParseEvent(json.RawMessage(body), slackevents.OptionNoVerifyToken())
		if err != nil {
			logger.Error("Failed to parse Slack event", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		_ = ctx // Suppress unused variable warning

		switch eventsAPIEvent.Type {
		case slackevents.URLVerification:
			var res *slackevents.ChallengeResponse
			if err := json.Unmarshal(body, &res); err != nil {
				logger.Error("Failed to unmarshal URL verification", zap.Error(err))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			if _, err := w.Write([]byte(res.Challenge)); err != nil {
				logger.Error("Failed to write challenge response", zap.Error(err))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		case slackevents.CallbackEvent:
			err := svc.slackHandler.HandleEvent(&eventsAPIEvent)
			if err != nil {
				logger.Error("Failed to handle callback event", zap.Error(err))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
	}
}

func (svc *CloudRunSlackBotHttp) SlackInteractionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		logger := svc.logger.With(zap.String("handler", "SlackInteractionHandler"))

		payload := r.FormValue("payload")
		var interaction slack.InteractionCallback
		if err := json.Unmarshal([]byte(payload), &interaction); err != nil {
			logger.Error("Failed to unmarshal interaction payload", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if err := svc.slackHandler.HandleInteraction(&interaction); err != nil {
			logger.Error("Failed to handle interaction", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		_ = ctx // Suppress unused variable warning
	}
}

// MultiProjectCloudRunSlackBotHttp handles multi-project HTTP mode
type MultiProjectCloudRunSlackBotHttp struct {
	client        *slack.Client
	slackHandler  *slackinternal.MultiProjectSlackEventHandler
	auditHandler  *pubsub.MultiProjectCloudRunAuditLogHandler
	signingSecret string
	logger        *zap.Logger
}

func NewMultiProjectCloudRunSlackBotHttp(cfg *config.Config, sClient *slack.Client, handler *slackinternal.MultiProjectSlackEventHandler, logger *zap.Logger) *MultiProjectCloudRunSlackBotHttp {
	return &MultiProjectCloudRunSlackBotHttp{
		client:        sClient,
		slackHandler:  handler,
		auditHandler:  pubsub.NewMultiProjectCloudRunAuditLogHandler(cfg, sClient, logger),
		signingSecret: cfg.SlackSigningSecret,
		logger:        logger,
	}
}

func (svc *MultiProjectCloudRunSlackBotHttp) Run() {
	http.HandleFunc("/slack/events", svc.SlackEventsHandler())
	http.HandleFunc("/slack/interaction", svc.SlackInteractionHandler())
	http.HandleFunc("/cloudrun/events", svc.auditHandler.HandleCloudRunAuditLogs)
	svc.logger.Info("Server listening", zap.Int("port", 8080))
	if err := http.ListenAndServe(":8080", nil); err != nil {
		svc.logger.Fatal("Server failed to start", zap.Error(err))
	}
}

func (svc *MultiProjectCloudRunSlackBotHttp) SlackEventsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		logger := svc.logger.With(zap.String("handler", "MultiProjectSlackEventsHandler"))

		body, err := io.ReadAll(r.Body)
		if err != nil {
			logger.Error("Failed to read request body", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Verify the request signature
		sv, err := slack.NewSecretsVerifier(r.Header, svc.signingSecret)
		if err != nil {
			logger.Error("Failed to create secrets verifier", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, err := sv.Write(body); err != nil {
			logger.Error("Failed to write body to verifier", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := sv.Ensure(); err != nil {
			logger.Error("Failed to verify request signature", zap.Error(err))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		eventsAPIEvent, err := slackevents.ParseEvent(json.RawMessage(body), slackevents.OptionNoVerifyToken())
		if err != nil {
			logger.Error("Failed to parse Slack event", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		_ = ctx // Suppress unused variable warning

		switch eventsAPIEvent.Type {
		case slackevents.URLVerification:
			var res *slackevents.ChallengeResponse
			if err := json.Unmarshal(body, &res); err != nil {
				logger.Error("Failed to unmarshal URL verification", zap.Error(err))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			if _, err := w.Write([]byte(res.Challenge)); err != nil {
				logger.Error("Failed to write challenge response", zap.Error(err))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		case slackevents.CallbackEvent:
			err := svc.slackHandler.HandleEvent(&eventsAPIEvent)
			if err != nil {
				logger.Error("Failed to handle callback event", zap.Error(err))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
	}
}

func (svc *MultiProjectCloudRunSlackBotHttp) SlackInteractionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		logger := svc.logger.With(zap.String("handler", "MultiProjectSlackInteractionHandler"))

		payload := r.FormValue("payload")
		var interaction slack.InteractionCallback
		if err := json.Unmarshal([]byte(payload), &interaction); err != nil {
			logger.Error("Failed to unmarshal interaction payload", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if err := svc.slackHandler.HandleInteraction(&interaction); err != nil {
			logger.Error("Failed to handle interaction", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		_ = ctx // Suppress unused variable warning
	}
}
