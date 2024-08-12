package cloudrunslackbot

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/nakamasato/cloud-run-slack-bot/pkg/logging"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/pubsub"
	slackinternal "github.com/nakamasato/cloud-run-slack-bot/pkg/slack"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"go.uber.org/zap"
)

type ServiceOption func(*CloudRunSlackBotHttp)

type CloudRunSlackBotHttp struct {
	client       *slack.Client
	slackHandler *slackinternal.SlackEventHandler
	auditHandler *pubsub.CloudRunAuditLogHandler
	logger       *zap.Logger
}

func WithLogger(l *zap.Logger) ServiceOption {
	return func(s *CloudRunSlackBotHttp) {
		s.logger = l
	}
}

func WithClient(c *slack.Client) ServiceOption {
	return func(s *CloudRunSlackBotHttp) {
		s.client = c
	}
}

func WithSlackHandler(h *slackinternal.SlackEventHandler) ServiceOption {
	return func(s *CloudRunSlackBotHttp) {
		s.slackHandler = h
	}
}

func NewCloudRunSlackBotHttp(channel string, sClient *slack.Client, handler *slackinternal.SlackEventHandler, opts ...ServiceOption) *CloudRunSlackBotHttp {
	s := &CloudRunSlackBotHttp{
		client:       sClient,
		slackHandler: handler,
		auditHandler: pubsub.NewCloudRunAuditLogHandler(channel, sClient),
	}
	for _, opt := range opts {
		opt(s)
	}
	// default logger
	if s.logger == nil {
		s.logger = zap.NewExample()
	}
	return s
}

// SlackEventsHandler starts http server
func (svc *CloudRunSlackBotHttp) Run() {
	http.Handle("/slack/events", logging.Middleware(svc.SlackEventsHandler()))
	http.Handle("/slack/interaction", logging.Middleware(svc.SlackInteractionHandler()))
	http.HandleFunc("/cloudrun/events", svc.auditHandler.HandleCloudRunAuditLogs)
	svc.logger.Info("Server listening")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		svc.logger.Error("failed to start server", zap.Error(err))
	}
}

// SlackEventsHandler is http.HandlerFunc for Slack Events API
func (svc *CloudRunSlackBotHttp) SlackEventsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			svc.logger.Error("failed to read request body", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		eventsAPIEvent, err := slackevents.ParseEvent(json.RawMessage(body), slackevents.OptionNoVerifyToken())
		if err != nil {
			svc.logger.Error("failed to parse event", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		switch eventsAPIEvent.Type {
		case slackevents.URLVerification:
			var res *slackevents.ChallengeResponse
			if err := json.Unmarshal(body, &res); err != nil {
				svc.logger.Error("failed to unmarshal challenge response", zap.Error(err))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			if _, err := w.Write([]byte(res.Challenge)); err != nil {
				svc.logger.Error("failed to write challenge response", zap.Error(err))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		case slackevents.CallbackEvent:
			err := svc.slackHandler.HandleEvent(&eventsAPIEvent)
			if err != nil {
				svc.logger.Error("failed to handle event", zap.Error(err))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
	}
}

func (svc *CloudRunSlackBotHttp) SlackInteractionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var interaction slack.InteractionCallback
		if err := json.Unmarshal([]byte(r.FormValue("payload")), &interaction); err != nil {
			svc.logger.Error("failed to unmarshal interaction", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		err := svc.slackHandler.HandleInteraction(&interaction)
		if err != nil {
			svc.logger.Error("failed to handle interaction", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
}
