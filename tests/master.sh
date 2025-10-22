#!/bin/bash
# Smithy Master Test Script
# Main orchestrator for Docker and Kubernetes tests
# Supports both BuildKit (default) and Buildah (legacy) images

set -e

# Script directory
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# Source configuration if exists
[ -f "${SCRIPT_DIR}/test-config.sh" ] && source "${SCRIPT_DIR}/test-config.sh"

# Default configuration - handle internal vs external registry
if [ -z "${RF_APP_HOST}" ]; then
    REGISTRY=${REGISTRY:-"ghcr.io"}
else
    REGISTRY="${RF_APP_HOST}:5000"
fi
NAMESPACE=${NAMESPACE:-"smithy-tests"}
SMITHY_IMAGE=${SMITHY_IMAGE:-"${REGISTRY}/rapidfort/smithy:latest"}
RF_SMITHY_TMPDIR=${RF_SMITHY_TMPDIR:-"/tmp"}
BUILDER=${BUILDER:-"buildkit"}  # buildkit (default) or buildah

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Test mode
TEST_MODE=""
CLEANUP_AFTER=false
STORAGE_DRIVER="both"

# Test counters
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# ============================================================================
# Usage Function
# ============================================================================

usage() {
    cat <<EOF
${CYAN}Smithy Master Test Script${NC}
Main orchestrator for Docker and Kubernetes tests

${YELLOW}USAGE:${NC}
    $0 [OPTIONS]

${YELLOW}OPTIONS:${NC}
    -h, --help                  Show this help message
    -m, --mode MODE             Test mode: docker, kubernetes, both (required)
    -r, --registry URL          Registry URL (default: ${REGISTRY})
    -i, --image IMAGE           Smithy image to test
    -b, --builder BUILDER       Builder: buildkit (default), buildah
    -s, --storage DRIVER        Storage driver: vfs, overlay, native, both (default: both)
    -c, --cleanup               Clean up resources after tests
    --namespace NAMESPACE       Kubernetes namespace (default: ${NAMESPACE})

${YELLOW}MODES:${NC}
    docker                      Run Docker tests only (rootless + rootful)
    kubernetes                  Run Kubernetes tests only (rootless + rootful)
    both                        Run all tests

${YELLOW}BUILDERS:${NC}
    buildkit                    Test BuildKit-based smithy (default, recommended)
    buildah                     Test Buildah-based smithy-bud (legacy)

${YELLOW}STORAGE DRIVERS:${NC}
    native                      Test native storage (BuildKit) or vfs (Buildah)
    vfs                         Test VFS storage (legacy, Buildah only)
    overlay                     Test Overlay storage with fuse-overlayfs
    both                        Test both primary driver and overlay (default)

${YELLOW}STORAGE MAPPING:${NC}
    BuildKit:
      - native:  Native snapshotter (default, secure)
      - overlay: fuse-overlayfs (high performance)
    Buildah:
      - vfs:     VFS storage (default, secure)
      - overlay: fuse-overlayfs (high performance)

${YELLOW}EXAMPLES:${NC}
    # Run all tests with BuildKit (default)
    $0 -m both

    # Run Docker tests only with BuildKit and native storage
    $0 -m docker -s native

    # Run tests with Buildah image
    $0 -m both -b buildah

    # Run Kubernetes tests with cleanup
    $0 -m kubernetes -c

    # Use specific image
    $0 -m both -i myregistry/smithy:test

${YELLOW}TEST COVERAGE:${NC}
    Docker Tests:
      - Rootless mode (UID 1000) with native/vfs and overlay
      - Rootful mode (UID 0) with native/vfs and overlay
      - Version checks, basic builds, build-args, Git repos
    
    Kubernetes Tests:
      - Rootless mode with capabilities
      - Rootful mode without capabilities
      - Both storage drivers
      - All build scenarios

${YELLOW}ENVIRONMENT VARIABLES:${NC}
    REGISTRY                    Override registry URL
    SMITHY_IMAGE                Override smithy image
    NAMESPACE                   Override K8s namespace
    RF_SMITHY_TMPDIR           Override temp directory
    BUILDER                     Override builder (buildkit/buildah)

EOF
    exit 0
}

