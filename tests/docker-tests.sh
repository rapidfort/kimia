#!/bin/bash
# Smithy Docker Test Suite
# Tests rootless mode (UID 1000) only
# Supports BuildKit (default) and Buildah (legacy) images
# Tests storage drivers based on builder:
#   - BuildKit: native (default), overlay
#   - Buildah: vfs (default), overlay
# Note: Uses native kernel overlayfs via user namespaces

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
    echo -e "${BLUE}â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•${NC}"
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
    local test_name="$1"
    local mode="$2"
    local driver="$3"
    local test_command="$4"
    
    # Generate meaningful filename: buildkit-rootless-native-version.sh
    local script_file="${SUITES_DIR}/${BUILDER}-${mode}-${driver}-${test_name}.sh"
    
    cat > "$script_file" <<TESTSCRIPT
#!/bin/bash
# Auto-generated Docker test script
# Builder: ${BUILDER}
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
echo -e "\${CYAN}â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•\${NC}"
echo -e "\${CYAN}  Docker Test: ${test_name}\${NC}"
echo -e "\${CYAN}  Builder: ${BUILDER}\${NC}"
echo -e "\${CYAN}  Mode: ${mode}\${NC}"
echo -e "\${CYAN}  Driver: ${driver}\${NC}"
echo -e "\${CYAN}â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•â•\${NC}"
echo ""

# Test execution
echo "Running test command..."
echo ""

if ${test_command}; then
    echo ""
    echo -e "\${GREEN}âœ“ Test PASSED\${NC}"
    exit 0
else
    exit_code=\$?
    echo ""
    echo -e "\${RED}âœ— Test FAILED (exit code: \${exit_code})\${NC}"
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
    
    # CREATE the test script file
    local script_file=$(create_test_script "$test_name" "$mode" "$driver" "$test_cmd")
    
    echo -e "${CYAN}[TEST $TOTAL_TESTS]${NC} ${test_name} (${BUILDER}, ${mode}, ${driver})"
    echo -e "${CYAN}  Script: $(basename $script_file)${NC}"
    
    # EXECUTE the test script
    if bash "$script_file" > /tmp/test-$$.log 2>&1; then
        echo -e "${GREEN}  âœ“ PASS${NC}"
        PASSED_TESTS=$((PASSED_TESTS + 1))
        TEST_RESULTS+=("PASS: ${test_name} (${BUILDER}, ${mode}, ${driver})")
    else
        echo -e "${RED}  âœ— FAIL${NC}"
        echo -e "${YELLOW}  To re-run: bash $script_file${NC}"
        cat /tmp/test-$$.log | sed 's/^/    /'
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TEST_RESULTS+=("FAIL: ${test_name} (${BUILDER}, ${mode}, ${driver})")
    fi
    
    rm -f /tmp/test-$$.log
    echo ""
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
        echo -e "${CYAN}Note: Overlay storage uses native kernel overlayfs (via user namespaces)${NC}"
        if [ "$BUILDER" = "buildkit" ]; then
            echo -e "${CYAN}      BuildKit: DAC_OVERRIDE + Unconfined seccomp/AppArmor${NC}"
        else
            echo -e "${CYAN}      Buildah: MKNOD + DAC_OVERRIDE + Unconfined seccomp/AppArmor${NC}"
            echo -e "${CYAN}      Buildah: tmpfs mount at ~/.local/share/containers (avoids overlay-on-overlay)${NC}"
        fi
        echo ""
    elif [ "$driver" = "native" ]; then
        echo -e "${CYAN}Note: Native snapshotter (BuildKit) - secure and performant${NC}"
        echo ""
    elif [ "$driver" = "vfs" ]; then
        echo -e "${CYAN}Note: VFS storage (Buildah) - most secure but slower${NC}"
        echo ""
    fi
    
    # Base Docker run command for rootless  
    local BASE_CMD="docker run --rm"
    BASE_CMD="$BASE_CMD --user 1000:1000"
    BASE_CMD="$BASE_CMD --cap-drop ALL"
    BASE_CMD="$BASE_CMD --cap-add SETUID"
    BASE_CMD="$BASE_CMD --cap-add SETGID"
    
    # Add additional capabilities for overlay (both BuildKit and Buildah)
    if [ "$driver" = "overlay" ]; then
        BASE_CMD="$BASE_CMD --cap-add DAC_OVERRIDE"
        BASE_CMD="$BASE_CMD --cap-add MKNOD"
        
        # For Buildah overlay: mount tmpfs to rootless storage path to avoid overlay-on-overlay
        if [ "$BUILDER" = "buildah" ]; then
            BASE_CMD="$BASE_CMD --tmpfs /home/smithy/.local/share/containers:rw,exec,uid=1000,gid=1000"
        fi
    fi
    
    # Security options for seccomp/apparmor
    # - BuildKit: Always needs unconfined (for all storage drivers)
    # - Buildah: Always needs unconfined (newuidmap/newgidmap are blocked by default seccomp)
    if [ "$BUILDER" = "buildkit" ] || [ "$BUILDER" = "buildah" ]; then
        BASE_CMD="$BASE_CMD --security-opt seccomp=unconfined"
        BASE_CMD="$BASE_CMD --security-opt apparmor=unconfined"
    fi
    
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
    
    # Test 3: Build from git - nginx
    run_test \
        "git-nginx" \
        "rootless" \
        "$driver" \
        $BASE_CMD \
        --context=https://github.com/nginxinc/docker-nginx.git \
        --git-branch=master \
        --dockerfile=mainline/alpine/Dockerfile \
        --destination=test-${BUILDER}-rootless-nginx-${driver}:latest \
        --storage-driver=${storage_flag} \
        --no-push \
        --verbosity=debug
    
    # Test 4: Build from git - redis
    run_test \
        "git-redis" \
        "rootless" \
        "$driver" \
        $BASE_CMD \
        --context=https://github.com/docker-library/redis.git \
        --dockerfile=7.2/alpine/Dockerfile \
        --destination=test-${BUILDER}-rootless-redis-${driver}:latest \
        --storage-driver=${storage_flag} \
        --no-push \
        --verbosity=debug
    
    # Test 5: Build from git with build args - postgres
    run_test \
        "git-postgres-args" \
        "rootless" \
        "$driver" \
        $BASE_CMD \
        --context=https://github.com/docker-library/postgres.git \
        --dockerfile=16/alpine3.22/Dockerfile \
        --destination=test-${BUILDER}-rootless-postgres-${driver}:latest \
        --build-arg=PG_MAJOR=16 \
        --storage-driver=${storage_flag} \
        --no-push \
        --verbosity=debug
}

