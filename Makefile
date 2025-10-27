# Smithy Makefile - Dual Image Build System (BuildKit + Buildah)
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

# Image names - BuildKit is default
IMAGE_NAME_BUILDKIT := smithy
IMAGE_NAME_BUILDAH := smithy-bud

# Smithy user configuration
SMITHY_USER := smithy
SMITHY_UID := 1000

# Test script location
TEST_SCRIPT := tests/master.sh

# Build arguments (shared by both)
BUILD_ARGS := \
              --build-arg BUILD_DATE=$(BUILD_DATE) \
              --build-arg COMMIT=$(COMMIT) \
              --build-arg BRANCH=$(BRANCH) \
              --build-arg RELEASE=$(RELEASE) \
              --build-arg SMITHY_USER=$(SMITHY_USER) \
              --build-arg SMITHY_UID=$(SMITHY_UID)

# Default target
.PHONY: all
all: build-all push-all

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
		LAST=`cat $(VERSION_FILE)`; \
		NEXT=$$((LAST + 1)); \
		echo "  Last Build: $(VERSION_BASE)-dev$$LAST"; \
		echo "  Next Build: $(VERSION_BASE)-dev$$NEXT"; \
	else \
		echo "  Next Build: $(VERSION_BASE)-dev1"; \
	fi
	@echo ""
	@echo "Images:"
	@echo "  smithy        - BuildKit-based (default, recommended)"
	@echo "  smithy-bud    - Buildah-based ('bud' = buildah build)"
	@echo ""
	@echo "━━━ Main Commands ━━━"
	@echo "  make all                - Build & push ALL images to dev registry"
	@echo "  make full               - Build, push & test ALL images"
	@echo ""
	@echo "━━━ Build Commands ━━━"
	@echo "  make build              - Build smithy image (BuildKit)"
	@echo "  make build-buildkit     - Build BuildKit image"
	@echo "  make build-buildah      - Build Buildah image"
	@echo "  make build-all          - Build BOTH images"
	@echo ""
	@echo "━━━ Push Commands ━━━"
	@echo "  make push               - Push to dev registry (BuildKit)"
	@echo "  make push-buildkit      - Push BuildKit image"
	@echo "  make push-buildah       - Push Buildah image"
	@echo "  make push-all           - Push BOTH images"
	@echo ""
	@echo "━━━ Test Commands ━━━"
	@echo "  make test               - Run Docker tests (BuildKit)"
	@echo "  make test-buildkit      - Test BuildKit image"
	@echo "  make test-buildah       - Test Buildah image"
	@echo "  make test-all           - Test BOTH images"
	@echo "  make test-clean         - Clean up test resources"
	@echo ""
	@echo "━━━ Utilities ━━━"
	@echo "  make run                - Run smithy container locally"
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
		echo "Last Dev Build: $(VERSION_BASE)-dev`cat $(VERSION_FILE)`"; \
	else \
		echo "No dev builds yet"; \
	fi

# =============================================================================
# BUILD TARGETS
# =============================================================================

# Internal build target for BuildKit (doesn't increment version)
.PHONY: _build-buildkit
_build-buildkit:
	@if [ -f $(VERSION_FILE) ]; then \
		VERSION=$(VERSION_BASE)-dev`cat $(VERSION_FILE)`; \
	else \
		echo "[ERROR] No version file found. This should not happen."; \
		exit 1; \
	fi; \
	echo "[BUILD-BUILDKIT] Building BuildKit image..."; \
	echo "Version: $$VERSION"; \
	BUILD_DATE=`date +%s` && \
	echo "Building $(IMAGE_NAME_BUILDKIT) Image: $(REGISTRY)/$(IMAGE_NAME_BUILDKIT):$$VERSION ..." && \
	docker build -t $(REGISTRY)/$(IMAGE_NAME_BUILDKIT):$$VERSION --build-arg VERSION=$$VERSION $(BUILD_ARGS) -f Dockerfile.buildkit . && \
	docker tag $(REGISTRY)/$(IMAGE_NAME_BUILDKIT):$$VERSION $(REGISTRY)/$(IMAGE_NAME_BUILDKIT):latest && \
	echo "[SUCCESS] BuildKit image complete! Version: $$VERSION" && \
	echo "[SUCCESS] Tagged as: latest"

