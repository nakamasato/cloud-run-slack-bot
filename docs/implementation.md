# Implementation

## `UploadFileV2Context` with `Channel`

If `UploadFileV2Context` is used without `Channel`, it's possible to upload file without sending a message.

But we can't use `URLPrivate` if you don't upload file with `Channel`. (ref: https://github.com/nakamasato/cloud-run-slack-bot/pull/56)


You can check: https://app.slack.com/block-kit-builder

```json
{
	"blocks": [
		{
			"type": "image",
			"slack_file": {
				"url": "https://files.slack.com/files-pri/TK8MCGJHH-F07GD216W4S/cloud-run-slack-bot-metrics.png"
			},
			"alt_text": "inspiration"
		}
	]
}
```

![](metrics-image.png)
