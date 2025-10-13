#!/bin/bash -e
# Smithy Master Test Script - Build Tests Only
# Supports both Docker and Kubernetes testing

set -e

if [ -z "${RF_APP_HOST}" ]; then
    REGISTRY=ghcr.io
else
    REGISTRY=${RF_APP_HOST}:5000}
fi

# Configuration
NAMESPACE="smithy-tests"
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
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

# ============================================================================
# Usage Function
# ============================================================================

usage() {
    cat <<EOF
Usage: $0 [OPTIONS]

Smithy Master Test Script - Build Testing

OPTIONS:
    -h, --help              Show this help message
    -m, --mode MODE         Test mode: docker, kubernetes, or both (required)
    -r, --registry URL      Registry URL (default: $REGISTRY)
    -i, --image IMAGE       Smithy image to test (default: auto-detect)
    -a, --auth MODE         Auth mode: none, credentials, or docker (default: none)
    -u, --user USERNAME     Registry username (for credentials auth)
    -p, --pass PASSWORD     Registry password (for credentials auth)
    -n, --namespace NS      Kubernetes namespace (default: $NAMESPACE)
    -c, --cleanup           Cleanup resources after tests
    -v, --verbose           Verbose output

EXAMPLES:
    # Run Docker tests without auth
    $0 -m docker

    # Run Kubernetes tests with credentials
    $0 -m kubernetes -a credentials -u myuser -p mypass

    # Run both tests with custom registry and cleanup
    $0 -m both -r myregistry:5000 -c

    # Run with custom smithy image
    $0 -m docker -i ghcr.io/rapidfort/smithy:latest

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
        # No authentication
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
        # Create base64 encoded auth
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
        # Copy existing Docker config
        cp "$HOME/.docker/config.json" "$DOCKER_CONFIG_DIR/config.json"
        log_verbose "Using existing Docker config"
    fi

    # Make config.json readable by smithy user (UID 1000)
    chmod 644 "$DOCKER_CONFIG_DIR/config.json"
    chmod 755 "$DOCKER_CONFIG_DIR"

    log_verbose "Fixed permissions on Docker config (readable by all)"
}

