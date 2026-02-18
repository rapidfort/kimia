#!/bin/bash
# Kimia Docker Test Suite
# Tests rootless mode (UID 1000) only
# Supports BuildKit (default) and Buildah (legacy) images
# Tests storage drivers based on builder:
#   - BuildKit: native (default), overlay
#   - Buildah: vfs (default), overlay
# Note: Uses native kernel overlayfs via user namespaces

set -e

export LC_ALL="${LC_ALL:-en_US.UTF-8}"
export LANG="${LANG:-en_US.UTF-8}"
export LANGUAGE="${LANGUAGE:-en_US.UTF-8}"

# Default configuration - handle internal vs external registry
if [ -z "${RF_APP_HOST}" ]; then
    REGISTRY=${REGISTRY:-"ghcr.io"}
else
    REGISTRY="${RF_APP_HOST}:5000"
fi

KIMIA_IMAGE=${KIMIA_IMAGE:-"${REGISTRY}/rapidfort/kimia:latest"}
RF_KIMIA_TMPDIR=${RF_KIMIA_TMPDIR:-"/tmp"}
BUILDER=${BUILDER:-"buildkit"}  # buildkit or buildah
STORAGE_DRIVER="both"
CLEANUP_AFTER=false
TEST_SUITE="all"  # all, simple, reproducible, attestation, signing

# Cosign configuration
COSIGN_KEY_PATH=${COSIGN_KEY_PATH:-"/tmp/cosign/cosign.key"}
COSIGN_PASSWORD=${COSIGN_PASSWORD:-"pib"}

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
# Usage Function
# ============================================================================

show_help() {
    echo -e "${CYAN}Kimia Docker Test Suite${NC}"
    echo "Tests rootless mode (UID 1000) with optional rootful mode"
    echo ""
    echo -e "${YELLOW}USAGE:${NC}"
    echo "    $0 [OPTIONS]"
    echo ""
    echo -e "${YELLOW}OPTIONS:${NC}"
    echo "    -h, --help              Show this help message"
    echo "    --registry URL          Registry URL (default: ghcr.io)"
    echo "    --image IMAGE           Kimia image to test"
    echo "    --builder TYPE          Builder: buildkit (default) or buildah"
    echo "    --storage DRIVER        Storage: both (default), native/vfs, overlay"
    echo "    --tests SUITE           Test suite to run (default: all)"
    echo "    --cleanup               Clean up resources after tests"
    echo ""
    echo -e "${YELLOW}TEST SUITES:${NC}"
    echo "    all                     Run all tests (default)"
    echo "    simple                  Basic tests (6 tests, ~5 min)"
    echo "                           - Version check, env check"
    echo "                           - Git builds: nginx, redis, postgres"
    echo "    reproducible            Reproducible build tests (3 tests, ~7 min)"
    echo "                           - Build twice, compare digests"
    echo "    attestation             Attestation tests (10 tests, ~15 min, BuildKit only)"
    echo "                           - Simple modes: default, min, max, off"
    echo "                           - Docker-style: sbom, provenance, scan options"
    echo "                           - Combined: both attestations, pass-through"
    echo "    signing                 Signing tests (1 test, ~3 min, BuildKit only)"
    echo "    caching                 Cache export/import tests (6 tests, ~10 min, BuildKit only)"
    echo "                           - Attestation with cosign signing"
    echo ""
    echo -e "${YELLOW}EXAMPLES:${NC}"
    echo "    # Run all tests"
    echo "    $0 --tests all"
    echo ""
    echo "    # Quick debug: run simple tests only"
    echo "    $0 --tests simple"
    echo ""
    echo "    # Test reproducible builds"
    echo "    $0 --tests reproducible"
    echo ""
    echo "    # Test attestation features (BuildKit only)"
    echo "    $0 --tests attestation --builder buildkit"
    echo ""
    echo "    # Test signing (requires cosign key)"
    echo "    $0 --tests signing --builder buildkit"
    echo ""
    echo -e "${YELLOW}NOTES:${NC}"
    echo "    - Attestation and signing tests require BuildKit builder"
    echo "    - Signing tests require COSIGN_KEY_PATH to be set"
    echo "    - Use --tests to run specific test suites for faster debugging"
    echo ""
    exit 0
}

# ============================================================================
# Argument Parsing
# ============================================================================

