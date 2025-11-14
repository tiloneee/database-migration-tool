# Makefile for Database Migration Tool

.PHONY: build clean install test run help docker-up docker-down deps

# Binary name
BINARY_NAME=migrate
BUILD_DIR=bin

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) -v main.go
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Build for multiple platforms
build-all:
	@echo "Building for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 main.go
	GOOS=darwin GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 main.go
	GOOS=darwin GOARCH=arm64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 main.go
	GOOS=windows GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe main.go
	@echo "Multi-platform build complete"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	@echo "Clean complete"

# Install dependencies
deps:
	@echo "Installing dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy
	@echo "Dependencies installed"

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run with arguments (e.g., make run ARGS="pull --dry-run")
run: build
	@echo "Running $(BINARY_NAME)..."
	./$(BUILD_DIR)/$(BINARY_NAME) $(ARGS)

# Install globally
install: build
	@echo "Installing $(BINARY_NAME) to GOPATH..."
	cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/
	@echo "Installed to $(GOPATH)/bin/$(BINARY_NAME)"

# Start all Docker containers (remote + local)
docker-up:
	@echo "Starting all Docker containers..."
	docker-compose -f docker-compose.full.yml up -d
	@echo "Waiting for PostgreSQL to be ready..."
	@sleep 5
	@docker exec db_remote pg_isready -U postgres && echo "✓ Remote DB ready"
	@docker exec db_local pg_isready -U postgres && echo "✓ Local DB ready"

# Start only local Docker container
docker-up-local:
	@echo "Starting local Docker container..."
	docker-compose -f docker-compose.yml up -d
	@echo "Waiting for PostgreSQL to be ready..."
	@sleep 5
	@docker exec db_local pg_isready -U postgres && echo "✓ Local DB ready"

# Start only remote Docker container
docker-up-remote:
	@echo "Starting remote Docker container..."
	docker-compose -f docker-compose.remote.yml up -d
	@echo "Waiting for PostgreSQL to be ready..."
	@sleep 5
	@docker exec db_remote pg_isready -U postgres && echo "✓ Remote DB ready"

# Stop all Docker containers
docker-down:
	@echo "Stopping all Docker containers..."
	docker-compose -f docker-compose.full.yml down

# Stop local container
docker-down-local:
	@echo "Stopping local Docker container..."
	docker-compose -f docker-compose.yml down

# Stop remote container
docker-down-remote:
	@echo "Stopping remote Docker container..."
	docker-compose -f docker-compose.remote.yml down

# Recreate all Docker containers (deletes data!)
docker-recreate:
	@echo "⚠️  Recreating all Docker containers (this will delete all data)..."
	docker-compose -f docker-compose.full.yml down -v
	docker-compose -f docker-compose.full.yml up -d
	@echo "Waiting for PostgreSQL to be ready..."
	@sleep 8
	@docker exec db_remote pg_isready -U postgres && echo "✓ Remote DB ready"
	@docker exec db_local pg_isready -U postgres && echo "✓ Local DB ready"

# Show database status
docker-status:
	@echo "Docker Container Status:"
	@docker ps -a --filter "name=db_" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

# Format code
fmt:
	@echo "Formatting code..."
	$(GOCMD) fmt ./...

# Lint code (requires golangci-lint)
lint:
	@echo "Linting code..."
	golangci-lint run

# Generate mock config files
setup:
	@echo "Setting up configuration files..."
	@if [ ! -f config.yaml ]; then cp config.yaml.example config.yaml; echo "Created config.yaml"; fi
	@if [ ! -f .env ]; then cp .env.example .env; echo "Created .env"; fi
	@echo "Setup complete. Please edit config.yaml or .env with your database credentials."

# Run migration commands (examples)
pull: build
	./$(BUILD_DIR)/$(BINARY_NAME) pull

schema: build
	./$(BUILD_DIR)/$(BINARY_NAME) schema --action apply

data: build
	./$(BUILD_DIR)/$(BINARY_NAME) data

verify: build
	./$(BUILD_DIR)/$(BINARY_NAME) verify

# Help
help:
	@echo "Database Migration Tool - Makefile Commands"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  build         - Build the binary"
	@echo "  build-all     - Build for multiple platforms"
	@echo "  clean         - Remove build artifacts"
	@echo "  deps          - Install dependencies"
	@echo "  test          - Run tests"
	@echo "  run           - Build and run (use ARGS='...' for arguments)"
	@echo "  install       - Install binary to GOPATH"
	@echo "  docker-up         - Start all Docker containers (remote + local)"
	@echo "  docker-up-local   - Start only local container"
	@echo "  docker-up-remote  - Start only remote container"
	@echo "  docker-down       - Stop all Docker containers"
	@echo "  docker-down-local - Stop local container"
	@echo "  docker-down-remote- Stop remote container"
	@echo "  docker-recreate   - Recreate all containers (deletes data!)"
	@echo "  docker-status     - Show container status"
	@echo "  fmt           - Format code"
	@echo "  lint          - Lint code (requires golangci-lint)"
	@echo "  setup         - Create config files from examples"
	@echo "  pull          - Run complete migration"
	@echo "  schema        - Run schema migration"
	@echo "  data          - Run data migration"
	@echo "  verify        - Verify migration"
	@echo "  help          - Show this help message"
	@echo ""
	@echo "Examples:"
	@echo "  make build"
	@echo "  make run ARGS='pull --dry-run'"
	@echo "  make docker-up"
	@echo "  make pull"
