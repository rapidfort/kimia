#!/bin/bash
# Kimia Kubernetes Test Suite
# Tests ROOTLESS mode (UID 1000) ONLY
# Kimia is designed for rootless operation in all environments
# Supports BuildKit (default) and Buildah (legacy) images
# Tests storage drivers based on builder:
#   - BuildKit: native (default), overlay
#   - Buildah: vfs (default), overlay (requires emptyDir at /home/kimia/.local)
# Note: Uses native kernel overlayfs via user namespaces (no fuse-overlayfs)

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
NAMESPACE=${NAMESPACE:-"kimia-tests"}
BUILDER=${BUILDER:-"buildkit"}  # buildkit or buildah
STORAGE_DRIVER="both"
CLEANUP_AFTER=false
TEST_SUITE="all"  # all, simple, reproducible, attestation, signing

# Cosign configuration
COSIGN_KEY_PATH=${COSIGN_KEY_PATH:-"/tmp/cosign/cosign.key"}
COSIGN_PASSWORD=${COSIGN_PASSWORD:-"1234567890"}

mkdir -p "$(dirname "$COSIGN_KEY_PATH")"

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

# Timeout settings
JOB_TIMEOUT=600  # 10 minutes

# ============================================================================
# Usage Function
# ============================================================================

show_help() {
    echo -e "${CYAN}Kimia Kubernetes Test Suite${NC}"
    echo "Tests rootless mode (UID 1000) in Kubernetes"
    echo ""
    echo -e "${YELLOW}USAGE:${NC}"
    echo "    $0 [OPTIONS]"
    echo ""
    echo -e "${YELLOW}OPTIONS:${NC}"
    echo "    -h, --help              Show this help message"
    echo "    --registry URL          Registry URL (default: ghcr.io)"
    echo "    --image IMAGE           Kimia image to test"
    echo "    --namespace NS          Kubernetes namespace (default: kimia-tests)"
    echo "    --builder TYPE          Builder: buildkit (default) or buildah"
    echo "    --storage DRIVER        Storage: both (default), native/vfs, overlay"
    echo "    --tests SUITE           Test suite to run (default: all)"
    echo "    --cleanup               Clean up namespace after tests"
    echo ""
    echo -e "${YELLOW}TEST SUITES:${NC}"
    echo "    all                     Run all tests (default)"
    echo "    simple                  Basic tests (7 tests, ~8 min)"
    echo "                           - Version, env check"
    echo "                           - Git builds with/without push"
    echo "    reproducible            Reproducible build tests (3 tests, ~10 min)"
    echo "                           - Build twice, compare digests"
    echo "    attestation             Attestation tests (10 tests, ~18 min, BuildKit only)"
    echo "                           - Simple modes: default, min, max, off"
    echo "                           - Docker-style: sbom, provenance, scan options"
    echo "                           - Combined: both attestations, pass-through"
    echo "    signing                 Signing tests (1 test, ~4 min, BuildKit only)"
    echo "                           - Attestation with cosign signing"
    echo ""
    echo -e "${YELLOW}EXAMPLES:${NC}"
    echo "    # Run all tests"
    echo "    $0 --tests all"
    echo ""
    echo "    # Quick debug: run simple tests only"
    echo "    $0 --tests simple"
    echo ""
    echo "    # Test reproducible builds in Kubernetes"
    echo "    $0 --tests reproducible"
    echo ""
    echo "    # Test attestation (BuildKit only)"
    echo "    $0 --tests attestation --builder buildkit"
    echo ""
    echo "    # Test signing (requires cosign secrets)"
    echo "    $0 --tests signing --builder buildkit"
    echo ""
    echo -e "${YELLOW}NOTES:${NC}"
    echo "    - Attestation and signing tests require BuildKit builder"
    echo "    - Signing tests require cosign-key and cosign-password secrets in namespace"
    echo "    - Registry authentication is auto-detected:"
    echo "        1. .env file with REGISTRY_USERNAME/REGISTRY_PASSWORD (highest priority)"
    echo "        2. Docker config.json at ~/.docker/config.json"
    echo "        3. Anonymous access (no authentication)"
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
        --namespace)
            NAMESPACE="$2"
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
if [[ ! "$TEST_SUITE" =~ ^(all|simple|reproducible|attestation|signing)$ ]]; then
    echo -e "${RED}Error: Invalid test suite '$TEST_SUITE'.${NC}"
    echo -e "${RED}Must be: all, simple, reproducible, attestation, or signing${NC}"
    exit 1
fi

# Validate attestation/signing tests require BuildKit
if [[ "$TEST_SUITE" =~ ^(attestation|signing)$ ]] && [ "$BUILDER" != "buildkit" ]; then
    echo -e "${RED}Error: ${TEST_SUITE} tests require BuildKit builder${NC}"
    echo -e "${YELLOW}Please use: --builder buildkit${NC}"
    exit 1
fi

# Create suites directory
mkdir -p "${SUITES_DIR}"

# ============================================================================
# Helper Functions
# ============================================================================

