package cloudrunslackbot

import (
	slackinternal "github.com/nakamasato/cloud-run-slack-bot/pkg/slack"
	"github.com/slack-go/slack"
)

type CloudRunSlackBotService interface {
	Run()
}

func NewCloudRunSlackBotService(sClient *slack.Client, channel, slackMode string, handler *slackinternal.SlackEventHandler) CloudRunSlackBotService {
	if slackMode == "socket" {
		return NewCloudRunSlackBotSocket(channel, sClient, handler)
	}
	return NewCloudRunSlackBotHttp(channel, sClient, handler)
}