test_registry_connection() {
    echo -e "${BLUE}Testing Registry Connection${NC}"
    echo "────────────────────────────────────"
    echo "Registry: $REGISTRY"

    # Test with curl
    echo -n "Testing registry API... "
    if curl -s -f -o /dev/null "http://$REGISTRY/v2/" 2>/dev/null; then
        echo -e "${GREEN}✓${NC}"
    else
        echo -e "${YELLOW}✗ (may require auth)${NC}"
    fi

    # Test with docker if credentials provided
    if [ -n "$REGISTRY_USER" ] && [ -n "$REGISTRY_PASS" ]; then
        echo -n "Testing Docker login... "
        if echo "$REGISTRY_PASS" | docker login "$REGISTRY" -u "$REGISTRY_USER" --password-stdin >/dev/null 2>&1; then
            echo -e "${GREEN}✓${NC}"
        else
            echo -e "${RED}✗${NC}"
        fi
    fi

    echo ""
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
# Docker Test Functions
# ============================================================================

run_docker_tests() {
    echo ""
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}                        DOCKER TEST SUITE                           ${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════════${NC}"
    echo ""

    # Setup
    TOTAL_TESTS=10
    PASSED_TESTS=0
    FAILED_TESTS=0
    TEST_RESULTS=()

    # Docker flags
    DOCKER_FLAGS="--rm --security-opt seccomp=unconfined --security-opt apparmor=unconfined --user 1000:1000"

    # Ensure docker config exists with proper permissions
    create_docker_config

    # Run tests
    docker_test_1_version
    docker_test_2_basic_build
    docker_test_3_build_args
    docker_test_4_labels
    docker_test_5_multistage
    docker_test_6_git_repo
    docker_test_7_cache
    docker_test_8_tar_export
    docker_test_9_multiple_dest
    docker_test_10_platform

    # Summary
    show_docker_summary
}

run_docker_test() {
    local test_num=$1
    local test_name="$2"
    echo ""
    echo -e "${BLUE}[Docker Test $test_num/$TOTAL_TESTS] $test_name${NC}"
    echo "────────────────────────────────────────────────────────────────────────────"
}

record_docker_result() {
    local test_num=$1
    local test_name="$2"
    local status=$3

    if [ $status -eq 0 ]; then
        echo -e "${GREEN}✓ PASSED${NC}"
        PASSED_TESTS=$((PASSED_TESTS + 1))
        TEST_RESULTS+=("✓ Docker Test $test_num: $test_name")
    else
        echo -e "${RED}✗ FAILED${NC}"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TEST_RESULTS+=("✗ Docker Test $test_num: $test_name")
    fi
}

docker_test_1_version() {
    run_docker_test 1 "Version Check"

    if [ "$VERBOSE" = true ]; then
        docker run $DOCKER_FLAGS $SMITHY_IMAGE --version
    else
        docker run $DOCKER_FLAGS $SMITHY_IMAGE --version >/dev/null 2>&1
    fi

    record_docker_result 1 "Version Check" $?
}

docker_test_2_basic_build() {
    run_docker_test 2 "Basic Build"

    cat > $RF_SMITHY_TMPDIR/Dockerfile.basic <<EOF
FROM alpine:latest
RUN echo "Test build" && apk add --no-cache curl
LABEL test="smithy-basic"
CMD ["/bin/sh"]
EOF

    if [ "$VERBOSE" = true ]; then
        docker run $DOCKER_FLAGS \
            -v $RF_SMITHY_TMPDIR:/workspace \
            -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
            -e HOME=/home/smithy \
            -e DOCKER_CONFIG=/home/smithy/.docker \
            $SMITHY_IMAGE \
            --context=/workspace \
            --dockerfile=Dockerfile.basic \
            --destination=$REGISTRY/test/basic:latest \
            --insecure-registry=$REGISTRY \
            --no-push
    else
        docker run $DOCKER_FLAGS \
            -v $RF_SMITHY_TMPDIR:/workspace \
            -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
            -e HOME=/home/smithy \
            -e DOCKER_CONFIG=/home/smithy/.docker \
            $SMITHY_IMAGE \
            --context=/workspace \
            --dockerfile=Dockerfile.basic \
            --destination=$REGISTRY/test/basic:latest \
            --insecure-registry=$REGISTRY \
            --no-push >/dev/null 2>&1
    fi

    record_docker_result 2 "Basic Build" $?
}

docker_test_3_build_args() {
    run_docker_test 3 "Build Arguments"

    cat > $RF_SMITHY_TMPDIR/Dockerfile.buildargs <<EOF
FROM alpine:latest
ARG VERSION=1.0
ARG BUILD_DATE
RUN echo "Version: \$VERSION" && echo "Build Date: \$BUILD_DATE"
LABEL version="\$VERSION"
LABEL build_date="\$BUILD_DATE"
EOF

    if [ "$VERBOSE" = true ]; then
        docker run $DOCKER_FLAGS \
            -v $RF_SMITHY_TMPDIR:/workspace \
            -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
            -e HOME=/home/smithy \
            -e DOCKER_CONFIG=/home/smithy/.docker \
            $SMITHY_IMAGE \
            --context=/workspace \
            --dockerfile=Dockerfile.buildargs \
            --destination=$REGISTRY/test/buildargs:latest \
            --build-arg VERSION=2.0 \
            --build-arg BUILD_DATE=$(date +%Y%m%d) \
            --insecure-registry=$REGISTRY \
            --no-push
    else
        docker run $DOCKER_FLAGS \
            -v $RF_SMITHY_TMPDIR:/workspace \
            -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
            -e HOME=/home/smithy \
            -e DOCKER_CONFIG=/home/smithy/.docker \
            $SMITHY_IMAGE \
            --context=/workspace \
            --dockerfile=Dockerfile.buildargs \
            --destination=$REGISTRY/test/buildargs:latest \
            --build-arg VERSION=2.0 \
            --build-arg BUILD_DATE=$(date +%Y%m%d) \
            --insecure-registry=$REGISTRY \
            --no-push >/dev/null 2>&1
    fi

    record_docker_result 3 "Build Arguments" $?
}

docker_test_4_labels() {
    run_docker_test 4 "Labels"

    cat > $RF_SMITHY_TMPDIR/Dockerfile.labels <<EOF
FROM alpine:latest
RUN echo "Testing labels"
EOF

    if [ "$VERBOSE" = true ]; then
        docker run $DOCKER_FLAGS \
            -v $RF_SMITHY_TMPDIR:/workspace \
            -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
            -e HOME=/home/smithy \
            -e DOCKER_CONFIG=/home/smithy/.docker \
            $SMITHY_IMAGE \
            --context=/workspace \
            --dockerfile=Dockerfile.labels \
            --destination=$REGISTRY/test/labels:latest \
            --label maintainer="smithy-test" \
            --label version="1.0" \
            --insecure-registry=$REGISTRY \
            --no-push
    else
        docker run $DOCKER_FLAGS \
            -v $RF_SMITHY_TMPDIR:/workspace \
            -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
            -e HOME=/home/smithy \
            -e DOCKER_CONFIG=/home/smithy/.docker \
            $SMITHY_IMAGE \
            --context=/workspace \
            --dockerfile=Dockerfile.labels \
            --destination=$REGISTRY/test/labels:latest \
            --label maintainer="smithy-test" \
            --label version="1.0" \
            --insecure-registry=$REGISTRY \
            --no-push >/dev/null 2>&1
    fi

    record_docker_result 4 "Labels" $?
}

docker_test_5_multistage() {
    run_docker_test 5 "Multi-stage Build"

    cat > $RF_SMITHY_TMPDIR/Dockerfile.multistage <<EOF
FROM alpine:latest AS builder
RUN echo "Building..." && apk add --no-cache curl

FROM alpine:latest
COPY --from=builder /usr/bin/curl /usr/bin/curl
RUN echo "Final stage"
CMD ["/bin/sh"]
EOF

    if [ "$VERBOSE" = true ]; then
        docker run $DOCKER_FLAGS \
            -v $RF_SMITHY_TMPDIR:/workspace \
            -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
            -e HOME=/home/smithy \
            -e DOCKER_CONFIG=/home/smithy/.docker \
            $SMITHY_IMAGE \
            --context=/workspace \
            --dockerfile=Dockerfile.multistage \
            --destination=$REGISTRY/test/multistage:latest \
            --insecure-registry=$REGISTRY \
            --no-push
    else
        docker run $DOCKER_FLAGS \
            -v $RF_SMITHY_TMPDIR:/workspace \
            -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
            -e HOME=/home/smithy \
            -e DOCKER_CONFIG=/home/smithy/.docker \
            $SMITHY_IMAGE \
            --context=/workspace \
            --dockerfile=Dockerfile.multistage \
            --destination=$REGISTRY/test/multistage:latest \
            --insecure-registry=$REGISTRY \
            --no-push >/dev/null 2>&1
    fi

    record_docker_result 5 "Multi-stage Build" $?
}

docker_test_6_git_repo() {
    run_docker_test 6 "Git Repository Build"

    if [ "$VERBOSE" = true ]; then
        docker run $DOCKER_FLAGS \
            -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
            -e HOME=/home/smithy \
            -e DOCKER_CONFIG=/home/smithy/.docker \
            $SMITHY_IMAGE \
            --context=https://github.com/nginxinc/docker-nginx.git \
            --git-branch=master \
            --dockerfile=mainline/alpine/Dockerfile \
            --destination=$REGISTRY/test/nginx-git:latest \
            --insecure-registry=$REGISTRY \
            --no-push
    else
        docker run $DOCKER_FLAGS \
            -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
            -e HOME=/home/smithy \
            -e DOCKER_CONFIG=/home/smithy/.docker \
            $SMITHY_IMAGE \
            --context=https://github.com/nginxinc/docker-nginx.git \
            --git-branch=master \
            --dockerfile=mainline/alpine/Dockerfile \
            --destination=$REGISTRY/test/nginx-git:latest \
            --insecure-registry=$REGISTRY \
            --no-push >/dev/null 2>&1
    fi

    record_docker_result 6 "Git Repository Build" $?
}

docker_test_7_cache() {
    run_docker_test 7 "Cache Build"

    cat > $RF_SMITHY_TMPDIR/Dockerfile.cache <<EOF
FROM alpine:latest
RUN apk add --no-cache curl wget
RUN echo "Layer 1"
RUN echo "Layer 2"
EOF

    if [ "$VERBOSE" = true ]; then
        docker run $DOCKER_FLAGS \
            -v $RF_SMITHY_TMPDIR:/workspace \
            -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
            -e HOME=/home/smithy \
            -e DOCKER_CONFIG=/home/smithy/.docker \
            $SMITHY_IMAGE \
            --context=/workspace \
            --dockerfile=Dockerfile.cache \
            --destination=$REGISTRY/test/cache:latest \
            --cache \
            --insecure-registry=$REGISTRY \
            --no-push
    else
        docker run $DOCKER_FLAGS \
            -v $RF_SMITHY_TMPDIR:/workspace \
            -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
            -e HOME=/home/smithy \
            -e DOCKER_CONFIG=/home/smithy/.docker \
            $SMITHY_IMAGE \
            --context=/workspace \
            --dockerfile=Dockerfile.cache \
            --destination=$REGISTRY/test/cache:latest \
            --cache \
            --insecure-registry=$REGISTRY \
            --no-push >/dev/null 2>&1
    fi

    record_docker_result 7 "Cache Build" $?
}

docker_test_8_tar_export() {
    run_docker_test 8 "TAR Export"

    cat > $RF_SMITHY_TMPDIR/Dockerfile.tar <<EOF
FROM alpine:latest
RUN echo "Export test"
EOF

    # Create output directory with proper permissions for smithy user (1000:1000)
    mkdir -p $RF_SMITHY_TMPDIR/output
    chmod 777 $RF_SMITHY_TMPDIR/output  # Make it writable by any user

    if [ "$VERBOSE" = true ]; then
        docker run $DOCKER_FLAGS \
            -v $RF_SMITHY_TMPDIR:/workspace \
            -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
            -e HOME=/home/smithy \
            -e DOCKER_CONFIG=/home/smithy/.docker \
            $SMITHY_IMAGE \
            --context=/workspace \
            --dockerfile=Dockerfile.tar \
            --destination=$REGISTRY/test/tar:latest \
            --tar-path=/workspace/output/test.tar \
            --insecure-registry=$REGISTRY \
            --no-push
    else
        docker run $DOCKER_FLAGS \
            -v $RF_SMITHY_TMPDIR:/workspace \
            -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
            -e HOME=/home/smithy \
            -e DOCKER_CONFIG=/home/smithy/.docker \
            $SMITHY_IMAGE \
            --context=/workspace \
            --dockerfile=Dockerfile.tar \
            --destination=$REGISTRY/test/tar:latest \
            --tar-path=/workspace/output/test.tar \
            --insecure-registry=$REGISTRY \
            --no-push >/dev/null 2>&1
    fi

    # Check if tar file was created
    if [ -f "$RF_SMITHY_TMPDIR/output/test.tar" ]; then
        record_docker_result 8 "TAR Export" 0
    else
        record_docker_result 8 "TAR Export" 1
    fi
}

docker_test_9_multiple_dest() {
    run_docker_test 9 "Multiple Destinations"

    cat > $RF_SMITHY_TMPDIR/Dockerfile.multidest <<EOF
FROM alpine:latest
RUN echo "Multiple destinations test"
EOF

    if [ "$VERBOSE" = true ]; then
        docker run $DOCKER_FLAGS \
            -v $RF_SMITHY_TMPDIR:/workspace \
            -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
            -e HOME=/home/smithy \
            -e DOCKER_CONFIG=/home/smithy/.docker \
            $SMITHY_IMAGE \
            --context=/workspace \
            --dockerfile=Dockerfile.multidest \
            --destination=$REGISTRY/test/multi:v1 \
            --destination=$REGISTRY/test/multi:latest \
            --insecure-registry=$REGISTRY \
            --no-push
    else
        docker run $DOCKER_FLAGS \
            -v $RF_SMITHY_TMPDIR:/workspace \
            -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
            -e HOME=/home/smithy \
            -e DOCKER_CONFIG=/home/smithy/.docker \
            $SMITHY_IMAGE \
            --context=/workspace \
            --dockerfile=Dockerfile.multidest \
            --destination=$REGISTRY/test/multi:v1 \
            --destination=$REGISTRY/test/multi:latest \
            --insecure-registry=$REGISTRY \
            --no-push >/dev/null 2>&1
    fi

    record_docker_result 9 "Multiple Destinations" $?
}

docker_test_10_platform() {
    run_docker_test 10 "Platform Specification"

    cat > $RF_SMITHY_TMPDIR/Dockerfile.platform <<EOF
FROM alpine:latest
RUN echo "Platform test"
EOF

    if [ "$VERBOSE" = true ]; then
        docker run $DOCKER_FLAGS \
            -v $RF_SMITHY_TMPDIR:/workspace \
            -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
            -e HOME=/home/smithy \
            -e DOCKER_CONFIG=/home/smithy/.docker \
            $SMITHY_IMAGE \
            --context=/workspace \
            --dockerfile=Dockerfile.platform \
            --destination=$REGISTRY/test/platform:latest \
            --custom-platform=linux/amd64 \
            --insecure-registry=$REGISTRY \
            --no-push
    else
        docker run $DOCKER_FLAGS \
            -v $RF_SMITHY_TMPDIR:/workspace \
            -v "$DOCKER_CONFIG_DIR":/home/smithy/.docker:ro \
            -e HOME=/home/smithy \
            -e DOCKER_CONFIG=/home/smithy/.docker \
            $SMITHY_IMAGE \
            --context=/workspace \
            --dockerfile=Dockerfile.platform \
            --destination=$REGISTRY/test/platform:latest \
            --custom-platform=linux/amd64 \
            --insecure-registry=$REGISTRY \
            --no-push >/dev/null 2>&1
    fi

    record_docker_result 10 "Platform Specification" $?
}

show_docker_summary() {
    echo ""
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}                     DOCKER TEST SUMMARY                            ${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════════${NC}"
    echo ""
    echo "Total Tests:  $TOTAL_TESTS"
    echo -e "Passed:       ${GREEN}$PASSED_TESTS${NC}"
    echo -e "Failed:       ${RED}$FAILED_TESTS${NC}"
    echo "Success Rate: $(( PASSED_TESTS * 100 / TOTAL_TESTS ))%"
    echo ""

    if [ "$VERBOSE" = true ] || [ $FAILED_TESTS -gt 0 ]; then
        echo "Test Results:"
        for result in "${TEST_RESULTS[@]}"; do
            echo "  $result"
        done
        echo ""
    fi

    # Return non-zero if any tests failed
    if [ $FAILED_TESTS -gt 0 ]; then
        return 1
    fi
    return 0
}

# ============================================================================
# Kubernetes Test Functions
# ============================================================================

run_kubernetes_tests() {
    echo ""
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}                      KUBERNETES TEST SUITE                         ${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════════${NC}"
    echo ""

    # Setup
    TOTAL_TESTS=5
    PASSED_TESTS=0
    FAILED_TESTS=0
    TEST_RESULTS=()

    # Check kubectl
    if ! command -v kubectl &> /dev/null; then
        echo -e "${RED}Error: kubectl not found${NC}"
        return 1
    fi

    # Check cluster connection
    if ! kubectl cluster-info &> /dev/null; then
        echo -e "${RED}Error: Cannot connect to Kubernetes cluster${NC}"
        return 1
    fi

    # Create namespace
    echo -e "${BLUE}Creating namespace: $NAMESPACE${NC}"
    kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f - >/dev/null 2>&1

    # Create Docker registry secret
    create_k8s_registry_secret

    # Create test ConfigMap
    create_k8s_test_configmap

    # Run test jobs
    k8s_test_1_version
    k8s_test_2_basic_build
    k8s_test_3_build_args
    k8s_test_4_git_repo
    k8s_test_5_multistage

    # Wait for completion
    echo ""
    echo -e "${BLUE}Waiting for all jobs to complete...${NC}"
    sleep 10

    # Show summary
    show_k8s_summary
}

create_k8s_registry_secret() {
    log_verbose "Creating Kubernetes registry secret..."

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
        # Create empty secret for no-auth mode
        kubectl create secret generic docker-registry-credentials \
            --namespace=$NAMESPACE \
            --from-literal=.dockerconfigjson='{"auths":{}}' \
            --type=kubernetes.io/dockerconfigjson \
            --dry-run=client -o yaml | kubectl apply -f - >/dev/null 2>&1
    fi
}

create_k8s_test_configmap() {
    log_verbose "Creating test ConfigMap..."

    cat > $RF_SMITHY_TMPDIR/Dockerfile.k8s <<EOF
FROM alpine:latest
RUN echo "Kubernetes test build"
LABEL test="kubernetes"
EOF

    kubectl create configmap test-dockerfiles \
        --namespace=$NAMESPACE \
        --from-file=Dockerfile=$RF_SMITHY_TMPDIR/Dockerfile.k8s \
        --dry-run=client -o yaml | kubectl apply -f - >/dev/null 2>&1
}

run_k8s_job() {
    local job_name="$1"
    local test_name="$2"
    local job_yaml="$3"

    echo ""
    echo -e "${BLUE}[Kubernetes Test] $test_name${NC}"
    echo "────────────────────────────────────────────────────────────────────────────"

    # Apply the job
    echo "$job_yaml" | kubectl apply -f - >/dev/null 2>&1

    # Wait for job to complete (timeout 5 minutes)
    if kubectl wait --for=condition=complete --timeout=300s job/$job_name -n $NAMESPACE >/dev/null 2>&1; then
        echo -e "${GREEN}✓ PASSED${NC}"
        PASSED_TESTS=$((PASSED_TESTS + 1))
        TEST_RESULTS+=("✓ K8s Test: $test_name")
        return 0
    else
        echo -e "${RED}✗ FAILED${NC}"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TEST_RESULTS+=("✗ K8s Test: $test_name")

        if [ "$VERBOSE" = true ]; then
            echo "Job logs:"
            kubectl logs -n $NAMESPACE job/$job_name --tail=50
        fi
        return 1
    fi
}

k8s_test_1_version() {
    run_k8s_job "test-01-version" "Version Check" "$(cat <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: test-01-version
  namespace: $NAMESPACE
spec:
  ttlSecondsAfterFinished: 300
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: smithy
        image: $SMITHY_IMAGE
        args:
        - "--version"
        securityContext:
          runAsUser: 1000
          runAsNonRoot: true
          allowPrivilegeEscalation: true
          capabilities:
            drop:
            - ALL
            add:
            - SETUID
            - SETGID
EOF
)"
}

