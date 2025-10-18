#!/bin/bash
# Smithy Docker Test Suite
# Tests both rootless (UID 1000) and rootful (UID 0) modes
# Tests both VFS and Overlay storage drivers

set -e

# Default configuration - handle internal vs external registry
if [ -z "${RF_APP_HOST}" ]; then
    REGISTRY=${REGISTRY:-"ghcr.io"}
else
    REGISTRY="${RF_APP_HOST}:5000"
fi

SMITHY_IMAGE=${SMITHY_IMAGE:-"${REGISTRY}/rapidfort/smithy:latest"}
RF_SMITHY_TMPDIR=${RF_SMITHY_TMPDIR:-"/tmp"}
STORAGE_DRIVER="both"
CLEANUP_AFTER=false

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

# ============================================================================
# Helper Functions
# ============================================================================

print_section() {
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
}

run_test() {
    local test_name="$1"
    local mode="$2"
    local driver="$3"
    shift 3
    local test_cmd="$@"
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    
    echo -e "${CYAN}[TEST $TOTAL_TESTS]${NC} ${test_name} (${mode}, ${driver})"
    echo -e "${CYAN}  Command: docker run ${test_cmd}${NC}"
    
    # Create temp directory for this test
    local test_tmpdir="${RF_SMITHY_TMPDIR}/smithy-docker-test-$-${TOTAL_TESTS}"
    mkdir -p "${test_tmpdir}"
    
    # Run the test - using eval to properly expand variables
    local start_time=$(date +%s)
    local test_output="${test_tmpdir}/output.log"
    
    if eval "docker run $test_cmd" > "${test_output}" 2>&1; then
        local end_time=$(date +%s)
        local duration=$((end_time - start_time))
        
        echo -e "${GREEN}  ✓ PASS${NC} (${duration}s)"
        PASSED_TESTS=$((PASSED_TESTS + 1))
        TEST_RESULTS+=("PASS: ${test_name} (${mode}, ${driver})")
        
        # Show ALL output
        echo -e "${CYAN}  Build output:${NC}"
        cat "${test_output}" | sed 's/^/    /'
    else
        local end_time=$(date +%s)
        local duration=$((end_time - start_time))
        
        echo -e "${RED}  ✗ FAIL${NC} (${duration}s)"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TEST_RESULTS+=("FAIL: ${test_name} (${mode}, ${driver})")
        
        # Show ALL error output
        echo -e "${RED}  Build output:${NC}"
        cat "${test_output}" | sed 's/^/    /'
    fi
    
    # Cleanup test directory
    rm -rf "${test_tmpdir}"
    echo ""
}

# ============================================================================
# Test Dockerfile Creation
# ============================================================================

