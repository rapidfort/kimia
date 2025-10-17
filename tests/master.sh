#!/bin/bash -e
# Smithy Master Test Script - Modular Build Tests
# Supports both Docker and Kubernetes testing with individual test scripts

set -e

if [ -z "${RF_APP_HOST}" ]; then
    REGISTRY=ghcr.io
else
    REGISTRY=${RF_APP_HOST}:5000}
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
NC='\033[0m' # No Color

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
VERBOSE=false
STORAGE_DRIVER="both"
RUN_MODE="blast" # blast, single, or list

# ============================================================================
# Usage Function
# ============================================================================

usage() {
    cat <<EOF
Usage: $0 [OPTIONS]

Smithy Master Test Script - Modular Build Testing

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
    -c, --cleanup           Cleanup resources after tests
    -v, --verbose           Verbose output

RUN MODES:
    blast               Run all tests (default)
    list                List all available test scripts
    single TEST_NAME    Run a specific test (e.g., docker_basic_build.sh)

EXAMPLES:
    # Run all tests (blast mode - default)
    $0 -m docker

    # List all available test scripts
    $0 -m docker -x list

    # Run a specific test
    $0 -m docker -x single -t docker_basic_build_vfs.sh

    # Test only VFS driver
    $0 -m docker -s vfs

    # Test only overlay driver  
    $0 -m docker -s overlay

    # Run Kubernetes tests with credentials
    $0 -m kubernetes -a credentials -u myuser -p mypass

    # Verbose output with cleanup
    $0 -m both -s both -v -c

MANUAL TEST EXECUTION:
    After running this script, individual test scripts are created in:
    ${SCRIPT_DIR}/suites/

    You can run them manually:
    cd ${SCRIPT_DIR}/suites
    ./docker_basic_build_vfs.sh
    ./kubernetes_git_repo_overlay.sh

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
            -a|--auth)
                AUTH_MODE="$2"
                shift 2
                ;;
            -u|--user)
                REGISTRY_USER="$2"
                shift 2
                ;;
            -p|--pass)
                REGISTRY_PASS="$2"
                shift 2
                ;;
            -n|--namespace)
                NAMESPACE="$2"
                shift 2
                ;;
            -s|--storage)
                STORAGE_DRIVER="$2"
                shift 2
                ;;
            -x|--run-mode)
                RUN_MODE="$2"
                shift 2
                ;;
            -t|--test)
                SINGLE_TEST="$2"
                shift 2
                ;;
            -c|--cleanup)
                CLEANUP_AFTER=true
                shift
                ;;
            -v|--verbose)
                VERBOSE=true
                shift
                ;;
            *)
                echo -e "${RED}Unknown option: $1${NC}"
                echo "Use -h or --help for usage information"
                exit 1
                ;;
        esac
    done
}

# ============================================================================
# Helper Functions
# ============================================================================

log_verbose() {
    if [ "$VERBOSE" = true ]; then
        echo -e "${CYAN}[VERBOSE]${NC} $*"
    fi
}

print_header() {
    echo -e "${CYAN}╔═══════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║                    SMITHY BUILD TEST SUITE                        ║${NC}"
    echo -e "${CYAN}║                        Version 1.0.0                              ║${NC}"
    echo -e "${CYAN}╚═══════════════════════════════════════════════════════════════════╝${NC}"
    echo ""
}

validate_args() {
    # Check required arguments
    if [ -z "$TEST_MODE" ]; then
        echo -e "${RED}Error: Test mode is required (-m/--mode)${NC}"
        echo "Valid modes: docker, kubernetes, both"
        exit 1
    fi

    # Validate test mode
    if [[ "$TEST_MODE" != "docker" && "$TEST_MODE" != "kubernetes" && "$TEST_MODE" != "both" ]]; then
        echo -e "${RED}Error: Invalid test mode: $TEST_MODE${NC}"
        echo "Valid modes: docker, kubernetes, both"
        exit 1
    fi

    # Validate auth mode
    if [[ "$AUTH_MODE" != "none" && "$AUTH_MODE" != "credentials" && "$AUTH_MODE" != "docker" ]]; then
        echo -e "${RED}Error: Invalid auth mode: $AUTH_MODE${NC}"
        echo "Valid modes: none, credentials, docker"
        exit 1
    fi

    # Validate storage driver
    if [[ "$STORAGE_DRIVER" != "vfs" && "$STORAGE_DRIVER" != "overlay" && "$STORAGE_DRIVER" != "both" ]]; then
        echo -e "${RED}Error: Invalid storage driver: $STORAGE_DRIVER${NC}"
        echo "Valid options: vfs, overlay, both"
        exit 1
    fi

    # Validate run mode
    if [[ "$RUN_MODE" != "blast" && "$RUN_MODE" != "list" && "$RUN_MODE" != "single" ]]; then
        echo -e "${RED}Error: Invalid run mode: $RUN_MODE${NC}"
        echo "Valid modes: blast, list, single"
        exit 1
    fi

    # Check for single test name
    if [ "$RUN_MODE" = "single" ] && [ -z "$SINGLE_TEST" ]; then
        echo -e "${RED}Error: Single mode requires test name (-t/--test)${NC}"
        exit 1
    fi

    # Check credentials if auth mode is credentials
    if [ "$AUTH_MODE" = "credentials" ]; then
        if [ -z "$REGISTRY_USER" ] || [ -z "$REGISTRY_PASS" ]; then
            echo -e "${RED}Error: Username and password required for credentials auth mode${NC}"
            exit 1
        fi
    fi

    # Check docker config if auth mode is docker
    if [ "$AUTH_MODE" = "docker" ]; then
        if [ ! -f "$HOME/.docker/config.json" ]; then
            echo -e "${RED}Error: Docker config not found at ~/.docker/config.json${NC}"
            echo "Please login first: docker login $REGISTRY"
            exit 1
        fi
    fi

    # Set default smithy image if not provided
    if [ -z "$SMITHY_IMAGE" ]; then
        SMITHY_IMAGE="$REGISTRY/smithy:latest"
        log_verbose "Using default smithy image: $SMITHY_IMAGE"
    fi
}

create_docker_config() {
    log_verbose "Creating Docker configuration..."

    # Create docker config directory
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
        log_verbose "Docker config created (no auth)"

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
        log_verbose "Docker config created with authentication"

    elif [ "$AUTH_MODE" == "docker" ]; then
        cp "$HOME/.docker/config.json" "$DOCKER_CONFIG_DIR/config.json"
        log_verbose "Using existing Docker config"
    fi

    chmod 644 "$DOCKER_CONFIG_DIR/config.json"
    chmod 755 "$DOCKER_CONFIG_DIR"
    log_verbose "Fixed permissions on Docker config"
}

cleanup_resources() {
    echo -e "${BLUE}Cleaning up resources...${NC}"

    if [[ "$TEST_MODE" == "docker" || "$TEST_MODE" == "both" ]]; then
        cleanup_docker
    fi

    if [[ "$TEST_MODE" == "kubernetes" || "$TEST_MODE" == "both" ]]; then
        cleanup_kubernetes
    fi
}

cleanup_docker() {
    log_verbose "Cleaning Docker test resources..."
    rm -f $RF_SMITHY_TMPDIR/Dockerfile.*
    rm -f $RF_SMITHY_TMPDIR/test.log
    rm -f $RF_SMITHY_TMPDIR/.dockerignore
    rm -rf $RF_SMITHY_TMPDIR/output
    rm -rf "$DOCKER_CONFIG_DIR"
    rm -rf $RF_SMITHY_TMPDIR/smithy-auth
    echo -e "${GREEN}✓ Docker cleanup complete${NC}"
}

