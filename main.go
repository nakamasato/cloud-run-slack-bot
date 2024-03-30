package main

import (
    "encoding/json"
    "io"
    "log"
    "net/http"
    "os"
    "strings"

    "github.com/slack-go/slack"
    "github.com/slack-go/slack/slackevents"
	// monitoring "cloud.google.com/go/monitoring/apiv3/v2"
)

func main() {
	api := slack.New(os.Getenv("SLACK_BOT_TOKEN"))

    http.HandleFunc("/slack/events", func(w http.ResponseWriter, r *http.Request) {
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
                    if _, _, err := api.PostMessage(event.Channel, slack.MsgOptionText("pong", false)); err != nil {
                        log.Println(err)
                        w.WriteHeader(http.StatusInternalServerError)
                        return
                    }
                }
            }
        }
    })

    log.Println("[INFO] Server listening")
    if err := http.ListenAndServe(":8080", nil); err != nil {
        log.Fatal(err)
    }
}