create_test_dockerfile() {
    local dockerfile="${RF_SMITHY_TMPDIR}/Dockerfile.smithy-test-$"
    
    cat > "$dockerfile" <<'EOF'
FROM docker.io/library/alpine:latest

# Install basic tools
RUN apk add --no-cache curl bash

# Create test content
RUN echo "Build completed successfully" > /test.txt
RUN echo "Build date: $(date)" >> /test.txt

# Add build args test
ARG VERSION=1.0
ARG BUILD_DATE=unknown

RUN echo "Version: ${VERSION}" >> /test.txt
RUN echo "Build date arg: ${BUILD_DATE}" >> /test.txt

# Add labels
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
    
    print_section "ROOTLESS MODE TESTS (UID 1000) - ${driver^^} STORAGE"
    
    local dockerfile=$(create_test_dockerfile)
    local dockerfile_name=$(basename "$dockerfile")
    local context_dir=$(dirname "$dockerfile")
    
    # Base Docker run command for rootless
    local BASE_CMD="--rm"
    BASE_CMD="$BASE_CMD --user 1000:1000"
    BASE_CMD="$BASE_CMD --cap-drop ALL"
    BASE_CMD="$BASE_CMD --cap-add SETUID"
    BASE_CMD="$BASE_CMD --cap-add SETGID"
    BASE_CMD="$BASE_CMD --security-opt seccomp=unconfined"
    BASE_CMD="$BASE_CMD --security-opt apparmor=unconfined"
    BASE_CMD="$BASE_CMD --device /dev/fuse"
    BASE_CMD="$BASE_CMD -e HOME=/home/smithy"
    BASE_CMD="$BASE_CMD -e DOCKER_CONFIG=/home/smithy/.docker"
    BASE_CMD="$BASE_CMD -v ${context_dir}:/workspace:ro"
    BASE_CMD="$BASE_CMD ${SMITHY_IMAGE}"
    
    # Test 1: Version check
    run_test \
        "Version Check" \
        "rootless" \
        "$driver" \
        $BASE_CMD \
        --version
    
    # Test 2: Check environment
    run_test \
        "Environment Check" \
        "rootless" \
        "$driver" \
        $BASE_CMD \
        check-environment
    
    # Test 3: Basic build
    run_test \
        "Basic Build" \
        "rootless" \
        "$driver" \
        $BASE_CMD \
        --context=/workspace \
        --dockerfile=${dockerfile_name} \
        --destination=test-rootless-basic-${driver}:latest \
        --storage-driver=$driver \
        --no-push \
        --verbosity=debug
    
    # Test 4: Build with args
    run_test \
        "Build with Arguments" \
        "rootless" \
        "$driver" \
        $BASE_CMD \
        --context=/workspace \
        --dockerfile=${dockerfile_name} \
        --destination=test-rootless-buildargs-${driver}:latest \
        --build-arg=VERSION=2.0 \
        --build-arg=BUILD_DATE=$(date +%Y%m%d) \
        --storage-driver=$driver \
        --no-push \
        --verbosity=debug
    
    # Test 5: Build with labels
    run_test \
        "Build with Labels" \
        "rootless" \
        "$driver" \
        $BASE_CMD \
        --context=/workspace \
        --dockerfile=${dockerfile_name} \
        --destination=test-rootless-labels-${driver}:latest \
        --label=test=true \
        --label=storage=${driver} \
        --storage-driver=$driver \
        --no-push \
        --verbosity=debug
    
    # Test 6: Git repository build
    run_test \
        "Git Repository Build" \
        "rootless" \
        "$driver" \
        $BASE_CMD \
        --context=https://github.com/nginxinc/docker-nginx.git \
        --git-branch=master \
        --dockerfile=mainline/alpine/Dockerfile \
        --destination=test-rootless-git-${driver}:latest \
        --storage-driver=$driver \
        --no-push \
        --verbosity=debug
    
    # Cleanup test dockerfile
    rm -f "$dockerfile"
}

# ============================================================================
# Rootful Mode Tests (UID 0)
# ============================================================================

run_rootful_tests() {
    local driver="$1"
    
    print_section "ROOTFUL MODE TESTS (UID 0) - ${driver^^} STORAGE"
    
    local dockerfile=$(create_test_dockerfile)
    local dockerfile_name=$(basename "$dockerfile")
    local context_dir=$(dirname "$dockerfile")
    
    # Base Docker run command for rootful
    local BASE_CMD="--rm"
    BASE_CMD="$BASE_CMD --user 0:0"
    BASE_CMD="$BASE_CMD --privileged"
    BASE_CMD="$BASE_CMD --device /dev/fuse"
    BASE_CMD="$BASE_CMD -e HOME=/root"
    BASE_CMD="$BASE_CMD -e DOCKER_CONFIG=/root/.docker"
    # Mount smithy's policy.json for root to use
    BASE_CMD="$BASE_CMD -v /home/smithy/.config/containers/policy.json:/root/.config/containers/policy.json:ro"
    BASE_CMD="$BASE_CMD -v ${context_dir}:/workspace:ro"
    BASE_CMD="$BASE_CMD ${SMITHY_IMAGE}"
    
    # Test 1: Version check
    run_test \
        "Version Check" \
        "rootful" \
        "$driver" \
        $BASE_CMD \
        --version
    
    # Test 2: Check environment
    run_test \
        "Environment Check" \
        "rootful" \
        "$driver" \
        $BASE_CMD \
        check-environment
    
    # Test 3: Basic build
    run_test \
        "Basic Build" \
        "rootful" \
        "$driver" \
        $BASE_CMD \
        --context=/workspace \
        --dockerfile=${dockerfile_name} \
        --destination=test-rootful-basic-${driver}:latest \
        --storage-driver=$driver \
        --no-push \
        --verbosity=debug
    
    # Test 4: Build with args
    run_test \
        "Build with Arguments" \
        "rootful" \
        "$driver" \
        $BASE_CMD \
        --context=/workspace \
        --dockerfile=${dockerfile_name} \
        --destination=test-rootful-buildargs-${driver}:latest \
        --build-arg=VERSION=2.0 \
        --build-arg=BUILD_DATE=$(date +%Y%m%d) \
        --storage-driver=$driver \
        --no-push \
        --verbosity=debug
    
    # Test 5: Build with labels
    run_test \
        "Build with Labels" \
        "rootful" \
        "$driver" \
        $BASE_CMD \
        --context=/workspace \
        --dockerfile=${dockerfile_name} \
        --destination=test-rootful-labels-${driver}:latest \
        --label=test=true \
        --label=storage=${driver} \
        --storage-driver=$driver \
        --no-push \
        --verbosity=debug
    
    # Test 6: Git repository build
    run_test \
        "Git Repository Build" \
        "rootful" \
        "$driver" \
        $BASE_CMD \
        --context=https://github.com/nginxinc/docker-nginx.git \
        --git-branch=master \
        --dockerfile=mainline/alpine/Dockerfile \
        --destination=test-rootful-git-${driver}:latest \
        --storage-driver=$driver \
        --no-push \
        --verbosity=debug
    
    # Cleanup test dockerfile
    rm -f "$dockerfile"
}