cleanup_kubernetes() {
    log_verbose "Cleaning Kubernetes test resources..."
    kubectl delete jobs --all -n $NAMESPACE --force --grace-period=0 --ignore-not-found=true >/dev/null 2>&1 || true
    kubectl delete namespace $NAMESPACE --force --grace-period=0 --ignore-not-found=true >/dev/null 2>&1 || true
    sleep 2
    echo -e "${GREEN}✓ Kubernetes cleanup complete${NC}"
}

# ============================================================================
# Test Suite Generation
# ============================================================================

create_suites_directory() {
    echo -e "${BLUE}Creating test suites directory...${NC}"
    rm -rf "$SUITES_DIR"
    mkdir -p "$SUITES_DIR"
    
    # Create common utilities script
    create_common_utils
    
    # Create run_all convenience script
    create_run_all_script
    
    # Create README
    create_suites_readme
    
    echo -e "${GREEN}✓ Test suites directory created: $SUITES_DIR${NC}"
    echo ""
}

create_common_utils() {
    cat > "$SUITES_DIR/common.sh" <<'COMMON_EOF'
#!/bin/bash
# Common utilities for Smithy test suites

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Export for child scripts
export RED GREEN YELLOW BLUE CYAN NC

log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $*"
}

log_debug() {
    if [ "$VERBOSE" = "true" ]; then
        echo -e "${CYAN}[DEBUG]${NC} $*"
    fi
}
COMMON_EOF
    chmod +x "$SUITES_DIR/common.sh"
}

create_run_all_script() {
    cat > "$SUITES_DIR/run_all.sh" <<'RUNALL_EOF'
#!/bin/bash
# Convenience script to run all tests in suites directory
# This script is auto-generated by master.sh

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source "$SCRIPT_DIR/common.sh"

# Configuration
FILTER="${1:-all}"
STOP_ON_FAIL="${STOP_ON_FAIL:-false}"

# Counters
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0
declare -a FAILED_LIST

print_usage() {
    cat <<EOF
Usage: $0 [FILTER]

Run all or filtered test suites.

FILTERS:
    all         Run all tests (default)
    docker      Run only Docker tests
    kubernetes  Run only Kubernetes tests
    vfs         Run only VFS storage driver tests
    overlay     Run only overlay storage driver tests

ENVIRONMENT VARIABLES:
    STOP_ON_FAIL=true   Stop execution on first failure
    VERBOSE=true        Enable verbose output

EXAMPLES:
    ./run_all.sh
    ./run_all.sh docker
    ./run_all.sh overlay
    STOP_ON_FAIL=true ./run_all.sh vfs
EOF
    exit 0
}

if [[ "$1" == "-h" || "$1" == "--help" ]]; then
    print_usage
fi

log_info "Starting test suite execution"
log_info "Filter: $FILTER"
echo ""

# Get list of test scripts
TEST_SCRIPTS=()

case $FILTER in
    all)
        while IFS= read -r script; do
            TEST_SCRIPTS+=("$script")
        done < <(find "$SCRIPT_DIR" -name "*.sh" -type f ! -name "common.sh" ! -name "run_all.sh" | sort)
        ;;
    docker)
        while IFS= read -r script; do
            TEST_SCRIPTS+=("$script")
        done < <(find "$SCRIPT_DIR" -name "docker_*.sh" -type f | sort)
        ;;
    kubernetes)
        while IFS= read -r script; do
            TEST_SCRIPTS+=("$script")
        done < <(find "$SCRIPT_DIR" -name "kubernetes_*.sh" -type f | sort)
        ;;
    vfs)
        while IFS= read -r script; do
            TEST_SCRIPTS+=("$script")
        done < <(find "$SCRIPT_DIR" -name "*_vfs.sh" -type f | sort)
        ;;
    overlay)
        while IFS= read -r script; do
            TEST_SCRIPTS+=("$script")
        done < <(find "$SCRIPT_DIR" -name "*_overlay.sh" -type f | sort)
        ;;
    *)
        log_error "Unknown filter: $FILTER"
        print_usage
        ;;
esac

TOTAL_TESTS=${#TEST_SCRIPTS[@]}

if [ $TOTAL_TESTS -eq 0 ]; then
    log_warning "No test scripts found matching filter: $FILTER"
    exit 0
fi

log_info "Found $TOTAL_TESTS tests to run"
echo ""

# Run tests
for script in "${TEST_SCRIPTS[@]}"; do
    TEST_NAME=$(basename "$script")
    
    echo -e "${CYAN}═══════════════════════════════════════════════${NC}"
    log_info "Running: $TEST_NAME"
    echo -e "${CYAN}═══════════════════════════════════════════════${NC}"
    
    if bash "$script"; then
        PASSED_TESTS=$((PASSED_TESTS + 1))
        log_success "PASSED: $TEST_NAME"
    else
        FAILED_TESTS=$((FAILED_TESTS + 1))
        FAILED_LIST+=("$TEST_NAME")
        log_error "FAILED: $TEST_NAME"
        
        if [ "$STOP_ON_FAIL" = "true" ]; then
            log_error "Stopping execution due to failure"
            break
        fi
    fi
    echo ""
done

# Summary
echo ""
echo -e "${CYAN}═══════════════════════════════════════════════════════${NC}"
echo -e "${CYAN}                    TEST SUMMARY                        ${NC}"
echo -e "${CYAN}═══════════════════════════════════════════════════════${NC}"
echo ""
echo "Filter:       $FILTER"
echo "Total Tests:  $TOTAL_TESTS"
echo -e "Passed:       ${GREEN}$PASSED_TESTS${NC}"
echo -e "Failed:       ${RED}$FAILED_TESTS${NC}"

if [ $TOTAL_TESTS -gt 0 ]; then
    SUCCESS_RATE=$(( PASSED_TESTS * 100 / TOTAL_TESTS ))
    echo "Success Rate: ${SUCCESS_RATE}%"
fi
echo ""

if [ $FAILED_TESTS -gt 0 ]; then
    echo -e "${RED}Failed Tests:${NC}"
    for test in "${FAILED_LIST[@]}"; do
        echo "  ✗ $test"
    done
    echo ""
    echo -e "${BLUE}To debug:${NC}"
    echo "  VERBOSE=true ./${FAILED_LIST[0]}"
    exit 1
else
    echo -e "${GREEN}✓ All tests passed!${NC}"
    exit 0
fi
RUNALL_EOF
    chmod +x "$SUITES_DIR/run_all.sh"
}

create_suites_readme() {
    cat > "$SUITES_DIR/README.md" <<'README_EOF'
# Smithy Test Suites

Individual test scripts for debugging and manual testing.

## Quick Start

```bash
# Run all tests
./run_all.sh

# Run only Docker tests
./run_all.sh docker

# Run only VFS driver tests
./run_all.sh vfs

# Run a single test
./docker_basic_build_vfs.sh

# Run with verbose output
VERBOSE=true ./docker_basic_build_vfs.sh
```

## Available Tests

### Docker Tests (per storage driver)
- `docker_version_check_[driver].sh` - Version verification
- `docker_basic_build_[driver].sh` - Basic image build
- `docker_build_args_[driver].sh` - Build arguments
- `docker_labels_[driver].sh` - Image labels
- `docker_multistage_[driver].sh` - Multi-stage builds
- `docker_git_repo_[driver].sh` - Git repository builds
- `docker_cache_[driver].sh` - Layer caching
- `docker_tar_export_[driver].sh` - TAR export
- `docker_multiple_dest_[driver].sh` - Multiple tags
- `docker_platform_[driver].sh` - Cross-platform

### Kubernetes Tests (per storage driver)
- `kubernetes_version_check_[driver].sh` - Version in K8s
- `kubernetes_basic_build_[driver].sh` - Basic K8s build
- `kubernetes_build_args_[driver].sh` - Build args in K8s
- `kubernetes_git_repo_[driver].sh` - Git builds in K8s
- `kubernetes_multistage_[driver].sh` - Multi-stage in K8s

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `REGISTRY` | `ghcr.io` | Container registry |
| `SMITHY_IMAGE` | `ghcr.io/rapidfort/smithy:latest` | Smithy image |
| `RF_SMITHY_TMPDIR` | `/tmp` | Temp directory |
| `VERBOSE` | `false` | Verbose output |
| `NAMESPACE` | `smithy-tests` | K8s namespace |

## Debugging

```bash
# View detailed output
VERBOSE=true ./docker_basic_build_vfs.sh

# Check Kubernetes logs
kubectl logs -n smithy-tests job/test-basic-vfs

# Run with custom settings
REGISTRY=myregistry:5000 ./docker_basic_build_overlay.sh
```

## CI/CD Integration

```bash
# In your CI pipeline
cd tests
./master.sh -m docker -s both
cd suites
./run_all.sh docker
```
README_EOF
}

