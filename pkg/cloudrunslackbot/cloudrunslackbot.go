package cloudrunslackbot

import (
	"github.com/nakamasato/cloud-run-slack-bot/pkg/config"
	slackinternal "github.com/nakamasato/cloud-run-slack-bot/pkg/slack"
	"github.com/slack-go/slack"
	"go.uber.org/zap"
)

type CloudRunSlackBotService interface {
	Run()
}

// NewCloudRunSlackBotService creates a service for single project (backward compatibility)
func NewCloudRunSlackBotService(sClient *slack.Client, channels map[string]string, defaultChannel string, slackMode string, handler *slackinternal.SlackEventHandler, signingSecret string, logger *zap.Logger) CloudRunSlackBotService {
	if slackMode == "socket" {
		return NewCloudRunSlackBotSocket(channels, defaultChannel, sClient, handler, logger)
	}
	return NewCloudRunSlackBotHttp(channels, defaultChannel, sClient, handler, signingSecret, logger)
}

// NewMultiProjectCloudRunSlackBotService creates a service for multi-project support
func NewMultiProjectCloudRunSlackBotService(sClient *slack.Client, cfg *config.Config, handler *slackinternal.MultiProjectSlackEventHandler, logger *zap.Logger) CloudRunSlackBotService {
	if cfg.SlackAppMode == "socket" {
		return NewMultiProjectCloudRunSlackBotSocket(cfg, sClient, handler, logger)
	}
	return NewMultiProjectCloudRunSlackBotHttp(cfg, sClient, handler, logger)
}