k8s_test_2_basic_build() {
    run_k8s_job "test-02-basic" "Basic Build" "$(cat <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: test-02-basic
  namespace: $NAMESPACE
spec:
  ttlSecondsAfterFinished: 300
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: smithy
        image: $SMITHY_IMAGE
        args:
        - "--context=/workspace"
        - "--dockerfile=Dockerfile"
        - "--destination=$REGISTRY/test/k8s-basic:latest"
        - "--insecure-registry=$REGISTRY"
        - "--no-push"
        securityContext:
          runAsUser: 1000
          runAsNonRoot: true
          allowPrivilegeEscalation: true
          capabilities:
            drop:
            - ALL
            add:
            - SETUID
            - SETGID
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
)"
}

k8s_test_3_build_args() {
    run_k8s_job "test-03-buildargs" "Build Arguments" "$(cat <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: test-03-buildargs
  namespace: $NAMESPACE
spec:
  ttlSecondsAfterFinished: 300
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: smithy
        image: $SMITHY_IMAGE
        args:
        - "--context=/workspace"
        - "--dockerfile=Dockerfile"
        - "--destination=$REGISTRY/test/k8s-buildargs:latest"
        - "--build-arg=VERSION=2.0"
        - "--build-arg=BUILD_DATE=$(date +%Y%m%d)"
        - "--insecure-registry=$REGISTRY"
        - "--no-push"
        securityContext:
          runAsUser: 1000
          runAsNonRoot: true
          allowPrivilegeEscalation: true
          capabilities:
            drop:
            - ALL
            add:
            - SETUID
            - SETGID
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
)"
}