print_section() {
    echo ""
    echo -e "${BLUE}═══════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}═══════════════════════════════════════════════════════${NC}"
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

# Get the actual storage flag value for kimia
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
# Test Suite Selection Functions
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

# ============================================================================
# Setup Namespace
# ============================================================================

setup_namespace() {
    echo -e "${CYAN}Setting up Kubernetes environment...${NC}"

    # Check kubectl
    if ! command -v kubectl &> /dev/null; then
        echo -e "${RED}Error: kubectl is not installed or not in PATH${NC}"
        exit 1
    fi

    # Check cluster connectivity
    if ! kubectl cluster-info &> /dev/null; then
        echo -e "${RED}Error: Cannot connect to Kubernetes cluster${NC}"
        exit 1
    fi

    echo "Creating namespace: ${NAMESPACE}"
    kubectl create namespace ${NAMESPACE} --dry-run=client -o yaml | kubectl apply -f - > /dev/null

    echo -e "${GREEN}✔ Namespace ready${NC}"
}

# ===========================================================================
# Cosign Key Generation and Secret Management
# ===========================================================================

# Generate cosign key if it doesn't exist
ensure_cosign_key() {
    local key_dir=$(dirname "${COSIGN_KEY_PATH}")
    local key_file="${COSIGN_KEY_PATH}"
    local pub_file="${key_file%.key}.pub"

    # Check if key already exists
    if [ -f "${key_file}" ] && [ -f "${pub_file}" ]; then
        echo -e "${GREEN}✔ Cosign key already exists: ${key_file}${NC}"
        return 0
    fi

    echo -e "${CYAN}Generating cosign key pair...${NC}"

    # Create directory if it doesn't exist
    mkdir -p "${key_dir}"

    # Generate cosign key pair
    if command -v cosign &> /dev/null; then
        # Use local cosign if available
        COSIGN_PASSWORD="${COSIGN_PASSWORD}" cosign generate-key-pair --output-key-prefix="${key_dir}/cosign"
    else
        # Use cosign from docker/podman image
        if command -v docker &> /dev/null; then
            docker run --rm \
                -e COSIGN_PASSWORD="${COSIGN_PASSWORD}" \
                -v "${key_dir}:/tmp/cosign" \
                gcr.io/projectsigstore/cosign:latest \
                generate-key-pair --output-key-prefix="/opt/rapidfort/local-bucket/cosign/cosign"
        elif command -v podman &> /dev/null; then
            podman run --rm \
                -e COSIGN_PASSWORD="${COSIGN_PASSWORD}" \
                -v "${key_dir}:/tmp/cosign:z" \
                gcr.io/projectsigstore/cosign:latest \
                generate-key-pair --output-key-prefix="/opt/rapidfort/local-bucket/cosign/cosign"
        else
            echo -e "${RED}✔ cosign binary not found and neither docker nor podman available${NC}"
            echo -e "${YELLOW}Install cosign: https://docs.sigstore.dev/cosign/installation/${NC}"
            return 1
        fi
    fi

    if [ -f "${key_file}" ] && [ -f "${pub_file}" ]; then
        echo -e "${GREEN}✔ Cosign key pair generated successfully${NC}"
        echo -e "${CYAN}  Private key: ${key_file}${NC}"
        echo -e "${CYAN}  Public key:  ${pub_file}${NC}"
        echo -e "${CYAN}  Password:    ${COSIGN_PASSWORD}${NC}"
        return 0
    else
        echo -e "${RED}✔ Failed to generate cosign key pair${NC}"
        return 1
    fi
}

# Create Kubernetes secrets for cosign key and password
setup_cosign_secrets() {
    echo -e "${CYAN}Setting up cosign secrets in Kubernetes...${NC}"

    # Generate cosign key if it doesn't exist
    if ! ensure_cosign_key; then
        echo -e "${YELLOW}⚠ Warning: Could not generate cosign key${NC}"
        echo -e "${YELLOW}⚠ Signing tests will be skipped${NC}"
        return 1
    fi

    local key_file="${COSIGN_KEY_PATH}"
    local pub_file="${key_file%.key}.pub"

    # Check if secrets already exist
    if kubectl get secret cosign-key -n ${NAMESPACE} &>/dev/null; then
        echo -e "${GREEN}✓ cosign-key secret already exists${NC}"
    else
        echo "Creating cosign-key secret..."
        kubectl create secret generic cosign-key \
            --from-file=cosign.key="${key_file}" \
            --from-file=cosign.pub="${pub_file}" \
            -n ${NAMESPACE}

        if [ $? -eq 0 ]; then
            echo -e "${GREEN}✓ cosign-key secret created${NC}"
        else
            echo -e "${RED}✗ Failed to create cosign-key secret${NC}"
            return 1
        fi
    fi

    if kubectl get secret cosign-password -n ${NAMESPACE} &>/dev/null; then
        echo -e "${GREEN}✓ cosign-password secret already exists${NC}"
    else
        echo "Creating cosign-password secret..."
        kubectl create secret generic cosign-password \
            --from-literal=COSIGN_PASSWORD="${COSIGN_PASSWORD}" \
            -n ${NAMESPACE}

        if [ $? -eq 0 ]; then
            echo -e "${GREEN}✓ cosign-password secret created${NC}"
        else
            echo -e "${RED}✗ Failed to create cosign-password secret${NC}"
            return 1
        fi
    fi

    echo -e "${GREEN}✓ Cosign secrets ready for signing tests${NC}"
    return 0
}

setup_registry_auth_secret() {

    echo -e "${CYAN}Setting up registry authentication secrets...${NC}"

    local env_file="${SCRIPT_DIR}/.env"

    # Check if secret already exists
    if kubectl get secret registry-auth -n ${NAMESPACE} &>/dev/null; then
        kubectl delete secret registry-auth -n ${NAMESPACE}
    fi

    # 1. Check for .env file
    if [ -f "$env_file" ]; then
        echo "Detected .env file - using environment-based authentication"

        # Source the .env file
        source "$env_file"

        # Validate required variables
        if [ -z "$DOCKER_USERNAME" ] || [ -z "$DOCKER_PASSWORD" ]; then
            echo -e "${RED}✗ Error: .env file must contain DOCKER_USERNAME and DOCKER_PASSWORD${NC}"
            return 1
        fi

        kubectl create secret generic registry-auth \
            --from-literal=method="env" \
            --from-literal=DOCKER_USERNAME="${DOCKER_USERNAME}" \
            --from-literal=DOCKER_PASSWORD="${DOCKER_PASSWORD}" \
            --from-literal=DOCKER_REGISTRY="${DOCKER_REGISTRY:-docker.io}" \
            -n ${NAMESPACE}

        local result=$?

        if [ $result -eq 0 ]; then
            echo -e "${GREEN}✓ registry-auth secret created (env-based)${NC}"
            return 0
        else
            echo -e "${RED}✗ Failed to create registry-auth secret${NC}"
            return 1
        fi
    fi

    # 2. Check for Docker config.json
    local docker_config_file="${HOME}/.docker/config.json"
    if [ -f "$docker_config_file" ]; then
        echo "Detected Docker config.json - using Docker config authentication"

        # Create secret from docker config with method marker
        kubectl create secret generic registry-auth \
            --from-literal=method="docker" \
            --from-file=config.json="${docker_config_file}" \
            -n ${NAMESPACE}

        if [ $? -eq 0 ]; then
            echo -e "${GREEN}✓ registry-auth secret created (docker config)${NC}"
            echo -e "${CYAN}  Source: ${docker_config_file}${NC}"
            echo -e "${CYAN}  Mount path: /home/kimia/.docker/config.json${NC}"
            return 0
        else
            echo -e "${RED}✗ Failed to create registry-auth secret${NC}"
            return 1
        fi
    fi

    # 3. Anonymous access (no authentication found)
    echo -e "${YELLOW}⚠ No authentication detected (.env or config.json)${NC}"
    echo -e "${YELLOW}⚠ Running in anonymous mode${NC}"

    return 0
}

# ===========================================================================
# Job YAML Generation (Rootless with storage-specific capabilities)
# ============================================================================

generate_job_yaml() {
    local job_name="$1"
    local driver="$2"
    local args="$3"
    local with_signing="${4:-false}"  # Optional: true if signing test

    # Get actual storage flag
    local storage_flag=$(get_storage_flag "$driver")

    # Use job name for YAML file: buildah-overlay-git-build-1234567890.yaml
    local yaml_file="${SUITES_DIR}/${job_name}.yaml"

    # Set capabilities based on storage driver and builder
    local caps_add="[SETUID, SETGID, DAC_OVERRIDE, MKNOD]"
    local pod_seccomp=""
    local pod_apparmor=""
    local container_seccomp=""
    local container_apparmor=""
    local volume_mounts=""
    local volumes=""
    local env_vars=""
    local has_volumes=false

    # CRITICAL FIX: BuildKit ALWAYS needs Unconfined seccomp + AppArmor for mount syscalls
    # This applies to BOTH native and overlay storage
    if [ "$BUILDER" = "buildkit" ]; then
        container_seccomp="seccompProfile:
            type: Unconfined"
        container_apparmor="appArmorProfile:
            type: Unconfined"
    fi

    # Overlay storage configuration
    if [ "$driver" = "overlay" ]; then
        # Overlay needs additional capabilities and profiles
        if [ "$BUILDER" = "buildah" ]; then
            # Buildah overlay REQUIRES emptyDir at /home/kimia/.local
            # This breaks nested overlayfs since container root is already overlay
            volume_mounts="        - name: kimia-local
          mountPath: /home/kimia/.local"
            volumes="      - name: kimia-local
        emptyDir: {}"
            has_volumes=true
        fi
    fi

    # Signing test needs cosign key and password
    if [ "$with_signing" = "true" ]; then
        volume_mounts="${volume_mounts}
        - name: cosign-key
          mountPath: /tmp/cosign
          readOnly: true"
        volumes="${volumes}
      - name: cosign-key
        secret:
          secretName: cosign-key
          defaultMode: 0400"
        env_vars="        - name: COSIGN_PASSWORD
          valueFrom:
            secretKeyRef:
              name: cosign-password
              key: COSIGN_PASSWORD"
        has_volumes=true
    fi

    if kubectl get secret registry-auth -n ${NAMESPACE} &>/dev/null; then
        # Detect method by checking the 'method' key in the secret
        auth_method=$(kubectl get secret registry-auth -n ${NAMESPACE} -o jsonpath='{.data.method}' 2>/dev/null | base64 -d 2>/dev/null)

        if [ "$auth_method" = "env" ]; then
            # Add environment variables from secret
            env_vars="${env_vars}
        - name: DOCKER_USERNAME
          valueFrom:
            secretKeyRef:
              name: registry-auth
              key: DOCKER_USERNAME
        - name: DOCKER_PASSWORD
          valueFrom:
            secretKeyRef:
              name: registry-auth
              key: DOCKER_PASSWORD
        - name: DOCKER_REGISTRY
          valueFrom:
            secretKeyRef:
              name: registry-auth
              key: DOCKER_REGISTRY"
        elif [ "$auth_method" = "docker" ]; then
            # Mount Docker config.json
            volume_mounts="${volume_mounts}
        - name: registry-auth
          mountPath: /home/kimia/.docker/config.json
          subPath: config.json
          readOnly: true"
            volumes="${volumes}
      - name: registry-auth
        secret:
          secretName: registry-auth
          defaultMode: 0400"
            has_volumes=true
        fi
    fi

    # Generate complete YAML
    cat > "${yaml_file}" << EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: ${job_name}
  namespace: ${NAMESPACE}
spec:
  ttlSecondsAfterFinished: 300
  backoffLimit: 0
  template:
    spec:
      restartPolicy: Never
      securityContext:
        runAsUser: 1000
        runAsGroup: 1000
        runAsNonRoot: true
        fsGroup: 1000
      containers:
      - name: kimia
        image: ${KIMIA_IMAGE}
        imagePullPolicy: Always
        args: ${args}
        securityContext:
          runAsUser: 1000
          runAsGroup: 1000
          runAsNonRoot: true
          allowPrivilegeEscalation: true
          capabilities:
            drop: [ALL]
            add: ${caps_add}
          ${container_seccomp}
          ${container_apparmor}
        resources:
          requests:
            memory: "512Mi"
            cpu: "500m"
          limits:
            memory: "2Gi"
            cpu: "2000m"
EOF

    # Add env vars if present
    if [ -n "$env_vars" ]; then
        cat >> "${yaml_file}" << EOF
        env:
${env_vars}
EOF
    fi

    # Add volume mounts if present
    if [ "$has_volumes" = true ]; then
        cat >> "${yaml_file}" << EOF
        volumeMounts:
${volume_mounts}
      volumes:
${volumes}
EOF
    fi
    echo "${yaml_file}"
}

# ============================================================================
# Run Kubernetes Job Test
# ============================================================================

run_k8s_test() {
    local test_name="$1"
    local driver="$2"
    local args="$3"
    local test_id="$4"
    local with_signing="${5:-false}"

    TOTAL_TESTS=$((TOTAL_TESTS + 1))

    echo ""
    echo -e "${CYAN}═══════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}  Test #${TOTAL_TESTS}: ${test_name}${NC}"
    echo -e "${CYAN}  Storage: ${driver} | Builder: ${BUILDER}${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════════════════${NC}"

    local start_time=$(date +%s)

    # Generate unique job name with timestamp
    local job_name="${BUILDER}-${driver}-${test_id}-$(date +%s)"

    # Create log file FIRST (before yaml generation)
    local log_file="${SUITES_DIR}/test-${job_name}.log"
    mkdir -p "${SUITES_DIR}"
    touch "${log_file}"

    # Generate YAML
    local yaml_file=$(generate_job_yaml "${job_name}" "${driver}" "${args}" "${with_signing}")

    echo -e "${CYAN}  YAML file: ${yaml_file}${NC}"
    echo -e "${CYAN}  Log file: ${log_file}${NC}"
    echo -e "${CYAN}  Creating job: ${job_name}${NC}"

    echo ""
    # Apply the job
    local apply_result
    apply_result=$(kubectl apply -f "${yaml_file}" -n ${NAMESPACE} 2>&1)
    if [ $? -ne 0 ]; then
        echo -e "${RED}✗ FAIL${NC} (Failed to create job)"
        echo "=== Job Creation Failed ===" | tee -a "${log_file}"
        echo "${apply_result}" | tee -a "${log_file}"
        echo "" | tee -a "${log_file}"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TEST_RESULTS+=("FAIL: ${test_name} (${BUILDER}, rootless, ${driver}) - Job creation failed")
        return
    fi

    # Log successful job creation
    echo "=== Job Created Successfully ===" >> "${log_file}"
    echo "${apply_result}" >> "${log_file}"
    echo "" >> "${log_file}"
    sleep 1
    # Wait for pod to be created
    echo -e "${CYAN}  Waiting for pod...${NC}"
    local pod_name=""
    for i in {1..30}; do
        pod_name=$(kubectl get pods -n ${NAMESPACE} --selector=job-name=${job_name} -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
        if [ -n "$pod_name" ]; then
            break
        fi
        sleep 1
    done

    if [ -z "$pod_name" ]; then
        echo -e "${RED}✗ FAIL${NC} (Pod not created)"
        echo "=== Pod Creation Timeout ===" | tee -a "${log_file}"
        kubectl describe job ${job_name} -n ${NAMESPACE} | tee -a "${log_file}"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TEST_RESULTS+=("FAIL: ${test_name} (${BUILDER}, rootless, ${driver}) - Pod creation timeout")
        kubectl delete job ${job_name} -n ${NAMESPACE} --force --grace-period=0 &> /dev/null || true
        return
    fi

    echo -e "${CYAN}  Pod: ${pod_name}${NC}"

    # Wait for container to start or complete
    echo -e "${CYAN}  Waiting for container to start${NC}"
    local ready=false
    for i in {1..60}; do
        local phase=$(kubectl get pod ${pod_name} -n ${NAMESPACE} -o jsonpath='{.status.phase}' 2>/dev/null)
        local container_ready=$(kubectl get pod ${pod_name} -n ${NAMESPACE} -o jsonpath='{.status.containerStatuses[0].ready}' 2>/dev/null)

        # Container is ready and running
        if [ "$container_ready" = "true" ]; then
            ready=true
            echo ""  # New line after progress
            break
        fi

        # Job completed successfully (fast jobs go directly to Succeeded)
        if [ "$phase" = "Succeeded" ]; then
            ready=true
            echo ""  # New line after progress
            echo -e "${CYAN}  Container completed${NC}"
            break
        fi

        # Show progress dots
        if [ $((i % 5)) -eq 0 ]; then
            echo -n "."
        fi

        # Check for failure states
        if [ "$phase" = "Failed" ]; then
            echo ""
            echo -e "${YELLOW}  Container failed to start. Checking reason...${NC}"
            local reason=$(kubectl get pod ${pod_name} -n ${NAMESPACE} -o jsonpath='{.status.containerStatuses[0].state.waiting.reason}' 2>/dev/null)
            case "$reason" in
                ImagePullBackOff|ErrImagePull)
                    echo -e "${RED}  Error: Cannot pull image ${KIMIA_IMAGE}${NC}"
                    ;;
                *)
                    echo -e "${YELLOW}  Reason: ${reason}${NC}"
                    ;;
            esac
        fi

        sleep 1
    done
    echo ""  # New line after progress

    if [ "$ready" = false ]; then
        echo -e "${RED}✗ FAIL${NC} (Container not ready after 60s)"
        echo "=== Container Start Timeout ===" | tee -a "${log_file}"
        echo -e "${RED}  Pod description:${NC}"
        kubectl describe pod ${pod_name} -n ${NAMESPACE} | tee -a "${log_file}" | sed 's/^/    /'
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TEST_RESULTS+=("FAIL: ${test_name} (${BUILDER}, rootless, ${driver}) - Container start timeout")
        kubectl delete job ${job_name} -n ${NAMESPACE} --force --grace-period=0 &> /dev/null || true
        echo ""
        return
    fi

    echo -e "${CYAN}  Streaming logs...${NC}"

    # Check if pod already completed (for fast jobs)
    local current_phase=$(kubectl get pod ${pod_name} -n ${NAMESPACE} -o jsonpath='{.status.phase}' 2>/dev/null)

    if [ "$current_phase" = "Succeeded" ] || [ "$current_phase" = "Failed" ]; then
        # Job already completed, just get the logs
        kubectl logs ${pod_name} -n ${NAMESPACE} 2>&1 | tee -a "${log_file}" | sed 's/^/    /'
        local logs_pid=0
    else
        # Job still running, stream logs
        kubectl logs -f ${pod_name} -n ${NAMESPACE} 2>&1 | tee -a "${log_file}" | sed 's/^/    /' &
        local logs_pid=$!
    fi

    # Wait for job to complete or fail - poll job status
    local job_status=""
    local elapsed=0
    while [ $elapsed -lt ${JOB_TIMEOUT} ]; do
        # Check job status
        job_status=$(kubectl get job ${job_name} -n ${NAMESPACE} -o jsonpath='{.status.conditions[?(@.type=="Complete")].status}' 2>/dev/null)
        if [ "$job_status" = "True" ]; then
            # Job completed successfully
            break
        fi

        job_status=$(kubectl get job ${job_name} -n ${NAMESPACE} -o jsonpath='{.status.conditions[?(@.type=="Failed")].status}' 2>/dev/null)
        if [ "$job_status" = "True" ]; then
            # Job failed
            break
        fi

        sleep 2
        elapsed=$((elapsed + 2))
    done

    # Stop log streaming (if it's still running)
    if [ $logs_pid -ne 0 ]; then
        kill $logs_pid 2>/dev/null || true
        wait $logs_pid 2>/dev/null || true
    fi

    # Give logs a moment to flush
    sleep 1

    # Capture final logs to file
    kubectl logs ${pod_name} -n ${NAMESPACE} >> "${log_file}" 2>&1 || true

    local end_time=$(date +%s)
    local duration=$((end_time - start_time))

    # Check if job completed successfully
    job_status=$(kubectl get job ${job_name} -n ${NAMESPACE} -o jsonpath='{.status.conditions[?(@.type=="Complete")].status}' 2>/dev/null)
    if [ "$job_status" = "True" ]; then
        echo -e "${GREEN}✓ PASS${NC} (${duration}s)"
        echo "=== Test PASSED ===" >> "${log_file}"
        PASSED_TESTS=$((PASSED_TESTS + 1))
        TEST_RESULTS+=("PASS: ${test_name} (${BUILDER}, rootless, ${driver})")
    else
        echo -e "${RED}✗ FAIL${NC} (${duration}s)"
        echo "=== Test FAILED ===" >> "${log_file}"

        # Get job status and events
        echo "" >> "${log_file}"
        echo "=== Job Status ===" >> "${log_file}"
        kubectl get job ${job_name} -n ${NAMESPACE} -o yaml >> "${log_file}" 2>&1 || true
        echo "" >> "${log_file}"
        echo "=== Pod Description ===" >> "${log_file}"
        kubectl describe pod ${pod_name} -n ${NAMESPACE} >> "${log_file}" 2>&1 || true

        FAILED_TESTS=$((FAILED_TESTS + 1))
        TEST_RESULTS+=("FAIL: ${test_name} (${BUILDER}, rootless, ${driver})")

        echo -e "${RED}  Complete pod logs:${NC}"
        kubectl logs ${pod_name} -n ${NAMESPACE} 2>&1 | sed 's/^/    /' || true
        echo -e "${YELLOW}  Check log file: ${log_file}${NC}"
    fi

    # Cleanup job (but keep YAML file for debugging)
    echo -e "${CYAN}  Cleaning up job...${NC}"
    kubectl delete job ${job_name} -n ${NAMESPACE} --force --grace-period=0 &> /dev/null || true

    echo ""
}