# ============================================================================
# Argument Parsing
# ============================================================================

parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                usage
                ;;
            -m|--mode)
                TEST_MODE="$2"
                shift 2
                ;;
            -r|--registry)
                REGISTRY="$2"
                shift 2
                ;;
            -i|--image)
                SMITHY_IMAGE="$2"
                shift 2
                ;;
            -b|--builder)
                BUILDER="$2"
                shift 2
                ;;
            -s|--storage)
                STORAGE_DRIVER="$2"
                shift 2
                ;;
            -c|--cleanup)
                CLEANUP_AFTER=true
                shift
                ;;
            --namespace)
                NAMESPACE="$2"
                shift 2
                ;;
            *)
                echo -e "${RED}Error: Unknown option: $1${NC}"
                usage
                ;;
        esac
    done

    # Validate required arguments
    if [ -z "$TEST_MODE" ]; then
        echo -e "${RED}Error: Test mode (-m/--mode) is required${NC}"
        usage
    fi

    # Validate test mode
    if [[ ! "$TEST_MODE" =~ ^(docker|kubernetes|both)$ ]]; then
        echo -e "${RED}Error: Invalid test mode. Must be: docker, kubernetes, or both${NC}"
        usage
    fi

    # Validate builder
    if [[ ! "$BUILDER" =~ ^(buildkit|buildah)$ ]]; then
        echo -e "${RED}Error: Invalid builder. Must be: buildkit or buildah${NC}"
        usage
    fi

    # Validate storage driver
    if [[ ! "$STORAGE_DRIVER" =~ ^(native|vfs|overlay|both)$ ]]; then
        echo -e "${RED}Error: Invalid storage driver. Must be: native, vfs, overlay, or both${NC}"
        usage
    fi

    # Auto-set image based on builder if not specified
    if [ -z "$SMITHY_IMAGE" ] || [ "$SMITHY_IMAGE" = "${REGISTRY}/rapidfort/smithy:latest" ]; then
        if [ "$BUILDER" = "buildah" ]; then
            SMITHY_IMAGE="${REGISTRY}/rapidfort/smithy-bud:latest"
            echo -e "${CYAN}Auto-selected Buildah image: ${SMITHY_IMAGE}${NC}"
        else
            SMITHY_IMAGE="${REGISTRY}/rapidfort/smithy:latest"
            echo -e "${CYAN}Auto-selected BuildKit image: ${SMITHY_IMAGE}${NC}"
        fi
    fi

    # Normalize storage driver for builder
    if [ "$STORAGE_DRIVER" = "native" ] && [ "$BUILDER" = "buildah" ]; then
        echo -e "${YELLOW}Warning: 'native' storage not supported by Buildah, using 'vfs' instead${NC}"
        STORAGE_DRIVER="vfs"
    fi
    
    if [ "$STORAGE_DRIVER" = "vfs" ] && [ "$BUILDER" = "buildkit" ]; then
        echo -e "${YELLOW}Warning: 'vfs' storage not recommended for BuildKit, consider 'native' instead${NC}"
    fi
}

# ============================================================================
# Helper Functions
# ============================================================================

# Cleanup function for interrupts
cleanup_on_interrupt() {
    echo ""
    echo -e "${YELLOW}Interrupted by user (Ctrl+C)${NC}"
    echo -e "${YELLOW}Stopping tests and cleaning up...${NC}"
    
    # Kill any running test scripts
    pkill -P $$ 2>/dev/null || true
    
    echo -e "${GREEN}✓ Cleanup completed${NC}"
    exit 130  # Standard exit code for SIGINT
}

print_header() {
    echo ""
    echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
    echo ""
}

print_test_summary() {
    echo ""
    echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}  TEST SUMMARY${NC}"
    echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
    echo -e "Total Tests:  ${TOTAL_TESTS}"
    echo -e "${GREEN}Passed:       ${PASSED_TESTS}${NC}"
    if [ $FAILED_TESTS -gt 0 ]; then
        echo -e "${RED}Failed:       ${FAILED_TESTS}${NC}"
    else
        echo -e "${GREEN}Failed:       ${FAILED_TESTS}${NC}"
    fi
    echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
    echo ""
}

