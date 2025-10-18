#!/bin/bash -e
# Smithy Master Test Script - Modular Build Tests
# ALL LOGS VERSION - Shows complete output from all tests
# Supports both Docker and Kubernetes testing with VFS and Overlay storage drivers

set -e

if [ -z "${RF_APP_HOST}" ]; then
    REGISTRY=ghcr.io
else
    REGISTRY=${RF_APP_HOST}:5000
fi

# Configuration
NAMESPACE="smithy-tests"
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
SUITES_DIR="${SCRIPT_DIR}/suites"
RF_SMITHY_TMPDIR=${RF_SMITHY_TMPDIR:-"/tmp"}

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

# Default values
TEST_MODE=""
AUTH_MODE="none"
REGISTRY_USER=""
REGISTRY_PASS=""
SMITHY_IMAGE=""
DOCKER_CONFIG_DIR="${RF_SMITHY_TMPDIR}/smithy-docker-config"
CLEANUP_AFTER=false
STORAGE_DRIVER="both"
RUN_MODE="blast"
VERBOSE=true  # ALWAYS VERBOSE - SHOW ALL LOGS

# ============================================================================
# Usage Function
# ============================================================================

usage() {
    cat <<EOF
Usage: $0 [OPTIONS]

Smithy Master Test Script - Modular Build Testing (ALL LOGS VERSION)

OPTIONS:
    -h, --help              Show this help message
    -m, --mode MODE         Test mode: docker, kubernetes, or both (required)
    -r, --registry URL      Registry URL (default: $REGISTRY)
    -i, --image IMAGE       Smithy image to test (default: auto-detect)
    -a, --auth MODE         Auth mode: none, credentials, or docker (default: none)
    -u, --user USERNAME     Registry username (for credentials auth)
    -p, --pass PASSWORD     Registry password (for credentials auth)
    -n, --namespace NS      Kubernetes namespace (default: $NAMESPACE)
    -s, --storage DRIVER    Storage driver to test: vfs, overlay, or both (default: both)
    -x, --run-mode MODE     Run mode: blast (all), list (show tests), or single TEST_NAME
    -t, --test TEST         Test name for single mode
    -c, --cleanup           Cleanup resources after tests

RUN MODES:
    blast               Run all tests (default) - SHOWS ALL LOGS
    list                List all available test scripts
    single TEST_NAME    Run a specific test - SHOWS ALL LOGS

EXAMPLES:
    # Run all tests with full logs (default)
    $0 -m docker

    # List available tests
    $0 -m docker -x list

    # Run specific test with full logs
    $0 -m docker -x single -t docker_basic_build_vfs.sh

    # Test only VFS driver
    $0 -m docker -s vfs

    # Test only overlay driver
    $0 -m docker -s overlay

    # Test both drivers with cleanup
    $0 -m both -s both -c

MANUAL TEST EXECUTION:
    After running this script, individual tests are in:
    ${SCRIPT_DIR}/suites/

    You can run them manually (full logs by default):
    cd ${SCRIPT_DIR}/suites
    ./docker_basic_build_vfs.sh
    ./kubernetes_git_repo_overlay.sh

OVERLAY DRIVER REQUIREMENTS:
    For overlay storage driver support:
    - Docker: Requires /dev/fuse device (run: sudo modprobe fuse)
    - Kubernetes: Requires privileged pods OR SYS_ADMIN capability

EOF
    exit 0
}

# ============================================================================
# Argument Parsing
# ============================================================================

parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help) usage ;;
            -m|--mode) TEST_MODE="$2"; shift 2 ;;
            -r|--registry) REGISTRY="$2"; shift 2 ;;
            -i|--image) SMITHY_IMAGE="$2"; shift 2 ;;
            -a|--auth) AUTH_MODE="$2"; shift 2 ;;
            -u|--user) REGISTRY_USER="$2"; shift 2 ;;
            -p|--pass) REGISTRY_PASS="$2"; shift 2 ;;
            -n|--namespace) NAMESPACE="$2"; shift 2 ;;
            -s|--storage) STORAGE_DRIVER="$2"; shift 2 ;;
            -x|--run-mode) RUN_MODE="$2"; shift 2 ;;
            -t|--test) SINGLE_TEST="$2"; shift 2 ;;
            -c|--cleanup) CLEANUP_AFTER=true; shift ;;
            *)
                echo -e "${RED}Unknown option: $1${NC}"
                echo "Use -h or --help for usage information"
                exit 1
                ;;
        esac
    done
}

# ============================================================================
# Generate Preflight Validation Tests
# ============================================================================

