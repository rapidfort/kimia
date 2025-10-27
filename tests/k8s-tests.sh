#!/bin/bash
# Smithy Kubernetes Test Suite
# Tests ROOTLESS mode (UID 1000) ONLY
# Smithy is designed for rootless operation in all environments
# Supports BuildKit (default) and Buildah (legacy) images
# Tests storage drivers based on builder:
#   - BuildKit: native (default), overlay
#   - Buildah: vfs (default), overlay (requires emptyDir at /home/smithy/.local)
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

SMITHY_IMAGE=${SMITHY_IMAGE:-"${REGISTRY}/rapidfort/smithy:latest"}
NAMESPACE=${NAMESPACE:-"smithy-tests"}
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

# Timeout settings
JOB_TIMEOUT=600  # 10 minutes

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
    echo -e "${BLUE}══════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}══════════════════════════════════════════════════════════${NC}"
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

    echo -e "${GREEN}✓ Namespace ready${NC}"
}

# ============================================================================
# Job YAML Generation (Rootless with storage-specific capabilities)
# ============================================================================

generate_job_yaml() {
    local job_name="$1"
    local driver="$2"
    local args="$3"

    # Get actual storage flag
    local storage_flag=$(get_storage_flag "$driver")

    # Use job name for YAML file: buildah-overlay-git-build-1234567890.yaml
    local yaml_file="${SUITES_DIR}/${job_name}.yaml"

    # Set capabilities based on storage driver and builder
    local caps_add="[SETUID, SETGID]"
    local pod_seccomp=""
    local pod_apparmor=""
    local container_seccomp=""
    local container_apparmor=""
    local volume_mounts=""
    local volumes=""
    local has_volumes=false

    # CRITICAL FIX: BuildKit ALWAYS needs Unconfined seccomp + AppArmor for mount syscalls
    # This applies to BOTH native and overlay storage
    if [ "$BUILDER" = "buildkit" ]; then
        pod_seccomp="seccompProfile:
          type: Unconfined"
        container_seccomp="seccompProfile:
            type: Unconfined"
        # AppArmor also blocks mount syscalls - must be Unconfined
        pod_apparmor="appArmorProfile:
          type: Unconfined"
        container_apparmor="appArmorProfile:
            type: Unconfined"
    fi

    # Overlay storage needs additional configuration
    if [ "$driver" = "overlay" ]; then
        # Add MKNOD and DAC_OVERRIDE for overlay
        caps_add="[SETUID, SETGID, MKNOD, DAC_OVERRIDE]"

        # Buildah with overlay also needs Unconfined seccomp + AppArmor
        if [ "$BUILDER" = "buildah" ]; then
            pod_seccomp="seccompProfile:
          type: Unconfined"
            container_seccomp="seccompProfile:
            type: Unconfined"
            pod_apparmor="appArmorProfile:
          type: Unconfined"
            container_apparmor="appArmorProfile:
            type: Unconfined"

            # CRITICAL: Buildah overlay needs emptyDir at /home/smithy/.local
            # Why: Cannot nest kernel overlayfs on top of kernel overlayfs (container root)
            # Container root = kernel overlayfs, storage needs kernel overlayfs = NESTED = FAILS
            # Solution: Mount emptyDir (tmpfs) to break the nesting
            # This allows: tmpfs → kernel overlayfs ✓ (instead of overlayfs → overlayfs ✗)
            # Note: Uses native kernel overlayfs via user namespaces (no fuse-overlayfs needed)
            volume_mounts="
        - name: smithy-local
          mountPath: /home/smithy/.local"
            volumes="
      - name: smithy-local
        emptyDir: {}"
            has_volumes=true
        fi
        # Note: BuildKit overlay doesn't need any volumes (no nesting issue)
    fi

    # Build the YAML - conditionally include volumeMounts and volumes
    if [ "$has_volumes" = true ]; then
        cat > "$yaml_file" <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: ${job_name}
  namespace: ${NAMESPACE}
  labels:
    app: smithy-test
    builder: ${BUILDER}
    mode: rootless
    driver: ${driver}
spec:
  backoffLimit: 0
  ttlSecondsAfterFinished: 300
  template:
    metadata:
      labels:
        app: smithy-test
        builder: ${BUILDER}
        mode: rootless
        driver: ${driver}
    spec:
      restartPolicy: Never
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        runAsGroup: 1000
        fsGroup: 1000
        ${pod_seccomp}
        ${pod_apparmor}
      containers:
      - name: smithy
        image: ${SMITHY_IMAGE}
        args: ${args}
        env:
        - name: SMITHY_USER
          value: "smithy"
        - name: STORAGE_DRIVER
          value: "${storage_flag}"
        securityContext:
          allowPrivilegeEscalation: true
          capabilities:
            add: ${caps_add}
            drop: [ALL]
          runAsUser: 1000
          runAsGroup: 1000
          ${container_seccomp}
          ${container_apparmor}
        volumeMounts:${volume_mounts}
      volumes:${volumes}
EOF
    else
        cat > "$yaml_file" <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: ${job_name}
  namespace: ${NAMESPACE}
  labels:
    app: smithy-test
    builder: ${BUILDER}
    mode: rootless
    driver: ${driver}
spec:
  backoffLimit: 0
  ttlSecondsAfterFinished: 300
  template:
    metadata:
      labels:
        app: smithy-test
        builder: ${BUILDER}
        mode: rootless
        driver: ${driver}
    spec:
      restartPolicy: Never
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        runAsGroup: 1000
        fsGroup: 1000
        ${pod_seccomp}
        ${pod_apparmor}
      containers:
      - name: smithy
        image: ${SMITHY_IMAGE}
        args: ${args}
        env:
        - name: SMITHY_USER
          value: "smithy"
        - name: STORAGE_DRIVER
          value: "${storage_flag}"
        securityContext:
          allowPrivilegeEscalation: true
          capabilities:
            add: ${caps_add}
            drop: [ALL]
          runAsUser: 1000
          runAsGroup: 1000
          ${container_seccomp}
          ${container_apparmor}
EOF
    fi

    echo "$yaml_file"
}

