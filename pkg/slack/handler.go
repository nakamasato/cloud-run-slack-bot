package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	monitoringineternal "github.com/nakamasato/go-cloud-run-alert-bot/pkg/monitoring"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"google.golang.org/api/run/v2"
	// monitoring "cloud.google.com/go/monitoring/apiv3/v2"
)

const (
	selectVersionAction = "select-version"
)

func (svc *SlackService) SlackEventsHandler() http.HandlerFunc {
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
			innerEvent := eventsAPIEvent.InnerEvent
			switch event := innerEvent.Data.(type) {
			case *slackevents.AppMentionEvent:
				message := strings.Split(event.Text, " ")
				if len(message) < 2 {
					w.WriteHeader(http.StatusBadRequest)
					return
				}

				command := message[1]
				switch command {
				case "ping":
					if _, _, err := svc.client.PostMessage(event.Channel, slack.MsgOptionText("pong", false)); err != nil {
						log.Println(err)
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
				case "deploy":
					text := slack.NewTextBlockObject(slack.MarkdownType, "Please select *version*.", false, false)
					textSection := slack.NewSectionBlock(text, nil, nil)

					versions := []string{"v1.0.0", "v1.1.0", "v1.1.1"}
					options := make([]*slack.OptionBlockObject, 0, len(versions))
					for _, v := range versions {
						optionText := slack.NewTextBlockObject(slack.PlainTextType, v, false, false)
						description := slack.NewTextBlockObject(slack.PlainTextType, "This is the version you want to deploy.", false, false)
						options = append(options, slack.NewOptionBlockObject(v, optionText, description))
					}

					placeholder := slack.NewTextBlockObject(slack.PlainTextType, "Select version", false, false)
					selectMenu := slack.NewOptionsSelectBlockElement(slack.OptTypeStatic, placeholder, "", options...)

					actionBlock := slack.NewActionBlock(selectVersionAction, selectMenu)

					fallbackText := slack.MsgOptionText("This client is not supported.", false)
					blocks := slack.MsgOptionBlocks(textSection, actionBlock)

					if _, err := svc.client.PostEphemeral(event.Channel, event.User, fallbackText, blocks); err != nil {
						log.Println(err)
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
				case "list":
					ctx := context.Background()
					runService, err := run.NewService(ctx)
					if err != nil {
						log.Fatalf("Failed to create run service: %v", err)
					}
					plrSvc := run.NewProjectsLocationsServicesService(runService)
					res, err := plrSvc.List("projects/" + os.Getenv("PROJECT") + "/locations/" + os.Getenv("REGION")).Do()
					if err != nil {
						log.Fatalf("Failed to list services: %v", err)
					}
					svcNames := []string{}
					for _, s := range res.Services {
						svcNames = append(svcNames, s.Name)
					}
					svcNamesStr := strings.Join(svcNames, "\n")
					if _, _, err := svc.client.PostMessage(event.Channel, slack.MsgOptionText(svcNamesStr, false)); err != nil {
						log.Println(err)
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
				case "metrics":
					ctx := context.Background()
					rc, err := svc.mClient.GetRequestCount(ctx, monitoringineternal.MonitorCondition{
						Project: os.Getenv("PROJECT"),
						Filters: []monitoringineternal.MonitorFilter{
							{"metric.type": "run.googleapis.com/request_count"},
							{"resource.labels.service_name": "go-cloud-run-alert-bot"}, // TODO: enable to specify service name
						},
					}, 24*time.Hour)
					msg := fmt.Sprintf("Request count: %d", rc)
					if err != nil {
						msg = "Failed to get metrics: " + err.Error()
					}
					if _, _, err := svc.client.PostMessage(event.Channel, slack.MsgOptionText(msg, false)); err != nil {
						log.Println(err)
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
				default:
					if _, _, err := svc.client.PostMessage(event.Channel, slack.MsgOptionText("The given command not supported. Please use supported commands: `ping`, `deploy`, and `list`", false)); err != nil {
						log.Println(err)
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
				}
			}
		}
	}
}
