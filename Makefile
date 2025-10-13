# Smithy Makefile - Container Build System
# Variables
REGISTRY ?= $(if $(RF_APP_HOST),$(RF_APP_HOST):5000/rapidfort,rapidfort)
NAMESPACE ?= default
RELEASE ?= 0

SHELL := /bin/bash

# Git version management
GIT_TAG := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
VERSION_BASE := $(patsubst v%,%,$(GIT_TAG))
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date +%s)

# Version file for dev builds
VERSION_FILE := .version

# GitHub Container Registry settings
GHCR_REGISTRY := ghcr.io/rapidfort
DOCKERBUILD_TEMP := ./build/rapidfort

# Architecture detection for multi-arch support
ARCH := $(shell uname -m | sed 's/x86_64/amd64/g' | sed 's/aarch64/arm64/g')
OS := linux

# Image name
IMAGE_NAME := smithy

# Smithy user configuration
SMITHY_USER := smithy
SMITHY_UID := 1000

# Test script location
TEST_SCRIPT := tests/master.sh

# Build arguments
BUILD_ARGS := \
              --build-arg BUILD_DATE=$(BUILD_DATE) \
              --build-arg COMMIT=$(COMMIT) \
              --build-arg BRANCH=$(BRANCH) \
              --build-arg RELEASE=$(RELEASE) \
              --build-arg SMITHY_USER=$(SMITHY_USER) \
              --build-arg SMITHY_UID=$(SMITHY_UID)

# Default target
.PHONY: all
all: build

# Print help
.PHONY: help
help:
	@echo "╔═══════════════════════════════════════════════════════════════════╗"
	@echo "║              SMITHY BUILD SYSTEM                                  ║"
	@echo "╚═══════════════════════════════════════════════════════════════════╝"
	@echo ""
	@echo "Version Info:"
	@echo "  Git Tag: $(GIT_TAG)"
	@echo "  Base Version: $(VERSION_BASE)"
	@if [ -f $(VERSION_FILE) ]; then \
		LAST=$$(cat $(VERSION_FILE)); \
		NEXT=$$((LAST + 1)); \
		echo "  Last Build: $(VERSION_BASE)-dev$$LAST"; \
		echo "  Next Build: $(VERSION_BASE)-dev$$NEXT"; \
	else \
		echo "  Next Build: $(VERSION_BASE)-dev1"; \
	fi
	@echo ""
	@echo "━━━ Development Commands ━━━"
	@echo "  make build              - Build smithy image"
	@echo "  make push               - Push to dev registry"
	@echo "  make run                - Run smithy container locally"
	@echo "  make test               - Run Docker tests"
	@echo "  make test-k8s           - Run Kubernetes tests"
	@echo "  make test-all           - Run all tests (Docker + Kubernetes)"
	@echo "  make test-clean         - Clean up test resources"
	@echo "  make test-verbose       - Run tests in verbose mode"
	@echo "  make test-debug-auth    - Debug authentication setup"
	@echo ""
	@echo "━━━ Utilities ━━━"
	@echo "  make version            - Show current versions"
	@echo "  make show-images        - Show local docker images"
	@echo "  make clean              - Clean build artifacts"
	@echo ""
	@echo "Environment Variables:"
	@echo "  REGISTRY                - Docker registry (default: based on RF_APP_HOST)"
	@echo ""

# Version info
.PHONY: version
version:
	@echo "━━━ Version Information ━━━"
	@echo "Git Tag:      $(GIT_TAG)"
	@echo "Base Version: $(VERSION_BASE)"
	@echo "Commit:       $(COMMIT)"
	@echo "Branch:       $(BRANCH)"
	@if [ -f $(VERSION_FILE) ]; then \
		echo "Last Dev Build: $(VERSION_BASE)-dev$$(cat $(VERSION_FILE))"; \
	else \
		echo "No dev builds yet"; \
	fi

