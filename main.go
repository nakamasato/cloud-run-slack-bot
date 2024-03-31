package main

import (
	"log"
	"net/http"
	"os"

	monitoringineternal "github.com/nakamasato/go-cloud-run-alert-bot/pkg/monitoring"
	slackinternal "github.com/nakamasato/go-cloud-run-alert-bot/pkg/slack"
)

func main() {

	mClient, err := monitoringineternal.NewMonitoringClient()
	if err != nil {
		log.Fatal(err)
	}
	svc := slackinternal.NewSlackService(
		os.Getenv("SLACK_BOT_TOKEN"),
		mClient,
	)
	defer mClient.Close()

	http.HandleFunc("/slack/events", svc.SlackEventsHandler())

	log.Println("[INFO] Server listening")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