while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            show_help
            ;;
        --registry)
            REGISTRY="$2"
            shift 2
            ;;
        --image)
            KIMIA_IMAGE="$2"
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
        --tests)
            TEST_SUITE="$2"
            shift 2
            ;;
        --cleanup)
            CLEANUP_AFTER=true
            shift
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            show_help
            ;;
    esac
done

# Validate builder
if [[ ! "$BUILDER" =~ ^(buildkit|buildah)$ ]]; then
    echo -e "${RED}Error: Invalid builder '$BUILDER'. Must be: buildkit or buildah${NC}"
    exit 1
fi

# Validate test suite
if [[ ! "$TEST_SUITE" =~ ^(all|simple|reproducible|attestation|signing|caching)$ ]]; then
    echo -e "${RED}Error: Invalid test suite '$TEST_SUITE'.${NC}"
    echo -e "${RED}Must be: all, simple, reproducible, attestation, or signing${NC}"
    exit 1
fi

# Validate attestation/signing tests require BuildKit
if [[ "$TEST_SUITE" =~ ^(attestation|signing|caching)$ ]] && [ "$BUILDER" != "buildkit" ]; then
    echo -e "${RED}Error: ${TEST_SUITE} tests require BuildKit builder${NC}"
    echo -e "${YELLOW}Please use: --builder buildkit${NC}"
    exit 1
fi

# Create suites directory
mkdir -p "${SUITES_DIR}"

# Option 1: Check for .env file
if [ -f "${SCRIPT_DIR}/.env" ]; then
    echo -e "${CYAN}Loading credentials from .env file...${NC}"
    set -a
    source "${SCRIPT_DIR}/.env"
    set +a
    if [ -n "$DOCKER_USERNAME" ] && [ -n "$DOCKER_PASSWORD" ] && [ -n "$DOCKER_REGISTRY" ]; then
        DOCKER_AUTH_METHOD="env"
    fi
fi

# Option 2: Check environment variables
if [ -z "$DOCKER_AUTH_METHOD" ]; then
    if [ -n "$DOCKER_USERNAME" ] && [ -n "$DOCKER_PASSWORD" ] && [ -n "$DOCKER_REGISTRY" ]; then
        echo -e "${CYAN}Using credentials from environment variables...${NC}"
        DOCKER_AUTH_METHOD="env"
    fi
fi

# Option 3: Check Docker config
if [ -z "$DOCKER_AUTH_METHOD" ]; then
    if [ -f "$HOME/.docker/config.json" ]; then
        echo -e "${CYAN}Using credentials from Docker config...${NC}"
        DOCKER_AUTH_METHOD="config"
    fi
fi

# Option 4: Anonymous mode
if [ -z "$DOCKER_AUTH_METHOD" ]; then
    echo -e "${YELLOW}Warning: No Docker credentials found. Continuing in anonymous mode...${NC}"
    echo -e "${YELLOW}Note: You may hit rate limits. Set DOCKER_USERNAME, DOCKER_PASSWORD, and DOCKER_REGISTRY to avoid this.${NC}"
    DOCKER_AUTH_METHOD="anonymous"
fi

# ============================================================================
# Helper Functions
# ============================================================================

print_section() {
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
}

# Generate cosign key if it doesn't exist
ensure_cosign_key() {
    local key_dir=$(dirname "${COSIGN_KEY_PATH}")
    local key_file="${COSIGN_KEY_PATH}"
    local pub_file="${key_file%.key}.pub"

    mkdir -p "${key_dir}"

    # Check if key already exists
    if [ -f "${key_file}" ] && [ -f "${pub_file}" ]; then
        echo -e "${GREEN}✓ Cosign key already exists: ${key_file}${NC}"
        return 0
    fi

    echo -e "${CYAN}Generating cosign key pair...${NC}"

    if command -v cosign &> /dev/null; then
        # Use local cosign if available
        COSIGN_PASSWORD="${COSIGN_PASSWORD}" cosign generate-key-pair --output-key-prefix="${key_dir}/cosign"
    else
        # Use cosign from docker image
        docker run --rm \
            -e COSIGN_PASSWORD="${COSIGN_PASSWORD}" \
            -v "${key_dir}:/tmp/cosign" \
            gcr.io/projectsigstore/cosign:latest \
            generate-key-pair --output-key-prefix="/tmp/cosign/cosign"
    fi
    chown -R 1000:1000 "${key_dir}/cosign"
    if [ -f "${key_file}" ] && [ -f "${pub_file}" ]; then
        echo -e "${GREEN}✓ Cosign key pair generated successfully${NC}"
        echo -e "${CYAN}  Private key: ${key_file}${NC}"
        echo -e "${CYAN}  Public key:  ${pub_file}${NC}"
        echo -e "${CYAN}  Password:    ${COSIGN_PASSWORD}${NC}"
    else
        echo -e "${RED}✗ Failed to generate cosign key pair${NC}"
        return 1
    fi
}

