#!/bin/bash
# Smithy Docker Test Suite
# Tests both rootless (UID 1000) and rootful (UID 0) modes
# Supports BuildKit (default) and Buildah (legacy) images
# Tests storage drivers based on builder:
#   - BuildKit: native (default), overlay
#   - Buildah: vfs (default), overlay
# Note: Overlay requires appropriate capabilities in rootless mode

set -e

# Default configuration - handle internal vs external registry
if [ -z "${RF_APP_HOST}" ]; then
    REGISTRY=${REGISTRY:-"ghcr.io"}
else
    REGISTRY="${RF_APP_HOST}:5000"
fi

SMITHY_IMAGE=${SMITHY_IMAGE:-"${REGISTRY}/rapidfort/smithy:latest"}
RF_SMITHY_TMPDIR=${RF_SMITHY_TMPDIR:-"/tmp"}
BUILDER=${BUILDER:-"buildkit"}  # buildkit or buildah
STORAGE_DRIVER="both"
CLEANUP_AFTER=false

# Script directory
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
SUITES_DIR="${SCRIPT_DIR}/suites"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Test counters
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0
declare -a TEST_RESULTS

# ============================================================================
# Argument Parsing
# ============================================================================

while [[ $# -gt 0 ]]; do
    case $1 in
        --registry)
            REGISTRY="$2"
            shift 2
            ;;
        --image)
            SMITHY_IMAGE="$2"
            shift 2
            ;;
        --builder)
            BUILDER="$2"
            shift 2
            ;;
        --storage)
            STORAGE_DRIVER="$2"
            shift 2
            ;;
        --cleanup)
            CLEANUP_AFTER=true
            shift
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            exit 1
            ;;
    esac
done

# Validate builder
if [[ ! "$BUILDER" =~ ^(buildkit|buildah)$ ]]; then
    echo -e "${RED}Error: Invalid builder '$BUILDER'. Must be: buildkit or buildah${NC}"
    exit 1
fi

# Create suites directory
mkdir -p "${SUITES_DIR}"

# ============================================================================
# Helper Functions
# ============================================================================

print_section() {
    echo ""
    echo -e "${BLUE}────────────────────────────────────────────────────────────${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}────────────────────────────────────────────────────────────${NC}"
    echo ""
}

# Get the primary storage driver name based on builder
get_primary_driver() {
    if [ "$BUILDER" = "buildkit" ]; then
        echo "native"
    else
        echo "vfs"
    fi
}

# Get the actual storage flag value for smithy
get_storage_flag() {
    local driver="$1"
    
    # BuildKit uses 'native' which maps to native snapshotter
    # Buildah uses 'vfs' which maps to VFS storage
    # Both support 'overlay'
    if [ "$driver" = "native" ] && [ "$BUILDER" = "buildah" ]; then
        echo "vfs"  # Fallback for buildah
    elif [ "$driver" = "vfs" ] && [ "$BUILDER" = "buildkit" ]; then
        echo "native"  # Fallback for buildkit
    else
        echo "$driver"
    fi
}

# ============================================================================
# Test Script Generator
# ============================================================================

create_test_script() {
    local test_type="$1"  # "happy" or "unhappy"
    local test_name="$2"
    local mode="$3"
    local driver="$4"
    local test_command="$5"
    
    # Sanitize test name for filename: replace spaces with dashes, lowercase
    local safe_name=$(echo "$test_name" | tr ' ' '-' | tr '[:upper:]' '[:lower:]')
    
    local script_file="${SUITES_DIR}/${test_type}-${BUILDER}-${mode}-${driver}-${safe_name}.sh"
    
    cat > "$script_file" <<TESTSCRIPT
#!/bin/bash
# Auto-generated Docker test script
# Builder: ${BUILDER}
# Type: ${test_type}
# Test: ${test_name}
# Mode: ${mode}
# Driver: ${driver}
# Generated: $(date)

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

echo ""
echo -e "\${CYAN}╔═══════════════════════════════════════════════════════╗\${NC}"
echo -e "\${CYAN}║ Docker Test: ${test_name}\${NC}"
echo -e "\${CYAN}║ Builder: ${BUILDER}\${NC}"
echo -e "\${CYAN}║ Type: ${test_type}\${NC}"
echo -e "\${CYAN}║ Mode: ${mode}\${NC}"
echo -e "\${CYAN}║ Driver: ${driver}\${NC}"
echo -e "\${CYAN}╚═══════════════════════════════════════════════════════╝\${NC}"
echo ""

# Test execution
echo "Running test command..."
echo ""

if ${test_command}; then
    echo ""
    echo -e "\${GREEN}✓ Test PASSED\${NC}"
    exit 0
else
    exit_code=\$?
    echo ""
    echo -e "\${RED}✗ Test FAILED (exit code: \${exit_code})\${NC}"
    exit \$exit_code
fi
TESTSCRIPT
    
    chmod +x "$script_file"
    echo "$script_file"
}