# ============================================================================
# Run Kubernetes Test
# ============================================================================
run_k8s_test() {
    local test_name="$1"
    local driver="$2"
    local args="$3"
    local test_slug="$4"  # Short identifier for the test type

    TOTAL_TESTS=$((TOTAL_TESTS + 1))

    # Get actual storage flag
    local storage_flag=$(get_storage_flag "$driver")

    echo -e "${CYAN}[TEST $TOTAL_TESTS]${NC} ${test_name} (${BUILDER}, rootless, ${driver})"

    # Generate meaningful job name: buildah-overlay-git-build-1234567890
    local timestamp=$(date +%s)
    local job_name="${BUILDER}-${driver}-${test_slug}-${timestamp}"
    local yaml_file=$(generate_job_yaml "$job_name" "$driver" "$args")

    echo -e "${CYAN}  Job: ${job_name}${NC}"
    echo -e "${CYAN}  YAML: $(basename $yaml_file)${NC}"

    local start_time=$(date +%s)

    # Create job
    if ! kubectl apply -f "$yaml_file" > /dev/null 2>&1; then
        echo -e "${RED}✗ FAIL${NC} (Failed to create job)"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TEST_RESULTS+=("FAIL: ${test_name} (${BUILDER}, rootless, ${driver})")
        echo ""
        return
    fi

    # Wait for pod to be created
    local pod_name=""
    local max_wait=30
    local elapsed=0

    while [ -z "$pod_name" ] && [ $elapsed -lt $max_wait ]; do
        pod_name=$(kubectl get pods -n ${NAMESPACE} -l job-name=${job_name} -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
        if [ -z "$pod_name" ]; then
            sleep 1
            elapsed=$((elapsed + 1))
        fi
    done

    if [ -z "$pod_name" ]; then
        echo -e "${RED}✗ FAIL${NC} (Pod not created after ${max_wait}s)"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TEST_RESULTS+=("FAIL: ${test_name} (${BUILDER}, rootless, ${driver}) - Pod creation timeout")
        kubectl delete job ${job_name} -n ${NAMESPACE} --force --grace-period=0 &> /dev/null || true
        echo ""
        return
    fi

    echo -e "${CYAN}  Pod: ${pod_name}${NC}"

    # Wait for pod to be in a state where we can get logs
    # Either Running, Succeeded, or Failed
    echo -e "${CYAN}  Waiting for container...${NC}"
    local ready=false
    for i in {1..60}; do
        local phase=$(kubectl get pod ${pod_name} -n ${NAMESPACE} -o jsonpath='{.status.phase}' 2>/dev/null)
        local container_state=$(kubectl get pod ${pod_name} -n ${NAMESPACE} -o jsonpath='{.status.containerStatuses[0].state}' 2>/dev/null)
        
        # Check if we can get logs
        if [[ "$phase" == "Running" ]] || [[ "$phase" == "Succeeded" ]] || [[ "$phase" == "Failed" ]]; then
            ready=true
            echo -e "${CYAN}  Container ready (phase: ${phase}, ${i}s)${NC}"
            break
        fi
        
        # Check for immediate failure conditions
        if [[ "$container_state" == *"waiting"* ]]; then
            local reason=$(kubectl get pod ${pod_name} -n ${NAMESPACE} -o jsonpath='{.status.containerStatuses[0].state.waiting.reason}' 2>/dev/null)
            local message=$(kubectl get pod ${pod_name} -n ${NAMESPACE} -o jsonpath='{.status.containerStatuses[0].state.waiting.message}' 2>/dev/null)
            
            # Fail immediately on these errors
            case "$reason" in
                ErrImagePull|ImagePullBackOff)
                    echo ""
                    echo -e "${RED}✗ FAIL${NC} (Image pull failed: ${reason})"
                    echo -e "${RED}  Error: ${message}${NC}"
                    echo -e "${RED}  Pod events:${NC}"
                    kubectl get events -n ${NAMESPACE} --field-selector involvedObject.name=${pod_name} --sort-by='.lastTimestamp' | tail -5 | sed 's/^/    /'
                    FAILED_TESTS=$((FAILED_TESTS + 1))
                    TEST_RESULTS+=("FAIL: ${test_name} (${BUILDER}, rootless, ${driver}) - ${reason}")
                    kubectl delete job ${job_name} -n ${NAMESPACE} --force --grace-period=0 &> /dev/null || true
                    echo ""
                    return
                    ;;
                CrashLoopBackOff)
                    echo ""
                    echo -e "${RED}✗ FAIL${NC} (Container crash loop: ${reason})"
                    echo -e "${RED}  Error: ${message}${NC}"
                    echo -e "${RED}  Pod logs:${NC}"
                    kubectl logs ${pod_name} -n ${NAMESPACE} 2>&1 | sed 's/^/    /' || true
                    FAILED_TESTS=$((FAILED_TESTS + 1))
                    TEST_RESULTS+=("FAIL: ${test_name} (${BUILDER}, rootless, ${driver}) - ${reason}")
                    kubectl delete job ${job_name} -n ${NAMESPACE} --force --grace-period=0 &> /dev/null || true
                    echo ""
                    return
                    ;;
                CreateContainerConfigError|CreateContainerError|InvalidImageName)
                    echo ""
                    echo -e "${RED}✗ FAIL${NC} (Container config error: ${reason})"
                    echo -e "${RED}  Error: ${message}${NC}"
                    FAILED_TESTS=$((FAILED_TESTS + 1))
                    TEST_RESULTS+=("FAIL: ${test_name} (${BUILDER}, rootless, ${driver}) - ${reason}")
                    kubectl delete job ${job_name} -n ${NAMESPACE} --force --grace-period=0 &> /dev/null || true
                    echo ""
                    return
                    ;;
                ContainerCreating|PodInitializing)
                    # Normal startup - show progress
                    echo -ne "\r${CYAN}  ${reason}... (${i}s)${NC}"
                    ;;
            esac
        fi
        
        sleep 1
    done
    echo ""  # New line after progress

    if [ "$ready" = false ]; then
        echo -e "${RED}✗ FAIL${NC} (Container not ready after 60s)"
        echo -e "${RED}  Pod description:${NC}"
        kubectl describe pod ${pod_name} -n ${NAMESPACE} | sed 's/^/    /'
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TEST_RESULTS+=("FAIL: ${test_name} (${BUILDER}, rootless, ${driver}) - Container start timeout")
        kubectl delete job ${job_name} -n ${NAMESPACE} --force --grace-period=0 &> /dev/null || true
        echo ""
        return
    fi

    echo -e "${CYAN}  Streaming logs...${NC}"

    # Stream logs in background
    kubectl logs -f ${pod_name} -n ${NAMESPACE} 2>&1 | sed 's/^/    /' &
    local logs_pid=$!

    # Wait for job to complete
    if kubectl wait --for=condition=complete job/${job_name} -n ${NAMESPACE} --timeout=${JOB_TIMEOUT}s &> /dev/null; then
        wait $logs_pid 2>/dev/null || true

        local end_time=$(date +%s)
        local duration=$((end_time - start_time))

        echo -e "${GREEN}✓ PASS${NC} (${duration}s)"
        PASSED_TESTS=$((PASSED_TESTS + 1))
        TEST_RESULTS+=("PASS: ${test_name} (${BUILDER}, rootless, ${driver})")
    else
        kill $logs_pid 2>/dev/null || true
        wait $logs_pid 2>/dev/null || true

        local end_time=$(date +%s)
        local duration=$((end_time - start_time))

        echo -e "${RED}✗ FAIL${NC} (${duration}s)"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TEST_RESULTS+=("FAIL: ${test_name} (${BUILDER}, rootless, ${driver})")

        echo -e "${RED}  Complete pod logs:${NC}"
        kubectl logs ${pod_name} -n ${NAMESPACE} 2>&1 | sed 's/^/    /' || true
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

    print_section "ROOTLESS MODE TESTS - ${BUILDER^^} with ${driver^^} STORAGE"

    if [ "$driver" = "overlay" ]; then
        echo -e "${CYAN}Note: Overlay storage uses native kernel overlayfs (via user namespaces)${NC}"
        if [ "$BUILDER" = "buildkit" ]; then
            echo -e "${CYAN}      BuildKit: DAC_OVERRIDE + Unconfined seccomp/AppArmor${NC}"
        else
            echo -e "${CYAN}      Buildah: MKNOD + Unconfined seccomp/AppArmor + emptyDir at /home/smithy/.local${NC}"
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

    # Test 3: Basic build from Git
    run_k8s_test \
        "Git Repository Build" \
        "$driver" \
        "[\"--context=https://github.com/nginxinc/docker-nginx.git\", \"--git-branch=master\", \"--dockerfile=mainline/alpine/Dockerfile\", \"--destination=test-${BUILDER}-k8s-rootless-git-${driver}:latest\", \"--storage-driver=${storage_flag}\", \"--no-push\", \"--verbosity=debug\"]" \
        "git-build"

    # Test 4: Build with arguments from Git
    run_k8s_test \
        "Build with Arguments" \
        "$driver" \
        "[\"--context=https://github.com/nginxinc/docker-nginx.git\", \"--git-branch=master\", \"--dockerfile=mainline/alpine/Dockerfile\", \"--destination=test-${BUILDER}-k8s-rootless-buildargs-${driver}:latest\", \"--build-arg=NGINX_VERSION=1.25\", \"--storage-driver=${storage_flag}\", \"--no-push\", \"--verbosity=debug\"]" \
        "buildargs"


    # Test 5: Git build WITH push to registry
    run_k8s_test \
        "Git Repository Build (Push)" \
        "$driver" \
        "[\"--context=https://github.com/nginxinc/docker-nginx.git\", \
        \"--git-branch=master\", \
        \"--dockerfile=mainline/alpine/Dockerfile\", \
        \"--destination=${REGISTRY}/${BUILDER}-k8s-rootless-git-${driver}:latest\", \
        \"--storage-driver=${storage_flag}\", \
        \"--insecure\", \
        \"--verbosity=debug\"]" \
        "git-build-push"

    # Test 6: Build with args AND push to registry
    run_k8s_test \
        "Build with Arguments (Push)" \
        "$driver" \
        "[\"--context=https://github.com/nginxinc/docker-nginx.git\", \
        \"--git-branch=master\", \
        \"--dockerfile=mainline/alpine/Dockerfile\", \
        \"--destination=${REGISTRY}/${BUILDER}-k8s-rootless-buildargs-${driver}:latest\", \
        \"--build-arg=NGINX_VERSION=1.25\", \
        \"--storage-driver=${storage_flag}\", \
        \"--insecure\", \
        \"--verbosity=debug\"]" \
        "buildargs-push"

    # Test 7: Reproducible builds
    local test_image="${REGISTRY}/${BUILDER}-k8s-reproducible-test-${driver}"
    
    run_k8s_test \
        "Reproducible Build #1" \
        "$driver" \
        "[\"--context=https://github.com/rapidfort/smithy.git\", \
        \"--git-branch=main\", \
        \"--dockerfile=tests/examples/Dockerfile\", \
        \"--destination=${test_image}:v1\", \
        \"--storage-driver=${storage_flag}\", \
        \"--reproducible\", \
        \"--insecure\", \
        \"--verbosity=debug\"]" \
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
        "[\"--context=https://github.com/rapidfort/smithy.git\", \
        \"--git-branch=main\", \
        \"--dockerfile=tests/examples/Dockerfile\", \
        \"--destination=${test_image}:v1\", \
        \"--storage-driver=${storage_flag}\", \
        \"--reproducible\", \
        \"--insecure\", \
        \"--verbosity=debug\"]" \
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
    echo -e "\${CYAN}  REPRODUCIBILITY RESULTS ${NC}"
    echo -e "\${CYAN}  Build #1 digest: ${digest1} ${NC}"
    echo -e "\${CYAN}  Build #2 digest: ${digest2} ${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════════════════${NC}"
    echo ""
}

