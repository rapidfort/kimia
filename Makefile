# Smithy OSS Makefile
SHELL := /bin/bash

# Version management
GIT_TAG := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
VERSION := $(patsubst v%,%,$(GIT_TAG))
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date +%s)

# Build variables
BINARY_NAME := smithy
BUILD_DIR := bin
REGISTRY := ghcr.io/rapidfort
IMAGE_NAME := smithy

# Build flags
LDFLAGS := -s -w \
	-X main.Version=$(VERSION) \
	-X main.BuildDate=$(BUILD_DATE) \
	-X main.CommitSHA=$(COMMIT) \
	-X main.Branch=$(BRANCH)

.PHONY: help
help:
	@echo "Smithy OSS Build System"
	@echo "======================="
	@echo ""
	@echo "Commands:"
	@echo "  make build       - Build smithy binary"
	@echo "  make test        - Run tests"
	@echo "  make clean       - Clean build artifacts"
	@echo "  make docker      - Build Docker image"
	@echo "  make version     - Show version info"
	@echo ""

.PHONY: build
build:
	@echo "[BUILD] Building smithy OSS v$(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build \
		-ldflags="$(LDFLAGS)" \
		-o $(BUILD_DIR)/$(BINARY_NAME) \
		./cmd/smithy
	@echo "[SUCCESS] Built: $(BUILD_DIR)/$(BINARY_NAME)"

.PHONY: test
test:
	@echo "[TEST] Running tests..."
	go test -v ./...

.PHONY: clean
clean:
	@echo "[CLEAN] Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	go clean

.PHONY: docker
docker:
	@echo "[DOCKER] Building image $(REGISTRY)/$(IMAGE_NAME):$(VERSION)..."
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BRANCH=$(BRANCH) \
		-t $(REGISTRY)/$(IMAGE_NAME):$(VERSION) \
		-t $(REGISTRY)/$(IMAGE_NAME):latest \
		.
	@echo "[SUCCESS] Built: $(REGISTRY)/$(IMAGE_NAME):$(VERSION)"

.PHONY: docker-push
docker-push: docker
	@echo "[PUSH] Pushing to $(REGISTRY)..."
	docker push $(REGISTRY)/$(IMAGE_NAME):$(VERSION)
	docker push $(REGISTRY)/$(IMAGE_NAME):latest

.PHONY: version
version:
	@echo "Version: $(VERSION)"
	@echo "Commit:  $(COMMIT)"
	@echo "Branch:  $(BRANCH)"
	@echo "Date:    $(BUILD_DATE)"

.PHONY: run
run: build
	@echo "[RUN] Running smithy..."
	./$(BUILD_DIR)/$(BINARY_NAME) --help

.DEFAULT_GOAL := help