generate_preflight_tests() {
    echo -e "${BLUE}Generating preflight validation tests...${NC}"
    
    cat > "$SUITES_DIR/preflight_validation.sh" <<PREFLIGHT_EOF
#!/bin/bash
# Preflight Validation Tests - All Capability Scenarios

set -e

SCRIPT_DIR="\$( cd "\$( dirname "\${BASH_SOURCE[0]}" )" && pwd )"
source "\${SCRIPT_DIR}/common.sh"

REGISTRY="${REGISTRY}"
SMITHY_IMAGE="${SMITHY_IMAGE}"

echo ""
echo -e "\${BLUE}═══════════════════════════════════════════════════════════\${NC}"
echo -e "\${BLUE}       PREFLIGHT VALIDATION TEST SUITE                     \${NC}"
echo -e "\${BLUE}═══════════════════════════════════════════════════════════\${NC}"
echo ""
echo "Registry: \$REGISTRY"
echo "Smithy Image: \$SMITHY_IMAGE"
echo ""

TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

run_preflight_test() {
    local test_name=\$1
    local expected_result=\$2
    shift 2
    local docker_args=("\$@")
    
    TOTAL_TESTS=\$((TOTAL_TESTS + 1))
    
    echo ""
    echo -e "\${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\${NC}"
    echo -e "\${YELLOW}Test #\${TOTAL_TESTS}: \${test_name}\${NC}"
    echo -e "\${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\${NC}"
    echo "Expected: \$expected_result"
    echo "Command: docker run \${docker_args[*]} \$SMITHY_IMAGE check-environment"
    echo ""
    
    local output_file="/tmp/preflight-test-\$\$.log"
    local actual_result="fail"
    
    if docker run --rm "\${docker_args[@]}" "\$SMITHY_IMAGE" check-environment > "\$output_file" 2>&1; then
        actual_result="pass"
    else
        actual_result="fail"
    fi
    
    echo -e "\${BLUE}Output:\${NC}"
    cat "\$output_file"
    echo ""
    
    if [ "\$expected_result" = "\$actual_result" ]; then
        echo -e "\${GREEN}✓ PASS\${NC} - Got expected result: \$actual_result"
        PASSED_TESTS=\$((PASSED_TESTS + 1))
        rm -f "\$output_file"
        return 0
    else
        echo -e "\${RED}✗ FAIL\${NC} - Expected \$expected_result, got \$actual_result"
        FAILED_TESTS=\$((FAILED_TESTS + 1))
        rm -f "\$output_file"
        return 1
    fi
}

# ROOT MODE TESTS
echo -e "\${CYAN}═══════════════════════════════════════════════════════════\${NC}"
echo -e "\${CYAN}  ROOT MODE TESTS                                          \${NC}"
echo -e "\${CYAN}═══════════════════════════════════════════════════════════\${NC}"

run_preflight_test "Root mode with VFS" "pass" \\
    --user 0:0 --security-opt seccomp=unconfined --security-opt apparmor=unconfined

run_preflight_test "Root mode without explicit capabilities" "pass" \\
    --user 0:0 --cap-drop ALL --security-opt seccomp=unconfined --security-opt apparmor=unconfined

run_preflight_test "Root mode with unnecessary capabilities" "pass" \\
    --user 0:0 --cap-drop ALL --cap-add SETUID --cap-add SETGID \\
    --security-opt seccomp=unconfined --security-opt apparmor=unconfined

# ROOTLESS SUCCESS CASES
echo ""
echo -e "\${CYAN}═══════════════════════════════════════════════════════════\${NC}"
echo -e "\${CYAN}  ROOTLESS MODE - SUCCESS CASES                            \${NC}"
echo -e "\${CYAN}═══════════════════════════════════════════════════════════\${NC}"

run_preflight_test "Rootless with SETUID and SETGID" "pass" \\
    --user 1000:1000 --cap-drop ALL --cap-add SETUID --cap-add SETGID \\
    --security-opt seccomp=unconfined --security-opt apparmor=unconfined

run_preflight_test "Rootless with all capabilities (privileged)" "pass" \\
    --user 1000:1000 --privileged --security-opt seccomp=unconfined --security-opt apparmor=unconfined

# ROOTLESS FAILURE CASES
echo ""
echo -e "\${CYAN}═══════════════════════════════════════════════════════════\${NC}"
echo -e "\${CYAN}  ROOTLESS MODE - FAILURE CASES                            \${NC}"
echo -e "\${CYAN}═══════════════════════════════════════════════════════════\${NC}"

run_preflight_test "Rootless without capabilities (SETUID binaries work in Docker)" "pass" \
    --user 1000:1000 --cap-drop ALL --security-opt seccomp=unconfined --security-opt apparmor=unconfined

run_preflight_test "Rootless with only SETUID" "pass" \\
    --user 1000:1000 --cap-drop ALL --cap-add SETUID \\
    --security-opt seccomp=unconfined --security-opt apparmor=unconfined

run_preflight_test "Rootless with only SETGID" "pass" \\
    --user 1000:1000 --cap-drop ALL --cap-add SETGID \\
    --security-opt seccomp=unconfined --security-opt apparmor=unconfined

run_preflight_test "Rootless with wrong capabilities (CHOWN, DAC_OVERRIDE)" "pass" \\
    --user 1000:1000 --cap-drop ALL --cap-add CHOWN --cap-add DAC_OVERRIDE \\
    --security-opt seccomp=unconfined --security-opt apparmor=unconfined

run_preflight_test "Rootless with NET_ADMIN only" "pass" \\
    --user 1000:1000 --cap-drop ALL --cap-add NET_ADMIN \\
    --security-opt seccomp=unconfined --security-opt apparmor=unconfined

# DIFFERENT USER IDS
echo ""
echo -e "\${CYAN}═══════════════════════════════════════════════════════════\${NC}"
echo -e "\${CYAN}  DIFFERENT USER IDS                                        \${NC}"
echo -e "\${CYAN}═══════════════════════════════════════════════════════════\${NC}"

run_preflight_test "UID 999 with capabilities" "pass" \\
    --user 999:999 --cap-drop ALL --cap-add SETUID --cap-add SETGID \\
    --security-opt seccomp=unconfined --security-opt apparmor=unconfined

run_preflight_test "UID 65534 (nobody) with capabilities" "pass" \\
    --user 65534:65534 --cap-drop ALL --cap-add SETUID --cap-add SETGID \\
    --security-opt seccomp=unconfined --security-opt apparmor=unconfined

run_preflight_test "UID 2000 without capabilities" "pass" \\
    --user 2000:2000 --cap-drop ALL --security-opt seccomp=unconfined --security-opt apparmor=unconfined

# EDGE CASES
echo ""
echo -e "\${CYAN}═══════════════════════════════════════════════════════════\${NC}"
echo -e "\${CYAN}  EDGE CASES                                                \${NC}"
echo -e "\${CYAN}═══════════════════════════════════════════════════════════\${NC}"

run_preflight_test "Root with no security-opt" "pass" \\
    --user 0:0

run_preflight_test "Rootless with capabilities and default seccomp" "fail" \\
    --user 1000:1000 --cap-drop ALL --cap-add SETUID --cap-add SETGID

run_preflight_test "Rootless with extra capabilities (SETUID, SETGID, CHOWN)" "pass" \\
    --user 1000:1000 --cap-drop ALL --cap-add SETUID --cap-add SETGID --cap-add CHOWN \\
    --security-opt seccomp=unconfined --security-opt apparmor=unconfined

# SUMMARY
echo ""
echo ""
echo -e "\${CYAN}═══════════════════════════════════════════════════════════\${NC}"
echo -e "\${CYAN}                    TEST SUMMARY                           \${NC}"
echo -e "\${CYAN}═══════════════════════════════════════════════════════════\${NC}"
echo ""
echo "Total Tests:  \$TOTAL_TESTS"
echo -e "Passed:       \${GREEN}\$PASSED_TESTS\${NC}"
echo -e "Failed:       \${RED}\$FAILED_TESTS\${NC}"
echo ""

if [ \$TOTAL_TESTS -gt 0 ]; then
    SUCCESS_RATE=\$(( PASSED_TESTS * 100 / TOTAL_TESTS ))
    echo "Success Rate: \${SUCCESS_RATE}%"
    echo ""
fi

if [ \$FAILED_TESTS -eq 0 ]; then
    echo -e "\${GREEN}✓ All preflight validation tests PASSED!\${NC}"
    exit 0
else
    echo -e "\${RED}✗ Some tests FAILED\${NC}"
    exit 1
fi
PREFLIGHT_EOF
    
    chmod +x "$SUITES_DIR/preflight_validation.sh"
    echo -e "${GREEN}✓ Generated preflight_validation.sh${NC}"
}

