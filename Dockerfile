# Stage 1: Build the Go binary
FROM golang:1.24-alpine AS builder

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o scraper ./cmd/scraper

# Stage 2: Run the binary with a minimal image
FROM alpine:latest

WORKDIR /root/

COPY --from=builder /app/scraper .

# Expose HTTP (8081) and gRPC (8080) ports
EXPOSE 8080
EXPOSE 8081

ENTRYPOINT ["./scraper"]