# ============================================================================
# Cleanup
# ============================================================================

cleanup() {
    if [ "$CLEANUP_AFTER" = true ]; then
        print_section "CLEANUP"
        
        echo "Removing temp files..."
        rm -f /tmp/test-*.log 2>/dev/null || true
        
        echo -e "${GREEN}âœ“ Cleanup completed${NC}"
    fi
}

cleanup_on_interrupt() {
    echo ""
    echo -e "${YELLOW}Interrupted by user (Ctrl+C)${NC}"
    echo -e "${YELLOW}Cleaning up...${NC}"
    
    rm -f /tmp/test-*.log 2>/dev/null || true
    
    echo -e "${GREEN}âœ“ Cleanup completed${NC}"
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
        echo -e "  native  â†’ Native snapshotter (default for BuildKit)"
        echo -e "  overlay â†’ Kernel overlayfs (high performance)"
    else
        echo -e "  vfs     â†’ VFS storage (default for Buildah)"
        echo -e "  overlay â†’ Kernel overlayfs (high performance)"
    fi
    echo ""
    
    echo -e "${CYAN}Note: All storage drivers use native kernel overlayfs via user namespaces${NC}"
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
        # Rootless tests only
        run_rootless_tests "$driver"
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
    
    echo -e "${GREEN}âœ“ All Docker tests passed successfully!${NC}"
    echo ""
    echo -e "${CYAN}Generated test scripts in: ${SUITES_DIR}/${NC}"
    echo -e "${CYAN}Example: bash ${SUITES_DIR}/${BUILDER}-rootless-${primary_driver}-version.sh${NC}"
    exit 0
}

# Trap cleanup on exit and interrupt
trap cleanup EXIT
trap cleanup_on_interrupt INT TERM

# Run main
main