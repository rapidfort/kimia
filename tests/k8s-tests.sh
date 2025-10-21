#!/bin/bash
# Smithy Kubernetes Test Suite
# Tests ROOTLESS mode (UID 1000) ONLY
# Rootful mode is NOT supported in Kubernetes environments
# Tests both VFS and Overlay storage drivers
# Note: Overlay with FUSE requires /dev/fuse on nodes (no MKNOD capability needed with FUSE)

set -e

# Default configuration - handle internal vs external registry
if [ -z "${RF_APP_HOST}" ]; then
    REGISTRY=${REGISTRY:-"ghcr.io"}
else
    REGISTRY="${RF_APP_HOST}:5000"
fi

SMITHY_IMAGE=${SMITHY_IMAGE:-"${REGISTRY}/rapidfort/smithy:latest"}
NAMESPACE=${NAMESPACE:-"smithy-tests"}
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

# Create suites directory
mkdir -p "${SUITES_DIR}"

# ============================================================================
# Helper Functions
# ============================================================================

print_section() {
    echo ""
    echo -e "${BLUE}════════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}════════════════════════════════════════════════════════════${NC}"
    echo ""
}

# ============================================================================
# Test Script Generator
# ============================================================================

create_test_script() {
    local test_type="$1"  # "happy" or "unhappy"
    local mode="$2"       # "rootless"
    local driver="$3"     # "vfs" or "overlay"
    local test_name="$4"  # e.g., "version"
    local args="$5"       # JSON array of args
    
    local script_name="${test_type}-${mode}-${driver}-${test_name}.sh"
    local script_path="${SUITES_DIR}/${script_name}"
    
    cat > "${script_path}" <<'SCRIPT_EOF'
#!/bin/bash
# Auto-generated test script
# Test: TEST_NAME_PLACEHOLDER
# Mode: MODE_PLACEHOLDER (UID 1000)
# Storage: DRIVER_PLACEHOLDER

set -e

NAMESPACE="NAMESPACE_PLACEHOLDER"
SMITHY_IMAGE="IMAGE_PLACEHOLDER"
DRIVER="DRIVER_PLACEHOLDER"
ARGS='ARGS_PLACEHOLDER'
JOB_TIMEOUT=600

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

echo -e "${CYAN}Running test: TEST_NAME_PLACEHOLDER${NC}"
echo -e "${CYAN}Mode: MODE_PLACEHOLDER (rootless, UID 1000)${NC}"
echo -e "${CYAN}Storage: ${DRIVER}${NC}"
echo ""

# Generate job YAML
JOB_NAME="test-$(date +%s)-$$"
YAML_FILE="/tmp/${JOB_NAME}.yaml"

# Determine capabilities and security settings based on storage driver
if [ "$DRIVER" = "overlay" ]; then
    # Overlay with FUSE: SETUID, SETGID, MKNOD, DAC_OVERRIDE + unconfined profiles
    CAPS_ADD="[SETUID, SETGID, MKNOD, DAC_OVERRIDE]"
    POD_SECCOMP='seccompProfile:
      type: Unconfined'
    CONTAINER_SECCOMP='seccompProfile:
        type: Unconfined'
    CONTAINER_APPARMOR='apparmor.security.beta.kubernetes.io/pod: unconfined'
    VOLUME_MOUNTS='
    - name: dev-fuse
      mountPath: /dev/fuse'
    VOLUMES='
  - name: dev-fuse
    hostPath:
      path: /dev/fuse
      type: CharDevice'
else
    # VFS: Only SETUID, SETGID
    CAPS_ADD="[SETUID, SETGID]"
    POD_SECCOMP=""
    CONTAINER_SECCOMP=""
    CONTAINER_APPARMOR=""
    VOLUME_MOUNTS=""
    VOLUMES=""
fi

cat > "$YAML_FILE" <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: ${JOB_NAME}
  namespace: ${NAMESPACE}
  labels:
    app: smithy-test
    mode: rootless
    driver: ${DRIVER}
spec:
  backoffLimit: 0
  ttlSecondsAfterFinished: 300
  template:
    metadata:
      labels:
        app: smithy-test
        mode: rootless
        driver: ${DRIVER}
      annotations:
        ${CONTAINER_APPARMOR}
    spec:
      restartPolicy: Never
      securityContext:
        runAsUser: 1000
        runAsGroup: 1000
        fsGroup: 1000
        ${POD_SECCOMP}
      containers:
      - name: smithy
        image: ${SMITHY_IMAGE}
        args: ${ARGS}
        env:
        - name: SMITHY_USER
          value: "smithy"
        - name: HOME
          value: "/home/smithy"
        - name: STORAGE_DRIVER
          value: "${DRIVER}"
        securityContext:
          allowPrivilegeEscalation: true
          capabilities:
            add: ${CAPS_ADD}
            drop: [ALL]
          runAsUser: 1000
          runAsGroup: 1000
          ${CONTAINER_SECCOMP}
        volumeMounts:
        - name: home
          mountPath: /home/smithy${VOLUME_MOUNTS}
      volumes:
      - name: home
        emptyDir: {}${VOLUMES}
EOF

echo -e "${CYAN}Creating Kubernetes job...${NC}"
kubectl apply -f "$YAML_FILE" > /dev/null

# Wait for pod to be created
echo -e "${CYAN}Waiting for pod to be created...${NC}"
sleep 2

POD_NAME=$(kubectl get pods -n ${NAMESPACE} -l job-name=${JOB_NAME} -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

if [ -z "$POD_NAME" ]; then
    echo -e "${RED}Failed to get pod name${NC}"
    kubectl delete job ${JOB_NAME} -n ${NAMESPACE} --force --grace-period=0 &> /dev/null || true
    rm -f "$YAML_FILE"
    exit 1
fi

echo -e "${CYAN}Pod: ${POD_NAME}${NC}"
echo -e "${CYAN}Streaming logs...${NC}"
echo ""

# Stream logs
kubectl logs -f ${POD_NAME} -n ${NAMESPACE} 2>&1 &
LOGS_PID=$!

# Wait for job completion
if kubectl wait --for=condition=complete job/${JOB_NAME} -n ${NAMESPACE} --timeout=${JOB_TIMEOUT}s &> /dev/null; then
    wait $LOGS_PID 2>/dev/null || true
    echo ""
    echo -e "${GREEN}✓ TEST PASSED${NC}"
    RESULT=0
else
    kill $LOGS_PID 2>/dev/null || true
    wait $LOGS_PID 2>/dev/null || true
    echo ""
    echo -e "${RED}✗ TEST FAILED${NC}"
    echo -e "${RED}Complete pod logs:${NC}"
    kubectl logs ${POD_NAME} -n ${NAMESPACE} 2>&1 || true
    RESULT=1
fi

# Cleanup
echo -e "${CYAN}Cleaning up...${NC}"
kubectl delete job ${JOB_NAME} -n ${NAMESPACE} --force --grace-period=0 &> /dev/null || true
rm -f "$YAML_FILE"

exit $RESULT
SCRIPT_EOF

    # Replace placeholders
    sed -i "s|TEST_NAME_PLACEHOLDER|${test_name}|g" "${script_path}"
    sed -i "s|MODE_PLACEHOLDER|${mode}|g" "${script_path}"
    sed -i "s|DRIVER_PLACEHOLDER|${driver}|g" "${script_path}"
    sed -i "s|NAMESPACE_PLACEHOLDER|${NAMESPACE}|g" "${script_path}"
    sed -i "s|IMAGE_PLACEHOLDER|${SMITHY_IMAGE}|g" "${script_path}"
    sed -i "s|ARGS_PLACEHOLDER|${args}|g" "${script_path}"
    
    chmod +x "${script_path}"
    
    echo "${script_path}"
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
        echo -e "${GREEN}✓ /dev/fuse is available on cluster nodes${NC}"
        echo -e "${GREEN}  Overlay storage will be tested with fuse-overlayfs${NC}"
        return 0
    else
        echo -e "${YELLOW}✗ /dev/fuse is NOT available on cluster nodes${NC}"
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
    
    local yaml_file="${SUITES_DIR}/job-${job_name}.yaml"
    
    # Determine capabilities based on storage driver
    # VFS: SETUID, SETGID
    # Overlay with FUSE: SETUID, SETGID, MKNOD, DAC_OVERRIDE + unconfined seccomp + unconfined apparmor
    local caps_add="[SETUID, SETGID]"
    local pod_seccomp=""
    local pod_apparmor=""
    local container_seccomp=""
    local container_apparmor=""
    local volume_mounts=""
    local volumes=""
    
    if [ "$driver" = "overlay" ]; then
        caps_add="[SETUID, SETGID, MKNOD, DAC_OVERRIDE]"
        pod_seccomp="seccompProfile:
          type: Unconfined"
        pod_apparmor="appArmorProfile:
          type: Unconfined"
        container_seccomp="seccompProfile:
            type: Unconfined"
        container_apparmor="appArmorProfile:
            type: Unconfined"
        volume_mounts="
        - name: dev-fuse
          mountPath: /dev/fuse"
        volumes="
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
    mode: rootless
    driver: ${driver}
spec:
  backoffLimit: 0
  ttlSecondsAfterFinished: 300
  template:
    metadata:
      labels:
        app: smithy-test
        mode: rootless
        driver: ${driver}
    spec:
      restartPolicy: Never
      securityContext:
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
        - name: HOME
          value: "/home/smithy"
        - name: STORAGE_DRIVER
          value: "${driver}"
        securityContext:
          allowPrivilegeEscalation: true
          capabilities:
            add: ${caps_add}
            drop: [ALL]
          runAsUser: 1000
          runAsGroup: 1000
          ${container_seccomp}
          ${container_apparmor}
        volumeMounts:
        - name: home
          mountPath: /home/smithy${volume_mounts}
      volumes:
      - name: home
        emptyDir: {}${volumes}
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
    
    echo -e "${CYAN}Test $TOTAL_TESTS: ${test_name} (rootless, ${driver})${NC}"
    
    # Generate test script
    local test_script=$(create_test_script "happy" "rootless" "$driver" "$(echo $test_name | tr ' ' '-' | tr '[:upper:]' '[:lower:]')" "$args")
    echo -e "${CYAN}  Generated: ${test_script}${NC}"
    
    # Generate job YAML
    local job_name="test-$(date +%s)-$$-$RANDOM"
    local yaml_file=$(generate_job_yaml "$job_name" "$driver" "$args")
    
    local start_time=$(date +%s)
    
    # Create job
    echo -e "${CYAN}  Creating job...${NC}"
    kubectl apply -f "$yaml_file" > /dev/null
    
    # Wait for pod to be created
    sleep 2
    
    local pod_name=$(kubectl get pods -n ${NAMESPACE} -l job-name=${job_name} -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
    
    if [ -z "$pod_name" ]; then
        echo -e "${RED}  ✗ FAIL${NC} (Failed to get pod name)"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TEST_RESULTS+=("FAIL: ${test_name} (rootless, ${driver})")
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
        
        echo -e "${GREEN}  ✓ PASS${NC} (${duration}s)"
        PASSED_TESTS=$((PASSED_TESTS + 1))
        TEST_RESULTS+=("PASS: ${test_name} (rootless, ${driver})")
    else
        kill $logs_pid 2>/dev/null || true
        wait $logs_pid 2>/dev/null || true
        
        local end_time=$(date +%s)
        local duration=$((end_time - start_time))
        
        echo -e "${RED}  ✗ FAIL${NC} (${duration}s)"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TEST_RESULTS+=("FAIL: ${test_name} (rootless, ${driver})")
        
        echo -e "${RED}  Complete pod logs:${NC}"
        kubectl logs ${pod_name} -n ${NAMESPACE} 2>&1 | sed 's/^/    /' || true
    fi
    
    # Cleanup job (but keep YAML file)
    echo -e "${CYAN}  Cleaning up job...${NC}"
    kubectl delete job ${job_name} -n ${NAMESPACE} --force --grace-period=0 &> /dev/null || true
    
    echo ""
}

# ============================================================================
# Rootless Mode Tests (ONLY mode supported in Kubernetes)
# ============================================================================

run_rootless_tests() {
    local driver="$1"
    
    print_section "ROOTLESS MODE TESTS - ${driver^^} STORAGE"
    
    if [ "$driver" = "overlay" ]; then
        echo -e "${CYAN}Note: Overlay storage uses fuse-overlayfs (requires /dev/fuse)${NC}"
        echo -e "${CYAN}      No MKNOD capability needed when using FUSE${NC}"
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
        "[\"--context=https://github.com/nginxinc/docker-nginx.git\", \"--git-branch=master\", \"--dockerfile=mainline/alpine/Dockerfile\", \"--destination=test-k8s-rootless-git-${driver}:latest\", \"--storage-driver=${driver}\", \"--no-push\", \"--verbosity=debug\"]"
    
    # Test 4: Build with arguments from Git
    run_k8s_test \
        "Build with Arguments" \
        "$driver" \
        "[\"--context=https://github.com/nginxinc/docker-nginx.git\", \"--git-branch=master\", \"--dockerfile=mainline/alpine/Dockerfile\", \"--destination=test-k8s-rootless-buildargs-${driver}:latest\", \"--build-arg=NGINX_VERSION=1.25\", \"--storage-driver=${driver}\", \"--no-push\", \"--verbosity=debug\"]"
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
    echo -e "  Registry:       ${REGISTRY}"
    echo -e "  Image:          ${SMITHY_IMAGE}"
    echo -e "  Namespace:      ${NAMESPACE}"
    echo -e "  Storage:        ${STORAGE_DRIVER}"
    echo -e "  Cleanup:        ${CLEANUP_AFTER}"
    echo -e "  Job Timeout:    ${JOB_TIMEOUT}s"
    echo -e "  Suites Dir:     ${SUITES_DIR}"
    echo ""
    echo -e "${YELLOW}NOTE: Kubernetes only supports ROOTLESS mode${NC}"
    echo -e "${YELLOW}      Rootful mode is NOT supported in K8s${NC}"
    echo ""
    echo -e "${CYAN}STORAGE REQUIREMENTS:${NC}"
    echo -e "  VFS:            No dependencies (recommended for K8s)"
    echo -e "  Overlay:        Requires /dev/fuse on nodes (uses fuse-overlayfs)"
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
    
    # Determine which drivers to test
    local drivers=()
    if [ "$STORAGE_DRIVER" = "both" ]; then
        # Always test VFS first
        drivers=("vfs")
        # Add overlay only if FUSE is available
        if [ "$fuse_available" = true ]; then
            drivers+=("overlay")
            echo -e "${GREEN}✓ Will test both VFS and Overlay storage${NC}"
        else
            echo -e "${YELLOW}⚠ Will test VFS only (overlay skipped - FUSE not available)${NC}"
        fi
    elif [ "$STORAGE_DRIVER" = "overlay" ]; then
        if [ "$fuse_available" = true ]; then
            drivers=("overlay")
            echo -e "${GREEN}✓ Will test Overlay storage${NC}"
        else
            echo -e "${RED}Error: Overlay storage requested but FUSE is not available${NC}"
            echo -e "${RED}Solution: Load FUSE module on nodes: 'sudo modprobe fuse'${NC}"
            exit 1
        fi
    else
        # VFS only
        drivers=("$STORAGE_DRIVER")
        echo -e "${GREEN}✓ Will test ${STORAGE_DRIVER^^} storage${NC}"
    fi
    
    echo ""
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${CYAN}Starting tests for storage drivers: ${drivers[@]}${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
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
    echo -e "${CYAN}Generated test scripts in: ${SUITES_DIR}/${NC}"
    echo -e "${CYAN}Example: bash ${SUITES_DIR}/happy-rootless-vfs-version.sh${NC}"
    exit 0
}

# Trap cleanup on exit and interrupt
trap cleanup EXIT
trap cleanup_on_interrupt INT TERM

# Run main
main