print_test_header() {
    echo ""
    echo -e "${CYAN}──────────────────────────────────────────────────────────${NC}"
    echo -e "${CYAN}Test: $1${NC}"
    echo -e "${CYAN}Mode: $2 | Storage: $3${NC}"
    echo -e "${CYAN}──────────────────────────────────────────────────────────${NC}"
}

run_test() {
    local test_name=$1
    local mode=$2
    local driver=$3
    shift 3
    local cmd="$@"

    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    print_test_header "$test_name" "$mode" "$driver"

    local log_file="${SUITES_DIR}/test-${test_name}-${mode}-${driver}.log"
    local start_time=$(date +%s)
    echo "Log: $log_file"
    echo "Command: $cmd"
    echo ""

    if eval "$cmd" > "$log_file" 2>&1; then
        local end_time=$(date +%s)
        local duration=$((end_time - start_time))
        echo -e "${GREEN}✓ PASSED${NC} (${duration}s)"
        PASSED_TESTS=$((PASSED_TESTS + 1))
        TEST_RESULTS+=("PASS: ${test_name} (${BUILDER}, ${mode}, ${driver}) - ${duration}s")
    else
        local end_time=$(date +%s)
        local duration=$((end_time - start_time))
        echo -e "${RED}✗ FAILED${NC} (${duration}s)"
        echo "Check log: $log_file"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TEST_RESULTS+=("FAIL: ${test_name} (${BUILDER}, ${mode}, ${driver}) - ${duration}s")
    fi
}

print_test_summary() {
    echo ""
    print_section "TEST SUMMARY"

    echo "Total Tests:  $TOTAL_TESTS"
    echo -e "${GREEN}Passed:       $PASSED_TESTS${NC}"
    if [ $FAILED_TESTS -gt 0 ]; then
        echo -e "${RED}Failed:       $FAILED_TESTS${NC}"
    else
        echo -e "${GREEN}Failed:       $FAILED_TESTS${NC}"
    fi
    echo ""

    echo "Detailed Results:"
    for result in "${TEST_RESULTS[@]}"; do
        if [[ $result == PASS:* ]]; then
            echo -e "  ${GREEN}$result${NC}"
        else
            echo -e "  ${RED}$result${NC}"
        fi
    done
    echo ""
}

# ============================================================================
# Test Functions by Suite
# ============================================================================

should_run_simple() {
    [[ "$TEST_SUITE" == "all" || "$TEST_SUITE" == "simple" ]]
}

should_run_reproducible() {
    [[ "$TEST_SUITE" == "all" || "$TEST_SUITE" == "reproducible" ]]
}

should_run_attestation() {
    [[ "$TEST_SUITE" == "all" || "$TEST_SUITE" == "attestation" ]] && [ "$BUILDER" = "buildkit" ]
}

should_run_signing() {
    [[ "$TEST_SUITE" == "all" || "$TEST_SUITE" == "signing" ]] && [ "$BUILDER" = "buildkit" ]
}

should_run_caching() {
    [[ "$TEST_SUITE" == "all" || "$TEST_SUITE" == "caching" ]] && [ "$BUILDER" = "buildkit" ]
}

