package slack

import (
	"log"
	"net/http"

	"encoding/json"
	"io"

	"github.com/nakamasato/go-cloud-run-alert-bot/pkg/monitoring"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
	"google.golang.org/api/run/v2"
)

type SlackService interface {
	Run()
}

type SlackEventService struct {
	client  *slack.Client
	handler *SlackEventHandler
}

// https://pkg.go.dev/github.com/slack-go/slack/socketmode
// https://github.com/slack-go/slack/blob/master/examples/socketmode/socketmode.go
// https://pkg.go.dev/github.com/slack-go/slack#readme-socketmode-event-handler-experimental
func NewSocketmodeHandler(token, appToken string, runSvc *run.ProjectsLocationsServicesService, mClient *monitoring.MonitoringClient) (*socketmode.SocketmodeHandler, error) {
	// https://pkg.go.dev/github.com/slack-go/slack/socketmode#New
	client := slack.New(token, slack.OptionAppLevelToken(appToken))
	sClient := socketmode.New(client)
	socketmodeHandler := socketmode.NewSocketmodeHandler(sClient)
	handler := &SlackEventHandler{client: client, mClient: mClient, runService: runSvc}
	socketmodeHandler.Handle(socketmode.EventTypeEventsAPI, handler.SocketmodeHandlerFuncEventsAPI)
	socketmodeHandler.Handle(socketmode.EventTypeConnecting, func(socketEvent *socketmode.Event, client *socketmode.Client) {
		log.Println("Connecting to Slack with Socket Mode...")
	})
	socketmodeHandler.Handle(socketmode.EventTypeConnectionError, func(socketEvent *socketmode.Event, client *socketmode.Client) {
		log.Println("Connection failed. Retrying later...")
	})
	socketmodeHandler.Handle(socketmode.EventTypeConnected, func(socketEvent *socketmode.Event, client *socketmode.Client) {
		log.Println("Connected to Slack with Socket Mode.")
	})
	socketmodeHandler.Handle(socketmode.EventTypeInteractive, func(socketEvent *socketmode.Event, client *socketmode.Client) {
		interaction, ok := socketEvent.Data.(slack.InteractionCallback)
		if !ok {
			return
		}
		// TODO: handle interactive message
		log.Println(interaction)
	})
	return socketmodeHandler, nil
}

func NewSlackEventService(token string, runSvc *run.ProjectsLocationsServicesService, mClient *monitoring.MonitoringClient) (*SlackEventService, error) {
	client := slack.New(token)
	return &SlackEventService{
		client:  client,
		handler: &SlackEventHandler{client: client, mClient: mClient, runService: runSvc},
	}, nil
}

// SlackEventsHandler starts http server
func (svc *SlackEventService) Run() {
	http.HandleFunc("/slack/events", svc.SlackEventsHandler())
	log.Println("[INFO] Server listening")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}

const (
	selectVersionAction = "select-version"
)

func (svc *SlackEventService) SlackEventsHandler() http.HandlerFunc {
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
			err := svc.handler.HandleEvents(&eventsAPIEvent)
			if err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
	}
}
