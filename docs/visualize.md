# Visualize

## [go-chart](https://github.com/wcharczuk/go-chart)

## [go-echarts](github.com/go-echarts/go-echarts) (Not using)

Originally, I tried to use [go-echarts](github.com/go-echarts/go-echarts) and [snapshort-choromedp](github.com/go-echarts/snapshot-chromedp) but it'd be more complicated to use them on Cloud Run.

Tried docker image ❌

I tried using chromedp-headless with Dockerfile but I want to keep the application setup simple. So I decided to choose a way that doesn't require chromedp-headless.

Dockerfile

```Dockerfile
FROM golang:1.21-bullseye AS builder

WORKDIR /app
COPY go.* ./
RUN go mod download

COPY . ./

RUN go build -v -o server

FROM chromedp/headless-shell:126.0.6439.0

COPY --from=builder /app/server /app/server

CMD ["/app/server"]
```

Tried sidecar container on Cloud Run ❌

```yaml
apiVersion: serving.knative.dev/v1
kind: Service
metadata:
  name: cloud-run-slack-bot
  annotations:
    run.googleapis.com/launch-stage: BETA
spec:
  template:
    metadata:
      annotations:
        run.googleapis.com/container-dependencies: "{cloud-run-slack-bot: [chromedp]}"
    spec:
      serviceAccountName: cloud-run-slack-bot
      containers:
      - image: chromedp/headless-shell
        name: chromedp
        ports:
        - name: http1
          containerPort: 9222
        resources:
          limits:
            cpu: 500m
            memory: 256Mi
      - image: nakamasato/cloud-run-slack-bot:pr-2-df80568
        name: cloud-run-slack-bot
        env:
          - name: SLACK_BOT_TOKEN
            valueFrom:
              secretKeyRef:
                name: slack-bot-token
                key: latest
          - name: PROJECT
            value: <project>
          - name: REGION
            value: asia-northeast1
          - name: SLACK_APP_MODE
            value: http
          - name: TMP_DIR
            value: /tmp
          - name: CHROMEDP_REMOTE_URL
            value: wss://localhost:9222
        resources:
          limits:
            cpu: 1000m
            memory: 512Mi
```

References:

1. https://hub.docker.com/r/chromedp/headless-shell/
1. https://github.com/chromedp/chromedp
1. https://cloud.tencent.com/developer/ask/sof/729353
1. https://github.com/go-echarts/snapshot-chromedp/blob/47575f6f0d3957501fff8cd8fa89c6f3e97916a4/render/chromedp.go