# ============================================================================
# Test Execution
# ============================================================================

run_test() {
    local test_name="$1"
    local mode="$2"
    local driver="$3"
    shift 3
    local test_cmd="$@"
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    
    # CREATE the happy case test script file
    local script_file=$(create_test_script "happy" "$test_name" "$mode" "$driver" "$test_cmd")
    
    echo -e "${CYAN}[TEST $TOTAL_TESTS]${NC} ${test_name} (${BUILDER}, ${mode}, ${driver})"
    echo -e "${CYAN}  Script: $(basename $script_file)${NC}"
    
    # EXECUTE the test script
    if bash "$script_file" > /tmp/test-$$.log 2>&1; then
        echo -e "${GREEN}  ✓ PASS${NC}"
        PASSED_TESTS=$((PASSED_TESTS + 1))
        TEST_RESULTS+=("PASS: ${test_name} (${BUILDER}, ${mode}, ${driver})")
    else
        echo -e "${RED}  ✗ FAIL${NC}"
        echo -e "${YELLOW}  To re-run: bash $script_file${NC}"
        cat /tmp/test-$$.log | sed 's/^/    /'
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TEST_RESULTS+=("FAIL: ${test_name} (${BUILDER}, ${mode}, ${driver})")
    fi
    
    rm -f /tmp/test-$$.log
    echo ""
}

# ============================================================================
# Dockerfile Generator
# ============================================================================

create_test_dockerfile() {
    local dockerfile="/tmp/Dockerfile.test-$$"
    
    cat > "$dockerfile" <<'EOF'
FROM alpine:latest

# Install basic tools
RUN apk add --no-cache bash curl

# Create test file
RUN echo "Test build successful" > /test.txt

# Build args test
ARG VERSION=1.0
ARG BUILD_DATE=unknown

RUN echo "Version: ${VERSION}" >> /test.txt
RUN echo "Build date: ${BUILD_DATE}" >> /test.txt

# Labels
LABEL maintainer="test@example.com"
LABEL version="${VERSION}"

CMD ["cat", "/test.txt"]
EOF
    
    echo "$dockerfile"
}
# ============================================================================
# Rootless Mode Tests (UID 1000)
# ============================================================================