# Internal build target for Buildah (doesn't increment version)
.PHONY: _build-buildah
_build-buildah:
	@if [ -f $(VERSION_FILE) ]; then \
		VERSION=$(VERSION_BASE)-dev`cat $(VERSION_FILE)`; \
	else \
		echo "[ERROR] No version file found. This should not happen."; \
		exit 1; \
	fi; \
	echo "[BUILD-BUILDAH] Building Buildah image..."; \
	echo "Version: $$VERSION"; \
	BUILD_DATE=`date +%s` && \
	echo "Building $(IMAGE_NAME_BUILDAH) Image: $(REGISTRY)/$(IMAGE_NAME_BUILDAH):$$VERSION ..." && \
	docker build -t $(REGISTRY)/$(IMAGE_NAME_BUILDAH):$$VERSION --build-arg VERSION=$$VERSION $(BUILD_ARGS) -f Dockerfile.buildah . && \
	docker tag $(REGISTRY)/$(IMAGE_NAME_BUILDAH):$$VERSION $(REGISTRY)/$(IMAGE_NAME_BUILDAH):latest && \
	echo "[SUCCESS] Buildah image complete! Version: $$VERSION" && \
	echo "[SUCCESS] Tagged as: latest"

# Build both images (increments version once)
.PHONY: build-all
build-all:
	@if [ -f $(VERSION_FILE) ]; then \
		BUILD_NUM=`cat $(VERSION_FILE)`; \
		NEXT_BUILD=$$((BUILD_NUM + 1)); \
	else \
		NEXT_BUILD=1; \
	fi; \
	echo $$NEXT_BUILD > $(VERSION_FILE); \
	echo "[BUILD-ALL] Building both images with version $(VERSION_BASE)-dev$$NEXT_BUILD"; \
	$(MAKE) _build-buildkit && \
	$(MAKE) _build-buildah && \
	echo "" && \
	echo "[SUCCESS] Both images built successfully!" && \
	echo "  - $(REGISTRY)/$(IMAGE_NAME_BUILDKIT):latest (BuildKit)" && \
	echo "  - $(REGISTRY)/$(IMAGE_NAME_BUILDAH):latest (Buildah)"

# Build BuildKit image only (increments version)
.PHONY: build-buildkit
build-buildkit:
	@if [ -f $(VERSION_FILE) ]; then \
		BUILD_NUM=`cat $(VERSION_FILE)`; \
		NEXT_BUILD=$$((BUILD_NUM + 1)); \
	else \
		NEXT_BUILD=1; \
	fi; \
	echo $$NEXT_BUILD > $(VERSION_FILE)
	@$(MAKE) _build-buildkit

# Build Buildah image only (increments version)
.PHONY: build-buildah
build-buildah:
	@if [ -f $(VERSION_FILE) ]; then \
		BUILD_NUM=`cat $(VERSION_FILE)`; \
		NEXT_BUILD=$$((BUILD_NUM + 1)); \
	else \
		NEXT_BUILD=1; \
	fi; \
	echo $$NEXT_BUILD > $(VERSION_FILE)
	@$(MAKE) _build-buildah

# Default build (BuildKit)
.PHONY: build
build: build-buildkit

# =============================================================================
# PUSH TARGETS
# =============================================================================

