FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o notification-worker ./cmd/notification-worker

FROM alpine:3.20
WORKDIR /app
COPY --from=builder /app/notification-worker /app/notification-worker
COPY --from=builder /app/configs /app/configs

ENTRYPOINT ["/app/notification-worker"]
