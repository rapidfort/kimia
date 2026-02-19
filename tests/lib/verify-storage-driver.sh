#!/bin/bash
# Unified Buildkit Storage Driver Verification
# Works for both Docker and Kubernetes modes
# Can be sourced as a function or run standalone
# Returns exit code: 0 for expected driver, 1 for unexpected/error

# Default configuration
VERIFY_MODE="${VERIFY_MODE:-auto}"              # docker, k8s, or auto
VERIFY_QUIET="${VERIFY_QUIET:-false}"           # true for minimal output
VERIFY_EXPECTED="${VERIFY_EXPECTED:-overlay}"   # overlay, native (buildkit), vfs (buildah), or any
VERIFY_NAMESPACE="${VERIFY_NAMESPACE:-default}" # k8s namespace
VERIFY_CONTAINER="${VERIFY_CONTAINER:-}"        # container/pod ID or name pattern
VERIFY_WAIT_TIMEOUT="${VERIFY_WAIT_TIMEOUT:-30}" # seconds to wait for buildkit/buildah
VERIFY_BUILDER="${VERIFY_BUILDER:-auto}"        # buildkit, buildah, or auto

# Color codes (disabled in quiet mode)
if [ "$VERIFY_QUIET" = "true" ]; then
    RED=''
    GREEN=''
    YELLOW=''
    NC=''
else
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    NC='\033[0m'
fi

# Log functions
log_info() {
    [ "$VERIFY_QUIET" = "false" ] && echo -e "${NC}$*${NC}"
}

log_success() {
    [ "$VERIFY_QUIET" = "false" ] && echo -e "${GREEN}✓ $*${NC}"
}

log_error() {
    echo -e "${RED}✗ $*${NC}" >&2
}

log_warning() {
    [ "$VERIFY_QUIET" = "false" ] && echo -e "${YELLOW}⚠ $*${NC}"
}

# Detect mode if auto
detect_mode() {
    if [ "$VERIFY_MODE" = "auto" ]; then
        if command -v kubectl &>/dev/null && kubectl cluster-info &>/dev/null; then
            VERIFY_MODE="k8s"
        elif command -v docker &>/dev/null && docker info &>/dev/null; then
            VERIFY_MODE="docker"
        else
            log_error "Cannot auto-detect mode. Neither kubectl nor docker available."
            return 1
        fi
    fi
    log_info "Mode: $VERIFY_MODE"
}

