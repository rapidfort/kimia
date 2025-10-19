#!/bin/bash
# Smithy Kubernetes Test Suite
# Tests both rootless (UID 1000) and rootful (UID 0) modes
# Tests both VFS and Overlay storage drivers

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

# ============================================================================
# Kubernetes Setup
# ============================================================================

setup_namespace() {
    print_section "KUBERNETES SETUP"
    
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
# Test Job Generation
# ============================================================================

generate_job_yaml() {
    local job_name="$1"
    local mode="$2"  # rootless or rootful
    local driver="$3"
    local args="$4"
    
    local yaml_file="/tmp/smithy-job-${job_name}.yaml"
    
    if [ "$mode" = "rootless" ]; then
        # Rootless configuration (UID 1000)
        cat > "$yaml_file" <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: ${job_name}
  namespace: ${NAMESPACE}
spec:
  ttlSecondsAfterFinished: 300
  backoffLimit: 0
  template:
    metadata:
      labels:
        app: smithy-test
        mode: rootless
        storage: ${driver}
    spec:
      restartPolicy: Never
      
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        runAsGroup: 1000
        fsGroup: 1000
      
      containers:
      - name: smithy
        image: ${SMITHY_IMAGE}
        imagePullPolicy: IfNotPresent
        
        args: ${args}
        
        securityContext:
          runAsUser: 1000
          allowPrivilegeEscalation: true
          capabilities:
            drop: [ALL]
            add: [SETUID, SETGID, MKNOD]
        
        env:
        - name: HOME
          value: /home/smithy
        - name: DOCKER_CONFIG
          value: /home/smithy/.docker
        
        resources:
          requests:
            memory: "2Gi"
            cpu: "1"
          limits:
            memory: "8Gi"
            cpu: "4"
            ephemeral-storage: "10Gi"
EOF
    else
        # Rootful configuration (UID 0)
        # Least privilege approach:
        # - VFS: No privileged needed
        # - Overlay: Privileged required for mount operations
        
        if [ "$driver" = "overlay" ]; then
            # Overlay storage needs privileged mode
            cat > "$yaml_file" <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: ${job_name}
  namespace: ${NAMESPACE}
spec:
  ttlSecondsAfterFinished: 300
  backoffLimit: 0
  template:
    metadata:
      labels:
        app: smithy-test
        mode: rootful
        storage: ${driver}
    spec:
      restartPolicy: Never
      
      securityContext:
        runAsUser: 0
        runAsGroup: 0
      
      containers:
      - name: smithy
        image: ${SMITHY_IMAGE}
        imagePullPolicy: IfNotPresent
        
        args: ${args}
        
        securityContext:
          runAsUser: 0
          privileged: true
        
        env:
        - name: HOME
          value: /root
        - name: DOCKER_CONFIG
          value: /root/.docker
        - name: BUILDAH_ISOLATION
          value: chroot
        
        resources:
          requests:
            memory: "2Gi"
            cpu: "1"
          limits:
            memory: "8Gi"
            cpu: "4"
            ephemeral-storage: "10Gi"
EOF
        else
            # VFS storage works without privileged
            cat > "$yaml_file" <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: ${job_name}
  namespace: ${NAMESPACE}
spec:
  ttlSecondsAfterFinished: 300
  backoffLimit: 0
  template:
    metadata:
      labels:
        app: smithy-test
        mode: rootful
        storage: ${driver}
    spec:
      restartPolicy: Never
      
      securityContext:
        runAsUser: 0
        runAsGroup: 0
      
      containers:
      - name: smithy
        image: ${SMITHY_IMAGE}
        imagePullPolicy: IfNotPresent
        
        args: ${args}
        
        securityContext:
          runAsUser: 0
          allowPrivilegeEscalation: true
          capabilities:
            drop: [ALL]
            add: [SETUID, SETGID, MKNOD]
        
        env:
        - name: HOME
          value: /root
        - name: DOCKER_CONFIG
          value: /root/.docker
        - name: BUILDAH_ISOLATION
          value: chroot
        
        resources:
          requests:
            memory: "2Gi"
            cpu: "1"
          limits:
            memory: "8Gi"
            cpu: "4"
            ephemeral-storage: "10Gi"
EOF
        fi
    fi
    
    echo "$yaml_file"
}

# ============================================================================
# Test Execution
# ============================================================================

run_k8s_test() {
    local test_name="$1"
    local mode="$2"
    local driver="$3"
    local args="$4"
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    
    # Generate safe job name
    local job_name="smithy-test-${mode}-${driver}-${TOTAL_TESTS}"
    job_name=$(echo "$job_name" | tr '[:upper:]' '[:lower:]' | tr '_' '-')
    
    echo -e "${CYAN}[TEST $TOTAL_TESTS]${NC} ${test_name} (${mode}, ${driver})"
    echo -e "${CYAN}  Job name: ${job_name}${NC}"
    
    # Generate job YAML
    local yaml_file=$(generate_job_yaml "$job_name" "$mode" "$driver" "$args")
    
    # Apply job
    echo -e "${CYAN}  Creating job...${NC}"
    if ! kubectl apply -f "$yaml_file" &> /dev/null; then
        echo -e "${RED}  ✗ FAIL${NC} - Failed to create job"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TEST_RESULTS+=("FAIL: ${test_name} (${mode}, ${driver}) - Job creation failed")
        rm -f "$yaml_file"
        return 1
    fi
    
    # Wait for pod to be created
    echo -e "${CYAN}  Waiting for pod to be created...${NC}"
    local pod_name=""
    local wait_count=0
    while [ -z "$pod_name" ] && [ $wait_count -lt 60 ]; do
        pod_name=$(kubectl get pods -n ${NAMESPACE} -l job-name=${job_name} -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)
        if [ -z "$pod_name" ]; then
            sleep 1
            wait_count=$((wait_count + 1))
        fi
    done
    
    if [ -z "$pod_name" ]; then
        echo -e "${RED}  ✗ FAIL${NC} - Pod did not get created within timeout"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TEST_RESULTS+=("FAIL: ${test_name} (${mode}, ${driver}) - Pod creation timeout")
        kubectl delete job ${job_name} -n ${NAMESPACE} --force --grace-period=0 &> /dev/null || true
        rm -f "$yaml_file"
        return 1
    fi
    
    echo -e "${CYAN}  Pod created: ${pod_name}${NC}"
    
    # Wait for pod to start (not Ready, but at least Running or better diagnostics)
    echo -e "${CYAN}  Waiting for pod to start (max 120s)...${NC}"
    wait_count=0
    while [ $wait_count -lt 120 ]; do
        pod_phase=$(kubectl get pod ${pod_name} -n ${NAMESPACE} -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")
        
        # Check if pod is running or succeeded
        if [ "$pod_phase" = "Running" ] || [ "$pod_phase" = "Succeeded" ]; then
            echo -e "${CYAN}  Pod phase: ${pod_phase}${NC}"
            break
        fi
        
        # Check if pod failed
        if [ "$pod_phase" = "Failed" ]; then
            echo -e "${RED}  Pod failed to start!${NC}"
            echo -e "${YELLOW}  Pod events:${NC}"
            kubectl describe pod ${pod_name} -n ${NAMESPACE} | grep -A 10 "Events:" | sed 's/^/    /'
            echo -e "${YELLOW}  Container status:${NC}"
            kubectl get pod ${pod_name} -n ${NAMESPACE} -o jsonpath='{.status.containerStatuses[0].state}' | sed 's/^/    /'
            echo ""
            
            FAILED_TESTS=$((FAILED_TESTS + 1))
            TEST_RESULTS+=("FAIL: ${test_name} (${mode}, ${driver}) - Pod failed to start")
            kubectl delete job ${job_name} -n ${NAMESPACE} --force --grace-period=0 &> /dev/null || true
            rm -f "$yaml_file"
            return 1
        fi
        
        # If still in Pending after 30 seconds, show diagnostics
        if [ $wait_count -eq 30 ] && [ "$pod_phase" = "Pending" ]; then
            echo -e "${YELLOW}  Pod still pending after 30s, checking status...${NC}"
            
            # Check container status
            container_state=$(kubectl get pod ${pod_name} -n ${NAMESPACE} -o jsonpath='{.status.containerStatuses[0].state}' 2>/dev/null || echo "{}")
            
            # Check for image pull issues
            if echo "$container_state" | grep -q "ImagePullBackOff\|ErrImagePull"; then
                echo -e "${RED}  ✗ Image pull failed!${NC}"
                echo -e "${YELLOW}  Container state:${NC}"
                echo "$container_state" | sed 's/^/    /'
                echo -e "${YELLOW}  Recent events:${NC}"
                kubectl describe pod ${pod_name} -n ${NAMESPACE} | grep -A 5 "Events:" | sed 's/^/    /'
                
                FAILED_TESTS=$((FAILED_TESTS + 1))
                TEST_RESULTS+=("FAIL: ${test_name} (${mode}, ${driver}) - Image pull failed")
                kubectl delete job ${job_name} -n ${NAMESPACE} --force --grace-period=0 &> /dev/null || true
                rm -f "$yaml_file"
                return 1
            fi
            
            # Check for other waiting reasons
            if echo "$container_state" | grep -q "ContainerCreating"; then
                echo -e "${YELLOW}  Container is still being created...${NC}"
                waiting_reason=$(kubectl get pod ${pod_name} -n ${NAMESPACE} -o jsonpath='{.status.containerStatuses[0].state.waiting.reason}' 2>/dev/null || echo "Unknown")
                waiting_message=$(kubectl get pod ${pod_name} -n ${NAMESPACE} -o jsonpath='{.status.containerStatuses[0].state.waiting.message}' 2>/dev/null || echo "No message")
                echo -e "${YELLOW}  Reason: ${waiting_reason}${NC}"
                echo -e "${YELLOW}  Message: ${waiting_message}${NC}"
            fi
        fi
        
        sleep 1
        wait_count=$((wait_count + 1))
    done
    
    # Final check after wait loop
    if [ $wait_count -ge 120 ]; then
        echo -e "${RED}  ✗ FAIL${NC} - Pod did not start within 120s timeout"
        echo -e "${YELLOW}  Final pod status:${NC}"
        kubectl describe pod ${pod_name} -n ${NAMESPACE} | grep -A 20 "Events:" | sed 's/^/    /'
        
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TEST_RESULTS+=("FAIL: ${test_name} (${mode}, ${driver}) - Pod start timeout")
        kubectl delete job ${job_name} -n ${NAMESPACE} --force --grace-period=0 &> /dev/null || true
        rm -f "$yaml_file"
        return 1
    fi
    
    # Stream logs
    local start_time=$(date +%s)
    
    # Stream logs in background while waiting for completion
    echo -e "${CYAN}  Build output:${NC}"
    kubectl logs -f ${pod_name} -n ${NAMESPACE} 2>&1 | sed 's/^/    /' &
    local logs_pid=$!
    
    # Wait for job to complete (not pod to be ready)
    if kubectl wait --for=condition=complete job/${job_name} -n ${NAMESPACE} --timeout=${JOB_TIMEOUT}s &> /dev/null; then
        # Wait for logs to finish streaming
        wait $logs_pid 2>/dev/null || true
        
        local end_time=$(date +%s)
        local duration=$((end_time - start_time))
        
        echo -e "${GREEN}  ✓ PASS${NC} (${duration}s)"
        PASSED_TESTS=$((PASSED_TESTS + 1))
        TEST_RESULTS+=("PASS: ${test_name} (${mode}, ${driver})")
    else
        # Kill log streaming if still running
        kill $logs_pid 2>/dev/null || true
        wait $logs_pid 2>/dev/null || true
        
        local end_time=$(date +%s)
        local duration=$((end_time - start_time))
        
        # Check if it failed
        local failed=$(kubectl get job ${job_name} -n ${NAMESPACE} -o jsonpath='{.status.failed}' 2>/dev/null || echo "0")
        
        if [ "$failed" = "1" ]; then
            echo -e "${RED}  ✗ FAIL${NC} (${duration}s) - Job failed"
        else
            echo -e "${RED}  ✗ FAIL${NC} (${duration}s) - Timeout"
        fi
        
        FAILED_TESTS=$((FAILED_TESTS + 1))
        TEST_RESULTS+=("FAIL: ${test_name} (${mode}, ${driver}) - Timeout or failure")
        
        # Show complete logs on failure
        echo -e "${RED}  Complete pod logs:${NC}"
        kubectl logs ${pod_name} -n ${NAMESPACE} 2>&1 | sed 's/^/    /' || true
    fi
    
    # Cleanup job
    echo -e "${CYAN}  Cleaning up job...${NC}"
    kubectl delete job ${job_name} -n ${NAMESPACE} --force --grace-period=0 &> /dev/null || true
    rm -f "$yaml_file"
    
    echo ""
}

# ============================================================================
# Rootless Mode Tests
# ============================================================================

run_rootless_tests() {
    local driver="$1"
    
    print_section "ROOTLESS MODE TESTS (UID 1000) - ${driver^^} STORAGE"
    
    # Test 1: Version check
    run_k8s_test \
        "Version Check" \
        "rootless" \
        "$driver" \
        "[\"--version\"]"
    
    # Test 2: Environment check
    run_k8s_test \
        "Environment Check" \
        "rootless" \
        "$driver" \
        "[\"check-environment\"]"
    
    # Test 3: Basic build from Git
    run_k8s_test \
        "Git Repository Build" \
        "rootless" \
        "$driver" \
        "[\"--context=https://github.com/nginxinc/docker-nginx.git\", \"--git-branch=master\", \"--dockerfile=mainline/alpine/Dockerfile\", \"--destination=test-k8s-rootless-git-${driver}:latest\", \"--storage-driver=${driver}\", \"--no-push\", \"--verbosity=debug\"]"
    
    # Test 4: Build with arguments from Git
    run_k8s_test \
        "Build with Arguments" \
        "rootless" \
        "$driver" \
        "[\"--context=https://github.com/nginxinc/docker-nginx.git\", \"--git-branch=master\", \"--dockerfile=mainline/alpine/Dockerfile\", \"--destination=test-k8s-rootless-buildargs-${driver}:latest\", \"--build-arg=NGINX_VERSION=1.25\", \"--storage-driver=${driver}\", \"--no-push\", \"--verbosity=debug\"]"
}

# ============================================================================
# Rootful Mode Tests
# ============================================================================

run_rootful_tests() {
    local driver="$1"
    
    print_section "ROOTFUL MODE TESTS (UID 0) - ${driver^^} STORAGE"
    
    # Test 1: Version check
    run_k8s_test \
        "Version Check" \
        "rootful" \
        "$driver" \
        "[\"--version\"]"
    
    # Test 2: Environment check
    run_k8s_test \
        "Environment Check" \
        "rootful" \
        "$driver" \
        "[\"check-environment\"]"
    
    # Test 3: Basic build from Git
    run_k8s_test \
        "Git Repository Build" \
        "rootful" \
        "$driver" \
        "[\"--context=https://github.com/nginxinc/docker-nginx.git\", \"--git-branch=master\", \"--dockerfile=mainline/alpine/Dockerfile\", \"--destination=test-k8s-rootful-git-${driver}:latest\", \"--storage-driver=${driver}\", \"--no-push\", \"--verbosity=debug\"]"
    
    # Test 4: Build with arguments from Git
    run_k8s_test \
        "Build with Arguments" \
        "rootful" \
        "$driver" \
        "[\"--context=https://github.com/nginxinc/docker-nginx.git\", \"--git-branch=master\", \"--dockerfile=mainline/alpine/Dockerfile\", \"--destination=test-k8s-rootful-buildargs-${driver}:latest\", \"--build-arg=NGINX_VERSION=1.25\", \"--storage-driver=${driver}\", \"--no-push\", \"--verbosity=debug\"]"
}

# ============================================================================
# Cleanup Function
# ============================================================================

cleanup() {
    if [ "$CLEANUP_AFTER" = true ]; then
        print_section "CLEANUP"
        
        echo "Deleting namespace: ${NAMESPACE}"
        kubectl delete namespace ${NAMESPACE} --force --grace-period=0 &> /dev/null || true
        
        echo "Removing temp files..."
        rm -f /tmp/smithy-job-*.yaml 2>/dev/null || true
        
        echo -e "${GREEN}✓ Cleanup completed${NC}"
    fi
}

# Cleanup function for interrupts
cleanup_on_interrupt() {
    echo ""
    echo -e "${YELLOW}Interrupted by user (Ctrl+C)${NC}"
    echo -e "${YELLOW}Cleaning up...${NC}"
    
    # Delete all test jobs
    kubectl delete jobs -n ${NAMESPACE} -l app=smithy-test --force --grace-period=0 &> /dev/null || true
    
    # Delete namespace
    kubectl delete namespace ${NAMESPACE} --force --grace-period=0 &> /dev/null || true
    
    # Remove temp files
    rm -f /tmp/smithy-job-*.yaml 2>/dev/null || true
    
    echo -e "${GREEN}✓ Cleanup completed${NC}"
    exit 130  # Standard exit code for SIGINT
}

# ============================================================================
# Main Execution
# ============================================================================

main() {
    print_section "KUBERNETES TEST SUITE"
    
    echo -e "${CYAN}Configuration:${NC}"
    echo -e "  Registry:       ${REGISTRY}"
    echo -e "  Image:          ${SMITHY_IMAGE}"
    echo -e "  Namespace:      ${NAMESPACE}"
    echo -e "  Storage:        ${STORAGE_DRIVER}"
    echo -e "  Cleanup:        ${CLEANUP_AFTER}"
    echo -e "  Job Timeout:    ${JOB_TIMEOUT}s"
    echo ""
    
    # Start overall timer
    local overall_start=$(date +%s)
    
    # Setup namespace
    setup_namespace
    
    # Determine which drivers to test
    local drivers=()
    if [ "$STORAGE_DRIVER" = "both" ]; then
        drivers=("vfs" "overlay")
    else
        drivers=("$STORAGE_DRIVER")
    fi
    
    # Run tests for each storage driver
    for driver in "${drivers[@]}"; do
        # Rootless tests
        run_rootless_tests "$driver"
        
        # Rootful tests
        run_rootful_tests "$driver"
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
        exit 1
    fi
    
    echo -e "${GREEN}✓ All Kubernetes tests passed successfully!${NC}"
    exit 0
}

# Trap cleanup on exit and interrupt
trap cleanup EXIT
trap cleanup_on_interrupt INT TERM

# Run main
main