generate_docker_tests() {
    local drivers=()
    
    if [ "$STORAGE_DRIVER" = "both" ]; then
        drivers=("vfs" "overlay")
    else
        drivers=("$STORAGE_DRIVER")
    fi
    
    for driver in "${drivers[@]}"; do
        generate_docker_version_check "$driver"
        generate_docker_basic_build "$driver"
        generate_docker_build_args "$driver"
        generate_docker_labels "$driver"
        generate_docker_multistage "$driver"
        generate_docker_git_repo "$driver"
        generate_docker_cache "$driver"
        generate_docker_tar_export "$driver"
        generate_docker_multiple_dest "$driver"
        generate_docker_platform "$driver"
    done
}

generate_docker_version_check() {
    local driver=$1
    local script_name="docker_version_check_${driver}.sh"
    
    cat > "$SUITES_DIR/$script_name" <<SCRIPT_EOF
#!/bin/bash -e
# Docker Test: Version Check [$driver]

SCRIPT_DIR="\$( cd "\$( dirname "\${BASH_SOURCE[0]}" )" && pwd )"
source "\$SCRIPT_DIR/common.sh"

REGISTRY="${REGISTRY}"
SMITHY_IMAGE="${SMITHY_IMAGE}"
DOCKER_CONFIG_DIR="${DOCKER_CONFIG_DIR}"

log_info "Running Docker Version Check [$driver]..."

docker run --rm \\
    --cap-drop ALL \\
    --cap-add SETUID \\
    --cap-add SETGID \\
    --security-opt seccomp=unconfined \\
    --security-opt apparmor=unconfined \\
    --user 1000:1000 \\
    \$SMITHY_IMAGE \\
    --version

if [ \$? -eq 0 ]; then
    log_success "Version check passed"
    exit 0
else
    log_error "Version check failed"
    exit 1
fi
SCRIPT_EOF
    chmod +x "$SUITES_DIR/$script_name"
}

generate_docker_basic_build() {
    local driver=$1
    local script_name="docker_basic_build_${driver}.sh"
    
    cat > "$SUITES_DIR/$script_name" <<'SCRIPT_EOF'
#!/bin/bash -e
# Docker Test: Basic Build [DRIVER]

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source "$SCRIPT_DIR/common.sh"

REGISTRY="REGISTRY_PLACEHOLDER"
SMITHY_IMAGE="SMITHY_IMAGE_PLACEHOLDER"
DOCKER_CONFIG_DIR="DOCKER_CONFIG_DIR_PLACEHOLDER"
RF_SMITHY_TMPDIR="${RF_SMITHY_TMPDIR:-/tmp}"
DRIVER="DRIVER_PLACEHOLDER"

log_info "Running Docker Basic Build [$DRIVER]..."

# Create Dockerfile
cat > $RF_SMITHY_TMPDIR/Dockerfile.basic <<EOF
FROM alpine:latest
RUN echo "Test build" && apk add --no-cache curl
LABEL test="smithy-basic"
CMD ["/bin/sh"]
EOF

# Run build
docker run --rm \
    --cap-drop ALL \
    --cap-add SETUID \
    --cap-add SETGID \
    --security-opt seccomp=unconfined \
    --security-opt apparmor=unconfined \
    --user 1000:1000 \
    -v $RF_SMITHY_TMPDIR:/workspace \
    -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
    -e HOME=/home/smithy \
    -e DOCKER_CONFIG=/home/smithy/.docker \
    $SMITHY_IMAGE \
    --context=/workspace \
    --dockerfile=Dockerfile.basic \
    --destination=$REGISTRY/test/basic-$DRIVER:latest \
    --storage-driver=$DRIVER \
    --insecure-registry=$REGISTRY \
    --no-push

if [ $? -eq 0 ]; then
    log_success "Basic build passed"
    rm -f $RF_SMITHY_TMPDIR/Dockerfile.basic
    exit 0
else
    log_error "Basic build failed"
    exit 1
fi
SCRIPT_EOF

    # Replace placeholders
    sed -i "s|DRIVER_PLACEHOLDER|$driver|g" "$SUITES_DIR/$script_name"
    sed -i "s|REGISTRY_PLACEHOLDER|$REGISTRY|g" "$SUITES_DIR/$script_name"
    sed -i "s|SMITHY_IMAGE_PLACEHOLDER|$SMITHY_IMAGE|g" "$SUITES_DIR/$script_name"
    sed -i "s|DOCKER_CONFIG_DIR_PLACEHOLDER|$DOCKER_CONFIG_DIR|g" "$SUITES_DIR/$script_name"
    
    chmod +x "$SUITES_DIR/$script_name"
}

generate_docker_build_args() {
    local driver=$1
    local script_name="docker_build_args_${driver}.sh"
    
    cat > "$SUITES_DIR/$script_name" <<'SCRIPT_EOF'
#!/bin/bash -e
# Docker Test: Build Arguments [DRIVER]

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source "$SCRIPT_DIR/common.sh"

REGISTRY="REGISTRY_PLACEHOLDER"
SMITHY_IMAGE="SMITHY_IMAGE_PLACEHOLDER"
DOCKER_CONFIG_DIR="DOCKER_CONFIG_DIR_PLACEHOLDER"
RF_SMITHY_TMPDIR="${RF_SMITHY_TMPDIR:-/tmp}"
DRIVER="DRIVER_PLACEHOLDER"

log_info "Running Docker Build Arguments [$DRIVER]..."

cat > $RF_SMITHY_TMPDIR/Dockerfile.buildargs <<EOF
FROM alpine:latest
ARG VERSION=1.0
ARG BUILD_DATE
RUN echo "Version: \$VERSION" && echo "Build Date: \$BUILD_DATE"
LABEL version="\$VERSION"
LABEL build_date="\$BUILD_DATE"
EOF

docker run --rm \
    --cap-drop ALL \
    --cap-add SETUID \
    --cap-add SETGID \
    --security-opt seccomp=unconfined \
    --security-opt apparmor=unconfined \
    --user 1000:1000 \
    -v $RF_SMITHY_TMPDIR:/workspace \
    -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
    -e HOME=/home/smithy \
    -e DOCKER_CONFIG=/home/smithy/.docker \
    $SMITHY_IMAGE \
    --context=/workspace \
    --dockerfile=Dockerfile.buildargs \
    --destination=$REGISTRY/test/buildargs-$DRIVER:latest \
    --build-arg VERSION=2.0 \
    --build-arg BUILD_DATE=$(date +%Y%m%d) \
    --storage-driver=$DRIVER \
    --insecure-registry=$REGISTRY \
    --no-push

if [ $? -eq 0 ]; then
    log_success "Build arguments test passed"
    rm -f $RF_SMITHY_TMPDIR/Dockerfile.buildargs
    exit 0
else
    log_error "Build arguments test failed"
    exit 1
fi
SCRIPT_EOF

    sed -i "s|DRIVER_PLACEHOLDER|$driver|g" "$SUITES_DIR/$script_name"
    sed -i "s|REGISTRY_PLACEHOLDER|$REGISTRY|g" "$SUITES_DIR/$script_name"
    sed -i "s|SMITHY_IMAGE_PLACEHOLDER|$SMITHY_IMAGE|g" "$SUITES_DIR/$script_name"
    sed -i "s|DOCKER_CONFIG_DIR_PLACEHOLDER|$DOCKER_CONFIG_DIR|g" "$SUITES_DIR/$script_name"
    
    chmod +x "$SUITES_DIR/$script_name"
}