# ============================================================================
# Rootless Mode Tests (ONLY mode supported in Kubernetes)
# ============================================================================

run_rootless_tests() {
    local driver="$1"

    # Get actual storage flag
    local storage_flag=$(get_storage_flag "$driver")

    # Convert to uppercase for display (POSIX compatible)
    local builder_upper=$(echo "$BUILDER" | awk '{print toupper($0)}')
    local driver_upper=$(echo "$driver" | awk '{print toupper($0)}')

    print_section "ROOTLESS MODE TESTS - ${builder_upper} with ${driver_upper} STORAGE"

    if [ "$driver" = "overlay" ]; then
        echo -e "${CYAN}Note: Overlay storage uses native kernel overlayfs (via user namespaces)${NC}"
        if [ "$BUILDER" = "buildkit" ]; then
            echo -e "${CYAN}      BuildKit: DAC_OVERRIDE + Unconfined seccomp/AppArmor${NC}"
        else
            echo -e "${CYAN}      Buildah: MKNOD + Unconfined seccomp/AppArmor + emptyDir at /home/kimia/.local${NC}"
            echo -e "${CYAN}      (emptyDir breaks nested overlayfs: container root = overlay)${NC}"
        fi
        echo ""
    elif [ "$driver" = "native" ]; then
        echo -e "${CYAN}Note: Native snapshotter (BuildKit) - secure and performant${NC}"
        echo -e "${CYAN}      Requires Unconfined seccomp/AppArmor for mount syscalls${NC}"
        echo ""
    elif [ "$driver" = "vfs" ]; then
        echo -e "${CYAN}Note: VFS storage (Buildah) - most secure but slower${NC}"
        echo ""
    fi

    # ========================================================================
    # SIMPLE TESTS (Tests 1-7)
    # ========================================================================

    if should_run_simple; then
        # Test 1: Version check
        run_k8s_test \
            "Version Check" \
            "$driver" \
            "[\"--version\"]" \
            "version"

        # Test 2: Environment check
        run_k8s_test \
            "Environment Check" \
            "$driver" \
            "[\"check-environment\"]" \
            "envcheck"

        # Test 3: Basic build from Git (no push)
        run_k8s_test \
            "Git Repository Build" \
            "$driver" \
            "[
                \"--context=https://github.com/nginxinc/docker-nginx.git\",
                \"--git-branch=master\",
                \"--dockerfile=mainline/alpine/Dockerfile\",
                \"--destination=test-${BUILDER}-k8s-rootless-git-${driver}:latest\",
                \"--storage-driver=${storage_flag}\",
                \"--no-push\",
                \"--verbosity=debug\"
            ]" \
            "git-build"

        # Test 4: Build with arguments from Git (no push)
        run_k8s_test \
            "Build with Arguments" \
            "$driver" \
            "[
                \"--context=https://github.com/nginxinc/docker-nginx.git\",
                \"--git-branch=master\",
                \"--dockerfile=mainline/alpine/Dockerfile\",
                \"--destination=test-${BUILDER}-k8s-rootless-buildargs-${driver}:latest\",
                \"--build-arg=NGINX_VERSION=1.25\",
                \"--storage-driver=${storage_flag}\",
                \"--no-push\",
                \"--verbosity=debug\"
            ]" \
            "buildargs"

        # Test 5: Git build WITH push to registry
        run_k8s_test \
            "Git Repository Build (Push)" \
            "$driver" \
            "[
                \"--context=https://github.com/nginxinc/docker-nginx.git\",
                \"--git-branch=master\",
                \"--dockerfile=mainline/alpine/Dockerfile\",
                \"--destination=${REGISTRY}/${BUILDER}-k8s-rootless-git-${driver}:latest\",
                \"--storage-driver=${storage_flag}\",
                \"--insecure\",
                \"--verbosity=debug\"
            ]" \
            "git-build-push"

        # Test 6: Build with args AND push to registry
        run_k8s_test \
            "Build with Arguments (Push)" \
            "$driver" \
            "[
                \"--context=https://github.com/nginxinc/docker-nginx.git\",
                \"--git-branch=master\",
                \"--dockerfile=mainline/alpine/Dockerfile\",
                \"--destination=${REGISTRY}/${BUILDER}-k8s-rootless-buildargs-${driver}:latest\",
                \"--build-arg=NGINX_VERSION=1.25\",
                \"--storage-driver=${storage_flag}\",
                \"--insecure\",
                \"--verbosity=debug\"
            ]" \
            "buildargs-push"

        # Test 7: Build with context-sub-path and push to registry
        run_k8s_test \
            "Build with Context Sub-path (Push)" \
            "$driver" \
            "[
                \"--context=https://github.com/docker-library/postgres.git\",
                \"--context-sub-path=18/alpine3.22\",
                \"--dockerfile=Dockerfile\",
                \"--destination=${REGISTRY}/${BUILDER}-k8s-postgres-buildargs-${driver}:latest\",
                \"--storage-driver=${storage_flag}\",
                \"--insecure\",
                \"--verbosity=debug\"
            ]" \
            "context-subpath-push"
    fi

    # ========================================================================
    # REPRODUCIBLE BUILD TESTS (Test 8)
    # ========================================================================

    if should_run_reproducible; then
        local test_image="${REGISTRY}/${BUILDER}-k8s-reproducible-test-${driver}"

        run_k8s_test \
            "Reproducible Build #1" \
            "$driver" \
            "[
                \"--context=https://github.com/rapidfort/kimia.git\",
                \"--git-branch=main\",
                \"--dockerfile=tests/examples/Dockerfile\",
                \"--destination=${test_image}:v1\",
                \"--storage-driver=${storage_flag}\",
                \"--reproducible\",
                \"--insecure\",
                \"--verbosity=debug\"
            ]" \
            "reproducible-build1"

        echo "Waiting 5 seconds before second build..."
        sleep 5

        docker pull ${test_image}:v1 || true

        # Extract digest from first build
        local digest1=$(docker inspect ${test_image}:v1 --format='{{index .RepoDigests 0}}' 2>/dev/null | cut -d'@' -f2)
        if [ -z "$digest1" ]; then
            echo "Warning: Could not extract digest from first build"
            digest1="none"
        fi
        echo "First build digest: ${digest1}"

        run_k8s_test \
            "Reproducible Build #2" \
            "$driver" \
            "[
                \"--context=https://github.com/rapidfort/kimia.git\",
                \"--git-branch=main\",
                \"--dockerfile=tests/examples/Dockerfile\",
                \"--destination=${test_image}:v1\",
                \"--storage-driver=${storage_flag}\",
                \"--reproducible\",
                \"--insecure\",
                \"--verbosity=debug\"
            ]" \
            "reproducible-build2"

        sleep 5
        docker pull ${test_image}:v1 || true

        # Extract digest from second build
        local digest2=$(docker inspect ${test_image}:v1 --format='{{index .RepoDigests 0}}' 2>/dev/null | cut -d'@' -f2)
        if [ -z "$digest2" ]; then
            echo "Warning: Could not extract digest from second build"
            digest2="none"
        fi
        echo "Second build digest: ${digest2}"

        # Compare digests
        echo ""
        echo -e "${CYAN}═══════════════════════════════════════════════════════${NC}"
        echo -e "${CYAN}  REPRODUCIBILITY RESULTS ${NC}"
        echo -e "${CYAN}  Build #1 digest: ${digest1} ${NC}"
        echo -e "${CYAN}  Build #2 digest: ${digest2} ${NC}"
        echo -e "${CYAN}═══════════════════════════════════════════════════════${NC}"
        echo ""

        TOTAL_TESTS=$((TOTAL_TESTS + 1))
        if [ "$digest1" = "$digest2" ] && [ "$digest1" != "none" ]; then
            echo -e "${GREEN}✓ SUCCESS: Builds are reproducible!${NC}"
            PASSED_TESTS=$((PASSED_TESTS + 1))
            TEST_RESULTS+=("PASS: reproducible-comparison (${BUILDER}, rootless, ${driver})")
        else
            echo -e "${RED}✗ FAILURE: Builds are NOT reproducible!${NC}"
            FAILED_TESTS=$((FAILED_TESTS + 1))
            TEST_RESULTS+=("FAIL: reproducible-comparison (${BUILDER}, rootless, ${driver})")
        fi
    fi

    # ========================================================================
    # ATTESTATION TESTS (Tests 9-11, BuildKit only)
    # ========================================================================

    if should_run_attestation; then
        # Test 9: Attestation - default mode (should default to min)
        run_k8s_test \
            "Attestation - Default (min)" \
            "$driver" \
            "[
                \"--context=https://github.com/rapidfort/kimia.git\",
                \"--git-branch=main\",
                \"--dockerfile=tests/examples/Dockerfile\",
                \"--destination=${REGISTRY}/${BUILDER}-k8s-attest-default-${driver}:latest\",
                \"--attestation\",
                \"--storage-driver=${storage_flag}\",
                \"--insecure\",
                \"--verbosity=debug\"
            ]" \
            "attestation-default"

        # Test 10: Attestation - explicit min (provenance only)
        run_k8s_test \
            "Attestation - Min (provenance)" \
            "$driver" \
            "[
                \"--context=https://github.com/rapidfort/kimia.git\",
                \"--git-branch=main\",
                \"--dockerfile=tests/examples/Dockerfile\",
                \"--destination=${REGISTRY}/${BUILDER}-k8s-attest-min-${driver}:latest\",
                \"--attestation=min\",
                \"--storage-driver=${storage_flag}\",
                \"--insecure\",
                \"--verbosity=debug\"
            ]" \
            "attestation-min"

        # Test 11: Attestation - max (SBOM + provenance)
        run_k8s_test \
            "Attestation - Max (SBOM+provenance)" \
            "$driver" \
            "[
                \"--context=https://github.com/rapidfort/kimia.git\",
                \"--git-branch=main\",
                \"--dockerfile=tests/examples/Dockerfile\",
                \"--destination=${REGISTRY}/${BUILDER}-k8s-attest-max-${driver}:latest\",
                \"--attestation=max\",
                \"--storage-driver=${storage_flag}\",
                \"--insecure\",
                \"--verbosity=debug\"
            ]" \
            "attestation-max"

        # ====================================================================
        # NEW ATTESTATION TESTS - 3-Level System
        # ====================================================================

        # Test 12: Attestation - explicit off (no attestations)
        run_k8s_test \
            "Attestation - Off (none)" \
            "$driver" \
            "[
                \"--context=https://github.com/rapidfort/kimia.git\",
                \"--git-branch=main\",
                \"--dockerfile=tests/examples/Dockerfile\",
                \"--destination=${REGISTRY}/${BUILDER}-k8s-attest-off-${driver}:latest\",
                \"--attestation=off\",
                \"--storage-driver=${storage_flag}\",
                \"--insecure\",
                \"--verbosity=debug\"
            ]" \
            "attestation-off"

        # Test 13: Docker-style - SBOM only
        run_k8s_test \
            "Attest - SBOM only" \
            "$driver" \
            "[
                \"--context=https://github.com/rapidfort/kimia.git\",
                \"--git-branch=main\",
                \"--dockerfile=tests/examples/Dockerfile\",
                \"--destination=${REGISTRY}/${BUILDER}-k8s-sbom-only-${driver}:latest\",
                \"--attest\",
                \"type=sbom\",
                \"--storage-driver=${storage_flag}\",
                \"--insecure\",
                \"--verbosity=debug\"
            ]" \
            "attest-sbom-only"

        # Test 14: Docker-style - Provenance only
        run_k8s_test \
            "Attest - Provenance only" \
            "$driver" \
            "[
                \"--context=https://github.com/rapidfort/kimia.git\",
                \"--git-branch=main\",
                \"--dockerfile=tests/examples/Dockerfile\",
                \"--destination=${REGISTRY}/${BUILDER}-k8s-prov-only-${driver}:latest\",
                \"--attest\",
                \"type=provenance,mode=max\",
                \"--storage-driver=${storage_flag}\",
                \"--insecure\",
                \"--verbosity=debug\"
            ]" \
            "attest-prov-only"

        # Test 15: Docker-style - SBOM with scan options
        run_k8s_test \
            "Attest - SBOM with scan" \
            "$driver" \
            "[
                \"--context=https://github.com/rapidfort/kimia.git\",
                \"--git-branch=main\",
                \"--dockerfile=tests/examples/Dockerfile\",
                \"--destination=${REGISTRY}/${BUILDER}-k8s-sbom-scan-${driver}:latest\",
                \"--attest\",
                \"type=sbom,scan-stage=true\",
                \"--storage-driver=${storage_flag}\",
                \"--insecure\",
                \"--verbosity=debug\"
            ]" \
            "attest-sbom-scan"

        # Test 16: Docker-style - Provenance with builder-id
        run_k8s_test \
            "Attest - Provenance with builder-id" \
            "$driver" \
            "[
                \"--context=https://github.com/rapidfort/kimia.git\",
                \"--git-branch=main\",
                \"--dockerfile=tests/examples/Dockerfile\",
                \"--destination=${REGISTRY}/${BUILDER}-k8s-prov-builderid-${driver}:latest\",
                \"--attest\",
                \"type=provenance,mode=max,builder-id=https://github.com/rapidfort/kimia/actions/runs/test\",
                \"--storage-driver=${storage_flag}\",
                \"--insecure\",
                \"--verbosity=debug\"
            ]" \
            "attest-prov-builderid"

        # Test 17: Docker-style - Both SBOM and Provenance
        run_k8s_test \
            "Attest - Both (SBOM+Prov)" \
            "$driver" \
            "[
                \"--context=https://github.com/rapidfort/kimia.git\",
                \"--git-branch=main\",
                \"--dockerfile=tests/examples/Dockerfile\",
                \"--destination=${REGISTRY}/${BUILDER}-k8s-both-${driver}:latest\",
                \"--attest\",
                \"type=sbom,scan-stage=true\",
                \"--attest\",
                \"type=provenance,mode=max,reproducible=true\",
                \"--storage-driver=${storage_flag}\",
                \"--insecure\",
                \"--verbosity=debug\"
            ]" \
            "attest-both"

        # Test 18: Pass-through - BuildKit option
        run_k8s_test \
            "BuildKit-opt - Pass-through" \
            "$driver" \
            "[
                \"--context=https://github.com/rapidfort/kimia.git\",
                \"--git-branch=main\",
                \"--dockerfile=tests/examples/Dockerfile\",
                \"--destination=${REGISTRY}/${BUILDER}-k8s-passthrough-${driver}:latest\",
                \"--attest\",
                \"type=sbom\",
                \"--buildkit-opt\",
                \"attest:provenance=mode=min\",
                \"--storage-driver=${storage_flag}\",
                \"--insecure\",
                \"--verbosity=debug\"
            ]" \
            "buildkit-opt-passthrough"
    fi

    # ========================================================================
    # SIGNING TESTS (Test 19, BuildKit only)
    # ========================================================================

    if should_run_signing; then
        # Test 19: Signing with attestation (requires cosign secrets)
        if kubectl get secret cosign-key -n ${NAMESPACE} &>/dev/null && \
           kubectl get secret cosign-password -n ${NAMESPACE} &>/dev/null; then
            run_k8s_test \
                "Attestation + Signing" \
                "$driver" \
                "[
                    \"--context=https://github.com/rapidfort/kimia.git\",
                    \"--git-branch=main\",
                    \"--dockerfile=tests/examples/Dockerfile\",
                    \"--destination=${REGISTRY}/${BUILDER}-k8s-attest-sign-${driver}:latest\",
                    \"--attestation=max\",
                    \"--sign\",
                    \"--cosign-key=/tmp/cosign/cosign.key\",
                    \"--cosign-password-env=COSIGN_PASSWORD\",
                    \"--storage-driver=${storage_flag}\",
                    \"--insecure\",
                    \"--verbosity=debug\"
                ]" \
                "attestation-sign" \
                "true"
        else
            echo -e "${YELLOW}Skipping signing test: cosign secrets not found${NC}"
            echo -e "${YELLOW}Create secrets with: kubectl create secret generic cosign-key ...${NC}"
        fi
    fi
}