run_rootless_tests() {
    local driver="$1"
    
    # Get the actual storage flag value
    local storage_flag=$(get_storage_flag "$driver")
    
    print_section "ROOTLESS MODE TESTS (UID 1000) - ${BUILDER^^} with ${driver^^} STORAGE"
    
    if [ "$driver" = "overlay" ]; then
        echo -e "${YELLOW}Note: Overlay storage requires additional capabilities in rootless mode${NC}"
        if [ "$BUILDER" = "buildkit" ]; then
            echo -e "${YELLOW}      BuildKit overlay uses fuse-overlayfs${NC}"
        else
            echo -e "${YELLOW}      Buildah overlay requires CAP_MKNOD${NC}"
        fi
        echo ""
    elif [ "$driver" = "native" ]; then
        echo -e "${CYAN}Note: Native snapshotter (BuildKit) provides security with good performance${NC}"
        echo ""
    elif [ "$driver" = "vfs" ]; then
        echo -e "${CYAN}Note: VFS storage (Buildah) is the most secure but slower${NC}"
        echo ""
    fi
    
    local dockerfile=$(create_test_dockerfile)
    local dockerfile_name=$(basename "$dockerfile")
    local context_dir=$(dirname "$dockerfile")
    
    # Base Docker run command for rootless
    local BASE_CMD="docker run --rm"
    BASE_CMD="$BASE_CMD --user 1000:1000"
    BASE_CMD="$BASE_CMD --cap-drop ALL"
    BASE_CMD="$BASE_CMD --cap-add SETUID"
    BASE_CMD="$BASE_CMD --cap-add SETGID"
    
    # Add additional capabilities for overlay
    if [ "$driver" = "overlay" ]; then
        if [ "$BUILDER" = "buildkit" ]; then
            # BuildKit with overlay needs DAC_OVERRIDE for fuse
            BASE_CMD="$BASE_CMD --cap-add DAC_OVERRIDE"
        else
            # Buildah with overlay needs MKNOD
            BASE_CMD="$BASE_CMD --cap-add MKNOD"
        fi
    fi
    
    BASE_CMD="$BASE_CMD --security-opt seccomp=unconfined"
    BASE_CMD="$BASE_CMD --security-opt apparmor=unconfined"
    BASE_CMD="$BASE_CMD --device /dev/fuse"
    BASE_CMD="$BASE_CMD -e HOME=/home/smithy"
    BASE_CMD="$BASE_CMD -e DOCKER_CONFIG=/home/smithy/.docker"
    BASE_CMD="$BASE_CMD -v ${context_dir}:/workspace:ro"
    BASE_CMD="$BASE_CMD ${SMITHY_IMAGE}"
    
    # Test 1: Version check
    run_test \
        "version" \
        "rootless" \
        "$driver" \
        $BASE_CMD --version
    
    # Test 2: Check environment
    run_test \
        "envcheck" \
        "rootless" \
        "$driver" \
        $BASE_CMD check-environment
    
    # Test 3: Basic build
    run_test \
        "basic-build" \
        "rootless" \
        "$driver" \
        $BASE_CMD \
        --context=/workspace \
        --dockerfile=${dockerfile_name} \
        --destination=test-${BUILDER}-rootless-basic-${driver}:latest \
        --storage-driver=${storage_flag} \
        --no-push \
        --verbosity=debug
    
    # Test 4: Build with args
    run_test \
        "build-args" \
        "rootless" \
        "$driver" \
        $BASE_CMD \
        --context=/workspace \
        --dockerfile=${dockerfile_name} \
        --destination=test-${BUILDER}-rootless-buildargs-${driver}:latest \
        --build-arg=VERSION=2.0 \
        --build-arg=BUILD_DATE=$(date +%Y%m%d) \
        --storage-driver=${storage_flag} \
        --no-push \
        --verbosity=debug
    
    # Test 5: Build with labels
    run_test \
        "labels" \
        "rootless" \
        "$driver" \
        $BASE_CMD \
        --context=/workspace \
        --dockerfile=${dockerfile_name} \
        --destination=test-${BUILDER}-rootless-labels-${driver}:latest \
        --label=test=true \
        --label=builder=${BUILDER} \
        --label=storage=${driver} \
        --storage-driver=${storage_flag} \
        --no-push \
        --verbosity=debug
    
    # Test 6: Git repository build
    run_test \
        "git-build" \
        "rootless" \
        "$driver" \
        $BASE_CMD \
        --context=https://github.com/nginxinc/docker-nginx.git \
        --git-branch=master \
        --dockerfile=mainline/alpine/Dockerfile \
        --destination=test-${BUILDER}-rootless-git-${driver}:latest \
        --storage-driver=${storage_flag} \
        --no-push \
        --verbosity=debug
    
    # Cleanup test dockerfile
    rm -f "$dockerfile"
}

# ============================================================================
# Rootful Mode Tests (UID 0) - Docker Only
# ============================================================================

