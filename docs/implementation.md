# Implementation

## `UploadFileV2Context` with `Channel`

If `UploadFileV2Context` is used without `Channel`, it's possible to upload file without sending a message.

But we can't use `URLPrivate` if you don't upload file with `Channel`. (ref: https://github.com/nakamasato/cloud-run-slack-bot/pull/56)

![](metrics-image.png)