generate_docker_labels() {
    local driver=$1
    local script_name="docker_labels_${driver}.sh"
    
    cat > "$SUITES_DIR/$script_name" <<'SCRIPT_EOF'
#!/bin/bash -e
# Docker Test: Labels [DRIVER]

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source "$SCRIPT_DIR/common.sh"

REGISTRY="REGISTRY_PLACEHOLDER"
SMITHY_IMAGE="SMITHY_IMAGE_PLACEHOLDER"
DOCKER_CONFIG_DIR="DOCKER_CONFIG_DIR_PLACEHOLDER"
RF_SMITHY_TMPDIR="${RF_SMITHY_TMPDIR:-/tmp}"
DRIVER="DRIVER_PLACEHOLDER"

log_info "Running Docker Labels Test [$DRIVER]..."

cat > $RF_SMITHY_TMPDIR/Dockerfile.labels <<EOF
FROM alpine:latest
RUN echo "Testing labels"
EOF

docker run --rm \
    --cap-drop ALL \
    --cap-add SETUID \
    --cap-add SETGID \
    --security-opt seccomp=unconfined \
    --security-opt apparmor=unconfined \
    --user 1000:1000 \
    -v $RF_SMITHY_TMPDIR:/workspace \
    -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
    -e HOME=/home/smithy \
    -e DOCKER_CONFIG=/home/smithy/.docker \
    $SMITHY_IMAGE \
    --context=/workspace \
    --dockerfile=Dockerfile.labels \
    --destination=$REGISTRY/test/labels-$DRIVER:latest \
    --label maintainer="smithy-test" \
    --label version="1.0" \
    --storage-driver=$DRIVER \
    --insecure-registry=$REGISTRY \
    --no-push

if [ $? -eq 0 ]; then
    log_success "Labels test passed"
    rm -f $RF_SMITHY_TMPDIR/Dockerfile.labels
    exit 0
else
    log_error "Labels test failed"
    exit 1
fi
SCRIPT_EOF

    sed -i "s|DRIVER_PLACEHOLDER|$driver|g" "$SUITES_DIR/$script_name"
    sed -i "s|REGISTRY_PLACEHOLDER|$REGISTRY|g" "$SUITES_DIR/$script_name"
    sed -i "s|SMITHY_IMAGE_PLACEHOLDER|$SMITHY_IMAGE|g" "$SUITES_DIR/$script_name"
    sed -i "s|DOCKER_CONFIG_DIR_PLACEHOLDER|$DOCKER_CONFIG_DIR|g" "$SUITES_DIR/$script_name"
    
    chmod +x "$SUITES_DIR/$script_name"
}

generate_docker_multistage() {
    local driver=$1
    local script_name="docker_multistage_${driver}.sh"
    
    cat > "$SUITES_DIR/$script_name" <<'SCRIPT_EOF'
#!/bin/bash -e
# Docker Test: Multi-stage Build [DRIVER]

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source "$SCRIPT_DIR/common.sh"

REGISTRY="REGISTRY_PLACEHOLDER"
SMITHY_IMAGE="SMITHY_IMAGE_PLACEHOLDER"
DOCKER_CONFIG_DIR="DOCKER_CONFIG_DIR_PLACEHOLDER"
RF_SMITHY_TMPDIR="${RF_SMITHY_TMPDIR:-/tmp}"
DRIVER="DRIVER_PLACEHOLDER"

log_info "Running Docker Multi-stage Build [$DRIVER]..."

cat > $RF_SMITHY_TMPDIR/Dockerfile.multistage <<EOF
FROM alpine:latest AS builder
RUN echo "Building..." && apk add --no-cache curl

FROM alpine:latest
COPY --from=builder /usr/bin/curl /usr/bin/curl
RUN echo "Final stage"
CMD ["/bin/sh"]
EOF

docker run --rm \
    --cap-drop ALL \
    --cap-add SETUID \
    --cap-add SETGID \
    --security-opt seccomp=unconfined \
    --security-opt apparmor=unconfined \
    --user 1000:1000 \
    -v $RF_SMITHY_TMPDIR:/workspace \
    -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
    -e HOME=/home/smithy \
    -e DOCKER_CONFIG=/home/smithy/.docker \
    $SMITHY_IMAGE \
    --context=/workspace \
    --dockerfile=Dockerfile.multistage \
    --destination=$REGISTRY/test/multistage-$DRIVER:latest \
    --storage-driver=$DRIVER \
    --insecure-registry=$REGISTRY \
    --no-push

if [ $? -eq 0 ]; then
    log_success "Multi-stage build passed"
    rm -f $RF_SMITHY_TMPDIR/Dockerfile.multistage
    exit 0
else
    log_error "Multi-stage build failed"
    exit 1
fi
SCRIPT_EOF

    sed -i "s|DRIVER_PLACEHOLDER|$driver|g" "$SUITES_DIR/$script_name"
    sed -i "s|REGISTRY_PLACEHOLDER|$REGISTRY|g" "$SUITES_DIR/$script_name"
    sed -i "s|SMITHY_IMAGE_PLACEHOLDER|$SMITHY_IMAGE|g" "$SUITES_DIR/$script_name"
    sed -i "s|DOCKER_CONFIG_DIR_PLACEHOLDER|$DOCKER_CONFIG_DIR|g" "$SUITES_DIR/$script_name"
    
    chmod +x "$SUITES_DIR/$script_name"
}

generate_docker_git_repo() {
    local driver=$1
    local script_name="docker_git_repo_${driver}.sh"
    
    cat > "$SUITES_DIR/$script_name" <<'SCRIPT_EOF'
#!/bin/bash -e
# Docker Test: Git Repository Build [DRIVER]

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source "$SCRIPT_DIR/common.sh"

REGISTRY="REGISTRY_PLACEHOLDER"
SMITHY_IMAGE="SMITHY_IMAGE_PLACEHOLDER"
DOCKER_CONFIG_DIR="DOCKER_CONFIG_DIR_PLACEHOLDER"
DRIVER="DRIVER_PLACEHOLDER"

log_info "Running Docker Git Repository Build [$DRIVER]..."

docker run --rm \
    --cap-drop ALL \
    --cap-add SETUID \
    --cap-add SETGID \
    --security-opt seccomp=unconfined \
    --security-opt apparmor=unconfined \
    --user 1000:1000 \
    -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
    -e HOME=/home/smithy \
    -e DOCKER_CONFIG=/home/smithy/.docker \
    $SMITHY_IMAGE \
    --context=https://github.com/nginxinc/docker-nginx.git \
    --git-branch=master \
    --dockerfile=mainline/alpine/Dockerfile \
    --destination=$REGISTRY/test/nginx-git-$DRIVER:latest \
    --storage-driver=$DRIVER \
    --insecure-registry=$REGISTRY \
    --no-push

if [ $? -eq 0 ]; then
    log_success "Git repository build passed"
    exit 0
else
    log_error "Git repository build failed"
    exit 1
fi
SCRIPT_EOF

    sed -i "s|DRIVER_PLACEHOLDER|$driver|g" "$SUITES_DIR/$script_name"
    sed -i "s|REGISTRY_PLACEHOLDER|$REGISTRY|g" "$SUITES_DIR/$script_name"
    sed -i "s|SMITHY_IMAGE_PLACEHOLDER|$SMITHY_IMAGE|g" "$SUITES_DIR/$script_name"
    sed -i "s|DOCKER_CONFIG_DIR_PLACEHOLDER|$DOCKER_CONFIG_DIR|g" "$SUITES_DIR/$script_name"
    
    chmod +x "$SUITES_DIR/$script_name"
}

