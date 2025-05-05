package cloudrunslackbot

import (
	"context"

	"github.com/nakamasato/cloud-run-slack-bot/pkg/logger"
	slackinternal "github.com/nakamasato/cloud-run-slack-bot/pkg/slack"
	"github.com/slack-go/slack"
	"go.uber.org/zap"
)

type CloudRunSlackBotService interface {
	Run()
}

func NewCloudRunSlackBotService(sClient *slack.Client, channels map[string]string, defaultChannel string, slackMode string, handler *slackinternal.SlackEventHandler, signingSecret string) CloudRunSlackBotService {
	// Get logger from context or create a new one
	ctx := context.Background()
	l := logger.FromContext(ctx)

	l.Info("Creating CloudRunSlackBotService",
		zap.String("slack_mode", slackMode),
		zap.String("default_channel", defaultChannel),
		zap.Any("channels", channels))

	if slackMode == "socket" {
		return NewCloudRunSlackBotSocket(channels, defaultChannel, sClient, handler)
	}
	return NewCloudRunSlackBotHttp(channels, defaultChannel, sClient, handler, signingSecret)
}
