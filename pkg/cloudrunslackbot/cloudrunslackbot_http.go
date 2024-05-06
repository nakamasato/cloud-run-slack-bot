package cloudrunslackbot

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/nakamasato/cloud-run-slack-bot/pkg/pubsub"
	slackinternal "github.com/nakamasato/cloud-run-slack-bot/pkg/slack"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

type CloudRunSlackBotHttp struct {
	client       *slack.Client
	slackHandler *slackinternal.SlackEventHandler
	auditHandler *pubsub.CloudRunAuditLogHandler
}

func NewCloudRunSlackBotHttp(channel string, sClient *slack.Client, handler *slackinternal.SlackEventHandler) *CloudRunSlackBotHttp {
	return &CloudRunSlackBotHttp{
		client:       sClient,
		slackHandler: handler,
		auditHandler: pubsub.NewCloudRunAuditLogHandler(channel, sClient),
	}
}

// SlackEventsHandler starts http server
func (svc *CloudRunSlackBotHttp) Run() {
	http.HandleFunc("/slack/events", svc.SlackEventsHandler())
	http.HandleFunc("/slack/interaction", svc.SlackInteractionHandler())
	http.HandleFunc("/cloudrun/events", svc.auditHandler.HandleCloudRunAuditLogs)
	log.Println("[INFO] Server listening")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}

// SlackEventsHandler is http.HandlerFunc for Slack Events API
func (svc *CloudRunSlackBotHttp) SlackEventsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		eventsAPIEvent, err := slackevents.ParseEvent(json.RawMessage(body), slackevents.OptionNoVerifyToken())
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		switch eventsAPIEvent.Type {
		case slackevents.URLVerification:
			var res *slackevents.ChallengeResponse
			if err := json.Unmarshal(body, &res); err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			if _, err := w.Write([]byte(res.Challenge)); err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		case slackevents.CallbackEvent:
			err := svc.slackHandler.HandleEvent(&eventsAPIEvent)
			if err != nil {
				log.Println(err)
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
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		err := svc.slackHandler.HandleInteraction(&interaction)
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
}
