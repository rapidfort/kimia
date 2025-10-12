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
	@echo "━━━ Quick Start ━━━"
	@echo "  make build              - Build smithy image with dev version"
	@echo "  make push               - Push to local registry"
	@echo "  make run                - Run smithy container locally"
	@echo ""
	@echo "━━━ Development Commands ━━━"
	@echo "  make build              - Build smithy image"
	@echo "  make push               - Push to dev registry"
	@echo "  make test               - Run Docker tests"
	@echo "  make test-k8s           - Run Kubernetes tests"
	@echo "  make test-all           - Run all tests (Docker + Kubernetes)"
	@echo ""
	@echo "━━━ Test Commands ━━━"
	@echo "  make test               - Run Docker tests"
	@echo "  make test-k8s           - Run Kubernetes tests"
	@echo "  make test-all           - Run both Docker and Kubernetes tests"
	@echo "  make test-clean         - Clean up test resources"
	@echo "  make test-verbose       - Run tests in verbose mode"
	@echo "  make test-debug-auth    - Debug authentication setup"
	@echo ""
	@echo "━━━ Release Commands ━━━"
	@echo "  make release            - Build and publish release to ghcr.io"
	@echo "  make release-info       - Show what will be released"
	@echo "  make check-release-env  - Check GitHub credentials"
	@echo ""
	@echo "  Multi-arch Release Workflow:"
	@echo "    1. Staging Release (run on BOTH amd64 and arm64 machines):"
	@echo "       make release RELEASE_TYPE=staging"
	@echo "    2. Finalize staging (optional, ensures both archs in manifest):"
	@echo "       make release-finalize RELEASE_TYPE=staging"
	@echo "    3. Promote staging to production:"
	@echo "       make release-publish VERSION=x.y.z-staging"
	@echo ""
	@echo "  Release Status Commands:"
	@echo "    make release-status RELEASE_TYPE=staging    - Check staging status"
	@echo "    make release-status RELEASE_TYPE=publish VERSION=x.y.z-staging"
	@echo ""
	@echo "━━━ Utilities ━━━"
	@echo "  make version            - Show current versions"
	@echo "  make show-images        - Show local docker images"
	@echo "  make clean              - Clean build artifacts"
	@echo "  make tag                - Create a new git tag"
	@echo "  make inspect            - Inspect the latest built image"
	@echo ""
	@echo "Environment Variables:"
	@echo "  REGISTRY                - Docker registry (default: based on RF_APP_HOST)"
	@echo "  GITHUB_TOKEN            - GitHub Personal Access Token (for releases)"
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
	@if [ "$(RELEASE_BUILD)" = "true" ]; then \
		VERSION=$(VERSION_BASE); \
		RELEASE=1; \
		echo "[BUILD] Building release image..."; \
		echo "Version: $$VERSION"; \
	else \
		if [ -f $(VERSION_FILE) ]; then \
			BUILD_NUM=$$(cat $(VERSION_FILE)); \
			NEXT_BUILD=$$((BUILD_NUM + 1)); \
		else \
			NEXT_BUILD=1; \
		fi; \
		echo $$NEXT_BUILD > $(VERSION_FILE); \
		VERSION=$(VERSION_BASE)-dev$$NEXT_BUILD; \
		echo "[BUILD] Building development image..."; \
		echo "Version: $$VERSION"; \
	fi; \
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

# Create a new git tag
.PHONY: tag
tag:
	@if [ -z "$(NEW_TAG)" ]; then \
		echo "[ERROR] Please specify NEW_TAG (e.g., make tag NEW_TAG=v1.0.1)"; \
		exit 1; \
	fi
	@echo "[TAG] Creating new git tag: $(NEW_TAG)"
	@git tag -a $(NEW_TAG) -m "Release $(NEW_TAG)"
	@echo "[TAG] Tag created. Push with: git push origin $(NEW_TAG)"

# =============================================================================
# RELEASE MANAGEMENT - Multi-arch support for ghcr.io
# =============================================================================

# Release type (staging or publish)
RELEASE_TYPE ?= staging

