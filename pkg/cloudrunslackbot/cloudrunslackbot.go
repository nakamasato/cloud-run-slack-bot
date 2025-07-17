package cloudrunslackbot

import (
	"github.com/nakamasato/cloud-run-slack-bot/pkg/config"
	slackinternal "github.com/nakamasato/cloud-run-slack-bot/pkg/slack"
	"github.com/slack-go/slack"
)

type CloudRunSlackBotService interface {
	Run()
}

// NewCloudRunSlackBotService creates a service for single project (backward compatibility)
func NewCloudRunSlackBotService(sClient *slack.Client, channels map[string]string, defaultChannel string, slackMode string, handler *slackinternal.SlackEventHandler, signingSecret string) CloudRunSlackBotService {
	if slackMode == "socket" {
		return NewCloudRunSlackBotSocket(channels, defaultChannel, sClient, handler)
	}
	return NewCloudRunSlackBotHttp(channels, defaultChannel, sClient, handler, signingSecret)
}

// NewMultiProjectCloudRunSlackBotService creates a service for multi-project support
func NewMultiProjectCloudRunSlackBotService(sClient *slack.Client, cfg *config.Config, handler *slackinternal.MultiProjectSlackEventHandler) CloudRunSlackBotService {
	if cfg.SlackAppMode == "socket" {
		return NewMultiProjectCloudRunSlackBotSocket(cfg, sClient, handler)
	}
	return NewMultiProjectCloudRunSlackBotHttp(cfg, sClient, handler)
}
