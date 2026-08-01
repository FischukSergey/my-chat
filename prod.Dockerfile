FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o main-service ./cmd/main-service

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /app/main-service /app/main-service
COPY --from=builder /app/configs /app/configs

EXPOSE 8080
EXPOSE 9100
ENTRYPOINT ["/app/main-service"]