run_rootful_tests() {
    local driver="$1"
    
    # Get the actual storage flag value
    local storage_flag=$(get_storage_flag "$driver")
    
    print_section "ROOTFUL MODE TESTS (UID 0) - ${BUILDER^^} with ${driver^^} STORAGE"
    
    echo -e "${YELLOW}WARNING: Rootful mode for Docker only (NOT for Kubernetes)${NC}"
    echo ""
    
    local dockerfile=$(create_test_dockerfile)
    local dockerfile_name=$(basename "$dockerfile")
    local context_dir=$(dirname "$dockerfile")
    
    # Base Docker run command for rootful
    local BASE_CMD="docker run --rm"
    BASE_CMD="$BASE_CMD --user 0:0"
    BASE_CMD="$BASE_CMD --privileged"
    BASE_CMD="$BASE_CMD --device /dev/fuse"
    BASE_CMD="$BASE_CMD -e HOME=/root"
    BASE_CMD="$BASE_CMD -e DOCKER_CONFIG=/root/.docker"
    BASE_CMD="$BASE_CMD -v ${context_dir}:/workspace:ro"
    BASE_CMD="$BASE_CMD ${SMITHY_IMAGE}"
    
    # Test 1: Version check
    run_test \
        "version" \
        "rootful" \
        "$driver" \
        $BASE_CMD --version
    
    # Test 2: Check environment
    run_test \
        "envcheck" \
        "rootful" \
        "$driver" \
        $BASE_CMD check-environment
    
    # Test 3: Basic build
    run_test \
        "basic-build" \
        "rootful" \
        "$driver" \
        $BASE_CMD \
        --context=/workspace \
        --dockerfile=${dockerfile_name} \
        --destination=test-${BUILDER}-rootful-basic-${driver}:latest \
        --storage-driver=${storage_flag} \
        --no-push \
        --verbosity=debug
    
    # Test 4: Build with args
    run_test \
        "build-args" \
        "rootful" \
        "$driver" \
        $BASE_CMD \
        --context=/workspace \
        --dockerfile=${dockerfile_name} \
        --destination=test-${BUILDER}-rootful-buildargs-${driver}:latest \
        --build-arg=VERSION=2.0 \
        --build-arg=BUILD_DATE=$(date +%Y%m%d) \
        --storage-driver=${storage_flag} \
        --no-push \
        --verbosity=debug
    
    # Test 5: Build with labels
    run_test \
        "labels" \
        "rootful" \
        "$driver" \
        $BASE_CMD \
        --context=/workspace \
        --dockerfile=${dockerfile_name} \
        --destination=test-${BUILDER}-rootful-labels-${driver}:latest \
        --label=test=true \
        --label=builder=${BUILDER} \
        --label=storage=${driver} \
        --storage-driver=${storage_flag} \
        --no-push \
        --verbosity=debug
    
    # Test 6: Git repository build
    run_test \
        "git-build" \
        "rootful" \
        "$driver" \
        $BASE_CMD \
        --context=https://github.com/nginxinc/docker-nginx.git \
        --git-branch=master \
        --dockerfile=mainline/alpine/Dockerfile \
        --destination=test-${BUILDER}-rootful-git-${driver}:latest \
        --storage-driver=${storage_flag} \
        --no-push \
        --verbosity=debug
    
    # Cleanup test dockerfile
    rm -f "$dockerfile"
}

# ============================================================================
# Cleanup
# ============================================================================

cleanup() {
    if [ "$CLEANUP_AFTER" = true ]; then
        print_section "CLEANUP"
        
        echo "Removing temp files..."
        rm -f /tmp/Dockerfile.test-* 2>/dev/null || true
        rm -f /tmp/test-*.log 2>/dev/null || true
        
        echo -e "${GREEN}✓ Cleanup completed${NC}"
    fi
}

cleanup_on_interrupt() {
    echo ""
    echo -e "${YELLOW}Interrupted by user (Ctrl+C)${NC}"
    echo -e "${YELLOW}Cleaning up...${NC}"
    
    rm -f /tmp/Dockerfile.test-* 2>/dev/null || true
    rm -f /tmp/test-*.log 2>/dev/null || true
    
    echo -e "${GREEN}✓ Cleanup completed${NC}"
    exit 130
}
# ============================================================================
# Main Execution
# ============================================================================