# ============================================================================
# Main Test Execution
# ============================================================================

run_docker_tests() {
    print_header "DOCKER TESTS"
    
    # Build command
    local cmd="${SCRIPT_DIR}/docker-tests.sh"
    cmd="$cmd --registry $REGISTRY"
    cmd="$cmd --image $SMITHY_IMAGE"
    cmd="$cmd --builder $BUILDER"
    cmd="$cmd --storage $STORAGE_DRIVER"
    
    [ "$CLEANUP_AFTER" = true ] && cmd="$cmd --cleanup"
    
    # Execute
    if bash $cmd; then
        echo -e "${GREEN}✓ Docker tests completed successfully${NC}"
        return 0
    else
        echo -e "${RED}✗ Docker tests failed${NC}"
        return 1
    fi
}

run_kubernetes_tests() {
    print_header "KUBERNETES TESTS"
    
    # Build command
    local cmd="${SCRIPT_DIR}/k8s-tests.sh"
    cmd="$cmd --registry $REGISTRY"
    cmd="$cmd --image $SMITHY_IMAGE"
    cmd="$cmd --namespace $NAMESPACE"
    cmd="$cmd --builder $BUILDER"
    cmd="$cmd --storage $STORAGE_DRIVER"
    
    [ "$CLEANUP_AFTER" = true ] && cmd="$cmd --cleanup"
    
    # Execute
    if bash $cmd; then
        echo -e "${GREEN}✓ Kubernetes tests completed successfully${NC}"
        return 0
    else
        echo -e "${RED}✗ Kubernetes tests failed${NC}"
        return 1
    fi
}

main() {
    parse_args "$@"
    
    print_header "SMITHY TEST SUITE"
    
    echo -e "${CYAN}Configuration:${NC}"
    echo -e "  Mode:           ${TEST_MODE}"
    echo -e "  Builder:        ${BUILDER}"
    echo -e "  Registry:       ${REGISTRY}"
    echo -e "  Image:          ${SMITHY_IMAGE}"
    echo -e "  Storage:        ${STORAGE_DRIVER}"
    echo -e "  Namespace:      ${NAMESPACE}"
    echo -e "  Cleanup:        ${CLEANUP_AFTER}"
    echo ""
    
    # Start overall timer
    local overall_start=$(date +%s)
    
    # Check if test scripts exist
    if [ "$TEST_MODE" = "docker" ] || [ "$TEST_MODE" = "both" ]; then
        if [ ! -f "${SCRIPT_DIR}/docker-tests.sh" ]; then
            echo -e "${RED}Error: docker-tests.sh not found${NC}"
            exit 1
        fi
    fi
    
    if [ "$TEST_MODE" = "kubernetes" ] || [ "$TEST_MODE" = "both" ]; then
        if [ ! -f "${SCRIPT_DIR}/k8s-tests.sh" ]; then
            echo -e "${RED}Error: k8s-tests.sh not found${NC}"
            exit 1
        fi
    fi
    
    # Track overall success
    local overall_success=true
    
    # Run tests based on mode
    case $TEST_MODE in
        docker)
            if ! run_docker_tests; then
                overall_success=false
            fi
            ;;
        kubernetes)
            if ! run_kubernetes_tests; then
                overall_success=false
            fi
            ;;
        both)
            if ! run_docker_tests; then
                overall_success=false
            fi
            
            if ! run_kubernetes_tests; then
                overall_success=false
            fi
            ;;
    esac
    
    # Calculate total time
    local overall_end=$(date +%s)
    local overall_duration=$((overall_end - overall_start))
    local overall_minutes=$((overall_duration / 60))
    local overall_seconds=$((overall_duration % 60))
    
    # Final summary
    print_header "FINAL RESULTS"
    
    echo -e "Total Time:   ${overall_minutes}m ${overall_seconds}s"
    echo ""
    
    if [ "$overall_success" = true ]; then
        echo -e "${GREEN}✓ All test suites completed successfully!${NC}"
        exit 0
    else
        echo -e "${RED}✗ Some test suites failed. Check logs above.${NC}"
        exit 1
    fi
}

# Trap interrupt signal
trap cleanup_on_interrupt INT TERM

# Run main
main "$@"