# Find container/pod
find_container() {
    local container_id=""

    if [ "$VERIFY_MODE" = "docker" ]; then
        if [ -n "$VERIFY_CONTAINER" ]; then
            # Try exact match first
            container_id=$(docker ps -q --filter "id=$VERIFY_CONTAINER" 2>/dev/null | head -1)

            # Try name pattern
            if [ -z "$container_id" ]; then
                container_id=$(docker ps -q --filter "name=$VERIFY_CONTAINER" 2>/dev/null | head -1)
            fi

            # Try image pattern
            if [ -z "$container_id" ]; then
                container_id=$(docker ps -q --filter "ancestor=$VERIFY_CONTAINER" 2>/dev/null | head -1)
            fi
        else
            # Find any kimia container
            container_id=$(docker ps -q --filter "ancestor=ghcr.io/rapidfort/kimia" 2>/dev/null | head -1)
            [ -z "$container_id" ] && container_id=$(docker ps -q --filter "name=kimia" 2>/dev/null | head -1)
        fi

        if [ -z "$container_id" ]; then
            log_error "No running container found"
            [ "$VERIFY_QUIET" = "false" ] && docker ps
            return 1
        fi

        VERIFY_CONTAINER="$container_id"
        log_info "Container: $(docker ps --filter "id=$container_id" --format '{{.Names}} ({{.ID}})')"

    elif [ "$VERIFY_MODE" = "k8s" ]; then
        if [ -n "$VERIFY_CONTAINER" ]; then
            # Try exact pod name
            if kubectl get pod "$VERIFY_CONTAINER" -n "$VERIFY_NAMESPACE" &>/dev/null; then
                container_id="$VERIFY_CONTAINER"
            else
                # Try label selector or name pattern
                container_id=$(kubectl get pods -n "$VERIFY_NAMESPACE" \
                    --field-selector=status.phase=Running \
                    -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
            fi
        else
            # Find any kimia pod
            container_id=$(kubectl get pods -n "$VERIFY_NAMESPACE" \
                -l app=kimia --field-selector=status.phase=Running \
                -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)

            [ -z "$container_id" ] && container_id=$(kubectl get pods -n "$VERIFY_NAMESPACE" \
                --field-selector=status.phase=Running \
                -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
        fi

        if [ -z "$container_id" ]; then
            log_error "No running pod found in namespace: $VERIFY_NAMESPACE"
            [ "$VERIFY_QUIET" = "false" ] && kubectl get pods -n "$VERIFY_NAMESPACE"
            return 1
        fi

        VERIFY_CONTAINER="$container_id"
        log_info "Pod: $container_id (namespace: $VERIFY_NAMESPACE)"
    fi

    return 0
}

# Execute command in container/pod
exec_in_container() {
    local cmd="$1"

    if [ "$VERIFY_MODE" = "docker" ]; then
        docker exec "$VERIFY_CONTAINER" sh -c "$cmd" 2>/dev/null
    elif [ "$VERIFY_MODE" = "k8s" ]; then
        kubectl exec -n "$VERIFY_NAMESPACE" "$VERIFY_CONTAINER" -- sh -c "$cmd" 2>/dev/null
    fi
}

# Detect builder (buildkit or buildah)
detect_builder() {
    if [ "$VERIFY_BUILDER" = "auto" ]; then
        log_info "Auto-detecting builder..."

        # Check for buildkitd process
        if exec_in_container "pgrep -x buildkitd" &>/dev/null; then
            VERIFY_BUILDER="buildkit"
            log_info "Detected: buildkit"
            return 0
        fi

        # Check for buildah process
        if exec_in_container "which buildah" &>/dev/null || \
           exec_in_container "test -f /usr/bin/buildah" &>/dev/null; then
            VERIFY_BUILDER="buildah"
            log_info "Detected: buildah"
            return 0
        fi

        # Default to buildkit if can't detect
        log_warning "Could not detect builder, defaulting to buildkit"
        VERIFY_BUILDER="buildkit"
        return 0
    else
        log_info "Builder: $VERIFY_BUILDER (manually specified)"
    fi
    return 0
}

# Wait for buildkitd/buildah to start
wait_for_builder() {
    local timeout=$VERIFY_WAIT_TIMEOUT
    local elapsed=0

    if [ "$VERIFY_BUILDER" = "buildkit" ]; then
        log_info "Waiting for buildkitd to start (timeout: ${timeout}s)..."

        while [ $elapsed -lt $timeout ]; do
            if exec_in_container "pgrep -x buildkitd" &>/dev/null; then
                log_success "buildkitd is running"
                return 0
            fi
            sleep 1
            elapsed=$((elapsed + 1))
        done

        log_error "buildkitd did not start within ${timeout}s"
        return 1
    else
        # For buildah, just check if buildah binary exists
        log_info "Checking for buildah..."
        if exec_in_container "which buildah" &>/dev/null; then
            log_success "buildah is available"
            return 0
        else
            log_error "buildah not found"
            return 1
        fi
    fi
}

# Check storage driver
check_storage_driver() {
    local found_overlay=false
    local found_native=false
    local found_vfs=false
    local storage_driver="unknown"
    local overlay_path=""
    local native_path=""
    local vfs_path=""
    local storage_base=""

    # Export detected driver for external use
    export DETECTED_STORAGE_DRIVER=""

    # Determine storage base directory based on builder
    if [ "$VERIFY_BUILDER" = "buildah" ]; then
        storage_base="/home/kimia/.local/share/containers/storage"
    else
        storage_base="/var/lib/buildkit"
    fi

    # Check for overlay using pattern matching (more flexible)
    overlay_dirs=$(exec_in_container "find $storage_base -maxdepth 2 -type d -name '*overlay*' 2>/dev/null" || echo "")
    if [ -n "$overlay_dirs" ]; then
        found_overlay=true
        storage_driver="overlay"
        # Use first match as primary path
        overlay_path=$(echo "$overlay_dirs" | head -1)
        [ "$VERIFY_QUIET" = "false" ] && log_info "  Found overlay directories: $(echo "$overlay_dirs" | tr '\n' ', ')"
    fi

    # Check for native using pattern matching (buildkit only)
    if [ "$VERIFY_BUILDER" = "buildkit" ]; then
        native_dirs=$(exec_in_container "find $storage_base -maxdepth 2 -type d -name '*native*' 2>/dev/null" || echo "")
        if [ -n "$native_dirs" ]; then
            found_native=true
            [ "$storage_driver" = "unknown" ] && storage_driver="native"
            # Use first match as primary path
            native_path=$(echo "$native_dirs" | head -1)
            [ "$VERIFY_QUIET" = "false" ] && log_info "  Found native directories: $(echo "$native_dirs" | tr '\n' ', ')"
        fi
    fi

    # Check for vfs using pattern matching (buildah only)
    if [ "$VERIFY_BUILDER" = "buildah" ]; then
        vfs_dirs=$(exec_in_container "find $storage_base -maxdepth 2 -type d -name '*vfs*' 2>/dev/null" || echo "")
        if [ -n "$vfs_dirs" ]; then
            found_vfs=true
            [ "$storage_driver" = "unknown" ] && storage_driver="vfs"
            # Use first match as primary path
            vfs_path=$(echo "$vfs_dirs" | head -1)
            [ "$VERIFY_QUIET" = "false" ] && log_info "  Found vfs directories: $(echo "$vfs_dirs" | tr '\n' ', ')"
        fi
    fi

    # Report findings
    if [ "$VERIFY_QUIET" = "false" ]; then
        log_info ""
        log_info "=== Storage Driver Detection (Builder: $VERIFY_BUILDER) ==="
        exec_in_container "ls -la $storage_base/" 2>/dev/null || true
        log_info ""
    fi

    # Determine result based on findings
    # Handle buildkit: overlay vs native
    if [ "$VERIFY_BUILDER" = "buildkit" ]; then
        if [ "$found_overlay" = true ] && [ "$found_native" = false ]; then
            storage_driver="overlay"
            DETECTED_STORAGE_DRIVER="overlay"

            log_success "Buildkit is using OVERLAY storage driver"
            [ "$VERIFY_QUIET" = "false" ] && log_info "  Location: $overlay_path"

            if [ "$VERIFY_EXPECTED" = "overlay" ] || [ "$VERIFY_EXPECTED" = "any" ]; then
                return 0
            else
                log_error "Expected: $VERIFY_EXPECTED, Found: overlay"
                return 1
            fi

        elif [ "$found_native" = true ] && [ "$found_overlay" = false ]; then
            storage_driver="native"
            DETECTED_STORAGE_DRIVER="native"

            if [ "$VERIFY_EXPECTED" = "native" ] || [ "$VERIFY_EXPECTED" = "any" ]; then
                log_success "Buildkit is using NATIVE storage driver"
                [ "$VERIFY_QUIET" = "false" ] && log_info "  Location: $native_path"
                return 0
            else
                log_error "Buildkit is using NATIVE storage driver (expected: $VERIFY_EXPECTED)"
                [ "$VERIFY_QUIET" = "false" ] && log_info "  Location: $native_path"
                return 1
            fi

        elif [ "$found_overlay" = true ] && [ "$found_native" = true ]; then
        log_warning "Both OVERLAY and NATIVE directories found"

        # Count snapshots to determine primary (look for snapshots subdirectory)
        local overlay_count=0
        local native_count=0

        # Try to count snapshots in overlay path
        if exec_in_container "test -d $overlay_path/snapshots" &>/dev/null; then
            overlay_count=$(exec_in_container "find $overlay_path/snapshots -mindepth 1 -maxdepth 1 -type d 2>/dev/null | wc -l" || echo "0")
        else
            # Maybe the path itself contains snapshots directly
            overlay_count=$(exec_in_container "find $overlay_path -mindepth 1 -maxdepth 1 -type d 2>/dev/null | wc -l" || echo "0")
        fi

        # Try to count snapshots in native path
        if exec_in_container "test -d $native_path/snapshots" &>/dev/null; then
            native_count=$(exec_in_container "find $native_path/snapshots -mindepth 1 -maxdepth 1 -type d 2>/dev/null | wc -l" || echo "0")
        else
            # Maybe the path itself contains snapshots directly
            native_count=$(exec_in_container "find $native_path -mindepth 1 -maxdepth 1 -type d 2>/dev/null | wc -l" || echo "0")
        fi

        if [ "$overlay_count" -gt "$native_count" ]; then
            storage_driver="overlay"
            log_info "  Primary: OVERLAY ($overlay_count snapshots vs $native_count native)"
        else
            storage_driver="native"
            log_info "  Primary: NATIVE ($native_count snapshots vs $overlay_count overlay)"
        fi

        DETECTED_STORAGE_DRIVER="$storage_driver"

        if [ "$VERIFY_EXPECTED" = "$storage_driver" ] || [ "$VERIFY_EXPECTED" = "any" ]; then
            return 0
        else
            log_error "Expected: $VERIFY_EXPECTED, Found: $storage_driver (mixed)"
            return 1
        fi

        else
            DETECTED_STORAGE_DRIVER="unknown"
            log_error "No snapshotter directory found for buildkit"
            [ "$VERIFY_QUIET" = "false" ] && log_info "  Expected: $storage_base/runc-overlayfs or $storage_base/runc-native"
            return 1
        fi

    # Handle buildah: overlay vs vfs
    elif [ "$VERIFY_BUILDER" = "buildah" ]; then
        if [ "$found_overlay" = true ] && [ "$found_vfs" = false ]; then
            storage_driver="overlay"
            DETECTED_STORAGE_DRIVER="overlay"

            log_success "Buildah is using OVERLAY storage driver"
            [ "$VERIFY_QUIET" = "false" ] && log_info "  Location: $overlay_path"
            [ "$VERIFY_QUIET" = "false" ] && log_info "  Note: Buildah overlay requires emptyDir at /home/kimia/.local"

            if [ "$VERIFY_EXPECTED" = "overlay" ] || [ "$VERIFY_EXPECTED" = "any" ]; then
                return 0
            else
                log_error "Expected: $VERIFY_EXPECTED, Found: overlay"
                return 1
            fi

        elif [ "$found_vfs" = true ] && [ "$found_overlay" = false ]; then
            storage_driver="vfs"
            DETECTED_STORAGE_DRIVER="vfs"

            if [ "$VERIFY_EXPECTED" = "vfs" ] || [ "$VERIFY_EXPECTED" = "any" ]; then
                log_success "Buildah is using VFS storage driver"
                [ "$VERIFY_QUIET" = "false" ] && log_info "  Location: $vfs_path"
                return 0
            else
                log_error "Buildah is using VFS storage driver (expected: $VERIFY_EXPECTED)"
                [ "$VERIFY_QUIET" = "false" ] && log_info "  Location: $vfs_path"
                [ "$VERIFY_QUIET" = "false" ] && log_info "  For buildah overlay, ensure emptyDir is mounted at /home/kimia/.local"
                return 1
            fi

        elif [ "$found_overlay" = true ] && [ "$found_vfs" = true ]; then
            log_warning "Both OVERLAY and VFS directories found"

            # Count items to determine primary
            local overlay_count=0
            local vfs_count=0

            if exec_in_container "test -d $overlay_path" &>/dev/null; then
                overlay_count=$(exec_in_container "find $overlay_path -mindepth 1 -maxdepth 1 -type d 2>/dev/null | wc -l" || echo "0")
            fi

            if exec_in_container "test -d $vfs_path" &>/dev/null; then
                vfs_count=$(exec_in_container "find $vfs_path -mindepth 1 -maxdepth 1 -type d 2>/dev/null | wc -l" || echo "0")
            fi

            if [ "$overlay_count" -gt "$vfs_count" ]; then
                storage_driver="overlay"
                log_info "  Primary: OVERLAY ($overlay_count items vs $vfs_count vfs)"
            else
                storage_driver="vfs"
                log_info "  Primary: VFS ($vfs_count items vs $overlay_count overlay)"
            fi

            DETECTED_STORAGE_DRIVER="$storage_driver"

            if [ "$VERIFY_EXPECTED" = "$storage_driver" ] || [ "$VERIFY_EXPECTED" = "any" ]; then
                return 0
            else
                log_error "Expected: $VERIFY_EXPECTED, Found: $storage_driver (mixed)"
                return 1
            fi

        else
            DETECTED_STORAGE_DRIVER="unknown"
            log_error "No storage directory found for buildah"
            [ "$VERIFY_QUIET" = "false" ] && log_info "  Expected: $storage_base/overlay or $storage_base/vfs"
            [ "$VERIFY_QUIET" = "false" ] && log_info "  Check if /home/kimia/.local is mounted (required for overlay)"
            return 1
        fi
    fi
}

# Main verification function
verify_storage_driver() {
    # Parse arguments if provided
    while [ $# -gt 0 ]; do
        case "$1" in
            --mode|-m)
                VERIFY_MODE="$2"
                shift 2
                ;;
            --builder|-b)
                VERIFY_BUILDER="$2"
                shift 2
                ;;
            --quiet|-q)
                VERIFY_QUIET="true"
                shift
                ;;
            --expected|-e)
                VERIFY_EXPECTED="$2"
                shift 2
                ;;
            --namespace|-n)
                VERIFY_NAMESPACE="$2"
                shift 2
                ;;
            --container|-c)
                VERIFY_CONTAINER="$2"
                shift 2
                ;;
            --wait|-w)
                VERIFY_WAIT_TIMEOUT="$2"
                shift 2
                ;;
            --help|-h)
                cat <<EOF