generate_docker_cache() {
    local driver=$1
    local script_name="docker_cache_${driver}.sh"
    
    cat > "$SUITES_DIR/$script_name" <<'SCRIPT_EOF'
#!/bin/bash -e
# Docker Test: Cache Build [DRIVER]

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source "$SCRIPT_DIR/common.sh"

REGISTRY="REGISTRY_PLACEHOLDER"
SMITHY_IMAGE="SMITHY_IMAGE_PLACEHOLDER"
DOCKER_CONFIG_DIR="DOCKER_CONFIG_DIR_PLACEHOLDER"
RF_SMITHY_TMPDIR="${RF_SMITHY_TMPDIR:-/tmp}"
DRIVER="DRIVER_PLACEHOLDER"

log_info "Running Docker Cache Build [$DRIVER]..."

cat > $RF_SMITHY_TMPDIR/Dockerfile.cache <<EOF
FROM alpine:latest
RUN apk add --no-cache curl wget
RUN echo "Layer 1"
RUN echo "Layer 2"
EOF

docker run --rm \
    --cap-drop ALL \
    --cap-add SETUID \
    --cap-add SETGID \
    --security-opt seccomp=unconfined \
    --security-opt apparmor=unconfined \
    --user 1000:1000 \
    -v $RF_SMITHY_TMPDIR:/workspace \
    -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
    -e HOME=/home/smithy \
    -e DOCKER_CONFIG=/home/smithy/.docker \
    $SMITHY_IMAGE \
    --context=/workspace \
    --dockerfile=Dockerfile.cache \
    --destination=$REGISTRY/test/cache-$DRIVER:latest \
    --cache \
    --storage-driver=$DRIVER \
    --insecure-registry=$REGISTRY \
    --no-push

if [ $? -eq 0 ]; then
    log_success "Cache build passed"
    rm -f $RF_SMITHY_TMPDIR/Dockerfile.cache
    exit 0
else
    log_error "Cache build failed"
    exit 1
fi
SCRIPT_EOF

    sed -i "s|DRIVER_PLACEHOLDER|$driver|g" "$SUITES_DIR/$script_name"
    sed -i "s|REGISTRY_PLACEHOLDER|$REGISTRY|g" "$SUITES_DIR/$script_name"
    sed -i "s|SMITHY_IMAGE_PLACEHOLDER|$SMITHY_IMAGE|g" "$SUITES_DIR/$script_name"
    sed -i "s|DOCKER_CONFIG_DIR_PLACEHOLDER|$DOCKER_CONFIG_DIR|g" "$SUITES_DIR/$script_name"
    
    chmod +x "$SUITES_DIR/$script_name"
}

generate_docker_tar_export() {
    local driver=$1
    local script_name="docker_tar_export_${driver}.sh"
    
    cat > "$SUITES_DIR/$script_name" <<'SCRIPT_EOF'
#!/bin/bash -e
# Docker Test: TAR Export [DRIVER]

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source "$SCRIPT_DIR/common.sh"

REGISTRY="REGISTRY_PLACEHOLDER"
SMITHY_IMAGE="SMITHY_IMAGE_PLACEHOLDER"
DOCKER_CONFIG_DIR="DOCKER_CONFIG_DIR_PLACEHOLDER"
RF_SMITHY_TMPDIR="${RF_SMITHY_TMPDIR:-/tmp}"
DRIVER="DRIVER_PLACEHOLDER"

log_info "Running Docker TAR Export [$DRIVER]..."

cat > $RF_SMITHY_TMPDIR/Dockerfile.tar <<EOF
FROM alpine:latest
RUN echo "Export test"
EOF

mkdir -p $RF_SMITHY_TMPDIR/output
chmod 777 $RF_SMITHY_TMPDIR/output

docker run --rm \
    --cap-drop ALL \
    --cap-add SETUID \
    --cap-add SETGID \
    --security-opt seccomp=unconfined \
    --security-opt apparmor=unconfined \
    --user 1000:1000 \
    -v $RF_SMITHY_TMPDIR:/workspace \
    -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
    -e HOME=/home/smithy \
    -e DOCKER_CONFIG=/home/smithy/.docker \
    $SMITHY_IMAGE \
    --context=/workspace \
    --dockerfile=Dockerfile.tar \
    --destination=$REGISTRY/test/tar-$DRIVER:latest \
    --tar-path=/workspace/output/test-$DRIVER.tar \
    --storage-driver=$DRIVER \
    --insecure-registry=$REGISTRY \
    --no-push

if [ -f "$RF_SMITHY_TMPDIR/output/test-$DRIVER.tar" ]; then
    log_success "TAR export passed"
    rm -f $RF_SMITHY_TMPDIR/Dockerfile.tar
    rm -f $RF_SMITHY_TMPDIR/output/test-$DRIVER.tar
    exit 0
else
    log_error "TAR export failed - file not created"
    exit 1
fi
SCRIPT_EOF

    sed -i "s|DRIVER_PLACEHOLDER|$driver|g" "$SUITES_DIR/$script_name"
    sed -i "s|REGISTRY_PLACEHOLDER|$REGISTRY|g" "$SUITES_DIR/$script_name"
    sed -i "s|SMITHY_IMAGE_PLACEHOLDER|$SMITHY_IMAGE|g" "$SUITES_DIR/$script_name"
    sed -i "s|DOCKER_CONFIG_DIR_PLACEHOLDER|$DOCKER_CONFIG_DIR|g" "$SUITES_DIR/$script_name"
    
    chmod +x "$SUITES_DIR/$script_name"
}

generate_docker_multiple_dest() {
    local driver=$1
    local script_name="docker_multiple_dest_${driver}.sh"
    
    cat > "$SUITES_DIR/$script_name" <<'SCRIPT_EOF'
#!/bin/bash -e
# Docker Test: Multiple Destinations [DRIVER]

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source "$SCRIPT_DIR/common.sh"

REGISTRY="REGISTRY_PLACEHOLDER"
SMITHY_IMAGE="SMITHY_IMAGE_PLACEHOLDER"
DOCKER_CONFIG_DIR="DOCKER_CONFIG_DIR_PLACEHOLDER"
RF_SMITHY_TMPDIR="${RF_SMITHY_TMPDIR:-/tmp}"
DRIVER="DRIVER_PLACEHOLDER"

log_info "Running Docker Multiple Destinations [$DRIVER]..."

cat > $RF_SMITHY_TMPDIR/Dockerfile.multidest <<EOF
FROM alpine:latest
RUN echo "Multiple destinations test"
EOF

docker run --rm \
    --cap-drop ALL \
    --cap-add SETUID \
    --cap-add SETGID \
    --security-opt seccomp=unconfined \
    --security-opt apparmor=unconfined \
    --user 1000:1000 \
    -v $RF_SMITHY_TMPDIR:/workspace \
    -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
    -e HOME=/home/smithy \
    -e DOCKER_CONFIG=/home/smithy/.docker \
    $SMITHY_IMAGE \
    --context=/workspace \
    --dockerfile=Dockerfile.multidest \
    --destination=$REGISTRY/test/multi-$DRIVER:v1 \
    --destination=$REGISTRY/test/multi-$DRIVER:latest \
    --storage-driver=$DRIVER \
    --insecure-registry=$REGISTRY \
    --no-push

if [ $? -eq 0 ]; then
    log_success "Multiple destinations test passed"
    rm -f $RF_SMITHY_TMPDIR/Dockerfile.multidest
    exit 0
else
    log_error "Multiple destinations test failed"
    exit 1
fi
SCRIPT_EOF

    sed -i "s|DRIVER_PLACEHOLDER|$driver|g" "$SUITES_DIR/$script_name"
    sed -i "s|REGISTRY_PLACEHOLDER|$REGISTRY|g" "$SUITES_DIR/$script_name"
    sed -i "s|SMITHY_IMAGE_PLACEHOLDER|$SMITHY_IMAGE|g" "$SUITES_DIR/$script_name"
    sed -i "s|DOCKER_CONFIG_DIR_PLACEHOLDER|$DOCKER_CONFIG_DIR|g" "$SUITES_DIR/$script_name"
    
    chmod +x "$SUITES_DIR/$script_name"
}

