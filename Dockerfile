FROM golang:1.21-bullseye AS builder

WORKDIR /app
COPY go.* ./
RUN go mod download

COPY . ./

RUN go build -v -o server

FROM chromedp/headless-shell:126.0.6439.0

COPY --from=builder /app/server /app/server

CMD ["/app/server"]