k8s_test_4_git_repo() {
    run_k8s_job "test-04-git" "Git Repository Build" "$(cat <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: test-04-git
  namespace: $NAMESPACE
spec:
  ttlSecondsAfterFinished: 300
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: smithy
        image: $SMITHY_IMAGE
        args:
        - "--context=https://github.com/nginxinc/docker-nginx.git"
        - "--git-branch=master"
        - "--dockerfile=mainline/alpine/Dockerfile"
        - "--destination=$REGISTRY/test/k8s-git:latest"
        - "--insecure-registry=$REGISTRY"
        - "--no-push"
        securityContext:
          runAsUser: 1000
          runAsNonRoot: true
          allowPrivilegeEscalation: true
          capabilities:
            drop:
            - ALL
            add:
            - SETUID
            - SETGID
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
)"
}

k8s_test_5_multistage() {
    run_k8s_job "test-05-multistage" "Multi-stage Build" "$(cat <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: test-05-multistage
  namespace: $NAMESPACE
spec:
  ttlSecondsAfterFinished: 300
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: smithy
        image: $SMITHY_IMAGE
        args:
        - "--context=/workspace"
        - "--dockerfile=Dockerfile"
        - "--destination=$REGISTRY/test/k8s-multistage:latest"
        - "--insecure-registry=$REGISTRY"
        - "--no-push"
        securityContext:
          runAsUser: 1000
          runAsNonRoot: true
          allowPrivilegeEscalation: true
          capabilities:
            drop:
            - ALL
            add:
            - SETUID
            - SETGID
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
)"
}

