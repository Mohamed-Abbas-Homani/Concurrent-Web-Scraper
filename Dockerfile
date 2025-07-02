# Stage 1: Build the Go binary
FROM golang:1.24-alpine AS builder

# Set environment variables for Go
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /app

# Copy go.mod and go.sum first to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the project
COPY . .

# Build the binary
RUN go build -o scraper ./cmd/scraper

# Stage 2: Run the binary with a minimal image
FROM alpine:latest

WORKDIR /root/

# Copy the binary from the builder stage
COPY --from=builder /app/scraper .

# If you need a config file or similar, copy it here
# COPY --from=builder /app/config.yaml .

# Set the entrypoint
ENTRYPOINT ["./scraper"]
