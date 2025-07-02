# Makefile

# Output binary name and location
BINARY_NAME=bin/scraper

# Default target: clean -> fmt -> test -> build
.PHONY: all
all: clean fmt test build

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