# ============================================================================
# Cleanup Function
# ============================================================================

cleanup() {
    if [ "$CLEANUP_AFTER" = true ]; then
        print_section "CLEANUP"

        echo "Deleting namespace: ${NAMESPACE}"
        kubectl delete namespace ${NAMESPACE} --force --grace-period=0 &> /dev/null || true

        echo -e "${GREEN}✓ Cleanup completed${NC}"
    fi
}

cleanup_on_interrupt() {
    echo ""
    echo -e "${YELLOW}Interrupted by user (Ctrl+C)${NC}"
    echo -e "${YELLOW}Cleaning up...${NC}"

    kubectl delete jobs -n ${NAMESPACE} -l app=smithy-test --force --grace-period=0 &> /dev/null || true
    kubectl delete namespace ${NAMESPACE} --force --grace-period=0 &> /dev/null || true

    echo -e "${GREEN}✓ Cleanup completed${NC}"
    exit 130
}

# ============================================================================
# Main Execution
# ============================================================================

main() {
    print_section "KUBERNETES TEST SUITE (ROOTLESS ONLY)"

    echo -e "${CYAN}Configuration:${NC}"
    echo -e "  Builder:        ${BUILDER}"
    echo -e "  Registry:       ${REGISTRY}"
    echo -e "  Image:          ${SMITHY_IMAGE}"
    echo -e "  Namespace:      ${NAMESPACE}"
    echo -e "  Storage:        ${STORAGE_DRIVER}"
    echo -e "  Cleanup:        ${CLEANUP_AFTER}"
    echo -e "  Job Timeout:    ${JOB_TIMEOUT}s"
    echo -e "  Suites Dir:     ${SUITES_DIR}"
    echo ""
    echo -e "${YELLOW}NOTE: Smithy runs in ROOTLESS mode only (UID 1000)${NC}"
    echo ""

    # Describe storage mappings
    echo -e "${CYAN}Storage Driver Mappings:${NC}"
    if [ "$BUILDER" = "buildkit" ]; then
        echo -e "  native  → Native snapshotter (default for BuildKit)"
        echo -e "  overlay → Kernel overlayfs (high performance)"
    else
        echo -e "  vfs     → VFS storage (default for Buildah)"
        echo -e "  overlay → Kernel overlayfs (high performance, requires emptyDir)"
    fi
    echo ""

    echo -e "${CYAN}SECURITY REQUIREMENTS:${NC}"
    if [ "$BUILDER" = "buildkit" ]; then
        echo -e "  Native:"
        echo -e "    - Capabilities: SETUID, SETGID"
        echo -e "    - Seccomp: Unconfined (for mount syscalls)"
        echo -e "    - AppArmor: Unconfined (for mount syscalls)"
        echo -e "    - Volumes: None"
        echo -e ""
        echo -e "  Overlay:"
        echo -e "    - Capabilities: SETUID, SETGID, MKNOD, DAC_OVERRIDE"
        echo -e "    - Seccomp: Unconfined (for mount syscalls)"
        echo -e "    - AppArmor: Unconfined (for mount syscalls)"
        echo -e "    - Volumes: None"
    else
        echo -e "  VFS:"
        echo -e "    - Capabilities: SETUID, SETGID"
        echo -e "    - Seccomp: RuntimeDefault"
        echo -e "    - AppArmor: RuntimeDefault"
        echo -e "    - Volumes: None"
        echo -e ""
        echo -e "  Overlay:"
        echo -e "    - Capabilities: SETUID, SETGID, MKNOD, DAC_OVERRIDE"
        echo -e "    - Seccomp: Unconfined"
        echo -e "    - AppArmor: Unconfined"
        echo -e "    - Volumes: /home/smithy/.local (emptyDir only)"
        echo -e "    - Note: emptyDir breaks nested overlayfs (container root = kernel overlay)"
        echo -e "    - Note: Uses native kernel overlayfs via user namespaces"
    fi
    echo ""

    # Start overall timer
    local overall_start=$(date +%s)

    # Setup namespace
    setup_namespace

    # Determine which drivers to test based on builder and storage selection
    local drivers=()
    local primary_driver=$(get_primary_driver)

    if [ "$STORAGE_DRIVER" = "both" ]; then
        # Test both primary driver and overlay
        drivers=("$primary_driver" "overlay")
        echo -e "${GREEN}✓ Will test both ${primary_driver} and overlay storage${NC}"
    elif [ "$STORAGE_DRIVER" = "overlay" ]; then
        drivers=("overlay")
        echo -e "${GREEN}✓ Will test overlay storage${NC}"
    elif [ "$STORAGE_DRIVER" = "native" ] || [ "$STORAGE_DRIVER" = "vfs" ]; then
        # Map to primary driver
        drivers=("$primary_driver")
        echo -e "${GREEN}✓ Will test ${primary_driver} storage${NC}"
    else
        drivers=("$STORAGE_DRIVER")
        echo -e "${GREEN}✓ Will test ${STORAGE_DRIVER} storage${NC}"
    fi

    echo ""
    echo -e "${CYAN}═══════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}Starting tests for storage drivers: ${drivers[@]}${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════════════════${NC}"
    echo ""

    # Run tests for each storage driver (ROOTLESS ONLY)
    for driver in "${drivers[@]}"; do
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

    echo -e "${GREEN}✓ All Kubernetes tests passed successfully!${NC}"
    echo ""
    echo -e "${CYAN}Generated YAML files in: ${SUITES_DIR}/${NC}"
    exit 0
}

# Trap cleanup on exit and interrupt
trap cleanup EXIT
trap cleanup_on_interrupt INT TERM

# Run main
main
