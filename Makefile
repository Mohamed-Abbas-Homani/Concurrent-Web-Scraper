# Makefile

# Output binary name and location
BINARY_NAME=bin/scraper

# Default target: clean -> fmt -> test -> build
.PHONY: all
all: clean fmt proto test build

# Build the binary
.PHONY: build
build:
	@echo "🔨 Building $(BINARY_NAME)..."
	@mkdir -p bin
	@go build -o $(BINARY_NAME) ./cmd/scraper

# Run tests
.PHONY: test
test:
	@echo "✅ Running tests..."
	@go test ./...

# Format code
.PHONY: fmt
fmt:
	@echo "🎨 Formatting code..."
	@go fmt ./...

# Clean up binaries
.PHONY: clean
clean:
	@echo "🧹 Cleaning up..."
	@rm -rf bin

# Docker image name
IMAGE_NAME=go-scraper

# Build Docker image
.PHONY: docker-build
docker-build:
	@echo "🐳 Building Docker image..."
	@docker build -t $(IMAGE_NAME) .

# Run Docker container
.PHONY: docker-run
docker-run:
	@echo "🚀 Running Docker container on ports 8080 (gRPC) and 8081 (HTTP)..."
	@docker run --rm -p 8080:8080 -p 8081:8081 $(IMAGE_NAME)

PROTO_DIR := pkg/types/proto
PROTO_FILES := $(PROTO_DIR)/scraper.proto
OUT_DIR := $(PROTO_DIR)

.PHONY: proto
proto:
	protoc \
	  --proto_path=$(PROTO_DIR) \
	  --go_out=$(OUT_DIR) --go_opt=paths=source_relative \
	  --go-grpc_out=$(OUT_DIR) --go-grpc_opt=paths=source_relative \
	  $(PROTO_FILES)