Usage: verify-storage-driver.sh [OPTIONS]

Verify buildkit/buildah storage driver in Docker or Kubernetes environments.

Options:
  -m, --mode MODE           Execution mode: docker, k8s, or auto (default: auto)
  -b, --builder BUILDER     Builder type: buildkit, buildah, or auto (default: auto)
  -e, --expected DRIVER     Expected driver (default: overlay)
                            - BuildKit: overlay, native, or any
                            - Buildah: overlay, vfs, or any
  -c, --container ID        Container ID/name (docker) or pod name (k8s)
  -n, --namespace NS        Kubernetes namespace (default: default)
  -w, --wait SECONDS        Timeout to wait for builder (default: 30)
  -q, --quiet               Minimal output mode
  -h, --help                Show this help

Environment Variables:
  VERIFY_MODE               Same as --mode
  VERIFY_BUILDER            Same as --builder
  VERIFY_EXPECTED           Same as --expected
  VERIFY_CONTAINER          Same as --container
  VERIFY_NAMESPACE          Same as --namespace
  VERIFY_WAIT_TIMEOUT       Same as --wait
  VERIFY_QUIET              Same as --quiet (true/false)

Exit Codes:
  0   Storage driver matches expected
  1   Storage driver does not match or error

Storage Drivers:
  BuildKit:
    - overlay: Fast, uses kernel overlay filesystem
    - native:  Default, chroot-based snapshotter

  Buildah:
    - overlay: Fast, requires emptyDir mount at /home/kimia/.local
    - vfs:     Default, copy-on-write with no mount requirements

Examples:
  # Auto-detect mode and builder, verify overlay
  ./verify-storage-driver.sh

  # Docker mode with buildkit
  ./verify-storage-driver.sh --mode docker --builder buildkit --expected overlay

  # Docker mode with buildah
  ./verify-storage-driver.sh --mode docker --builder buildah --expected overlay

  # Kubernetes mode with buildah, expect vfs
  ./verify-storage-driver.sh --mode k8s --builder buildah --expected vfs --namespace kimia

  # Quiet mode for test integration
  ./verify-storage-driver.sh --quiet --builder buildah --expected overlay

  # Use as a function (source it first)
  source verify-storage-driver.sh
  VERIFY_MODE=docker VERIFY_BUILDER=buildah VERIFY_EXPECTED=overlay verify_storage_driver
EOF
                return 0
                ;;
            *)
                log_error "Unknown option: $1"
                return 1
                ;;
        esac
    done

    # Execute verification steps
    detect_mode || return 1
    find_container || return 1
    detect_builder || return 1
    wait_for_builder || return 1
    check_storage_driver

    return $?
}

# If script is executed (not sourced), run main function
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
    verify_storage_driver "$@"
    exit $?
fi