# Check release environment (GitHub credentials)
.PHONY: check-release-env
check-release-env:
	@if [ -z "$(GITHUB_TOKEN)" ]; then \
		echo "[ERROR] GITHUB_TOKEN environment variable is required"; \
		echo ""; \
		echo "To create a Personal Access Token (PAT):"; \
		echo "  1. Go to https://github.com/settings/tokens"; \
		echo "  2. Click 'Generate new token' -> 'Generate new token (classic)'"; \
		echo "  3. Select scopes: write:packages, read:packages, delete:packages"; \
		echo "  4. Copy the token and export it:"; \
		echo "     export GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"; \
		echo ""; \
		exit 1; \
	fi
	@echo "[OK] GitHub token found"
	@echo "[OK] Current Architecture: $(ARCH)"
	@echo "[OK] Current Host: $$(hostname)"

# Release info
.PHONY: release-info
release-info:
	@echo "━━━ RELEASE INFO ━━━"
	@echo "Version:      $(VERSION_BASE)"
	@echo "Release Type: $(RELEASE_TYPE)"
	@echo "Architecture: $(ARCH)"
	@echo ""
	@if [ "$(RELEASE_TYPE)" = "staging" ]; then \
		echo "STAGING Release will create:"; \
		echo "  Architecture-specific ghcr.io tag:"; \
		echo "    - $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging-$(ARCH)"; \
		echo ""; \
		echo "  Multi-arch manifest (after both architectures build):"; \
		echo "    - $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging"; \
		echo ""; \
		echo "  Note: Run on both amd64 and arm64 machines for multi-arch support"; \
	elif [ "$(RELEASE_TYPE)" = "publish" ]; then \
		if [ -z "$(VERSION)" ]; then \
			echo "[ERROR] VERSION parameter required for publish (e.g., VERSION=1.0.0-staging)"; \
			exit 1; \
		fi; \
		BASE_VERSION=$${VERSION%-staging}; \
		echo "PUBLISH Release will create from staging $(VERSION):"; \
		echo "  Production Image:"; \
		echo "    - $(GHCR_REGISTRY)/$(IMAGE_NAME):$$BASE_VERSION"; \
		echo "  Latest tag:"; \
		echo "    - $(GHCR_REGISTRY)/$(IMAGE_NAME):latest"; \
	fi

