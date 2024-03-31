package slack

import (
	"github.com/nakamasato/go-cloud-run-alert-bot/pkg/monitoring"
	"github.com/slack-go/slack"
)

type SlackService struct {
	client  *slack.Client
	mClient *monitoring.MonitoringClient
}

func NewSlackService(token string, mClient *monitoring.MonitoringClient) *SlackService {
	return &SlackService{
		client:  slack.New(token),
		mClient: mClient,
	}
}
