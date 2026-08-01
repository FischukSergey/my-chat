FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o message-expirer ./cmd/message-expirer

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /app/message-expirer /app/message-expirer
COPY --from=builder /app/configs /app/configs

EXPOSE 9101
ENTRYPOINT ["/app/message-expirer"]