# Push BuildKit image
.PHONY: push-buildkit
push-buildkit:
	@if [ "$(RELEASE_BUILD)" = "true" ]; then \
		VERSION=$(VERSION_BASE); \
	else \
		if [ -f $(VERSION_FILE) ]; then \
			VERSION=$(VERSION_BASE)-dev`cat $(VERSION_FILE)`; \
		else \
			echo "[ERROR] No build found. Run 'make build-buildkit' first"; \
			exit 1; \
		fi; \
	fi; \
	echo "[PUSH-BUILDKIT] Pushing BuildKit image version $$VERSION ..." && \
	if ! docker image inspect $(REGISTRY)/$(IMAGE_NAME_BUILDKIT):$$VERSION > /dev/null 2>&1; then \
		echo "[ERROR] Image $(REGISTRY)/$(IMAGE_NAME_BUILDKIT):$$VERSION not found. Run 'make build-buildkit' first"; \
		exit 1; \
	fi && \
	docker push $(REGISTRY)/$(IMAGE_NAME_BUILDKIT):$$VERSION && \
	if [ "$(RELEASE_BUILD)" != "true" ]; then \
		echo "[PUSH-BUILDKIT] Pushing latest tag..." && \
		docker push $(REGISTRY)/$(IMAGE_NAME_BUILDKIT):latest; \
	fi && \
	echo "[SUCCESS] BuildKit image push complete!"

# Push Buildah image
.PHONY: push-buildah
push-buildah:
	@if [ "$(RELEASE_BUILD)" = "true" ]; then \
		VERSION=$(VERSION_BASE); \
	else \
		if [ -f $(VERSION_FILE) ]; then \
			VERSION=$(VERSION_BASE)-dev`cat $(VERSION_FILE)`; \
		else \
			echo "[ERROR] No build found. Run 'make build-buildah' first"; \
			exit 1; \
		fi; \
	fi; \
	echo "[PUSH-BUILDAH] Pushing Buildah image version $$VERSION ..." && \
	if ! docker image inspect $(REGISTRY)/$(IMAGE_NAME_BUILDAH):$$VERSION > /dev/null 2>&1; then \
		echo "[ERROR] Image $(REGISTRY)/$(IMAGE_NAME_BUILDAH):$$VERSION not found. Run 'make build-buildah' first"; \
		exit 1; \
	fi && \
	docker push $(REGISTRY)/$(IMAGE_NAME_BUILDAH):$$VERSION && \
	if [ "$(RELEASE_BUILD)" != "true" ]; then \
		echo "[PUSH-BUILDAH] Pushing latest tag..." && \
		docker push $(REGISTRY)/$(IMAGE_NAME_BUILDAH):latest; \
	fi && \
	echo "[SUCCESS] Buildah image push complete!"

# Push both images
.PHONY: push-all
push-all: push-buildkit push-buildah
	@echo ""
	@echo "[SUCCESS] Both images pushed successfully!"

# Default push (BuildKit)
.PHONY: push
push: push-buildkit

# =============================================================================
# TEST TARGETS (using original test script)
# =============================================================================

.PHONY: test-buildkit
test-buildkit: check-test-script
	@echo "[TEST-BUILDKIT] Testing BuildKit image..."
	@if [ -f $(VERSION_FILE) ]; then \
		VERSION=$(VERSION_BASE)-dev`cat $(VERSION_FILE)`; \
	else \
		echo "[WARNING] No build found. Using latest image"; \
		VERSION=latest; \
	fi; \
	echo "Testing BuildKit image: $(REGISTRY)/$(IMAGE_NAME_BUILDKIT):$$VERSION"; \
	$(TEST_SCRIPT) -m both -b buildkit -r $(REGISTRY) -i $(REGISTRY)/$(IMAGE_NAME_BUILDKIT):$$VERSION

.PHONY: test-buildah
test-buildah: check-test-script
	@echo "[TEST-BUILDAH] Testing Buildah image..."
	@if [ -f $(VERSION_FILE) ]; then \
		VERSION=$(VERSION_BASE)-dev`cat $(VERSION_FILE)`; \
	else \
		echo "[WARNING] No build found. Using latest image"; \
		VERSION=latest; \
	fi; \
	echo "Testing Buildah image: $(REGISTRY)/$(IMAGE_NAME_BUILDAH):$$VERSION"; \
	$(TEST_SCRIPT) -m both -b buildah -r $(REGISTRY) -i $(REGISTRY)/$(IMAGE_NAME_BUILDAH):$$VERSION