# ============================================================================
# Helper Functions
# ============================================================================

print_header() {
    echo -e "${CYAN}╔═══════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║                    SMITHY BUILD TEST SUITE                        ║${NC}"
    echo -e "${CYAN}║                        Version 1.0.0                              ║${NC}"
    echo -e "${CYAN}║                       ALL LOGS ENABLED                            ║${NC}"
    echo -e "${CYAN}╚═══════════════════════════════════════════════════════════════════╝${NC}"
    echo ""
}

validate_args() {
    if [ -z "$TEST_MODE" ]; then
        echo -e "${RED}Error: Test mode is required (-m/--mode)${NC}"
        exit 1
    fi

    if [[ "$TEST_MODE" != "docker" && "$TEST_MODE" != "kubernetes" && "$TEST_MODE" != "both" ]]; then
        echo -e "${RED}Error: Invalid test mode: $TEST_MODE${NC}"
        exit 1
    fi

    if [[ "$AUTH_MODE" != "none" && "$AUTH_MODE" != "credentials" && "$AUTH_MODE" != "docker" ]]; then
        echo -e "${RED}Error: Invalid auth mode: $AUTH_MODE${NC}"
        exit 1
    fi

    if [[ "$STORAGE_DRIVER" != "vfs" && "$STORAGE_DRIVER" != "overlay" && "$STORAGE_DRIVER" != "both" ]]; then
        echo -e "${RED}Error: Invalid storage driver: $STORAGE_DRIVER${NC}"
        exit 1
    fi

    if [[ "$RUN_MODE" != "blast" && "$RUN_MODE" != "list" && "$RUN_MODE" != "single" ]]; then
        echo -e "${RED}Error: Invalid run mode: $RUN_MODE${NC}"
        exit 1
    fi

    if [ "$RUN_MODE" = "single" ] && [ -z "$SINGLE_TEST" ]; then
        echo -e "${RED}Error: Single mode requires test name (-t/--test)${NC}"
        exit 1
    fi

    if [ "$AUTH_MODE" = "credentials" ]; then
        if [ -z "$REGISTRY_USER" ] || [ -z "$REGISTRY_PASS" ]; then
            echo -e "${RED}Error: Username and password required for credentials auth${NC}"
            exit 1
        fi
    fi

    if [ "$AUTH_MODE" = "docker" ]; then
        if [ ! -f "$HOME/.docker/config.json" ]; then
            echo -e "${RED}Error: Docker config not found at ~/.docker/config.json${NC}"
            exit 1
        fi
    fi

    if [ -z "$SMITHY_IMAGE" ]; then
        SMITHY_IMAGE="$REGISTRY/rapidfort/smithy:latest"
    fi
}

create_docker_config() {
    echo -e "${BLUE}Creating Docker configuration...${NC}"
    rm -rf "$DOCKER_CONFIG_DIR"
    mkdir -p "$DOCKER_CONFIG_DIR"

    if [ "$AUTH_MODE" == "none" ]; then
        cat > "$DOCKER_CONFIG_DIR/config.json" <<EOF
{
  "auths": {
    "$REGISTRY": {},
    "docker.io": {},
    "quay.io": {},
    "ghcr.io": {}
  }
}
EOF
    elif [ "$AUTH_MODE" == "credentials" ]; then
        AUTH_BASE64=$(echo -n "${REGISTRY_USER}:${REGISTRY_PASS}" | base64 -w 0)
        cat > "$DOCKER_CONFIG_DIR/config.json" <<EOF
{
  "auths": {
    "$REGISTRY": {
      "auth": "$AUTH_BASE64"
    },
    "docker.io": {},
    "quay.io": {},
    "ghcr.io": {}
  }
}
EOF
    elif [ "$AUTH_MODE" == "docker" ]; then
        cp "$HOME/.docker/config.json" "$DOCKER_CONFIG_DIR/config.json"
    fi

    chmod 644 "$DOCKER_CONFIG_DIR/config.json"
    chmod 755 "$DOCKER_CONFIG_DIR"
}

cleanup_resources() {
    echo -e "${BLUE}Cleaning up resources...${NC}"
    if [[ "$TEST_MODE" == "docker" || "$TEST_MODE" == "both" ]]; then
        rm -f $RF_SMITHY_TMPDIR/Dockerfile.*
        rm -rf $RF_SMITHY_TMPDIR/output
        rm -rf "$DOCKER_CONFIG_DIR"
        rm -rf $RF_SMITHY_TMPDIR/smithy-auth
        echo -e "${GREEN}✓ Docker cleanup complete${NC}"
    fi
    if [[ "$TEST_MODE" == "kubernetes" || "$TEST_MODE" == "both" ]]; then
        kubectl delete jobs --all -n $NAMESPACE --force --grace-period=0 --ignore-not-found=true 2>&1 || true
        kubectl delete namespace $NAMESPACE --force --grace-period=0 --ignore-not-found=true 2>&1 || true
        echo -e "${GREEN}✓ Kubernetes cleanup complete${NC}"
    fi
}

