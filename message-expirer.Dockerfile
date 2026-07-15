FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o message-expirer ./cmd/message-expirer

FROM alpine:3.20
WORKDIR /app
COPY --from=builder /app/message-expirer /app/message-expirer
COPY --from=builder /app/configs /app/configs

ENTRYPOINT ["/app/message-expirer"]