# ============================================================================
# Cleanup Function
# ============================================================================

cleanup() {
    if [ "$CLEANUP_AFTER" = true ]; then
        print_section "CLEANUP"
        
        echo "Removing test images..."
        docker images | grep "test-root" | awk '{print $3}' | xargs -r docker rmi -f 2>/dev/null || true
        
        echo "Removing test files..."
        rm -f ${RF_SMITHY_TMPDIR}/Dockerfile.smithy-test-* 2>/dev/null || true
        
        echo -e "${GREEN}✓ Cleanup completed${NC}"
    fi
}

# Cleanup function for interrupts
cleanup_on_interrupt() {
    echo ""
    echo -e "${YELLOW}Interrupted by user (Ctrl+C)${NC}"
    echo -e "${YELLOW}Cleaning up...${NC}"
    
    # Stop any running containers
    docker ps -q --filter "ancestor=${SMITHY_IMAGE}" | xargs -r docker stop 2>/dev/null || true
    
    # Remove test images
    docker images | grep "test-root" | awk '{print $3}' | xargs -r docker rmi -f 2>/dev/null || true
    
    # Remove test files
    rm -f ${RF_SMITHY_TMPDIR}/Dockerfile.smithy-test-* 2>/dev/null || true
    rm -rf ${RF_SMITHY_TMPDIR}/smithy-docker-test-* 2>/dev/null || true
    
    echo -e "${GREEN}✓ Cleanup completed${NC}"
    exit 130  # Standard exit code for SIGINT
}

# ============================================================================
# Main Execution
# ============================================================================

main() {
    print_section "DOCKER TEST SUITE"
    
    echo -e "${CYAN}Configuration:${NC}"
    echo -e "  Registry:       ${REGISTRY}"
    echo -e "  Image:          ${SMITHY_IMAGE}"
    echo -e "  Storage:        ${STORAGE_DRIVER}"
    echo -e "  Cleanup:        ${CLEANUP_AFTER}"
    echo ""
    
    # Start overall timer
    local overall_start=$(date +%s)
    
    # Check if Docker is available
    if ! command -v docker &> /dev/null; then
        echo -e "${RED}Error: Docker is not installed or not in PATH${NC}"
        exit 1
    fi
    
    # Determine which drivers to test
    local drivers=()
    if [ "$STORAGE_DRIVER" = "both" ]; then
        drivers=("vfs" "overlay")
    else
        drivers=("$STORAGE_DRIVER")
    fi
    
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
        exit 1
    fi
    
    echo -e "${GREEN}✓ All Docker tests passed successfully!${NC}"
    exit 0
}

# Trap cleanup on exit and interrupt
trap cleanup EXIT
trap cleanup_on_interrupt INT TERM

# Run main
main