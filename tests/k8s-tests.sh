#!/bin/bash
# Smithy Kubernetes Test Suite
# Tests ROOTLESS mode (UID 1000) ONLY
# Smithy is designed for rootless operation in all environments
# Supports BuildKit (default) and Buildah images
# Tests storage drivers based on builder:
#   - BuildKit: native (default), overlay
#   - Buildah: vfs (default), overlay
# Note: Overlay with FUSE requires /dev/fuse on nodes

set -e

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
    echo -e "${BLUE}â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢${NC}"
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

    echo -e "${GREEN}âœ“ Namespace ready${NC}"
}

# ============================================================================
# FUSE Availability Check
# ============================================================================

check_fuse_availability() {
    echo ""
    echo -e "${CYAN}Checking FUSE availability on cluster nodes...${NC}"

    # Create a test pod to check /dev/fuse
    local test_pod="fuse-check-$$"

    cat > /tmp/fuse-check-pod.yaml <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: ${test_pod}
  namespace: ${NAMESPACE}
spec:
  restartPolicy: Never
  containers:
  - name: checker
    image: alpine:latest
    command: ['sh', '-c', 'if [ -c /dev/fuse ]; then echo "FUSE_AVAILABLE"; ls -l /dev/fuse; else echo "FUSE_NOT_AVAILABLE"; fi']
    volumeMounts:
    - name: dev-fuse
      mountPath: /dev/fuse
  volumes:
  - name: dev-fuse
    hostPath:
      path: /dev/fuse
      type: CharDevice
EOF

    kubectl apply -f /tmp/fuse-check-pod.yaml > /dev/null 2>&1

    # Wait for pod to complete
    local wait_count=0
    while [ $wait_count -lt 30 ]; do
        local phase=$(kubectl get pod ${test_pod} -n ${NAMESPACE} -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")
        if [ "$phase" = "Succeeded" ] || [ "$phase" = "Failed" ]; then
            break
        fi
        sleep 1
        wait_count=$((wait_count + 1))
    done

    # Get the result
    local result=$(kubectl logs ${test_pod} -n ${NAMESPACE} 2>/dev/null || echo "FUSE_NOT_AVAILABLE")

    # Cleanup
    kubectl delete pod ${test_pod} -n ${NAMESPACE} --force --grace-period=0 > /dev/null 2>&1
    rm -f /tmp/fuse-check-pod.yaml

    echo -e "${CYAN}FUSE check result:${NC}"
    echo "$result" | sed 's/^/  /'
    echo ""

    if [[ "$result" == *"FUSE_AVAILABLE"* ]]; then
        echo -e "${GREEN}âœ“ /dev/fuse is available on cluster nodes${NC}"
        echo -e "${GREEN}  Overlay storage will be tested with fuse-overlayfs${NC}"
        return 0
    else
        echo -e "${YELLOW}âš  /dev/fuse is NOT available on cluster nodes${NC}"
        echo -e "${YELLOW}  Overlay storage will be skipped${NC}"
        echo -e "${YELLOW}  To enable overlay: run 'sudo modprobe fuse' on all nodes${NC}"
        return 1
    fi
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

    local yaml_file="${SUITES_DIR}/job-${job_name}.yaml"

    # Set capabilities based on storage driver and builder
    local caps_add="[SETUID, SETGID]"
    local pod_seccomp=""
    local pod_apparmor=""
    local container_seccomp=""
    local container_apparmor=""
    local volume_mounts=""
    local volumes=""

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
        # Add MKNOD, DAC_OVERRIDE, and SYS_ADMIN for overlay with fuse-overlayfs
        caps_add="[SETUID, SETGID, MKNOD, DAC_OVERRIDE, SYS_ADMIN]"

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
        fi

        # Mount /dev/fuse for fuse-overlayfs
        volume_mounts="
        volumeMounts:
        - name: dev-fuse
          mountPath: /dev/fuse"
        volumes="
      volumes:
      - name: dev-fuse
        hostPath:
          path: /dev/fuse
          type: CharDevice"
    fi

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
          ${container_apparmor}${volume_mounts}${volumes}
EOF

    echo "$yaml_file"
}

# ============================================================================
# Run Kubernetes Test
# ============================================================================

run_k8s_test() {
    local test_name="$1"
    local driver="$2"
    local args="$3"

    TOTAL_TESTS=$((TOTAL_TESTS + 1))

    # Get actual storage flag
    local storage_flag=$(get_storage_flag "$driver")

    echo -e "${CYAN}[TEST $TOTAL_TESTS]${NC} ${test_name} (${BUILDER}, rootless, ${driver})"

    # Generate job YAML
    local job_name="test-${BUILDER}-$(date +%s)-$$-$RANDOM"
    local yaml_file=$(generate_job_yaml "$job_name" "$driver" "$args")

    echo -e "${CYAN}  YAML: $(basename $yaml_file)${NC}"

    local start_time=$(date +%s)

    # Create job
    echo -e "${CYAN}  Creating job...${NC}"
    if ! kubectl apply -f "$yaml_file" > /dev/null 2>&1; then
        echo -e "${RED}  âœ— FAIL${NC} (Failed to create job)"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TEST_RESULTS+=("FAIL: ${test_name} (${BUILDER}, rootless, ${driver})")
        echo ""
        return
    fi

    # Wait for pod to be created
    sleep 2

    local pod_name=$(kubectl get pods -n ${NAMESPACE} -l job-name=${job_name} -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

    if [ -z "$pod_name" ]; then
        echo -e "${RED}  âœ— FAIL${NC} (Failed to get pod name)"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TEST_RESULTS+=("FAIL: ${test_name} (${BUILDER}, rootless, ${driver})")
        kubectl delete job ${job_name} -n ${NAMESPACE} --force --grace-period=0 &> /dev/null || true
        echo ""
        return
    fi

    echo -e "${CYAN}  Pod: ${pod_name}${NC}"
    echo -e "${CYAN}  Streaming logs...${NC}"

    # Stream logs in background
    kubectl logs -f ${pod_name} -n ${NAMESPACE} 2>&1 | sed 's/^/    /' &
    local logs_pid=$!

    # Wait for job to complete
    if kubectl wait --for=condition=complete job/${job_name} -n ${NAMESPACE} --timeout=${JOB_TIMEOUT}s &> /dev/null; then
        wait $logs_pid 2>/dev/null || true

        local end_time=$(date +%s)
        local duration=$((end_time - start_time))

        echo -e "${GREEN}  âœ“ PASS${NC} (${duration}s)"
        PASSED_TESTS=$((PASSED_TESTS + 1))
        TEST_RESULTS+=("PASS: ${test_name} (${BUILDER}, rootless, ${driver})")
    else
        kill $logs_pid 2>/dev/null || true
        wait $logs_pid 2>/dev/null || true

        local end_time=$(date +%s)
        local duration=$((end_time - start_time))

        echo -e "${RED}  âœ— FAIL${NC} (${duration}s)"
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
        echo -e "${CYAN}Note: Overlay storage uses fuse-overlayfs (requires /dev/fuse)${NC}"
        if [ "$BUILDER" = "buildkit" ]; then
            echo -e "${CYAN}      BuildKit needs: DAC_OVERRIDE capability + Unconfined seccomp/AppArmor${NC}"
        else
            echo -e "${CYAN}      Buildah needs: MKNOD capability + Unconfined seccomp/AppArmor${NC}"
        fi
        echo ""
    elif [ "$driver" = "native" ]; then
        echo -e "${CYAN}Note: Native snapshotter (BuildKit) - secure and performant${NC}"
        echo -e "${CYAN}      Requires Unconfined seccomp + AppArmor for mount syscalls${NC}"
        echo ""
    elif [ "$driver" = "vfs" ]; then
        echo -e "${CYAN}Note: VFS storage (Buildah) - most secure but slower${NC}"
        echo ""
    fi

    # Test 1: Version check
    run_k8s_test \
        "Version Check" \
        "$driver" \
        "[\"--version\"]"

    # Test 2: Environment check
    run_k8s_test \
        "Environment Check" \
        "$driver" \
        "[\"check-environment\"]"

    # Test 3: Basic build from Git
    run_k8s_test \
        "Git Repository Build" \
        "$driver" \
        "[\"--context=https://github.com/nginxinc/docker-nginx.git\", \"--git-branch=master\", \"--dockerfile=mainline/alpine/Dockerfile\", \"--destination=test-${BUILDER}-k8s-rootless-git-${driver}:latest\", \"--storage-driver=${storage_flag}\", \"--no-push\", \"--verbosity=debug\"]"

    # Test 4: Build with arguments from Git
    run_k8s_test \
        "Build with Arguments" \
        "$driver" \
        "[\"--context=https://github.com/nginxinc/docker-nginx.git\", \"--git-branch=master\", \"--dockerfile=mainline/alpine/Dockerfile\", \"--destination=test-${BUILDER}-k8s-rootless-buildargs-${driver}:latest\", \"--build-arg=NGINX_VERSION=1.25\", \"--storage-driver=${storage_flag}\", \"--no-push\", \"--verbosity=debug\"]"
}

# ============================================================================
# Cleanup Function
# ============================================================================

cleanup() {
    if [ "$CLEANUP_AFTER" = true ]; then
        print_section "CLEANUP"

        echo "Deleting namespace: ${NAMESPACE}"
        kubectl delete namespace ${NAMESPACE} --force --grace-period=0 &> /dev/null || true

        echo -e "${GREEN}âœ“ Cleanup completed${NC}"
    fi
}

cleanup_on_interrupt() {
    echo ""
    echo -e "${YELLOW}Interrupted by user (Ctrl+C)${NC}"
    echo -e "${YELLOW}Cleaning up...${NC}"

    kubectl delete jobs -n ${NAMESPACE} -l app=smithy-test --force --grace-period=0 &> /dev/null || true
    kubectl delete namespace ${NAMESPACE} --force --grace-period=0 &> /dev/null || true

    echo -e "${GREEN}âœ“ Cleanup completed${NC}"
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
        echo -e "  native  â†’ Native snapshotter (default for BuildKit)"
        echo -e "  overlay â†’ fuse-overlayfs (high performance)"
    else
        echo -e "  vfs     â†’ VFS storage (default for Buildah)"
        echo -e "  overlay â†’ fuse-overlayfs (high performance)"
    fi
    echo ""

    echo -e "${CYAN}SECURITY REQUIREMENTS:${NC}"
    if [ "$BUILDER" = "buildkit" ]; then
        echo -e "  Native:"
        echo -e "    - Capabilities: SETUID, SETGID"
        echo -e "    - Seccomp: Unconfined (for mount syscalls)"
        echo -e "    - AppArmor: Unconfined (for mount syscalls)"
        echo -e ""
        echo -e "  Overlay:"
        echo -e "    - Capabilities: SETUID, SETGID, MKNOD, DAC_OVERRIDE"
        echo -e "    - Seccomp: Unconfined (for mount syscalls)"
        echo -e "    - AppArmor: Unconfined (for mount syscalls)"
        echo -e "    - Requires: /dev/fuse on nodes"
    else
        echo -e "  VFS:"
        echo -e "    - Capabilities: SETUID, SETGID"
        echo -e "    - Seccomp: RuntimeDefault (default)"
        echo -e "    - AppArmor: RuntimeDefault (default)"
        echo -e ""
        echo -e "  Overlay:"
        echo -e "    - Capabilities: SETUID, SETGID, MKNOD, DAC_OVERRIDE"
        echo -e "    - Seccomp: Unconfined"
        echo -e "    - AppArmor: Unconfined"
        echo -e "    - Requires: /dev/fuse on nodes"
    fi
    echo ""

    # Start overall timer
    local overall_start=$(date +%s)

    # Setup namespace
    setup_namespace

    # Check FUSE availability for overlay storage
    local fuse_available=false
    if check_fuse_availability; then
        fuse_available=true
    fi

    # Determine which drivers to test based on builder and storage selection
    local drivers=()
    local primary_driver=$(get_primary_driver)

    if [ "$STORAGE_DRIVER" = "both" ]; then
        # Always test primary driver first
        drivers=("$primary_driver")
        # Add overlay only if FUSE is available
        if [ "$fuse_available" = true ]; then
            drivers+=("overlay")
            echo -e "${GREEN}âœ“ Will test both ${primary_driver} and overlay storage${NC}"
        else
            echo -e "${YELLOW}âš  Will test ${primary_driver} only (overlay skipped - FUSE not available)${NC}"
        fi
    elif [ "$STORAGE_DRIVER" = "overlay" ]; then
        if [ "$fuse_available" = true ]; then
            drivers=("overlay")
            echo -e "${GREEN}âœ“ Will test overlay storage${NC}"
        else
            echo -e "${RED}Error: Overlay storage requested but FUSE is not available${NC}"
            echo -e "${RED}Solution: Load FUSE module on nodes: 'sudo modprobe fuse'${NC}"
            exit 1
        fi
    elif [ "$STORAGE_DRIVER" = "native" ] || [ "$STORAGE_DRIVER" = "vfs" ]; then
        # Map to primary driver
        drivers=("$primary_driver")
        echo -e "${GREEN}âœ“ Will test ${primary_driver} storage${NC}"
    else
        drivers=("$STORAGE_DRIVER")
        echo -e "${GREEN}âœ“ Will test ${STORAGE_DRIVER} storage${NC}"
    fi

    echo ""
    echo -e "${CYAN}â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢${NC}"
    echo -e "${CYAN}Starting tests for storage drivers: ${drivers[@]}${NC}"
    echo -e "${CYAN}â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢â€¢${NC}"
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

    echo -e "${GREEN}âœ“ All Kubernetes tests passed successfully!${NC}"
    echo ""
    echo -e "${CYAN}Generated YAML files in: ${SUITES_DIR}/${NC}"
    exit 0
}

# Trap cleanup on exit and interrupt
trap cleanup EXIT
trap cleanup_on_interrupt INT TERM

# Run main
main