generate_docker_platform() {
    local driver=$1
    local script_name="docker_platform_${driver}.sh"
    
    cat > "$SUITES_DIR/$script_name" <<'SCRIPT_EOF'
#!/bin/bash -e
# Docker Test: Platform Specification [DRIVER]

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source "$SCRIPT_DIR/common.sh"

REGISTRY="REGISTRY_PLACEHOLDER"
SMITHY_IMAGE="SMITHY_IMAGE_PLACEHOLDER"
DOCKER_CONFIG_DIR="DOCKER_CONFIG_DIR_PLACEHOLDER"
RF_SMITHY_TMPDIR="${RF_SMITHY_TMPDIR:-/tmp}"
DRIVER="DRIVER_PLACEHOLDER"

log_info "Running Docker Platform Specification [$DRIVER]..."

cat > $RF_SMITHY_TMPDIR/Dockerfile.platform <<EOF
FROM alpine:latest
RUN echo "Platform test"
EOF

docker run --rm \
    --cap-drop ALL \
    --cap-add SETUID \
    --cap-add SETGID \
    --security-opt seccomp=unconfined \
    --security-opt apparmor=unconfined \
    --user 1000:1000 \
    -v $RF_SMITHY_TMPDIR:/workspace \
    -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
    -e HOME=/home/smithy \
    -e DOCKER_CONFIG=/home/smithy/.docker \
    $SMITHY_IMAGE \
    --context=/workspace \
    --dockerfile=Dockerfile.platform \
    --destination=$REGISTRY/test/platform-$DRIVER:latest \
    --custom-platform=linux/amd64 \
    --storage-driver=$DRIVER \
    --insecure-registry=$REGISTRY \
    --no-push

if [ $? -eq 0 ]; then
    log_success "Platform specification test passed"
    rm -f $RF_SMITHY_TMPDIR/Dockerfile.platform
    exit 0
else
    log_error "Platform specification test failed"
    exit 1
fi
SCRIPT_EOF

    sed -i "s|DRIVER_PLACEHOLDER|$driver|g" "$SUITES_DIR/$script_name"
    sed -i "s|REGISTRY_PLACEHOLDER|$REGISTRY|g" "$SUITES_DIR/$script_name"
    sed -i "s|SMITHY_IMAGE_PLACEHOLDER|$SMITHY_IMAGE|g" "$SUITES_DIR/$script_name"
    sed -i "s|DOCKER_CONFIG_DIR_PLACEHOLDER|$DOCKER_CONFIG_DIR|g" "$SUITES_DIR/$script_name"
    
    chmod +x "$SUITES_DIR/$script_name"
}

generate_kubernetes_tests() {
    local drivers=()
    
    if [ "$STORAGE_DRIVER" = "both" ]; then
        drivers=("vfs" "overlay")
    else
        drivers=("$STORAGE_DRIVER")
    fi
    
    for driver in "${drivers[@]}"; do
        generate_k8s_version_check "$driver"
        generate_k8s_basic_build "$driver"
        generate_k8s_build_args "$driver"
        generate_k8s_git_repo "$driver"
        generate_k8s_multistage "$driver"
    done
}

generate_k8s_version_check() {
    local driver=$1
    local script_name="kubernetes_version_check_${driver}.sh"
    
    cat > "$SUITES_DIR/$script_name" <<SCRIPT_EOF
#!/bin/bash -e
# Kubernetes Test: Version Check [$driver]

SCRIPT_DIR="\$( cd "\$( dirname "\${BASH_SOURCE[0]}" )" && pwd )"
source "\$SCRIPT_DIR/common.sh"

REGISTRY="${REGISTRY}"
SMITHY_IMAGE="${SMITHY_IMAGE}"
NAMESPACE="${NAMESPACE}"
DRIVER="$driver"

log_info "Running Kubernetes Version Check [\$DRIVER]..."

kubectl apply -f - <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: test-version-\$DRIVER
  namespace: \$NAMESPACE
spec:
  ttlSecondsAfterFinished: 300
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
        args:
        - "--version"
        securityContext:
          runAsUser: 1000
          runAsNonRoot: true
          allowPrivilegeEscalation: true
          capabilities:
            drop: [ALL]
            add: [SETUID, SETGID]
EOF

kubectl wait --for=condition=complete --timeout=300s job/test-version-\$DRIVER -n \$NAMESPACE

if [ \$? -eq 0 ]; then
    log_success "Kubernetes version check passed"
    exit 0
else
    log_error "Kubernetes version check failed"
    kubectl logs -n \$NAMESPACE job/test-version-\$DRIVER --tail=50
    exit 1
fi
SCRIPT_EOF
    
    chmod +x "$SUITES_DIR/$script_name"
}

generate_k8s_basic_build() {
    local driver=$1
    local script_name="kubernetes_basic_build_${driver}.sh"
    
    cat > "$SUITES_DIR/$script_name" <<SCRIPT_EOF
#!/bin/bash -e
# Kubernetes Test: Basic Build [$driver]

SCRIPT_DIR="\$( cd "\$( dirname "\${BASH_SOURCE[0]}" )" && pwd )"
source "\$SCRIPT_DIR/common.sh"

REGISTRY="${REGISTRY}"
SMITHY_IMAGE="${SMITHY_IMAGE}"
NAMESPACE="${NAMESPACE}"
DRIVER="$driver"

log_info "Running Kubernetes Basic Build [\$DRIVER]..."

kubectl apply -f - <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: test-basic-\$DRIVER
  namespace: \$NAMESPACE
spec:
  ttlSecondsAfterFinished: 300
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
        args:
        - "--context=/workspace"
        - "--dockerfile=Dockerfile"
        - "--destination=\$REGISTRY/test/k8s-basic-\$DRIVER:latest"
        - "--storage-driver=\$DRIVER"
        - "--insecure-registry=\$REGISTRY"
        - "--no-push"
        securityContext:
          runAsUser: 1000
          runAsNonRoot: true
          allowPrivilegeEscalation: true
          capabilities:
            drop: [ALL]
            add: [SETUID, SETGID]
        volumeMounts:
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
            path: config.json
EOF

kubectl wait --for=condition=complete --timeout=300s job/test-basic-\$DRIVER -n \$NAMESPACE

if [ \$? -eq 0 ]; then
    log_success "Kubernetes basic build passed"
    exit 0
else
    log_error "Kubernetes basic build failed"
    kubectl logs -n \$NAMESPACE job/test-basic-\$DRIVER --tail=50
    exit 1
fi
SCRIPT_EOF
    
    chmod +x "$SUITES_DIR/$script_name"
}

