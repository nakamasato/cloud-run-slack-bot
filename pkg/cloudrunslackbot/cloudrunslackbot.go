package cloudrunslackbot

import (
	slackinternal "github.com/nakamasato/cloud-run-slack-bot/pkg/slack"
	"github.com/slack-go/slack"
)

type CloudRunSlackBotService interface {
	Run()
}

func NewCloudRunSlackBotService(sClient *slack.Client, channels map[string]string, slackMode string, handler *slackinternal.SlackEventHandler, signingSecret string) CloudRunSlackBotService {
	if slackMode == "socket" {
		return NewCloudRunSlackBotSocket(channels, sClient, handler)
	}
	return NewCloudRunSlackBotHttp(channels, sClient, handler, signingSecret)
}