show_k8s_summary() {
    echo ""
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}                     KUBERNETES TEST SUMMARY                        ${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════════${NC}"
    echo ""
    echo "Total Tests:  $TOTAL_TESTS"
    echo -e "Passed:       ${GREEN}$PASSED_TESTS${NC}"
    echo -e "Failed:       ${RED}$FAILED_TESTS${NC}"
    echo "Success Rate: $(( PASSED_TESTS * 100 / TOTAL_TESTS ))%"
    echo ""

    if [ "$VERBOSE" = true ] || [ $FAILED_TESTS -gt 0 ]; then
        echo "Test Results:"
        for result in "${TEST_RESULTS[@]}"; do
            echo "  $result"
        done
        echo ""
    fi

    if [ $FAILED_TESTS -gt 0 ]; then
        return 1
    fi
    return 0
}

# ============================================================================
# Main Execution
# ============================================================================

main() {
    # Parse command line arguments
    parse_args "$@"

    # If no arguments provided, show usage
    if [ $# -eq 0 ]; then
        usage
    fi

    # Validate arguments
    validate_args

    # Print header
    print_header

    # Show configuration
    echo -e "${BLUE}Configuration:${NC}"
    echo "  Test Mode:      $TEST_MODE"
    echo "  Registry:       $REGISTRY"
    echo "  Auth Mode:      $AUTH_MODE"
    echo "  Smithy Image:   $SMITHY_IMAGE"
    if [ "$TEST_MODE" == "kubernetes" ] || [ "$TEST_MODE" == "both" ]; then
        echo "  K8s Namespace:  $NAMESPACE"
    fi
    echo ""

    # Test registry connection if verbose
    if [ "$VERBOSE" = true ]; then
        test_registry_connection
    fi

    # Create docker config with proper permissions
    create_docker_config

    # Run tests based on mode
    EXIT_CODE=0
    case $TEST_MODE in
        docker)
            if ! run_docker_tests; then
                EXIT_CODE=1
            fi
            ;;
        kubernetes)
            if ! run_kubernetes_tests; then
                EXIT_CODE=1
            fi
            ;;
        both)
            if ! run_docker_tests; then
                EXIT_CODE=1
            fi
            if ! run_kubernetes_tests; then
                EXIT_CODE=1
            fi
            ;;
    esac

    # Cleanup if requested
    if [ "$CLEANUP_AFTER" = true ]; then
        cleanup_resources
    fi

    # Final summary
    echo ""
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}                        ALL TESTS COMPLETE                          ${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════════${NC}"
    echo ""

    if [ $EXIT_CODE -eq 0 ]; then
        echo -e "${GREEN}✓ All tests completed successfully!${NC}"
    else
        echo -e "${RED}✗ Some tests failed. Check the output above for details.${NC}"
    fi

    exit $EXIT_CODE
}

# Run main function
main "$@"