generate_k8s_build_args() {
    local driver=$1
    local script_name="kubernetes_build_args_${driver}.sh"
    
    cat > "$SUITES_DIR/$script_name" <<SCRIPT_EOF
#!/bin/bash -e
# Kubernetes Test: Build Arguments [$driver]

SCRIPT_DIR="\$( cd "\$( dirname "\${BASH_SOURCE[0]}" )" && pwd )"
source "\$SCRIPT_DIR/common.sh"

REGISTRY="${REGISTRY}"
SMITHY_IMAGE="${SMITHY_IMAGE}"
NAMESPACE="${NAMESPACE}"
DRIVER="$driver"

log_info "Running Kubernetes Build Arguments [\$DRIVER]..."

kubectl apply -f - <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: test-buildargs-\$DRIVER
  namespace: \$NAMESPACE
spec:
  ttlSecondsAfterFinished: 300
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
        args:
        - "--context=/workspace"
        - "--dockerfile=Dockerfile"
        - "--destination=\$REGISTRY/test/k8s-buildargs-\$DRIVER:latest"
        - "--build-arg=VERSION=2.0"
        - "--build-arg=BUILD_DATE=\$(date +%Y%m%d)"
        - "--storage-driver=\$DRIVER"
        - "--insecure-registry=\$REGISTRY"
        - "--no-push"
        securityContext:
          runAsUser: 1000
          runAsNonRoot: true
          allowPrivilegeEscalation: true
          capabilities:
            drop: [ALL]
            add: [SETUID, SETGID]
        volumeMounts:
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
            path: config.json
EOF

kubectl wait --for=condition=complete --timeout=300s job/test-buildargs-\$DRIVER -n \$NAMESPACE

if [ \$? -eq 0 ]; then
    log_success "Kubernetes build arguments test passed"
    exit 0
else
    log_error "Kubernetes build arguments test failed"
    kubectl logs -n \$NAMESPACE job/test-buildargs-\$DRIVER --tail=50
    exit 1
fi
SCRIPT_EOF
    
    chmod +x "$SUITES_DIR/$script_name"
}

generate_k8s_git_repo() {
    local driver=$1
    local script_name="kubernetes_git_repo_${driver}.sh"
    
    cat > "$SUITES_DIR/$script_name" <<SCRIPT_EOF
#!/bin/bash -e
# Kubernetes Test: Git Repository Build [$driver]

SCRIPT_DIR="\$( cd "\$( dirname "\${BASH_SOURCE[0]}" )" && pwd )"
source "\$SCRIPT_DIR/common.sh"

REGISTRY="${REGISTRY}"
SMITHY_IMAGE="${SMITHY_IMAGE}"
NAMESPACE="${NAMESPACE}"
DRIVER="$driver"

log_info "Running Kubernetes Git Repository Build [\$DRIVER]..."

kubectl apply -f - <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: test-git-\$DRIVER
  namespace: \$NAMESPACE
spec:
  ttlSecondsAfterFinished: 300
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
        args:
        - "--context=https://github.com/nginxinc/docker-nginx.git"
        - "--git-branch=master"
        - "--dockerfile=mainline/alpine/Dockerfile"
        - "--destination=\$REGISTRY/test/k8s-git-\$DRIVER:latest"
        - "--storage-driver=\$DRIVER"
        - "--insecure-registry=\$REGISTRY"
        - "--no-push"
        securityContext:
          runAsUser: 1000
          runAsNonRoot: true
          allowPrivilegeEscalation: true
          capabilities:
            drop: [ALL]
            add: [SETUID, SETGID]
        volumeMounts:
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
            path: config.json
EOF

kubectl wait --for=condition=complete --timeout=300s job/test-git-\$DRIVER -n \$NAMESPACE

if [ \$? -eq 0 ]; then
    log_success "Kubernetes git repository build passed"
    exit 0
else
    log_error "Kubernetes git repository build failed"
    kubectl logs -n \$NAMESPACE job/test-git-\$DRIVER --tail=50
    exit 1
fi
SCRIPT_EOF
    
    chmod +x "$SUITES_DIR/$script_name"
}

generate_k8s_multistage() {
    local driver=$1
    local script_name="kubernetes_multistage_${driver}.sh"
    
    cat > "$SUITES_DIR/$script_name" <<SCRIPT_EOF
#!/bin/bash -e
# Kubernetes Test: Multi-stage Build [$driver]

SCRIPT_DIR="\$( cd "\$( dirname "\${BASH_SOURCE[0]}" )" && pwd )"
source "\$SCRIPT_DIR/common.sh"

REGISTRY="${REGISTRY}"
SMITHY_IMAGE="${SMITHY_IMAGE}"
NAMESPACE="${NAMESPACE}"
DRIVER="$driver"

log_info "Running Kubernetes Multi-stage Build [\$DRIVER]..."

kubectl apply -f - <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: test-multistage-\$DRIVER
  namespace: \$NAMESPACE
spec:
  ttlSecondsAfterFinished: 300
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
        args:
        - "--context=/workspace"
        - "--dockerfile=Dockerfile"
        - "--destination=\$REGISTRY/test/k8s-multistage-\$DRIVER:latest"
        - "--storage-driver=\$DRIVER"
        - "--insecure-registry=\$REGISTRY"
        - "--no-push"
        securityContext:
          runAsUser: 1000
          runAsNonRoot: true
          allowPrivilegeEscalation: true
          capabilities:
            drop: [ALL]
            add: [SETUID, SETGID]
        volumeMounts:
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
            path: config.json
EOF

kubectl wait --for=condition=complete --timeout=300s job/test-multistage-\$DRIVER -n \$NAMESPACE

if [ \$? -eq 0 ]; then
    log_success "Kubernetes multi-stage build passed"
    exit 0
else
    log_error "Kubernetes multi-stage build failed"
    kubectl logs -n \$NAMESPACE job/test-multistage-\$DRIVER --tail=50
    exit 1
fi
SCRIPT_EOF
    
    chmod +x "$SUITES_DIR/$script_name"
}

# ============================================================================
# Test Execution Functions
# ============================================================================

list_tests() {
    echo -e "${CYAN}Available Test Scripts:${NC}"
    echo ""
    
    if [[ "$TEST_MODE" == "docker" || "$TEST_MODE" == "both" ]]; then
        echo -e "${YELLOW}Docker Tests:${NC}"
        ls -1 "$SUITES_DIR"/docker_*.sh 2>/dev/null | while read script; do
            echo "  $(basename "$script")"
        done
        echo ""
    fi
    
    if [[ "$TEST_MODE" == "kubernetes" || "$TEST_MODE" == "both" ]]; then
        echo -e "${YELLOW}Kubernetes Tests:${NC}"
        ls -1 "$SUITES_DIR"/kubernetes_*.sh 2>/dev/null | while read script; do
            echo "  $(basename "$script")"
        done
        echo ""
    fi
    
    echo -e "${BLUE}To run a specific test:${NC}"
    echo "  cd $SUITES_DIR"
    echo "  ./docker_basic_build_vfs.sh"
    echo ""
    echo -e "${BLUE}Or use single mode:${NC}"
    echo "  $0 -m docker -x single -t docker_basic_build_vfs.sh"
}

run_single_test() {
    local test_script="$SUITES_DIR/$SINGLE_TEST"
    
    if [ ! -f "$test_script" ]; then
        echo -e "${RED}Error: Test script not found: $SINGLE_TEST${NC}"
        echo ""
        echo "Available tests:"
        list_tests
        exit 1
    fi
    
    echo -e "${BLUE}Running single test: $SINGLE_TEST${NC}"
    echo ""
    
    bash "$test_script"
    
    if [ $? -eq 0 ]; then
        echo ""
        echo -e "${GREEN}✓ Test passed!${NC}"
        exit 0
    else
        echo ""
        echo -e "${RED}✗ Test failed!${NC}"
        exit 1
    fi
}