# ============================================================================
# Test Suite Generation
# ============================================================================

create_suites_directory() {
    echo -e "${BLUE}Creating test suites directory...${NC}"
    
    if [ ! -d "$SUITES_DIR" ]; then
        mkdir -p "$SUITES_DIR"
        echo -e "${BLUE}Created new suites directory${NC}"
    else
        echo -e "${BLUE}Suites directory exists, updating scripts...${NC}"
        rm -f "$SUITES_DIR"/docker_*.sh 2>/dev/null || true
        rm -f "$SUITES_DIR"/kubernetes_*.sh 2>/dev/null || true
        rm -f "$SUITES_DIR"/preflight_validation.sh 2>/dev/null || true  # ADD THIS LINE
    fi
    
    create_common_utils
    create_suites_readme
    echo -e "${GREEN}✓ Test suites directory ready: $SUITES_DIR${NC}"
    echo ""
}

create_common_utils() {
    cat > "$SUITES_DIR/common.sh" <<'COMMON_EOF'
#!/bin/bash
# Common utilities for Smithy test suites - ALL LOGS VERSION

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

export RED GREEN YELLOW BLUE CYAN NC

log_info() { echo -e "${BLUE}[INFO]${NC} $*"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} $*"; }
log_debug() { echo -e "${CYAN}[DEBUG]${NC} $*"; }

# Always verbose - show all output
VERBOSE=true
COMMON_EOF
    chmod +x "$SUITES_DIR/common.sh"
}

create_suites_readme() {
    cat > "$SUITES_DIR/README.md" <<'README_EOF'
# Smithy Test Suites (ALL LOGS VERSION)

Individual test scripts that show complete output for debugging.

## Quick Start
```bash
# All tests show full logs by default
./docker_basic_build_vfs.sh
./kubernetes_git_repo_overlay.sh
```

## Overlay Driver Requirements
- **Docker**: Needs `/dev/fuse` (run: `sudo modprobe fuse`)
- **Kubernetes**: Needs privileged pods OR `SYS_ADMIN` capability

## Environment Variables
- `REGISTRY` - Container registry (default: ghcr.io)
- `SMITHY_IMAGE` - Smithy image (default: ghcr.io/rapidfort/smithy:latest)
- `NAMESPACE` - K8s namespace (default: smithy-tests)

## Notes
All tests output complete logs by default. No VERBOSE flag needed.
README_EOF
}