run_rootless_tests() {
    local driver=$1
    local storage_flag=""

    print_section "ROOTLESS MODE TESTS (UID 1000) - $driver"

    # Set storage flag based on builder and driver
    if [ "$BUILDER" = "buildkit" ]; then
        if [ "$driver" != "overlay" ]; then
            storage_flag="native"
        else
            storage_flag="overlay"
        fi
    else  # buildah
        KIMIA_IMAGE=$(echo "$KIMIA_IMAGE" | sed 's|/kimia\([:@]\)|/kimia-bud\1|; s|/kimia$|/kimia-bud|')
        if [ "$driver" != "overlay" ]; then
            storage_flag="vfs"
        else
            storage_flag="overlay"
        fi
    fi

    # Base docker run command
    local BASE_CMD="docker run --rm"
    BASE_CMD="$BASE_CMD --user 1000:1000"
    #BASE_CMD="$BASE_CMD -v ${RF_KIMIA_TMPDIR}:/tmp"
    BASE_CMD="$BASE_CMD --cap-add SETUID --cap-add SETGID"
    BASE_CMD="$BASE_CMD --security-opt seccomp=unconfined"
    BASE_CMD="$BASE_CMD --security-opt apparmor=unconfined"

    # Add Docker auth if available
    if [ "$DOCKER_AUTH_METHOD" = "env" ]; then
        BASE_CMD="$BASE_CMD -e DOCKER_USERNAME=${DOCKER_USERNAME}"
        BASE_CMD="$BASE_CMD -e DOCKER_PASSWORD=${DOCKER_PASSWORD}"
        BASE_CMD="$BASE_CMD -e DOCKER_REGISTRY=${DOCKER_REGISTRY}"
    elif [ "$DOCKER_AUTH_METHOD" = "config" ]; then
        BASE_CMD="$BASE_CMD -v $HOME/.docker/config.json:/home/kimia/.docker/config.json:ro"
    fi

    # Add emptyDir for buildah overlay
    if [ "$BUILDER" = "buildah" ] && [ "$driver" = "overlay" ]; then
        BASE_CMD="$BASE_CMD -v /tmp/kimia-buildah-overlay:/home/kimia/.local"
        mkdir -p /tmp/kimia-buildah-overlay
        chown 1000:1000 /tmp/kimia-buildah-overlay
    fi

    if should_run_signing; then
        ensure_cosign_key
        chown -R 1000:1000 "$(dirname ${COSIGN_KEY_PATH})"
        BASE_CMD="$BASE_CMD -e COSIGN_PASSWORD=${COSIGN_PASSWORD}"
        BASE_CMD="$BASE_CMD -v ${COSIGN_KEY_PATH}:/tmp/cosign/cosign.key:ro"
    fi

    BASE_CMD="$BASE_CMD ${KIMIA_IMAGE}"


    # ========================================================================
    # SIMPLE TESTS (Tests 1-6)
    # ========================================================================

    if should_run_simple; then
        #Test 1: Version check
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

        # Test 3: Build from git - nginx
        echo $BASE_CMD
        run_test \
            "git-nginx" \
            "rootless" \
            "$driver" \
            $BASE_CMD \
            --context=https://github.com/nginxinc/docker-nginx.git \
            --git-branch=master \
            --dockerfile=mainline/alpine/Dockerfile \
            --destination=${REGISTRY}/${BUILDER}-rootless-nginx-${driver}:latest \
            --storage-driver=${storage_flag} \
            --insecure \
            --verbosity=debug

        #Test 4: Build from git with context sub-path - alpine
        echo $BASE_CMD

        run_test \
            "https-alpine-subpath" \
            "rootless" \
            "$driver" \
            $BASE_CMD \
            --context=https://github.com/alpinelinux/docker-alpine.git \
            --context-sub-path="" \
            --dockerfile=Dockerfile \
            --destination=${REGISTRY}/${BUILDER}-rootless-sub-path-alpine-${driver}:latest \
            --storage-driver=${storage_flag} \
            --insecure \
            --verbosity=debug

        #Test 5: Build from git with context sub-path
        echo $BASE_CMD
        run_test \
            "git-alpine-subpath" \
            "rootless" \
            "$driver" \
            $BASE_CMD \
            --context=git://github.com/alpinelinux/docker-alpine.git \
            --context-sub-path="" \
            --dockerfile=Dockerfile \
            --destination=${REGISTRY}/${BUILDER}-rootless-sub-path-alpine-${driver}:latest \
            --storage-driver=${storage_flag} \
            --insecure \
            --verbosity=debug
    fi

    # ========================================================================
    # REPRODUCIBLE BUILD TESTS (Test 6)
    # ========================================================================

    if should_run_reproducible; then
        local test_image="${REGISTRY}/${BUILDER}-reproducible-test-${driver}"

        # First build
        echo ""
        echo -e "${CYAN}Building image (first build)...${NC}"
        run_test \
            "reproducible-build1" \
            "rootless" \
            "$driver" \
            $BASE_CMD \
            --context=https://github.com/rapidfort/kimia.git \
            --git-branch=main \
            --dockerfile=tests/examples/Dockerfile \
            --destination=${test_image}:v1 \
            --storage-driver=${storage_flag} \
            --reproducible \
            --insecure \
            --verbosity=debug

        docker pull ${test_image}:v1 || true

        # Extract digest from first build
        local digest1=$(docker inspect ${test_image}:v1 --format='{{index .RepoDigests 0}}' 2>/dev/null | cut -d'@' -f2)
        if [ -z "$digest1" ]; then
            echo -e "${YELLOW}Warning: Could not extract digest from first build${NC}"
            digest1="none"
        fi
        echo "First build digest: $digest1"

        # Wait a moment
        sleep 2

        # Second build
        echo ""
        echo -e "${CYAN}Building image (second build)...${NC}"
        run_test \
            "reproducible-build2" \
            "rootless" \
            "$driver" \
            $BASE_CMD \
            --context=https://github.com/rapidfort/kimia.git \
            --git-branch=main \
            --dockerfile=tests/examples/Dockerfile \
            --destination=${test_image}:v2 \
            --storage-driver=${storage_flag} \
            --reproducible \
            --insecure \
            --verbosity=debug

        docker pull ${test_image}:v2 || true

        # Extract digest from second build
        local digest2=$(docker inspect ${test_image}:v2 --format='{{index .RepoDigests 0}}' 2>/dev/null | cut -d'@' -f2)
        if [ -z "$digest2" ]; then
            echo -e "${YELLOW}Warning: Could not extract digest from second build${NC}"
            digest2="none"
        fi
        echo "Second build digest: $digest2"

        # Compare digests
        echo ""
        echo -e "${CYAN}Comparing digests...${NC}"
        TOTAL_TESTS=$((TOTAL_TESTS + 1))

        if [ "$digest1" = "$digest2" ] && [ "$digest1" != "none" ]; then
            echo -e "${GREEN}✓ SUCCESS: Builds are reproducible!${NC}"
            echo "Digest: $digest1"
            PASSED_TESTS=$((PASSED_TESTS + 1))
            TEST_RESULTS+=("PASS: reproducible-comparison (${BUILDER}, rootless, ${driver})")
        else
            echo -e "${RED}✗ FAILURE: Builds are NOT reproducible!${NC}"
            echo "First:  $digest1"
            echo "Second: $digest2"
            FAILED_TESTS=$((FAILED_TESTS + 1))
            TEST_RESULTS+=("FAIL: reproducible-comparison (${BUILDER}, rootless, ${driver})")
        fi

        # Cleanup test images
        docker rmi ${test_image}:v1 2>/dev/null || true
        docker rmi ${test_image}:v2 2>/dev/null || true
        echo ""
    fi

    # ========================================================================
    # ATTESTATION TESTS (Tests 7, BuildKit only)
    # ========================================================================

    if should_run_attestation; then
        # Test 7: Attestation - default mode (should default to min)
        run_test \
            "attestation-default" \
            "rootless" \
            "$driver" \
            $BASE_CMD \
            --context=https://github.com/rapidfort/kimia.git \
            --git-branch=main \
            --dockerfile=tests/examples/Dockerfile \
            --destination=${REGISTRY}/${BUILDER}-rootless-attest-default-${driver}:latest \
            --attestation \
            --storage-driver=${storage_flag} \
            --insecure \
            --verbosity=debug

        # Test 8: Attestation - explicit min (provenance only)
        run_test \
            "attestation-min" \
            "rootless" \
            "$driver" \
            $BASE_CMD \
            --context=https://github.com/rapidfort/kimia.git \
            --git-branch=main \
            --dockerfile=tests/examples/Dockerfile \
            --destination=${REGISTRY}/${BUILDER}-rootless-attest-min-${driver}:latest \
            --attestation=min \
            --storage-driver=${storage_flag} \
            --insecure \
            --verbosity=debug

        # Test 9: Attestation - max (SBOM + provenance)
        run_test \
            "attestation-max" \
            "rootless" \
            "$driver" \
            $BASE_CMD \
            --context=https://github.com/rapidfort/kimia.git \
            --git-branch=main \
            --dockerfile=tests/examples/Dockerfile \
            --destination=${REGISTRY}/${BUILDER}-rootless-attest-max-${driver}:latest \
            --attestation=max \
            --storage-driver=${storage_flag} \
            --insecure \
            --verbosity=debug

        # ====================================================================
        # NEW ATTESTATION TESTS - 3-Level System
        # ====================================================================

        # Test 10: Attestation - explicit off (no attestations)
        run_test \
            "attestation-off" \
            "rootless" \
            "$driver" \
            $BASE_CMD \
            --context=https://github.com/rapidfort/kimia.git \
            --git-branch=main \
            --dockerfile=tests/examples/Dockerfile \
            --destination=${REGISTRY}/${BUILDER}-rootless-attest-off-${driver}:latest \
            --attestation=off \
            --storage-driver=${storage_flag} \
            --insecure \
            --verbosity=debug

        # Test 11: Docker-style - SBOM only
        run_test \
            "attest-sbom-only" \
            "rootless" \
            "$driver" \
            $BASE_CMD \
            --context=https://github.com/rapidfort/kimia.git \
            --git-branch=main \
            --dockerfile=tests/examples/Dockerfile \
            --destination=${REGISTRY}/${BUILDER}-rootless-sbom-only-${driver}:latest \
            --attest type=sbom \
            --storage-driver=${storage_flag} \
            --insecure \
            --verbosity=debug

        # Test 12: Docker-style - Provenance only with mode=max
        run_test \
            "attest-prov-only" \
            "rootless" \
            "$driver" \
            $BASE_CMD \
            --context=https://github.com/rapidfort/kimia.git \
            --git-branch=main \
            --dockerfile=tests/examples/Dockerfile \
            --destination=${REGISTRY}/${BUILDER}-rootless-prov-only-${driver}:latest \
            --attest type=provenance,mode=max \
            --storage-driver=${storage_flag} \
            --insecure \
            --verbosity=debug

        # Test 13: Docker-style - SBOM with scan options
        run_test \
            "attest-sbom-scan" \
            "rootless" \
            "$driver" \
            $BASE_CMD \
            --context=https://github.com/rapidfort/kimia.git \
            --git-branch=main \
            --dockerfile=tests/examples/Dockerfile \
            --destination=${REGISTRY}/${BUILDER}-rootless-sbom-scan-${driver}:latest \
            --attest type=sbom,scan-stage=true \
            --storage-driver=${storage_flag} \
            --insecure \
            --verbosity=debug

        # Test 14: Docker-style - Provenance with builder-id
        run_test \
            "attest-prov-builderid" \
            "rootless" \
            "$driver" \
            $BASE_CMD \
            --context=https://github.com/rapidfort/kimia.git \
            --git-branch=main \
            --dockerfile=tests/examples/Dockerfile \
            --destination=${REGISTRY}/${BUILDER}-rootless-prov-builderid-${driver}:latest \
            --attest type=provenance,mode=max,builder-id=https://github.com/rapidfort/kimia/actions/runs/test \
            --storage-driver=${storage_flag} \
            --insecure \
            --verbosity=debug

        # Test 15: Docker-style - Both SBOM and Provenance
        run_test \
            "attest-both" \
            "rootless" \
            "$driver" \
            $BASE_CMD \
            --context=https://github.com/rapidfort/kimia.git \
            --git-branch=main \
            --dockerfile=tests/examples/Dockerfile \
            --destination=${REGISTRY}/${BUILDER}-rootless-both-${driver}:latest \
            --attest type=sbom,scan-stage=true \
            --attest type=provenance,mode=max,reproducible=true \
            --storage-driver=${storage_flag} \
            --insecure \
            --verbosity=debug

        # Test 16: Pass-through - BuildKit option
        run_test \
            "buildkit-opt-passthrough" \
            "rootless" \
            "$driver" \
            $BASE_CMD \
            --context=https://github.com/rapidfort/kimia.git \
            --git-branch=main \
            --dockerfile=tests/examples/Dockerfile \
            --destination=${REGISTRY}/${BUILDER}-rootless-passthrough-${driver}:latest \
            --attest type=sbom \
            --buildkit-opt attest:provenance=mode=min \
            --storage-driver=${storage_flag} \
            --insecure \
            --verbosity=debug
    fi

    # ========================================================================
    # CACHE EXPORT/IMPORT TESTS (BuildKit only)
    # ========================================================================

    if should_run_caching; then
        # Test: Export inline cache (embedded in pushed image)
        run_test \
            "cache-export-inline" \
            "rootless" \
            "$driver" \
            $BASE_CMD \
            --context=https://github.com/rapidfort/kimia.git \
            --git-branch=main \
            --dockerfile=tests/examples/Dockerfile \
            --destination=${REGISTRY}/${BUILDER}-rootless-cache-inline-${driver}:latest \
            --cache \
            --export-cache type=inline \
            --storage-driver=${storage_flag} \
            --insecure \
            --verbosity=debug

        # Test: Export registry cache (push cache layers to separate tag)
        local cache_ref="${REGISTRY}/${BUILDER}-cache-${driver}:buildcache"
        run_test \
            "cache-export-registry" \
            "rootless" \
            "$driver" \
            $BASE_CMD \
            --context=https://github.com/rapidfort/kimia.git \
            --git-branch=main \
            --dockerfile=tests/examples/Dockerfile \
            --destination=${REGISTRY}/${BUILDER}-rootless-cache-export-${driver}:latest \
            --cache \
            --export-cache "type=registry,ref=${cache_ref},mode=max" \
            --storage-driver=${storage_flag} \
            --insecure \
            --verbosity=debug

        # Test: Import + export registry cache (full roundtrip)
        # This test verifies that a second build can import the cache exported above
        run_test \
            "cache-roundtrip-registry" \
            "rootless" \
            "$driver" \
            $BASE_CMD \
            --context=https://github.com/rapidfort/kimia.git \
            --git-branch=main \
            --dockerfile=tests/examples/Dockerfile \
            --destination=${REGISTRY}/${BUILDER}-rootless-cache-roundtrip-${driver}:latest \
            --cache \
            --import-cache "type=registry,ref=${cache_ref}" \
            --export-cache "type=registry,ref=${cache_ref},mode=max" \
            --storage-driver=${storage_flag} \
            --insecure \
            --verbosity=debug

        # Test: Local cache export (useful for CI systems with mounted volumes)
        # Note: BASE_CMD_WITH_VOL inserts the -v volume mount before the image name
        local local_cache_dir="/tmp/kimia-cache-${driver}"
        mkdir -p "${local_cache_dir}"
        chown 1000:1000 "${local_cache_dir}"
        local BASE_CMD_NOVOL="${BASE_CMD%${KIMIA_IMAGE}}"
        local BASE_CMD_WITH_VOL="${BASE_CMD_NOVOL} -v ${local_cache_dir}:${local_cache_dir} ${KIMIA_IMAGE}"
        run_test \
            "cache-export-local" \
            "rootless" \
            "$driver" \
            $BASE_CMD_WITH_VOL \
            --context=https://github.com/rapidfort/kimia.git \
            --git-branch=main \
            --dockerfile=tests/examples/Dockerfile \
            --destination=${REGISTRY}/${BUILDER}-rootless-cache-local-export-${driver}:latest \
            --cache \
            --export-cache "type=local,dest=${local_cache_dir},mode=max" \
            --storage-driver=${storage_flag} \
            --insecure \
            --verbosity=debug

        # Test: Local cache import (use the exported cache from above)
        run_test \
            "cache-import-local" \
            "rootless" \
            "$driver" \
            $BASE_CMD_WITH_VOL \
            --context=https://github.com/rapidfort/kimia.git \
            --git-branch=main \
            --dockerfile=tests/examples/Dockerfile \
            --destination=${REGISTRY}/${BUILDER}-rootless-cache-local-import-${driver}:latest \
            --cache \
            --import-cache "type=local,src=${local_cache_dir}" \
            --storage-driver=${storage_flag} \
            --insecure \
            --verbosity=debug

        # Test: Reproducible build ignores cache flags (should warn but not fail)
        run_test \
            "cache-ignored-in-reproducible" \
            "rootless" \
            "$driver" \
            $BASE_CMD \
            --context=https://github.com/rapidfort/kimia.git \
            --git-branch=main \
            --dockerfile=tests/examples/Dockerfile \
            --destination=${REGISTRY}/${BUILDER}-rootless-cache-repro-${driver}:latest \
            --reproducible \
            --import-cache "type=registry,ref=${cache_ref}" \
            --export-cache "type=registry,ref=${cache_ref},mode=max" \
            --storage-driver=${storage_flag} \
            --insecure \
            --verbosity=debug
    fi

    # ========================================================================
    # SIGNING TESTS (Test 17, BuildKit only)
    # ========================================================================

    if should_run_signing; then
        # Test 17: Signing with attestation (requires cosign key)
        if [ -n "${COSIGN_KEY_PATH}" ] && [ -f "${COSIGN_KEY_PATH}" ]; then
            run_test \
                "attestation-sign" \
                "rootless" \
                "$driver" \
                $BASE_CMD \
                --context=https://github.com/rapidfort/kimia.git \
                --git-branch=main \
                --dockerfile=tests/examples/Dockerfile \
                --destination=${REGISTRY}/${BUILDER}-rootless-attest-sign-${driver}:latest \
                --attestation=max \
                --sign \
                --cosign-key=/tmp/cosign/cosign.key \
                --cosign-password-env=COSIGN_PASSWORD \
                --storage-driver=${storage_flag} \
                --insecure \
                --verbosity=debug
        else
            echo -e "${YELLOW}Skipping signing test: COSIGN_KEY_PATH not set or file not found${NC}"
            echo -e "${YELLOW}Set COSIGN_KEY_PATH environment variable to enable signing tests${NC}"
        fi
    fi
}

