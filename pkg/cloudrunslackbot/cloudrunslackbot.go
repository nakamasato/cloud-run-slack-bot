package cloudrunslackbot

import (
	"errors"

	slackinternal "github.com/nakamasato/cloud-run-slack-bot/pkg/slack"
	"github.com/slack-go/slack"
)

type CloudRunSlackBotService interface {
	Run()
}

func NewCloudRunSlackBotService(sClient *slack.Client, slackMode string, handler *slackinternal.SlackEventHandler) (CloudRunSlackBotService, error) {
	if slackMode == "http" {
		return NewCloudRunSlackBotHttp(sClient, handler)
	} else if slackMode == "socket" {
		return NewCloudRunSlackBotSocket(sClient, handler)
	}
	return nil, errors.New("slackMode must be either 'http' or 'socket'")
}