# ============================================================================
# Cleanup Function
# ============================================================================

cleanup_on_interrupt() {
    echo ""
    echo -e "${YELLOW}Interrupted by user (Ctrl+C)${NC}"
    echo -e "${YELLOW}Stopping tests and cleaning up...${NC}"

    # Delete any running jobs in the namespace
    echo "Deleting any running jobs..."
    kubectl delete jobs -n ${NAMESPACE} --all --force --grace-period=0 &>/dev/null || true

    # Print partial results if any tests were run
    if [ ${TOTAL_TESTS} -gt 0 ]; then
        print_test_summary
    fi

    # Optionally cleanup namespace
    if [ "$CLEANUP_AFTER" = true ]; then
        echo "Deleting namespace: ${NAMESPACE}"
        kubectl delete namespace ${NAMESPACE} --force --grace-period=0 &>/dev/null || true
    fi

    echo -e "${GREEN}Cleanup completed${NC}"
    exit 130  # Standard exit code for SIGINT
}

cleanup() {
    if [ "$CLEANUP_AFTER" = true ]; then
        print_section "CLEANUP"

        echo "Deleting namespace: ${NAMESPACE}"
        kubectl delete namespace ${NAMESPACE} --force --grace-period=0 &> /dev/null || true

        echo -e "${GREEN}✓ Cleanup completed${NC}"
    fi
}