# Main build target
.PHONY: build
build:
	@if [ -f $(VERSION_FILE) ]; then \
		BUILD_NUM=$$(cat $(VERSION_FILE)); \
		NEXT_BUILD=$$((BUILD_NUM + 1)); \
	else \
		NEXT_BUILD=1; \
	fi; \
	echo $$NEXT_BUILD > $(VERSION_FILE); \
	VERSION=$(VERSION_BASE)-dev$$NEXT_BUILD; \
	echo "[BUILD] Building development image..."; \
	echo "Version: $$VERSION"; \
	BUILD_DATE=$$(date +%s) && \
	echo "Building $(IMAGE_NAME) Image: $(REGISTRY)/$(IMAGE_NAME):$$VERSION ..." && \
	docker build -t $(REGISTRY)/$(IMAGE_NAME):$$VERSION --build-arg VERSION=$$VERSION $(BUILD_ARGS) -f Dockerfile . && \
	docker tag $(REGISTRY)/$(IMAGE_NAME):$$VERSION $(REGISTRY)/$(IMAGE_NAME):latest && \
	echo "[SUCCESS] Build complete! Version: $$VERSION" && \
	echo "[SUCCESS] Tagged as: latest"

# Push image to dev registry
.PHONY: push
push:
	@if [ "$(RELEASE_BUILD)" = "true" ]; then \
		VERSION=$(VERSION_BASE); \
	else \
		if [ -f $(VERSION_FILE) ]; then \
			VERSION=$(VERSION_BASE)-dev$$(cat $(VERSION_FILE)); \
		else \
			echo "[ERROR] No build found. Run 'make build' first"; \
			exit 1; \
		fi; \
	fi; \
	echo "[PUSH] Pushing image version $$VERSION ..." && \
	if ! docker image inspect $(REGISTRY)/$(IMAGE_NAME):$$VERSION > /dev/null 2>&1; then \
		echo "[ERROR] Image $(REGISTRY)/$(IMAGE_NAME):$$VERSION not found. Run 'make build' first"; \
		exit 1; \
	fi && \
	docker push $(REGISTRY)/$(IMAGE_NAME):$$VERSION && \
	if [ "$(RELEASE_BUILD)" != "true" ]; then \
		echo "[PUSH] Pushing latest tag..." && \
		docker push $(REGISTRY)/$(IMAGE_NAME):latest; \
	fi && \
	echo "[SUCCESS] Push complete!"

# Run smithy container locally for testing
.PHONY: run
run:
	@if [ -f $(VERSION_FILE) ]; then \
		VERSION=$(VERSION_BASE)-dev$$(cat $(VERSION_FILE)); \
	else \
		echo "[ERROR] No build found. Run 'make build' first"; \
		exit 1; \
	fi; \
	echo "[RUN] Running smithy container version $$VERSION..."; \
	docker run --rm -it \
		--security-opt seccomp=unconfined \
		--security-opt apparmor=unconfined \
		--user $(SMITHY_UID):$(SMITHY_UID) \
		-e GIT_REPO="https://github.com/nginxinc/docker-nginx.git" \
		-e GIT_BRANCH="master" \
		-e DOCKERFILE_PATH="mainline/alpine/Dockerfile" \
		-e IMAGE_NAME="test-nginx" \
		-e IMAGE_TAG="test-$$(date +%s)" \
		-e PUSH_IMAGE="false" \
		-e HOME=/home/$(SMITHY_USER) \
		-e DOCKER_CONFIG=/home/$(SMITHY_USER)/.docker \
		$(REGISTRY)/$(IMAGE_NAME):$$VERSION

# Test the build with Docker tests
.PHONY: test
test: check-test-script
	@echo "[TEST] Running Docker tests..."
	@if [ -f $(VERSION_FILE) ]; then \
		VERSION=$(VERSION_BASE)-dev$$(cat $(VERSION_FILE)); \
	else \
		echo "[WARNING] No build found. Using latest image"; \
		VERSION=latest; \
	fi; \
	echo "Testing with image: $(REGISTRY)/$(IMAGE_NAME):$$VERSION"; \
	$(TEST_SCRIPT) -m docker -r $(REGISTRY) -i $(REGISTRY)/$(IMAGE_NAME):$$VERSION

