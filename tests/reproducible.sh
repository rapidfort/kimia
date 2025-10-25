#!/bin/bash
# test-reproducible.sh - Test reproducible builds with registry
#
# Usage: 
#   ./test-reproducible.sh buildkit [smithy-image]
#   ./test-reproducible.sh buildah [smithy-image]

set -e

# Configuration
BUILDER="${1:-buildkit}"
SMITHY_IMAGE="${2}"
EPOCH="${SOURCE_DATE_EPOCH:-1609459200}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

if [ -z "${RF_APP_HOST}" ]; then
    REGISTRY=${REGISTRY:-"ghcr.io"}
else
    REGISTRY="${RF_APP_HOST}:5000"
fi

TEST_IMAGE="${REGISTRY}/smithy-repro-test"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log() { echo -e "${GREEN}[INFO]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
debug() { echo -e "${BLUE}[DEBUG]${NC} $1"; }

# Determine Smithy image if not provided (same pattern as master.sh/docker-tests.sh)
if [ -z "$SMITHY_IMAGE" ]; then
    # Use the same registry detection as docker-tests.sh
    if [ -z "${RF_APP_HOST}" ]; then
        REGISTRY=${REGISTRY:-"ghcr.io"}
    else
        REGISTRY="${RF_APP_HOST}:5000"
    fi
    
    # Get version if available
    if [ -f "$PROJECT_ROOT/.version" ]; then
        VERSION=$(cat "$PROJECT_ROOT/.version")
        GIT_TAG=$(git -C "$PROJECT_ROOT" describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
        VERSION_BASE=$(echo "$GIT_TAG" | sed 's/^v//')
        FULL_VERSION="${VERSION_BASE}-dev${VERSION}"
    else
        FULL_VERSION="latest"
    fi
    
    if [ "$BUILDER" = "buildah" ]; then
        SMITHY_IMAGE="${REGISTRY}/rapidfort/smithy-bud:${FULL_VERSION}"
    else
        SMITHY_IMAGE="${REGISTRY}/rapidfort/smithy:${FULL_VERSION}"
    fi
fi

echo "================================================================"
echo "  Smithy Reproducible Build Test"
echo "================================================================"
echo "  Builder:       $BUILDER"
echo "  Smithy Image:  $SMITHY_IMAGE"
echo "  Test Image:    $TEST_IMAGE:v1"
echo "  Registry:      $REGISTRY"
echo "  Epoch:         $EPOCH ($(date -d "@$EPOCH" 2>/dev/null || date -r "$EPOCH"))"
echo "================================================================"
echo ""

# Check dependencies
for cmd in docker curl jq; do
    if ! command -v $cmd &> /dev/null; then
        error "$cmd is required but not installed"
    fi
done


# Create temporary directory for test files
TEST_DIR=$(mktemp -d)
chown -R 1000:1000 "$TEST_DIR"
trap "rm -rf $TEST_DIR" EXIT

# Create test Dockerfile with pinned base image and package versions
cat > $TEST_DIR/Dockerfile << 'DOCKERFILE'
# Pin base image by SHA256 digest for reproducibility (alpine:3.22)
FROM alpine@sha256:4b7ce07002c69e8f3d704a9c5d6fd3053be500b7f1c69fc0d80990c2ad8dd412

# Pin package versions for reproducibility
RUN apk add --no-cache \
    curl=8.14.1-r2 \
    ca-certificates=20250911-r0

WORKDIR /app
RUN echo "reproducible test" > file.txt
CMD ["sh"]
DOCKERFILE

log "Test Dockerfile created"
echo ""

# Get storage driver flag based on builder (same as docker-tests.sh)
get_storage_flag() {
    if [ "$BUILDER" = "buildkit" ]; then
        echo "native"
    else
        echo "vfs"
    fi
}

STORAGE_FLAG=$(get_storage_flag)

# Base Docker run command following docker-tests.sh pattern EXACTLY
BASE_CMD="docker run --rm"
BASE_CMD="$BASE_CMD --user 1000:1000"
BASE_CMD="$BASE_CMD --cap-drop ALL"
BASE_CMD="$BASE_CMD --cap-add SETUID"
BASE_CMD="$BASE_CMD --cap-add SETGID"
BASE_CMD="$BASE_CMD --security-opt seccomp=unconfined"
BASE_CMD="$BASE_CMD --security-opt apparmor=unconfined"
BASE_CMD="$BASE_CMD -v $TEST_DIR:/workspace"
BASE_CMD="$BASE_CMD -e SOURCE_DATE_EPOCH=$EPOCH"
BASE_CMD="$BASE_CMD -e HOME=/tmp"
BASE_CMD="$BASE_CMD $SMITHY_IMAGE"

# Function to build and get digest from registry
build_and_get_digest() {
    local build_num=$1
    
    log "Build #$build_num: Building with Smithy ($BUILDER)..." >&2
    
    # Build command following docker-tests.sh pattern
    # BUT WITH PUSH (not --no-push) to get digest from registry
    BUILD_CMD="$BASE_CMD"
    BUILD_CMD="$BUILD_CMD --context /workspace"
    BUILD_CMD="$BUILD_CMD --dockerfile /workspace/Dockerfile"
    BUILD_CMD="$BUILD_CMD --destination $TEST_IMAGE:v1"
    BUILD_CMD="$BUILD_CMD --storage-driver $STORAGE_FLAG"
    BUILD_CMD="$BUILD_CMD --build-arg BUILD_DATE=$EPOCH"
    BUILD_CMD="$BUILD_CMD --label version=1.0.0"
    BUILD_CMD="$BUILD_CMD --label build.date=$EPOCH"
    BUILD_CMD="$BUILD_CMD --insecure"
    BUILD_CMD="$BUILD_CMD --verbosity=debug"
    
    debug "Build command: $BUILD_CMD" >&2
    
    echo "" >&2
    log "Executing build command:" >&2
    echo "$BUILD_CMD" >&2
    echo "" >&2
    
    if ! eval $BUILD_CMD; then
        error "Build #$build_num failed"
    fi
    
    # Get digest from registry
    log "Retrieving digest from registry..." >&2
    
    # Extract registry path for curl
    local registry_path=$(echo "$TEST_IMAGE" | sed "s|${REGISTRY}/||")
    local registry_url="http://${REGISTRY}"
    
    local digest=$(curl -sf -H "Accept: application/vnd.docker.distribution.manifest.v2+json" \
        "${registry_url}/v2/${registry_path}/manifests/v1" | \
        jq -r '.config.digest')
    
    if [ -z "$digest" ] || [ "$digest" = "null" ]; then
        error "Failed to get digest for build #$build_num"
    fi
    
    log "Build #$build_num digest: $digest" >&2
    echo $digest
}

# Perform two builds
log "===== FIRST BUILD ====="
DIGEST1=$(build_and_get_digest 1)
echo ""

log "Cleaning local image cache..."
docker rmi $TEST_IMAGE:v1 2>/dev/null || true
echo ""

log "===== SECOND BUILD ====="
DIGEST2=$(build_and_get_digest 2)
echo ""

# Compare digests
echo "================================================================"
echo "  RESULTS"
echo "================================================================"
echo "  Build #1 digest: $DIGEST1"
echo "  Build #2 digest: $DIGEST2"
echo "================================================================"
echo ""

if [ "$DIGEST1" = "$DIGEST2" ]; then
    log "✅ SUCCESS: Builds are reproducible!"
    log "Both builds produced identical digest: $DIGEST1"
    exit 0
else
    error "❌ FAILURE: Builds are NOT reproducible!"
fi