# Build and push for current architecture with manifest update
.PHONY: _release-build-push-manifest
_release-build-push-manifest: check-release-env
	@echo "[BUILD] Building for architecture: $(ARCH) on host: $$(hostname)..."
	@echo "[BUILD] Machine info: $$(uname -m)"
	@echo "[BUILD] Release type: $(RELEASE_TYPE)"
	
	@# Build with RELEASE_BUILD flag
	@echo "[BUILD] Running RELEASE_BUILD=true make build..."
	@RELEASE_BUILD=true $(MAKE) build
	
	@# Login to ghcr.io
	@echo "[LOGIN] Logging into ghcr.io..."
	@echo "$(GITHUB_TOKEN)" | docker login -u $$(echo $(GITHUB_TOKEN) | cut -d'_' -f1) --password-stdin ghcr.io
	
	@# Tag and push based on release type
	@if [ "$(RELEASE_TYPE)" = "staging" ]; then \
		echo "[TAG] Tagging image for staging with -staging-$(ARCH) suffix..."; \
		docker tag $(REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE) $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging-$(ARCH); \
		echo "[PUSH] Pushing staging $(ARCH) image to ghcr.io..."; \
		docker push $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging-$(ARCH); \
		echo "[PUSH] Successfully pushed staging $(ARCH) image!"; \
		echo "[MANIFEST] Creating/updating staging manifest for $(ARCH)..."; \
		docker manifest rm $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging 2>/dev/null || true; \
		if [ "$(ARCH)" = "amd64" ]; then \
			ALT_ARCH=arm64; \
		else \
			ALT_ARCH=amd64; \
		fi; \
		CURRENT_EXISTS=false; \
		ALT_EXISTS=false; \
		if docker pull $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging-$(ARCH) >/dev/null 2>&1; then \
			CURRENT_EXISTS=true; \
		fi; \
		if docker pull $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging-$$ALT_ARCH >/dev/null 2>&1; then \
			ALT_EXISTS=true; \
		fi; \
		if [ "$$CURRENT_EXISTS" = "true" ] && [ "$$ALT_EXISTS" = "true" ]; then \
			echo "[MANIFEST] Creating manifest with both architectures"; \
			docker manifest create $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging \
				$(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging-$(ARCH) \
				$(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging-$$ALT_ARCH; \
			docker manifest annotate $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging \
				$(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging-$(ARCH) --arch $(ARCH) --os $(OS); \
			docker manifest annotate $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging \
				$(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging-$$ALT_ARCH --arch $$ALT_ARCH --os $(OS); \
		elif [ "$$CURRENT_EXISTS" = "true" ]; then \
			echo "[MANIFEST] Creating manifest with $(ARCH) only (waiting for other architecture)"; \
			docker manifest create $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging \
				$(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging-$(ARCH); \
			docker manifest annotate $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging \
				$(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging-$(ARCH) --arch $(ARCH) --os $(OS); \
		fi; \
		docker manifest push $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging; \
	else \
		echo "[TAG] Tagging image for production with -$(ARCH) suffix..."; \
		docker tag $(REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE) $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-$(ARCH); \
		echo "[PUSH] Pushing production $(ARCH) image to ghcr.io..."; \
		docker push $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-$(ARCH); \
		echo "[PUSH] Successfully pushed production $(ARCH) image!"; \
		echo "[MANIFEST] Creating/updating production manifest for $(ARCH)..."; \
		docker manifest rm $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE) 2>/dev/null || true; \
		if [ "$(ARCH)" = "amd64" ]; then \
			ALT_ARCH=arm64; \
		else \
			ALT_ARCH=amd64; \
		fi; \
		CURRENT_EXISTS=false; \
		ALT_EXISTS=false; \
		if docker pull $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-$(ARCH) >/dev/null 2>&1; then \
			CURRENT_EXISTS=true; \
		fi; \
		if docker pull $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-$$ALT_ARCH >/dev/null 2>&1; then \
			ALT_EXISTS=true; \
		fi; \
		if [ "$$CURRENT_EXISTS" = "true" ] && [ "$$ALT_EXISTS" = "true" ]; then \
			echo "[MANIFEST] Creating manifest with both architectures"; \
			docker manifest create $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE) \
				$(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-$(ARCH) \
				$(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-$$ALT_ARCH; \
			docker manifest annotate $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE) \
				$(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-$(ARCH) --arch $(ARCH) --os $(OS); \
			docker manifest annotate $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE) \
				$(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-$$ALT_ARCH --arch $$ALT_ARCH --os $(OS); \
		elif [ "$$CURRENT_EXISTS" = "true" ]; then \
			echo "[MANIFEST] Creating manifest with $(ARCH) only (waiting for other architecture)"; \
			docker manifest create $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE) \
				$(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-$(ARCH); \
			docker manifest annotate $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE) \
				$(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-$(ARCH) --arch $(ARCH) --os $(OS); \
		fi; \
		docker manifest push $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE); \
	fi
	
	@echo "[SUCCESS] Completed $(ARCH) build and manifest updates!"

# Finalize manifests after both architectures complete
.PHONY: release-finalize
release-finalize: check-release-env
	@if [ "$(RELEASE_TYPE)" = "staging" ]; then \
		echo "[FINALIZE] Finalizing staging release $(VERSION_BASE)-staging..."; \
		echo "[FINALIZE] Recreating manifest to ensure both architectures are included..."; \
		echo "[LOGIN] Logging into ghcr.io..."; \
		echo "$(GITHUB_TOKEN)" | docker login -u $$(echo $(GITHUB_TOKEN) | cut -d'_' -f1) --password-stdin ghcr.io; \
		docker manifest rm $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging 2>/dev/null || true; \
		AMD64_EXISTS=false; \
		ARM64_EXISTS=false; \
		if docker pull $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging-amd64 >/dev/null 2>&1; then \
			AMD64_EXISTS=true; \
		fi; \
		if docker pull $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging-arm64 >/dev/null 2>&1; then \
			ARM64_EXISTS=true; \
		fi; \
		if [ "$$AMD64_EXISTS" = "true" ] && [ "$$ARM64_EXISTS" = "true" ]; then \
			echo "[MANIFEST] Creating manifest with both amd64 and arm64"; \
			docker manifest create $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging \
				$(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging-amd64 \
				$(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging-arm64; \
			docker manifest annotate $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging \
				$(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging-amd64 --arch amd64 --os linux; \
			docker manifest annotate $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging \
				$(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging-arm64 --arch arm64 --os linux; \
			docker manifest push $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION_BASE)-staging; \
			echo "[SUCCESS] Finalized staging manifest with both architectures!"; \
		else \
			echo "[WARN] Missing architectures (amd64: $$AMD64_EXISTS, arm64: $$ARM64_EXISTS)"; \
			echo "Run 'make release RELEASE_TYPE=staging' on the missing architecture"; \
		fi; \
	else \
		echo "[ERROR] release-finalize only applies to staging releases"; \
		exit 1; \
	fi

# Publish from staging to production
.PHONY: _release-publish-from-staging
_release-publish-from-staging: check-release-env
	@if [ -z "$(VERSION)" ]; then \
		echo "[ERROR] VERSION parameter required (e.g., VERSION=1.0.0-staging)"; \
		exit 1; \
	fi
	@if ! echo "$(VERSION)" | grep -q "staging"; then \
		echo "[ERROR] VERSION must be a staging version (e.g., 1.0.0-staging)"; \
		exit 1; \
	fi
	
	@BASE_VERSION=$${VERSION%-staging} && \
	echo "[PUBLISH] Publishing from staging $(VERSION) to production $$BASE_VERSION..." && \
	echo "[LOGIN] Logging into ghcr.io..." && \
	echo "$(GITHUB_TOKEN)" | docker login -u $$(echo $(GITHUB_TOKEN) | cut -d'_' -f1) --password-stdin ghcr.io && \
	\
	for arch in amd64 arm64; do \
		echo "[COPY] Copying $(IMAGE_NAME):$(VERSION)-$$arch → $(IMAGE_NAME):$$BASE_VERSION-$$arch"; \
		docker pull $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION)-$$arch && \
		docker tag $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION)-$$arch $(GHCR_REGISTRY)/$(IMAGE_NAME):$$BASE_VERSION-$$arch && \
		docker push $(GHCR_REGISTRY)/$(IMAGE_NAME):$$BASE_VERSION-$$arch && \
		docker tag $(GHCR_REGISTRY)/$(IMAGE_NAME):$(VERSION)-$$arch $(GHCR_REGISTRY)/$(IMAGE_NAME):latest-$$arch && \
		docker push $(GHCR_REGISTRY)/$(IMAGE_NAME):latest-$$arch || \
		echo "[WARN] Architecture $$arch not found (might be expected)"; \
	done && \
	\
	echo "[MANIFEST] Creating production manifest..." && \
	docker manifest rm $(GHCR_REGISTRY)/$(IMAGE_NAME):$$BASE_VERSION 2>/dev/null || true; \
	docker manifest create $(GHCR_REGISTRY)/$(IMAGE_NAME):$$BASE_VERSION \
		$(GHCR_REGISTRY)/$(IMAGE_NAME):$$BASE_VERSION-amd64 \
		$(GHCR_REGISTRY)/$(IMAGE_NAME):$$BASE_VERSION-arm64 2>/dev/null && \
	docker manifest annotate $(GHCR_REGISTRY)/$(IMAGE_NAME):$$BASE_VERSION \
		$(GHCR_REGISTRY)/$(IMAGE_NAME):$$BASE_VERSION-amd64 --arch amd64 --os linux && \
	docker manifest annotate $(GHCR_REGISTRY)/$(IMAGE_NAME):$$BASE_VERSION \
		$(GHCR_REGISTRY)/$(IMAGE_NAME):$$BASE_VERSION-arm64 --arch arm64 --os linux && \
	docker manifest push $(GHCR_REGISTRY)/$(IMAGE_NAME):$$BASE_VERSION || \
	echo "[WARN] Failed to create manifest (check if both architectures exist)"; \
	\
	echo "[MANIFEST] Updating latest manifest..." && \
	docker manifest rm $(GHCR_REGISTRY)/$(IMAGE_NAME):latest 2>/dev/null || true; \
	if docker pull $(GHCR_REGISTRY)/$(IMAGE_NAME):latest-amd64 >/dev/null 2>&1 && \
	   docker pull $(GHCR_REGISTRY)/$(IMAGE_NAME):latest-arm64 >/dev/null 2>&1; then \
		docker manifest create $(GHCR_REGISTRY)/$(IMAGE_NAME):latest \
			$(GHCR_REGISTRY)/$(IMAGE_NAME):latest-amd64 \
			$(GHCR_REGISTRY)/$(IMAGE_NAME):latest-arm64; \
		docker manifest annotate $(GHCR_REGISTRY)/$(IMAGE_NAME):latest \
			$(GHCR_REGISTRY)/$(IMAGE_NAME):latest-amd64 --arch amd64 --os linux; \
		docker manifest annotate $(GHCR_REGISTRY)/$(IMAGE_NAME):latest \
			$(GHCR_REGISTRY)/$(IMAGE_NAME):latest-arm64 --arch arm64 --os linux; \
		docker manifest push $(GHCR_REGISTRY)/$(IMAGE_NAME):latest; \
	fi; \
	\
	echo "[SUCCESS] Published production release $$BASE_VERSION!"

# Check release status
.PHONY: release-status
release-status: check-release-env
	@if [ "$(RELEASE_TYPE)" = "staging" ]; then \
		echo "[STATUS] Checking staging release $(VERSION_BASE)-staging..."; \
		TARGET_VERSION="$(VERSION_BASE)-staging"; \
	elif [ "$(RELEASE_TYPE)" = "publish" ] && [ -n "$(VERSION)" ]; then \
		BASE_VERSION=$${VERSION%-staging} && \
		echo "[STATUS] Checking production release $$BASE_VERSION..." && \
		TARGET_VERSION="$$BASE_VERSION"; \
	else \
		echo "[ERROR] Invalid parameters. Use:"; \
		echo "  make release-status RELEASE_TYPE=staging"; \
		echo "  make release-status RELEASE_TYPE=publish VERSION=x.y.z-staging"; \
		exit 1; \
	fi && \
	echo ""; \
	echo "$(GITHUB_TOKEN)" | docker login -u $$(echo $(GITHUB_TOKEN) | cut -d'_' -f1) --password-stdin ghcr.io > /dev/null 2>&1; \
	echo "Checking manifest for $$TARGET_VERSION:"; \
	echo -n "  $(IMAGE_NAME): "; \
	if docker manifest inspect $(GHCR_REGISTRY)/$(IMAGE_NAME):$$TARGET_VERSION >/dev/null 2>&1; then \
		archs=$$(docker manifest inspect $(GHCR_REGISTRY)/$(IMAGE_NAME):$$TARGET_VERSION 2>/dev/null | \
			jq -r '.manifests[].platform.architecture' 2>/dev/null | sort | tr '\n' ' '); \
		echo "✓ manifest exists [$$archs]"; \
	else \
		echo "✗ manifest not found"; \
	fi; \
	echo ""; \
	echo "Checking architecture-specific images:"; \
	for arch in amd64 arm64; do \
		echo -n "  $$arch: "; \
		if docker pull $(GHCR_REGISTRY)/$(IMAGE_NAME):$$TARGET_VERSION-$$arch >/dev/null 2>&1; then \
			echo "✓"; \
		else \
			echo "✗"; \
		fi; \
	done

# Main release target
.PHONY: release
release: check-release-env
	@if [ "$(RELEASE_TYPE)" = "staging" ]; then \
		echo "[RELEASE] Starting STAGING release $(VERSION_BASE)-staging on $(ARCH)..."; \
		echo "[BUILD] Building and pushing for $(ARCH) architecture..."; \
		$(MAKE) _release-build-push-manifest RELEASE=1; \
		echo ""; \
		echo "[SUCCESS] $(ARCH) staging release complete!"; \
		echo ""; \
		echo "IMPORTANT: To complete multi-arch support, run this same command on the other architecture."; \
		echo "After both architectures are built, optionally run:"; \
		echo "  make release-finalize RELEASE_TYPE=staging"; \
		echo ""; \
		echo "Current status:"; \
		$(MAKE) release-status RELEASE_TYPE=staging; \
	elif [ "$(RELEASE_TYPE)" = "publish" ]; then \
		echo "[RELEASE] Starting PUBLISH from staging..."; \
		$(MAKE) _release-publish-from-staging; \
	else \
		echo "[ERROR] Invalid RELEASE_TYPE. Use 'staging' or 'publish'"; \
		exit 1; \
	fi

# Helper targets
.PHONY: release-staging
release-staging:
	@$(MAKE) release RELEASE_TYPE=staging 

.PHONY: release-publish
release-publish:
	@if [ -z "$(VERSION)" ]; then \
		echo "[ERROR] VERSION required (e.g., make release-publish VERSION=1.0.0-staging)"; \
		exit 1; \
	fi
	@$(MAKE) release RELEASE_TYPE=publish VERSION=$(VERSION)

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