# Kubernetes tests
.PHONY: test-k8s
test-k8s: check-test-script
	@echo "[TEST-K8S] Running Kubernetes test suite..."
	@if [ -f $(VERSION_FILE) ]; then \
		VERSION=$(VERSION_BASE)-dev$$(cat $(VERSION_FILE)); \
	else \
		echo "[WARNING] No build found. Using latest image"; \
		VERSION=latest; \
	fi; \
	echo "Testing with image: $(REGISTRY)/$(IMAGE_NAME):$$VERSION"; \
	$(TEST_SCRIPT) -m kubernetes -r $(REGISTRY) -i $(REGISTRY)/$(IMAGE_NAME):$$VERSION

# Run all tests (Docker + Kubernetes)
.PHONY: test-all
test-all: check-test-script
	@echo "[TEST-ALL] Running all test suites..."
	@if [ -f $(VERSION_FILE) ]; then \
		VERSION=$(VERSION_BASE)-dev$$(cat $(VERSION_FILE)); \
	else \
		echo "[WARNING] No build found. Using latest image"; \
		VERSION=latest; \
	fi; \
	echo "Testing with image: $(REGISTRY)/$(IMAGE_NAME):$$VERSION"; \
	$(TEST_SCRIPT) -m both -r $(REGISTRY) -i $(REGISTRY)/$(IMAGE_NAME):$$VERSION

# Clean up test resources
.PHONY: test-clean
test-clean:
	@echo "[TEST-CLEAN] Cleaning up test resources..."
	@if [ -x $(TEST_SCRIPT) ]; then \
		$(TEST_SCRIPT) -m both -c -r $(REGISTRY); \
	else \
		echo "[INFO] Test script not found, performing basic cleanup..."; \
		rm -rf /tmp/smithy-docker-config 2>/dev/null || true; \
		rm -f /tmp/Dockerfile.* 2>/dev/null || true; \
		rm -f /tmp/test.log 2>/dev/null || true; \
		rm -rf /tmp/output 2>/dev/null || true; \
		rm -rf /tmp/smithy-auth 2>/dev/null || true; \
		kubectl delete namespace smithy-tests --force --grace-period=0 --ignore-not-found=true 2>/dev/null || true; \
	fi
	@echo "[TEST-CLEAN] Cleanup complete"

# Run tests in verbose mode
.PHONY: test-verbose
test-verbose: check-test-script
	@echo "[TEST-VERBOSE] Running tests in verbose mode..."
	@if [ -f $(VERSION_FILE) ]; then \
		VERSION=$(VERSION_BASE)-dev$$(cat $(VERSION_FILE)); \
	else \
		VERSION=latest; \
	fi; \
	$(TEST_SCRIPT) -m both -v -r $(REGISTRY) -i $(REGISTRY)/$(IMAGE_NAME):$$VERSION

# Debug authentication setup
.PHONY: test-debug-auth
test-debug-auth: check-test-script
	@echo "[TEST-DEBUG-AUTH] Debugging authentication..."
	@if [ -f $(VERSION_FILE) ]; then \
		VERSION=$(VERSION_BASE)-dev$$(cat $(VERSION_FILE)); \
	else \
		VERSION=latest; \
	fi; \
	$(TEST_SCRIPT) --debug-auth -r $(REGISTRY) -i $(REGISTRY)/$(IMAGE_NAME):$$VERSION

# Check if test script exists
.PHONY: check-test-script
check-test-script:
	@if [ ! -f $(TEST_SCRIPT) ]; then \
		echo "[ERROR] Test script not found: $(TEST_SCRIPT)"; \
		echo "Please ensure tests/master.sh exists and is executable"; \
		exit 1; \
	fi
	@if [ ! -x $(TEST_SCRIPT) ]; then \
		echo "[INFO] Making test script executable..."; \
		chmod +x $(TEST_SCRIPT); \
	fi