# ============================================================================
# Test Summary
# ============================================================================

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
# Main
# ============================================================================

main() {
    local start_time=$(date +%s)

    print_section "KIMIA KUBERNETES TEST SUITE"

    echo -e "${CYAN}Configuration:${NC}"
    echo "  Builder:      $BUILDER"
    echo "  Registry:     $REGISTRY"
    echo "  Image:        $KIMIA_IMAGE"
    echo "  Namespace:    $NAMESPACE"
    echo "  Storage:      $STORAGE_DRIVER"
    echo "  Test Suite:   $TEST_SUITE"
    echo ""

    # Display test suite information
    case $TEST_SUITE in
        simple)
            echo -e "${CYAN}Running: Simple Tests (7 tests)${NC}"
            echo "  - Version and environment checks"
            echo "  - Basic Git builds with/without push"
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
                echo "  - Simple (7 tests)"
                echo "  - Reproducible (3 tests)"
                echo "  - Attestation (3 tests)"
                echo "  - Signing (1 test if cosign secrets available)"
            else
                echo "  - Simple (7 tests)"
                echo "  - Reproducible (3 tests)"
                echo "  - Attestation and Signing skipped (BuildKit only)"
            fi
            ;;
    esac
    echo ""

    # Setup namespace
    setup_namespace

    # Setup cosign secrets for signing tests
    setup_cosign_secrets

    # Registry authentication setup
    setup_registry_auth_secret

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