.PHONY: test-all
test-all: test-buildkit test-buildah
	@echo ""
	@echo "[SUCCESS] Both images tested!"

.PHONY: test
test: test-buildkit


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

.PHONY: test-debug-auth
test-debug-auth: check-test-script
	@echo "[TEST-DEBUG-AUTH] Debugging authentication..."
	@if [ -f $(VERSION_FILE) ]; then \
		VERSION=$(VERSION_BASE)-dev`cat $(VERSION_FILE)`; \
	else \
		VERSION=latest; \
	fi; \
	$(TEST_SCRIPT) --debug-auth -r $(REGISTRY) -i $(REGISTRY)/$(IMAGE_NAME_BUILDKIT):$$VERSION

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

# =============================================================================
# RUN & UTILITY TARGETS
# =============================================================================

.PHONY: run
run:
	@if [ -f $(VERSION_FILE) ]; then \
		VERSION=$(VERSION_BASE)-dev`cat $(VERSION_FILE)`; \
	else \
		echo "[ERROR] No build found. Run 'make build' first"; \
		exit 1; \
	fi; \
	echo "[RUN] Running smithy container version $$VERSION..."; \
	docker run --rm -it \
		--security-opt seccomp=unconfined \
		--security-opt apparmor=unconfined \
		--user $(SMITHY_UID):$(SMITHY_UID) \
		-e HOME=/home/$(SMITHY_USER) \
		-e DOCKER_CONFIG=/home/$(SMITHY_USER)/.docker \
		$(REGISTRY)/$(IMAGE_NAME_BUILDKIT):$$VERSION

.PHONY: show-images
show-images:
	@echo "[IMAGES] Local Smithy images:"
	@docker images | grep -E "$(REGISTRY)/($(IMAGE_NAME_BUILDKIT)|$(IMAGE_NAME_BUILDAH))" | head -20 || echo "No images found"

.PHONY: inspect
inspect:
	@if [ -f $(VERSION_FILE) ]; then \
		VERSION=$(VERSION_BASE)-dev`cat $(VERSION_FILE)`; \
	else \
		echo "[ERROR] No build found. Run 'make build' first"; \
		exit 1; \
	fi; \
	echo "[INSPECT] Inspecting BuildKit image: $(REGISTRY)/$(IMAGE_NAME_BUILDKIT):$$VERSION"; \
	echo ""; \
	echo "=== Image Details ==="; \
	docker inspect $(REGISTRY)/$(IMAGE_NAME_BUILDKIT):$$VERSION --format '{{json .Config}}' | jq '.Labels, .Env'

.PHONY: clean
clean:
	@echo "[CLEAN] Cleaning..."
	@if [ -f $(VERSION_FILE) ]; then \
		echo "  Removing version file (was at build `cat $(VERSION_FILE)`)"; \
		rm -f $(VERSION_FILE); \
	fi
	@rm -rf $(DOCKERBUILD_TEMP)
	@rm -rf build buildtmp
	@echo "[CLEAN] Done"

# =============================================================================
# SHORTCUTS
# =============================================================================

.PHONY: bp
bp: build-buildkit push-buildkit
	@echo "[SUCCESS] Build and push complete (BuildKit)!"

.PHONY: bp-all
bp-all: build-all push-all
	@echo "[SUCCESS] Build and push complete (both images)!"

.PHONY: bpt
bpt: build push test
	@echo "[SUCCESS] Build, push, and test complete!"

.PHONY: full
full: build-all push-all test-all
	@echo "[SUCCESS] Full cycle complete (all images)!"

.DEFAULT_GOAL := help