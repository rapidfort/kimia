#!/bin/bash
# Smithy Master Test Script
# Main orchestrator for Docker and Kubernetes tests

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
    -s, --storage DRIVER        Storage driver: vfs, overlay, both (default: both)
    -c, --cleanup               Clean up resources after tests
    --namespace NAMESPACE       Kubernetes namespace (default: ${NAMESPACE})

${YELLOW}MODES:${NC}
    docker                      Run Docker tests only (rootless + rootful)
    kubernetes                  Run Kubernetes tests only (rootless + rootful)
    both                        Run all tests

${YELLOW}STORAGE DRIVERS:${NC}
    vfs                         Test VFS storage driver only
    overlay                     Test Overlay storage driver only
    both                        Test both drivers (default)

${YELLOW}EXAMPLES:${NC}
    # Run all tests
    $0 -m both

    # Run Docker tests only with VFS
    $0 -m docker -s vfs

    # Run Kubernetes tests with cleanup
    $0 -m kubernetes -c

    # Use specific image
    $0 -m both -i myregistry/smithy:test

${YELLOW}TEST COVERAGE:${NC}
    Docker Tests:
      - Rootless mode (UID 1000) with VFS and Overlay
      - Rootful mode (UID 0) with VFS and Overlay
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

    # Validate storage driver
    if [[ ! "$STORAGE_DRIVER" =~ ^(vfs|overlay|both)$ ]]; then
        echo -e "${RED}Error: Invalid storage driver. Must be: vfs, overlay, or both${NC}"
        usage
    fi
}

# ============================================================================
# Helper Functions
# ============================================================================

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

# Run main
main "$@"