FROM golang:1.25-alpine AS builder

WORKDIR /src/go-agent
COPY go-agent/go.mod go-agent/go.sum ./
RUN go mod download

COPY go-agent ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/openclaw-go ./cmd/openclaw-go

FROM alpine:3.21

RUN apk add --no-cache ca-certificates
WORKDIR /app

COPY --from=builder /out/openclaw-go /usr/local/bin/openclaw-go
COPY openclaw-go.example.toml /app/openclaw-go.toml

EXPOSE 8765 8766

ENTRYPOINT ["openclaw-go", "--config", "/app/openclaw-go.toml"]