# Quick test after build
.PHONY: test-quick
test-quick: build
	@echo "[TEST-QUICK] Running quick version test..."
	@if [ -f $(VERSION_FILE) ]; then \
		VERSION=$(VERSION_BASE)-dev$$(cat $(VERSION_FILE)); \
	else \
		VERSION=$(VERSION_BASE)-dev1; \
	fi; \
	echo "Testing with image: $(REGISTRY)/$(IMAGE_NAME):$$VERSION"; \
	docker run --rm \
		--security-opt seccomp=unconfined \
		--security-opt apparmor=unconfined \
		--user $(SMITHY_UID):$(SMITHY_UID) \
		$(REGISTRY)/$(IMAGE_NAME):$$VERSION --version || \
	docker run --rm \
		--security-opt seccomp=unconfined \
		--security-opt apparmor=unconfined \
		--user $(SMITHY_UID):$(SMITHY_UID) \
		$(REGISTRY)/$(IMAGE_NAME):$$VERSION buildah version

# Inspect the latest built image
.PHONY: inspect
inspect:
	@if [ -f $(VERSION_FILE) ]; then \
		VERSION=$(VERSION_BASE)-dev$$(cat $(VERSION_FILE)); \
	else \
		echo "[ERROR] No build found. Run 'make build' first"; \
		exit 1; \
	fi; \
	echo "[INSPECT] Inspecting image: $(REGISTRY)/$(IMAGE_NAME):$$VERSION"; \
	echo ""; \
	echo "=== Image Details ==="; \
	docker inspect $(REGISTRY)/$(IMAGE_NAME):$$VERSION --format '{{json .Config}}' | jq '.Labels, .Env' ; \
	echo ""; \
	echo "=== Entrypoint ==="; \
	docker inspect $(REGISTRY)/$(IMAGE_NAME):$$VERSION --format '{{json .Config.Entrypoint}}' | jq . ; \
	echo ""; \
	echo "=== CMD ==="; \
	docker inspect $(REGISTRY)/$(IMAGE_NAME):$$VERSION --format '{{json .Config.Cmd}}' | jq . ; \
	echo ""; \
	echo "=== Working Directory ==="; \
	docker inspect $(REGISTRY)/$(IMAGE_NAME):$$VERSION --format '{{.Config.WorkingDir}}' ; \
	echo ""; \
	echo "=== User ==="; \
	docker inspect $(REGISTRY)/$(IMAGE_NAME):$$VERSION --format '{{.Config.User}}'

# Show images
.PHONY: show-images
show-images:
	@echo "[IMAGES] Local Smithy images:"
	@docker images | grep -E "$(REGISTRY)/$(IMAGE_NAME)" | head -10 || echo "No images found"

# Clean
.PHONY: clean
clean:
	@echo "[CLEAN] Cleaning..."
	@if [ -f $(VERSION_FILE) ]; then \
		echo "  Removing version file (was at build $$(cat $(VERSION_FILE)))"; \
		rm -f $(VERSION_FILE); \
	fi
	@rm -rf $(DOCKERBUILD_TEMP)
	@rm -rf build buildtmp
	@echo "[CLEAN] Done"

# =============================================================================
# SHORTCUTS & CONVENIENCE TARGETS
# =============================================================================

# Build and push in one command
.PHONY: bp
bp: build push
	@echo "[SUCCESS] Build and push complete!"

# Build, push, and test
.PHONY: bpt
bpt: build push test
	@echo "[SUCCESS] Build, push, and test complete!"

# Full cycle: build, push, test all
.PHONY: full
full: build push test-all
	@echo "[SUCCESS] Full cycle complete!"

.DEFAULT_GOAL := help