main() {
    print_section "DOCKER TEST SUITE"
    
    # Check Docker
    if ! command -v docker &> /dev/null; then
        echo -e "${RED}Error: Docker is not installed or not in PATH${NC}"
        exit 1
    fi
    
    echo -e "${CYAN}Configuration:${NC}"
    echo -e "  Builder:        ${BUILDER}"
    echo -e "  Registry:       ${REGISTRY}"
    echo -e "  Image:          ${SMITHY_IMAGE}"
    echo -e "  Storage:        ${STORAGE_DRIVER}"
    echo -e "  Cleanup:        ${CLEANUP_AFTER}"
    echo -e "  Suites Dir:     ${SUITES_DIR}"
    echo ""
    
    # Describe storage mappings
    echo -e "${CYAN}Storage Driver Mappings:${NC}"
    if [ "$BUILDER" = "buildkit" ]; then
        echo -e "  native  → Native snapshotter (default for BuildKit)"
        echo -e "  overlay → fuse-overlayfs (high performance)"
    else
        echo -e "  vfs     → VFS storage (default for Buildah)"
        echo -e "  overlay → fuse-overlayfs (high performance)"
    fi
    echo ""
    
    # Start overall timer
    local overall_start=$(date +%s)
    
    # Determine which drivers to test based on builder and storage selection
    local drivers=()
    local primary_driver=$(get_primary_driver)
    
    if [ "$STORAGE_DRIVER" = "both" ]; then
        drivers=("$primary_driver" "overlay")
        echo -e "${CYAN}Testing both ${primary_driver} and overlay storage${NC}"
    elif [ "$STORAGE_DRIVER" = "native" ] || [ "$STORAGE_DRIVER" = "vfs" ]; then
        # Map to primary driver
        drivers=("$primary_driver")
        echo -e "${CYAN}Testing ${primary_driver} storage only${NC}"
    elif [ "$STORAGE_DRIVER" = "overlay" ]; then
        drivers=("overlay")
        echo -e "${CYAN}Testing overlay storage only${NC}"
    else
        drivers=("$STORAGE_DRIVER")
        echo -e "${CYAN}Testing ${STORAGE_DRIVER} storage${NC}"
    fi
    echo ""
    
    # Run tests for each storage driver
    for driver in "${drivers[@]}"; do
        # Rootless tests
        run_rootless_tests "$driver"
        
        # Rootful tests
        run_rootful_tests "$driver"
    done
    
    # Cleanup if requested
    cleanup
    
    # Calculate total time
    local overall_end=$(date +%s)
    local overall_duration=$((overall_end - overall_start))
    local overall_minutes=$((overall_duration / 60))
    local overall_seconds=$((overall_duration % 60))
    
    # Print summary
    print_section "TEST SUMMARY"
    
    echo -e "Builder:      ${BUILDER}"
    echo -e "Total Tests:  ${TOTAL_TESTS}"
    echo -e "${GREEN}Passed:       ${PASSED_TESTS}${NC}"
    
    if [ $FAILED_TESTS -gt 0 ]; then
        echo -e "${RED}Failed:       ${FAILED_TESTS}${NC}"
    else
        echo -e "${GREEN}Failed:       ${FAILED_TESTS}${NC}"
    fi
    
    echo -e "Total Time:   ${overall_minutes}m ${overall_seconds}s"
    echo ""
    
    if [ $FAILED_TESTS -gt 0 ]; then
        echo -e "${RED}Failed tests:${NC}"
        for result in "${TEST_RESULTS[@]}"; do
            if [[ $result == FAIL* ]]; then
                echo -e "${RED}  - $result${NC}"
            fi
        done
        echo ""
        echo -e "${YELLOW}Re-run individual tests from:${NC}"
        echo -e "${YELLOW}  ${SUITES_DIR}/${NC}"
        exit 1
    fi
    
    echo -e "${GREEN}✓ All Docker tests passed successfully!${NC}"
    echo ""
    echo -e "${CYAN}Generated test scripts in: ${SUITES_DIR}/${NC}"
    echo -e "${CYAN}Example: bash ${SUITES_DIR}/happy-${BUILDER}-rootless-${primary_driver}-version.sh${NC}"
    exit 0
}

# Trap cleanup on exit and interrupt
trap cleanup EXIT
trap cleanup_on_interrupt INT TERM

# Run main
main