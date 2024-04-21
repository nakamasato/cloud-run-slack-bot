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

type SlackSocketService struct {
	// https://pkg.go.dev/github.com/slack-go/slack/socketmode#Client
	sClient *socketmode.Client
	handler *SlackEventHandler
}

func NewSlackEventService(token string, runSvc *run.ProjectsLocationsServicesService, mClient *monitoring.MonitoringClient) (*SlackEventService, error) {
	client := slack.New(token)
	return &SlackEventService{
		client:  client,
		handler: &SlackEventHandler{client: client, mClient: mClient, runService: runSvc},
	}, nil
}

func NewSlackSocketService(token, appToken string, runSvc *run.ProjectsLocationsServicesService, mClient *monitoring.MonitoringClient) (*SlackSocketService, error) {
	// https://pkg.go.dev/github.com/slack-go/slack/socketmode#New
	client := slack.New(token, slack.OptionAppLevelToken(appToken))
	sClient := socketmode.New(client)
	return &SlackSocketService{
		sClient: sClient,
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

// runSocket start socket mode
// https://pkg.go.dev/github.com/slack-go/slack/socketmode
// https://github.com/slack-go/slack/blob/master/examples/socketmode/socketmode.go
func (svc *SlackSocketService) Run() {
	go func() {
		for socketEvent := range svc.sClient.Events {
			switch socketEvent.Type {
			case socketmode.EventTypeConnecting:
				log.Println("Connecting to Slack with Socket Mode...")
			case socketmode.EventTypeConnectionError:
				log.Println("Connection failed. Retrying later...")
			case socketmode.EventTypeConnected:
				log.Println("Connected to Slack with Socket Mode.")
			case socketmode.EventTypeEventsAPI:
				event, ok := socketEvent.Data.(slackevents.EventsAPIEvent)
				if !ok {
					continue
				}
				svc.sClient.Ack(*socketEvent.Request)
				err := svc.handler.HandleEvents(&event)
				if err != nil {
					log.Println(err)
				}
			case socketmode.EventTypeInteractive:
				interaction, ok := socketEvent.Data.(slack.InteractionCallback)
				if !ok {
					continue
				}
				// TODO: handle interactive message
				log.Println(interaction)
			}
		}
	}()

	err := svc.sClient.Run()
	if err != nil {
		log.Print(err)
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