run_all_tests() {
    echo -e "${BLUE}Running all tests in blast mode...${NC}"
    echo ""
    
    local test_scripts=()
    
    if [[ "$TEST_MODE" == "docker" || "$TEST_MODE" == "both" ]]; then
        while IFS= read -r script; do
            test_scripts+=("$script")
        done < <(ls -1 "$SUITES_DIR"/docker_*.sh 2>/dev/null)
    fi
    
    if [[ "$TEST_MODE" == "kubernetes" || "$TEST_MODE" == "both" ]]; then
        while IFS= read -r script; do
            test_scripts+=("$script")
        done < <(ls -1 "$SUITES_DIR"/kubernetes_*.sh 2>/dev/null)
    fi
    
    TOTAL_TESTS=${#test_scripts[@]}
    PASSED_TESTS=0
    FAILED_TESTS=0
    
    for script in "${test_scripts[@]}"; do
        local test_name=$(basename "$script")
        echo -e "${CYAN}Running: $test_name${NC}"
        
        if bash "$script" 2>&1 | grep -q "SUCCESS"; then
            PASSED_TESTS=$((PASSED_TESTS + 1))
            TEST_RESULTS+=("✓ $test_name")
        else
            FAILED_TESTS=$((FAILED_TESTS + 1))
            TEST_RESULTS+=("✗ $test_name")
        fi
        echo ""
    done
    
    # Show summary
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}                          TEST SUMMARY                              ${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════════${NC}"
    echo ""
    echo "Total Tests:  $TOTAL_TESTS"
    echo -e "Passed:       ${GREEN}$PASSED_TESTS${NC}"
    echo -e "Failed:       ${RED}$FAILED_TESTS${NC}"
    echo "Success Rate: $(( PASSED_TESTS * 100 / TOTAL_TESTS ))%"
    echo ""
    
    echo "Test Results:"
    for result in "${TEST_RESULTS[@]}"; do
        echo "  $result"
    done
    echo ""
    
    if [ $FAILED_TESTS -gt 0 ]; then
        return 1
    fi
    return 0
}

# ============================================================================
# Kubernetes Setup
# ============================================================================

setup_kubernetes() {
    if [[ "$TEST_MODE" == "kubernetes" || "$TEST_MODE" == "both" ]]; then
        echo -e "${BLUE}Setting up Kubernetes resources...${NC}"
        
        # Check kubectl
        if ! command -v kubectl &> /dev/null; then
            echo -e "${RED}Error: kubectl not found${NC}"
            exit 1
        fi
        
        # Check cluster connection
        if ! kubectl cluster-info &> /dev/null; then
            echo -e "${RED}Error: Cannot connect to Kubernetes cluster${NC}"
            exit 1
        fi
        
        # Create namespace
        kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f - >/dev/null 2>&1
        
        # Create Docker registry secret
        if [ "$AUTH_MODE" == "credentials" ]; then
            kubectl create secret docker-registry docker-registry-credentials \
                --namespace=$NAMESPACE \
                --docker-server=$REGISTRY \
                --docker-username=$REGISTRY_USER \
                --docker-password=$REGISTRY_PASS \
                --dry-run=client -o yaml | kubectl apply -f - >/dev/null 2>&1
        elif [ "$AUTH_MODE" == "docker" ]; then
            kubectl create secret generic docker-registry-credentials \
                --namespace=$NAMESPACE \
                --from-file=.dockerconfigjson=$HOME/.docker/config.json \
                --type=kubernetes.io/dockerconfigjson \
                --dry-run=client -o yaml | kubectl apply -f - >/dev/null 2>&1
        else
            kubectl create secret generic docker-registry-credentials \
                --namespace=$NAMESPACE \
                --from-literal=.dockerconfigjson='{"auths":{}}' \
                --type=kubernetes.io/dockerconfigjson \
                --dry-run=client -o yaml | kubectl apply -f - >/dev/null 2>&1
        fi
        
        # Create test ConfigMap
        cat > $RF_SMITHY_TMPDIR/Dockerfile.k8s <<EOF
FROM alpine:latest
RUN echo "Kubernetes test build"
LABEL test="kubernetes"
EOF
        kubectl create configmap test-dockerfiles \
            --namespace=$NAMESPACE \
            --from-file=Dockerfile=$RF_SMITHY_TMPDIR/Dockerfile.k8s \
            --dry-run=client -o yaml | kubectl apply -f - >/dev/null 2>&1
        
        echo -e "${GREEN}✓ Kubernetes setup complete${NC}"
        echo ""
    fi
}

# ============================================================================
# Main Execution
# ============================================================================

main() {
    parse_args "$@"
    
    if [ $# -eq 0 ]; then
        usage
    fi
    
    validate_args
    print_header
    
    echo -e "${BLUE}Configuration:${NC}"
    echo "  Test Mode:        $TEST_MODE"
    echo "  Run Mode:         $RUN_MODE"
    echo "  Storage Driver:   $STORAGE_DRIVER"
    echo "  Registry:         $REGISTRY"
    echo "  Auth Mode:        $AUTH_MODE"
    echo "  Smithy Image:     $SMITHY_IMAGE"
    if [ "$TEST_MODE" == "kubernetes" ] || [ "$TEST_MODE" == "both" ]; then
        echo "  K8s Namespace:    $NAMESPACE"
    fi
    echo ""
    
    # Create suites directory and generate test scripts
    create_suites_directory
    
    if [[ "$TEST_MODE" == "docker" || "$TEST_MODE" == "both" ]]; then
        echo -e "${BLUE}Generating Docker test scripts...${NC}"
        generate_docker_tests
        echo -e "${GREEN}✓ Docker test scripts generated${NC}"
        echo ""
    fi
    
    if [[ "$TEST_MODE" == "kubernetes" || "$TEST_MODE" == "both" ]]; then
        echo -e "${BLUE}Generating Kubernetes test scripts...${NC}"
        generate_kubernetes_tests
        echo -e "${GREEN}✓ Kubernetes test scripts generated${NC}"
        echo ""
    fi
    
    # Handle run modes
    case $RUN_MODE in
        list)
            list_tests
            exit 0
            ;;
        single)
            create_docker_config
            setup_kubernetes
            run_single_test
            ;;
        blast)
            create_docker_config
            setup_kubernetes
            EXIT_CODE=0
            if ! run_all_tests; then
                EXIT_CODE=1
            fi
            
            if [ "$CLEANUP_AFTER" = true ]; then
                cleanup_resources
            fi
            
            echo ""
            echo -e "${CYAN}═══════════════════════════════════════════════════════════════════${NC}"
            echo -e "${CYAN}                        ALL TESTS COMPLETE                          ${NC}"
            echo -e "${CYAN}═══════════════════════════════════════════════════════════════════${NC}"
            echo ""
            
            if [ $EXIT_CODE -eq 0 ]; then
                echo -e "${GREEN}✓ All tests completed successfully!${NC}"
                echo ""
                echo -e "${BLUE}Test scripts available in: $SUITES_DIR${NC}"
                echo "You can run individual tests manually:"
                echo "  cd $SUITES_DIR"
                echo "  ./docker_basic_build_vfs.sh"
            else
                echo -e "${RED}✗ Some tests failed. Check the output above for details.${NC}"
                echo ""
                echo -e "${BLUE}Debug failed tests individually:${NC}"
                echo "  cd $SUITES_DIR"
                echo "  ./failed_test_name.sh"
            fi
            
            exit $EXIT_CODE
            ;;
    esac
}

main "$@"