generate_docker_tests() {
    local drivers=()
    [ "$STORAGE_DRIVER" = "both" ] && drivers=("vfs" "overlay") || drivers=("$STORAGE_DRIVER")
    
    for driver in "${drivers[@]}"; do
        generate_docker_test "$driver" "version_check" "Version Check" "" "--version"
        generate_docker_test "$driver" "basic_build" "Basic Build" "
cat > \$RF_SMITHY_TMPDIR/Dockerfile.basic <<'DOCKERFILEEOF'
FROM alpine:latest
RUN echo \"Test build\" && apk add --no-cache curl
LABEL test=\"smithy-basic\"
CMD [\"/bin/sh\"]
DOCKERFILEEOF" \
"--context=/workspace --dockerfile=Dockerfile.basic --destination=\$REGISTRY/test/basic-\$DRIVER:latest --storage-driver=\$DRIVER --insecure-registry=\$REGISTRY --no-push --verbosity=debug"

        generate_docker_test "$driver" "build_args" "Build Arguments" "
cat > \$RF_SMITHY_TMPDIR/Dockerfile.buildargs <<'DOCKERFILEEOF'
FROM alpine:latest
ARG VERSION=1.0
ARG BUILD_DATE
RUN echo \"Version: \$VERSION\" && echo \"Build Date: \$BUILD_DATE\"
LABEL version=\"\$VERSION\" build_date=\"\$BUILD_DATE\"
DOCKERFILEEOF" \
"--context=/workspace --dockerfile=Dockerfile.buildargs --destination=\$REGISTRY/test/buildargs-\$DRIVER:latest --build-arg VERSION=2.0 --build-arg BUILD_DATE=\$(date +%Y%m%d) --storage-driver=\$DRIVER --insecure-registry=\$REGISTRY --no-push --verbosity=debug"

        generate_docker_test "$driver" "labels" "Labels" "
cat > \$RF_SMITHY_TMPDIR/Dockerfile.labels <<'DOCKERFILEEOF'
FROM alpine:latest
RUN echo \"Testing labels\"
DOCKERFILEEOF" \
"--context=/workspace --dockerfile=Dockerfile.labels --destination=\$REGISTRY/test/labels-\$DRIVER:latest --label maintainer=\"smithy-test\" --label version=\"1.0\" --storage-driver=\$DRIVER --insecure-registry=\$REGISTRY --no-push --verbosity=debug"

        generate_docker_test "$driver" "multistage" "Multi-stage Build" "
cat > \$RF_SMITHY_TMPDIR/Dockerfile.multistage <<'DOCKERFILEEOF'
FROM alpine:latest AS builder
RUN echo \"Building...\" && apk add --no-cache curl
FROM alpine:latest
COPY --from=builder /usr/bin/curl /usr/bin/curl
RUN echo \"Final stage\"
CMD [\"/bin/sh\"]
DOCKERFILEEOF" \
"--context=/workspace --dockerfile=Dockerfile.multistage --destination=\$REGISTRY/test/multistage-\$DRIVER:latest --storage-driver=\$DRIVER --insecure-registry=\$REGISTRY --no-push --verbosity=debug"

        generate_docker_test "$driver" "git_repo" "Git Repository Build" "" \
"--context=https://github.com/nginxinc/docker-nginx.git --git-branch=master --dockerfile=mainline/alpine/Dockerfile --destination=\$REGISTRY/test/nginx-git-\$DRIVER:latest --storage-driver=\$DRIVER --insecure-registry=\$REGISTRY --no-push --verbosity=debug"

        generate_docker_test "$driver" "cache" "Cache Build" "
cat > \$RF_SMITHY_TMPDIR/Dockerfile.cache <<'DOCKERFILEEOF'
FROM alpine:latest
RUN apk add --no-cache curl wget
RUN echo \"Layer 1\"
RUN echo \"Layer 2\"
DOCKERFILEEOF" \
"--context=/workspace --dockerfile=Dockerfile.cache --destination=\$REGISTRY/test/cache-\$DRIVER:latest --cache --storage-driver=\$DRIVER --insecure-registry=\$REGISTRY --no-push --verbosity=debug"

        generate_docker_test "$driver" "tar_export" "TAR Export" "
cat > \$RF_SMITHY_TMPDIR/Dockerfile.tar <<'DOCKERFILEEOF'
FROM alpine:latest
RUN echo \"Export test\"
DOCKERFILEEOF
mkdir -p \$RF_SMITHY_TMPDIR/output
chmod 777 \$RF_SMITHY_TMPDIR/output" \
"--context=/workspace --dockerfile=Dockerfile.tar --destination=\$REGISTRY/test/tar-\$DRIVER:latest --tar-path=/workspace/output/test-\$DRIVER.tar --storage-driver=\$DRIVER --insecure-registry=\$REGISTRY --no-push --verbosity=debug"

        generate_docker_test "$driver" "multiple_dest" "Multiple Destinations" "
cat > \$RF_SMITHY_TMPDIR/Dockerfile.multidest <<'DOCKERFILEEOF'
FROM alpine:latest
RUN echo \"Multiple destinations test\"
DOCKERFILEEOF" \
"--context=/workspace --dockerfile=Dockerfile.multidest --destination=\$REGISTRY/test/multi-\$DRIVER:v1 --destination=\$REGISTRY/test/multi-\$DRIVER:latest --storage-driver=\$DRIVER --insecure-registry=\$REGISTRY --no-push --verbosity=debug"

        generate_docker_test "$driver" "platform" "Platform Specification" "
cat > \$RF_SMITHY_TMPDIR/Dockerfile.platform <<'DOCKERFILEEOF'
FROM alpine:latest
RUN echo \"Platform test\"
DOCKERFILEEOF" \
"--context=/workspace --dockerfile=Dockerfile.platform --destination=\$REGISTRY/test/platform-\$DRIVER:latest --custom-platform=linux/amd64 --storage-driver=\$DRIVER --insecure-registry=\$REGISTRY --no-push --verbosity=debug"
    done
}

generate_docker_test() {
    local driver=$1
    local test_name=$2
    local test_desc=$3
    local dockerfile_content=$4
    local smithy_args=$5
    local script_name="docker_${test_name}_${driver}.sh"
    
    cat > "$SUITES_DIR/$script_name" <<SCRIPT_EOF
#!/bin/bash -e
# Docker Test: $test_desc [$driver] - ALL LOGS VERSION

SCRIPT_DIR="\$( cd "\$( dirname "\${BASH_SOURCE[0]}" )" && pwd )"
source "\$SCRIPT_DIR/common.sh"

REGISTRY="$REGISTRY"
SMITHY_IMAGE="$SMITHY_IMAGE"
DOCKER_CONFIG_DIR="$DOCKER_CONFIG_DIR"
RF_SMITHY_TMPDIR="\${RF_SMITHY_TMPDIR:-/tmp}"
DRIVER="$driver"

echo ""
echo -e "${CYAN}═══════════════════════════════════════════════════════════════════${NC}"
log_info "Docker $test_desc [\$DRIVER] - SHOWING ALL LOGS"
echo -e "${CYAN}═══════════════════════════════════════════════════════════════════${NC}"
echo ""

# Base Docker security flags
DOCKER_FLAGS=(
    --rm
    --cap-drop ALL
    --cap-add SETUID
    --cap-add SETGID
    --security-opt seccomp=unconfined
    --security-opt apparmor=unconfined
    --user 1000:1000
)

# Add /dev/fuse for overlay driver
if [ "\$DRIVER" = "overlay" ]; then
    if [ -e /dev/fuse ]; then
        DOCKER_FLAGS+=(--device /dev/fuse)
        log_debug "Using overlay with /dev/fuse"
    else
        log_error "/dev/fuse not available"
        log_error "Run: sudo modprobe fuse"
        exit 1
    fi
fi

$dockerfile_content

# Volume mounts for tests that need workspace
VOLUME_MOUNTS=()
if [[ "$smithy_args" == *"/workspace"* ]]; then
    VOLUME_MOUNTS+=(-v "\$RF_SMITHY_TMPDIR:/workspace")
fi
VOLUME_MOUNTS+=(-v "\$DOCKER_CONFIG_DIR:/home/smithy/.docker:ro")

log_info "Running smithy with full output..."
echo ""

# Run build - SHOW ALL OUTPUT
docker run "\${DOCKER_FLAGS[@]}" \\
    "\${VOLUME_MOUNTS[@]}" \\
    -e HOME=/home/smithy \\
    -e DOCKER_CONFIG=/home/smithy/.docker \\
    \$SMITHY_IMAGE \\
    $smithy_args

EXIT_CODE=\$?

echo ""
echo -e "${CYAN}═══════════════════════════════════════════════════════════════════${NC}"

# Cleanup
rm -f \$RF_SMITHY_TMPDIR/Dockerfile.${test_name} 2>/dev/null || true
[ -f "\$RF_SMITHY_TMPDIR/output/test-\$DRIVER.tar" ] && rm -f "\$RF_SMITHY_TMPDIR/output/test-\$DRIVER.tar"

if [ \$EXIT_CODE -eq 0 ]; then
    log_success "$test_desc [\$DRIVER] PASSED"
    exit 0
else
    log_error "$test_desc [\$DRIVER] FAILED"
    exit 1
fi
SCRIPT_EOF
    chmod +x "$SUITES_DIR/$script_name"
}

generate_kubernetes_tests() {
    local drivers=()
    [ "$STORAGE_DRIVER" = "both" ] && drivers=("vfs" "overlay") || drivers=("$STORAGE_DRIVER")
    
    for driver in "${drivers[@]}"; do
        generate_k8s_test "$driver" "version-check" "Version Check" "--version --verbosity=debug"
        generate_k8s_test "$driver" "basic-build" "Basic Build" "--context=/workspace --dockerfile=Dockerfile --destination=$REGISTRY/test/k8s-basic-$driver:latest --storage-driver=$driver --insecure-registry=$REGISTRY --no-push --verbosity=debug"
        generate_k8s_test "$driver" "build-args" "Build Arguments" "--context=/workspace --dockerfile=Dockerfile --destination=$REGISTRY/test/k8s-buildargs-$driver:latest --build-arg=VERSION=2.0 --build-arg=BUILD_DATE=\$(date +%Y%m%d) --storage-driver=$driver --insecure-registry=$REGISTRY --no-push --verbosity=debug"
        generate_k8s_test "$driver" "git-repo" "Git Repository Build" "--context=https://github.com/nginxinc/docker-nginx.git --git-branch=master --dockerfile=mainline/alpine/Dockerfile --destination=$REGISTRY/test/k8s-git-$driver:latest --storage-driver=$driver --insecure-registry=$REGISTRY --no-push --verbosity=debug"
        generate_k8s_test "$driver" "multistage" "Multi-stage Build" "--context=/workspace --dockerfile=Dockerfile --destination=$REGISTRY/test/k8s-multistage-$driver:latest --storage-driver=$driver --insecure-registry=$REGISTRY --no-push --verbosity=debug"
    done
}

generate_k8s_test() {
    local driver=$1
    local test_name=$2
    local test_desc=$3
    local smithy_args=$4
    local script_name="kubernetes_${test_name}_${driver}.sh"
    
    # Convert underscores to hyphens for Kubernetes naming (RFC 1123)
    local k8s_job_name=$(echo "test-${test_name}-${driver}" | tr '_' '-')
    
    # Properly parse args into YAML array format
    # This handles command substitutions like $(date +%Y%m%d)
    local args_yaml=""
    
    # Use eval to expand command substitutions, then use printf %q to properly quote
    local evaluated_args=$(eval echo "$smithy_args")
    
    # Now split and format for YAML
    # We need to be careful with spaces in values
    local current_arg=""
    for word in $evaluated_args; do
        if [[ $word == --* ]]; then
            # This is a new argument
            if [[ -n $current_arg ]]; then
                args_yaml="${args_yaml}        - \"${current_arg}\"
"
            fi
            current_arg="$word"
        else
            # This is a continuation of the previous argument
            if [[ $current_arg == *=* ]] || [[ $current_arg != --* ]]; then
                # Part of a --flag=value or standalone value
                if [[ -n $current_arg ]]; then
                    current_arg="${current_arg} ${word}"
                else
                    current_arg="$word"
                fi
            else
                # This is a value for a --flag without =
                current_arg="${current_arg} ${word}"
            fi
        fi
    done
    
    # Don't forget the last argument
    if [[ -n $current_arg ]]; then
        args_yaml="${args_yaml}        - \"${current_arg}\"
"
    fi
    
    # Determine if we need workspace volume
    local volume_section=""
    if [[ "$smithy_args" == *"/workspace"* ]]; then
        volume_section="        volumeMounts:
        - name: dockerfile
          mountPath: /workspace
        - name: docker-config
          mountPath: /home/smithy/.docker
        env:
        - name: DOCKER_CONFIG
          value: /home/smithy/.docker
      volumes:
      - name: dockerfile
        configMap:
          name: test-dockerfiles
      - name: docker-config
        secret:
          secretName: docker-registry-credentials
          items:
          - key: .dockerconfigjson
            path: config.json"
    else
        volume_section="        volumeMounts:
        - name: docker-config
          mountPath: /home/smithy/.docker
        env:
        - name: DOCKER_CONFIG
          value: /home/smithy/.docker
      volumes:
      - name: docker-config
        secret:
          secretName: docker-registry-credentials
          items:
          - key: .dockerconfigjson
            path: config.json"
    fi
    
    # Overlay driver capabilities
    local security_context=""
    if [ "$driver" = "overlay" ]; then
        security_context="          runAsUser: 1000
          runAsNonRoot: true
          allowPrivilegeEscalation: true
          capabilities:
            drop: [ALL]
            add: [SETUID, SETGID, MKNOD, DAC_OVERRIDE]"
    else
        security_context="          runAsUser: 1000
          runAsNonRoot: true
          allowPrivilegeEscalation: true
          capabilities:
            drop: [ALL]
            add: [SETUID, SETGID]"
    fi
    
    cat > "$SUITES_DIR/$script_name" <<SCRIPT_EOF
#!/bin/bash -e
# Kubernetes Test: $test_desc [$driver] - ALL LOGS VERSION

SCRIPT_DIR="\$( cd "\$( dirname "\${BASH_SOURCE[0]}" )" && pwd )"
source "\$SCRIPT_DIR/common.sh"

REGISTRY="$REGISTRY"
SMITHY_IMAGE="$SMITHY_IMAGE"
NAMESPACE="$NAMESPACE"
DRIVER="$driver"
JOB_NAME="$k8s_job_name"

echo ""
echo -e "${CYAN}═══════════════════════════════════════════════════════════════════${NC}"
log_info "Kubernetes $test_desc [\$DRIVER] - SHOWING ALL LOGS"
echo -e "${CYAN}═══════════════════════════════════════════════════════════════════${NC}"
echo ""

log_info "Creating Kubernetes job..."

kubectl apply -f - <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: \$JOB_NAME
  namespace: \$NAMESPACE
spec:
  ttlSecondsAfterFinished: 300
  backoffLimit: 1
  template:
    spec:
      restartPolicy: Never
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000
      containers:
      - name: smithy
        image: \$SMITHY_IMAGE
        imagePullPolicy: IfNotPresent
        args:
$args_yaml
        securityContext:
$security_context
$volume_section
EOF

echo ""
log_info "Job created, waiting for pod to start..."

# Wait for pod
POD_NAME=""
for i in {1..30}; do
    POD_NAME=\$(kubectl get pods -n \$NAMESPACE -l job-name=\$JOB_NAME -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
    if [ -n "\$POD_NAME" ]; then
        break
    fi
    sleep 1
done

if [ -z "\$POD_NAME" ]; then
    log_error "Pod not found after 30 seconds"
    kubectl describe job/\$JOB_NAME -n \$NAMESPACE
    exit 1
fi

log_info "Pod: \$POD_NAME"

# Wait for pod to be running or completed
log_info "Waiting for pod to be ready..."
kubectl wait --for=condition=Ready pod/\$POD_NAME -n \$NAMESPACE --timeout=60s 2>/dev/null || true

# Stream logs
echo ""
log_info "Streaming job logs..."
echo "─────────────────────────────────────────────────────────────────"
kubectl logs -f pod/\$POD_NAME -n \$NAMESPACE 2>&1 || true
echo "─────────────────────────────────────────────────────────────────"
echo ""

# Check job status
JOB_STATUS=\$(kubectl get job \$JOB_NAME -n \$NAMESPACE -o jsonpath='{.status.conditions[?(@.type=="Complete")].status}' 2>/dev/null || echo "")

if [ "\$JOB_STATUS" = "True" ]; then
    log_success "Job completed successfully"
    kubectl delete job/\$JOB_NAME -n \$NAMESPACE --wait=false 2>/dev/null || true
    
    echo ""
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════════${NC}"
    log_success "Kubernetes $test_desc [\$DRIVER] PASSED"
    exit 0
else
    log_error "Job failed or timed out"
    
    echo ""
    log_info "Job status:"
    kubectl describe job/\$JOB_NAME -n \$NAMESPACE 2>&1 || true
    
    echo ""
    log_info "Pod details:"
    kubectl get pods -n \$NAMESPACE -l job-name=\$JOB_NAME 2>&1 || true
    kubectl describe pod \$POD_NAME -n \$NAMESPACE 2>&1 | tail -30
    
    kubectl delete job/\$JOB_NAME -n \$NAMESPACE --wait=false 2>/dev/null || true
    
    echo ""
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════════${NC}"
    log_error "Kubernetes $test_desc [\$DRIVER] FAILED"
    exit 1
fi
SCRIPT_EOF
    chmod +x "$SUITES_DIR/$script_name"
}


setup_kubernetes() {
    if [[ "$TEST_MODE" == "kubernetes" || "$TEST_MODE" == "both" ]]; then
        echo -e "${BLUE}Setting up Kubernetes resources...${NC}"
        
        if ! command -v kubectl &> /dev/null; then
            echo -e "${RED}Error: kubectl not found${NC}"
            exit 1
        fi
        
        if ! kubectl cluster-info &> /dev/null; then
            echo -e "${RED}Error: Cannot connect to Kubernetes cluster${NC}"
            exit 1
        fi
        
        kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
        
        if [ "$AUTH_MODE" == "credentials" ]; then
            kubectl create secret docker-registry docker-registry-credentials \
                --namespace=$NAMESPACE \
                --docker-server=$REGISTRY \
                --docker-username=$REGISTRY_USER \
                --docker-password=$REGISTRY_PASS \
                --dry-run=client -o yaml | kubectl apply -f -
        elif [ "$AUTH_MODE" == "docker" ]; then
            kubectl create secret generic docker-registry-credentials \
                --namespace=$NAMESPACE \
                --from-file=.dockerconfigjson=$HOME/.docker/config.json \
                --type=kubernetes.io/dockerconfigjson \
                --dry-run=client -o yaml | kubectl apply -f -
        else
            kubectl create secret generic docker-registry-credentials \
                --namespace=$NAMESPACE \
                --from-literal=.dockerconfigjson='{"auths":{}}' \
                --type=kubernetes.io/dockerconfigjson \
                --dry-run=client -o yaml | kubectl apply -f -
        fi
        
        cat > $RF_SMITHY_TMPDIR/Dockerfile.k8s <<EOF
FROM alpine:latest
RUN echo "Kubernetes test build"
LABEL test="kubernetes"
EOF
        kubectl create configmap test-dockerfiles \
            --namespace=$NAMESPACE \
            --from-file=Dockerfile=$RF_SMITHY_TMPDIR/Dockerfile.k8s \
            --dry-run=client -o yaml | kubectl apply -f -
        
        echo -e "${GREEN}✓ Kubernetes setup complete${NC}"
        echo ""
    fi
}

list_tests() {
    echo -e "${CYAN}Available Test Scripts (All show full logs):${NC}"
    echo ""
    
    # ADD THIS SECTION
    echo -e "${YELLOW}Preflight Validation Tests:${NC}"
    if [ -f "$SUITES_DIR/preflight_validation.sh" ]; then
        echo "  preflight_validation.sh    (16 tests: root, rootless, capabilities)"
    fi
    echo ""
    
    if [[ "$TEST_MODE" == "kubernetes" || "$TEST_MODE" == "both" ]]; then
        echo -e "${YELLOW}Kubernetes Tests:${NC}"
        ls -1 "$SUITES_DIR"/kubernetes_*.sh 2>/dev/null | while read script; do
            echo "  $(basename "$script")"
        done
        echo ""
    fi
    
    echo -e "${BLUE}To run manually (all show full logs):${NC}"
    echo "  cd $SUITES_DIR"
    echo "  ./docker_basic_build_vfs.sh"
    echo "  ./kubernetes_git_repo_overlay.sh"
}

run_single_test() {
    local test_script="$SUITES_DIR/$SINGLE_TEST"
    
    if [ ! -f "$test_script" ]; then
        echo -e "${RED}Error: Test not found: $SINGLE_TEST${NC}"
        list_tests
        exit 1
    fi
    
    echo -e "${BLUE}Running: $SINGLE_TEST (with full logs)${NC}"
    echo ""
    bash "$test_script"
    exit $?
}

run_all_tests() {
    echo -e "${BLUE}Running all tests with full logs...${NC}"
    echo ""
    
    local test_scripts=()

    if [[ "$TEST_MODE" == "docker" || "$TEST_MODE" == "both" ]]; then
        if [ -f "$SUITES_DIR/preflight_validation.sh" ]; then
            test_scripts+=("$SUITES_DIR/preflight_validation.sh")
        fi
    fi

    if [[ "$TEST_MODE" == "docker" || "$TEST_MODE" == "both" ]]; then
        while IFS= read -r script; do 
            local script_name=$(basename "$script")
            if [[ "$script_name" != "preflight_validation.sh" && "$script_name" != "common.sh" && "$script_name" != "README.md" ]]; then
                test_scripts+=("$script")
            fi
        done < <(ls -1 "$SUITES_DIR"/docker_*.sh 2>/dev/null || true)
    fi

    # Add Kubernetes tests
    [[ "$TEST_MODE" == "kubernetes" || "$TEST_MODE" == "both" ]] && \
        while IFS= read -r script; do test_scripts+=("$script"); done < <(ls -1 "$SUITES_DIR"/kubernetes_*.sh 2>/dev/null || true)

    TOTAL_TESTS=${#test_scripts[@]}
    PASSED_TESTS=0
    FAILED_TESTS=0
    
    for script in "${test_scripts[@]}"; do
        local test_name=$(basename "$script")
        
        echo ""
        echo -e "${YELLOW}▶ Running: $test_name${NC}"
        echo ""
        
        if bash "$script"; then
            PASSED_TESTS=$((PASSED_TESTS + 1))
            TEST_RESULTS+=("✓ $test_name")
        else
            FAILED_TESTS=$((FAILED_TESTS + 1))
            TEST_RESULTS+=("✗ $test_name")
        fi
        
        echo ""
    done
    
    echo ""
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}                        TEST SUMMARY                                ${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════════${NC}"
    echo ""
    echo "Total: $TOTAL_TESTS | Passed: ${GREEN}$PASSED_TESTS${NC} | Failed: ${RED}$FAILED_TESTS${NC}"
    [ $TOTAL_TESTS -gt 0 ] && echo "Success Rate: $(( PASSED_TESTS * 100 / TOTAL_TESTS ))%"
    echo ""
    
    for result in "${TEST_RESULTS[@]}"; do echo "  $result"; done
    echo ""
    
    [ $FAILED_TESTS -gt 0 ] && return 1 || return 0
}

# ============================================================================
# Main Execution
# ============================================================================

main() {
    parse_args "$@"
    [ $# -eq 0 ] && usage
    validate_args
    print_header
    
    echo -e "${BLUE}Configuration:${NC}"
    echo "  Test Mode:      $TEST_MODE"
    echo "  Run Mode:       $RUN_MODE"
    echo "  Storage Driver: $STORAGE_DRIVER"
    echo "  Registry:       $REGISTRY"
    echo "  Smithy Image:   $SMITHY_IMAGE"
    echo "  Logs:           ENABLED (ALL OUTPUT)"
    [[ "$TEST_MODE" == "kubernetes" || "$TEST_MODE" == "both" ]] && echo "  K8s Namespace:  $NAMESPACE"
    echo ""
    
    # Check for /dev/fuse if testing overlay with Docker
    if [[ "$TEST_MODE" == "docker" || "$TEST_MODE" == "both" ]] && \
       [[ "$STORAGE_DRIVER" == "overlay" || "$STORAGE_DRIVER" == "both" ]]; then
        if [ ! -e /dev/fuse ]; then
            echo -e "${YELLOW}WARNING: /dev/fuse not available - overlay tests will fail${NC}"
            echo "Run: sudo modprobe fuse"
            echo ""
        else
            echo -e "${GREEN}✓ /dev/fuse available - overlay driver supported${NC}"
            echo ""
        fi
    fi
    
    create_suites_directory
    
    [[ "$TEST_MODE" == "docker" || "$TEST_MODE" == "both" ]] && {
        echo -e "${BLUE}Generating Docker test scripts (with full logging)...${NC}"
        generate_docker_tests
        echo -e "${GREEN}✓ Docker tests generated${NC}"
        echo ""
    }
    
    # Generate preflight validation tests
    echo -e "${BLUE}Generating preflight validation tests...${NC}"
    generate_preflight_tests
    echo -e "${GREEN}✓ Preflight tests generated${NC}"
    echo ""
    
    [[ "$TEST_MODE" == "kubernetes" || "$TEST_MODE" == "both" ]] && {
        echo -e "${BLUE}Generating Kubernetes test scripts (with full logging)...${NC}"
        generate_kubernetes_tests
        echo -e "${GREEN}✓ Kubernetes tests generated${NC}"
        echo ""
    }

    case $RUN_MODE in
        list) list_tests; exit 0 ;;
        single)
            create_docker_config
            setup_kubernetes
            run_single_test
            ;;
        blast)
            create_docker_config
            setup_kubernetes
            EXIT_CODE=0
            run_all_tests || EXIT_CODE=1
            
            [ "$CLEANUP_AFTER" = true ] && cleanup_resources
            
            echo ""
            echo -e "${CYAN}═══════════════════════════════════════════════════════════════════${NC}"
            echo -e "${CYAN}                      ALL TESTS COMPLETE                            ${NC}"
            echo -e "${CYAN}═══════════════════════════════════════════════════════════════════${NC}"
            echo ""
            
            if [ $EXIT_CODE -eq 0 ]; then
                echo -e "${GREEN}✓ All tests passed with full logs!${NC}"
                echo ""
                echo -e "${BLUE}Test scripts available in: $SUITES_DIR${NC}"
                echo "Run individual tests: cd $SUITES_DIR && ./docker_basic_build_vfs.sh"
            else
                echo -e "${RED}✗ Some tests failed (check logs above)${NC}"
                echo ""
                echo -e "${BLUE}Re-run failed test: cd $SUITES_DIR && ./failed_test.sh${NC}"
            fi
            
            exit $EXIT_CODE
            ;;
    esac
}

main "$@"