package slack

import (
	"github.com/slack-go/slack"
)

type Client interface {
	PostMessage(channel string, options ...slack.MsgOption) (string, string, error)
}

type DummySlackClient struct{}

func (c DummySlackClient) PostMessage(channel string, options ...slack.MsgOption) (string, string, error) {
	return "", "", nil
}