# ============================================================================
# Cleanup
# ============================================================================

cleanup_on_interrupt() {
    echo ""
    echo -e "${YELLOW}Interrupted by user (Ctrl+C)${NC}"
    echo -e "${YELLOW}Stopping tests and cleaning up...${NC}"

    # Kill any running docker containers
    echo "Stopping any running test containers..."
    docker ps -q --filter "ancestor=${KIMIA_IMAGE}" | xargs -r docker stop 2>/dev/null || true

    # Print partial results if any tests were run
    if [ ${TOTAL_TESTS} -gt 0 ]; then
        print_test_summary
    fi

    echo -e "${GREEN}Cleanup completed${NC}"
    exit 130  # Standard exit code for SIGINT
}

cleanup() {
    if [ "$CLEANUP_AFTER" = true ]; then
        print_section "CLEANUP"

        echo "Removing temp files..."
        rm -f /tmp/test-*.log 2>/dev/null || true

        echo -e "${GREEN}✓ Cleanup completed${NC}"
    fi
}

# ============================================================================
# Main
# ============================================================================

main() {
    local start_time=$(date +%s)

    print_section "KIMIA DOCKER TEST SUITE"

    echo -e "${CYAN}Configuration:${NC}"
    echo "  Builder:      $BUILDER"
    echo "  Registry:     $REGISTRY"
    echo "  Image:        $KIMIA_IMAGE"
    echo "  Storage:      $STORAGE_DRIVER"
    echo "  Test Suite:   $TEST_SUITE"
    echo "  Auth Method:  $DOCKER_AUTH_METHOD"
    echo ""

    # Display test suite information
    case $TEST_SUITE in
        simple)
            echo -e "${CYAN}Running: Simple Tests (6 tests)${NC}"
            echo "  - Version and environment checks"
            echo "  - Basic Git repository builds"
            ;;
        reproducible)
            echo -e "${CYAN}Running: Reproducible Build Tests (3 tests)${NC}"
            echo "  - Build same image twice"
            echo "  - Compare digests for reproducibility"
            ;;
        attestation)
            echo -e "${CYAN}Running: Attestation Tests (3 tests, BuildKit only)${NC}"
            echo "  - Default, min, and max attestation modes"
            ;;
        signing)
            echo -e "${CYAN}Running: Signing Tests (1 test, BuildKit only)${NC}"
            echo "  - Attestation with cosign signing"
            ;;
        all)
            echo -e "${CYAN}Running: All Tests${NC}"
            if [ "$BUILDER" = "buildkit" ]; then
                echo "  - Simple (6 tests)"
                echo "  - Reproducible (3 tests)"
                echo "  - Attestation (3 tests)"
                echo "  - Signing (1 test if cosign key available)"
            else
                echo "  - Simple (6 tests)"
                echo "  - Reproducible (3 tests)"
                echo "  - Attestation and Signing skipped (BuildKit only)"
            fi
            ;;
    esac
    echo ""

    # Determine which storage drivers to test
    case $STORAGE_DRIVER in
        both)
            if [ "$BUILDER" = "buildkit" ]; then
                run_rootless_tests "native"
                run_rootless_tests "overlay"
            else
                run_rootless_tests "vfs"
                run_rootless_tests "overlay"
            fi
            ;;
        native|vfs)
            if [ "$BUILDER" = "buildkit" ]; then
                run_rootless_tests "native"
            else
                run_rootless_tests "vfs"
            fi
            ;;
        overlay)
            run_rootless_tests "overlay"
            ;;
    esac

    # Cleanup if requested
    cleanup

    # Print summary
    print_test_summary

    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    local minutes=$((duration / 60))
    local seconds=$((duration % 60))

    echo "Total Time: ${minutes}m ${seconds}s"
    echo ""

    # Exit with appropriate code
    if [ $FAILED_TESTS -gt 0 ]; then
        echo -e "${RED}✗ Some tests failed${NC}"
        exit 1
    else
        echo -e "${GREEN}✓ All tests passed!${NC}"
        exit 0
    fi
}

# Trap interrupt signal
trap cleanup_on_interrupt INT TERM

# Run main
main
