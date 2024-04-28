FROM golang:1.21-buster AS builder

WORKDIR /app
COPY go.* ./
RUN go mod download

COPY . ./

RUN go build -v -o server

FROM chromedp/headless-shell:latest

COPY --from=builder /app/server /app/server

CMD ["/app/server"]
