#!/bin/bash
set -e

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if user-config.yaml exists
if [ ! -f "user-config.yaml" ]; then
    echo -e "${RED}Error: user-config.yaml not found${NC}"
    echo "Please copy user-config.yaml.example to user-config.yaml and configure it"
    exit 1
fi

# Parse YAML (requires yq - https://github.com/mikefarah/yq)
if ! command -v yq &> /dev/null; then
    echo -e "${RED}Error: yq is required but not installed${NC}"
    echo "Install: brew install yq (Mac) or see https://github.com/mikefarah/yq"
    exit 1
fi

# Function to show usage
usage() {
    echo "Usage: $0 <image-name> [options]"
    echo ""
    echo "Available images:"
    for dir in templates/*/; do
        echo "  - $(basename "$dir")"
    done
    echo ""
    echo "Options:"
    echo "  --dry-run    Show generated YAML without applying"
    echo "  --delete     Delete the job"
    echo "  --image <image:tag>          Override builder container image"
    echo ""
    echo "Registry format:"
    echo "  --registry <registry-url>    (e.g., docker.io, gcr.io, harbor.example.com)"    
    echo "Examples:"
    echo "  $0 alpine              # Build alpine image"
    echo "  $0 nginx --dry-run     # Show nginx manifest"
    echo "  $0 postgres --delete   # Delete postgres job"
    echo "  $0 postgres --image custom-builder:latest"
    echo "  $0 postgres --registry harbor.example.com --registry ghcr.io"
    echo ""

    exit 1
}

# Parse arguments
if [ $# -lt 1 ]; then
    usage
fi

IMAGE_NAME=$1
DRY_RUN=false
DELETE=false
OVERRIDE_IMAGE=""

# Arrays to store additional registries
declare -a ADDITIONAL_REGISTRIES

shift
while [[ $# -gt 0 ]]; do
    case $1 in
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --delete)
            DELETE=true
            shift
            # If next argument is not a flag, it's the image name
            if [[ $# -gt 0 ]] && [[ "$1" != --* ]]; then
                IMAGE_NAME="$1"
                shift
            fi            
            ;;
        --image)
            if [ $# -lt 2 ]; then
                echo -e "${RED}Error: --image requires an image:tag argument${NC}"
                exit 1
            fi
            shift
            OVERRIDE_IMAGE="$1"
            shift
            ;;
        --tag)
            if [ $# -lt 2 ]; then
                echo -e "${RED}Error: --tag requires a tag argument${NC}"
                exit 1
            fi
            shift
            IMAGE_TAG="$1"
            shift
            ;;            
        --registry)
            if [ $# -lt 2 ]; then
                echo -e "${RED}Error: --registry requires a registry URL argument${NC}"
                exit 1
            fi
            shift
            ADDITIONAL_REGISTRIES+=("$1")
            shift
            ;;                              
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            usage
            ;;
    esac
done

# For delete operation, if no image name specified, show available options
if [ "$DELETE" = true ] && [ -z "$IMAGE_NAME" ]; then
    echo -e "${RED}Error: --delete requires an image name${NC}"
    echo ""
    echo "Usage: $0 --delete <image-name>"
    echo "   or: $0 <image-name> --delete"
    echo ""
    echo "Available images:"
    for dir in templates/*/; do
        echo "  - $(basename "$dir")"
    done
    echo ""
    echo "Examples:"
    echo "  $0 --delete alpine"
    echo "  $0 alpine --delete"
    exit 1
fi

# For non-delete operations, image name is required
if [ -z "$IMAGE_NAME" ]; then
    echo -e "${RED}Error: Image name is required${NC}"
    usage
fi

# Apply tag if specified
if [ -n "$IMAGE_TAG" ]; then
    # Remove any existing tag from destination
    DESTINATION_NO_TAG=$(echo "$DESTINATION" | sed 's/:[^:]*$//')
    DESTINATION="${DESTINATION_NO_TAG}:${IMAGE_TAG}"
fi

# Handle delete
if [ "$DELETE" = true ]; then
    echo -e "${YELLOW}Deleting job...${NC}"
    kubectl delete job -l "image=${IMAGE_NAME}" -n "$NAMESPACE" 2>/dev/null || echo "No jobs found"
    exit 0
fi

# Check if template exists
TEMPLATE_DIR="templates/${IMAGE_NAME}"
if [ ! -d "$TEMPLATE_DIR" ]; then
    echo -e "${RED}Error: Template for '${IMAGE_NAME}' not found${NC}"
    echo "Available templates:"
    ls -1 templates/
    exit 1
fi

# Read user config
REGISTRY=$(yq '.registry' user-config.yaml)
NAMESPACE=$(yq '.namespace // "default"' user-config.yaml)
SECRET_NAME=$(yq '.auth.secretName // ""' user-config.yaml)
USERNAME=$(yq '.auth.username // ""' user-config.yaml)
PASSWORD=$(yq '.auth.password // ""' user-config.yaml)
EMAIL=$(yq '.auth.email // ""' user-config.yaml)
SERVER=$(yq '.auth.server // ""' user-config.yaml)

# Check for image-specific override
OVERRIDE_DEST=$(yq ".overrides.${IMAGE_NAME}.destination // \"\"" user-config.yaml)
if [ -n "$OVERRIDE_DEST" ] && [ "$OVERRIDE_DEST" != "null" ]; then
    DESTINATION="$OVERRIDE_DEST"
else
    DESTINATION="${REGISTRY}/${IMAGE_NAME}"
fi

# Apply tag if specified
if [ -n "$IMAGE_TAG" ]; then
    # Remove any existing tag from destination
    DESTINATION_NO_TAG=$(echo "$DESTINATION" | sed 's/:[^:]*$//')
    DESTINATION="${DESTINATION_NO_TAG}:${IMAGE_TAG}"
fi

echo -e "${GREEN}Building ${IMAGE_NAME}${NC}"
echo "Destination: ${DESTINATION}"
echo "Namespace: ${NAMESPACE}"

# Show overrides
if [ -n "$OVERRIDE_IMAGE" ]; then
    echo -e "${YELLOW}Builder image: ${OVERRIDE_IMAGE}${NC}"
fi

if [ -n "$IMAGE_TAG" ]; then
    echo -e "${YELLOW}Image tag: ${IMAGE_TAG}${NC}"
fi

if [ ${#ADDITIONAL_REGISTRIES[@]} -gt 0 ]; then
    echo -e "${YELLOW}Additional registries to configure:${NC}"
    for registry in "${ADDITIONAL_REGISTRIES[@]}"; do
        echo "  - ${registry}"
    done
fi

# Check if template has initContainers
INIT_CONTAINER_COUNT=$(yq eval '.spec.template.spec.initContainers | length' "$TMP_DIR/job.yaml" 2>/dev/null || echo "0")
if [ "$INIT_CONTAINER_COUNT" != "0" ] && [ "$INIT_CONTAINER_COUNT" != "null" ] && [ $INIT_CONTAINER_COUNT -gt 0 ]; then
    echo -e "${YELLOW}Template has ${INIT_CONTAINER_COUNT} initContainer(s) - volume mounts will be added to all${NC}"
fi

# Create temporary directory for kustomization
TMP_DIR=$(mktemp -d)
trap "rm -rf $TMP_DIR" EXIT

# Copy template
cp -r "$TEMPLATE_DIR"/* "$TMP_DIR/"

# Create or verify secret
if [ -n "$USERNAME" ] && [ "$USERNAME" != "null" ]; then
    echo -e "${YELLOW}Creating registry secret from credentials...${NC}"
    SECRET_NAME="registry-credentials-${IMAGE_NAME}"
    
    kubectl create secret docker-registry "$SECRET_NAME" \
        --docker-server="${SERVER:-$REGISTRY}" \
        --docker-username="$USERNAME" \
        --docker-password="$PASSWORD" \
        --docker-email="${EMAIL:-none@example.com}" \
        --namespace="$NAMESPACE" \
        --dry-run=client -o yaml | kubectl apply -f -
elif [ -n "$SECRET_NAME" ] && [ "$SECRET_NAME" != "null" ]; then
    echo -e "${YELLOW}Using existing secret: ${SECRET_NAME}${NC}"
    
    # Verify secret exists
    if ! kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" &> /dev/null; then
        echo -e "${RED}Error: Secret '${SECRET_NAME}' not found in namespace '${NAMESPACE}'${NC}"
        exit 1
    fi
else
    echo -e "${RED}Error: No authentication configured in user-config.yaml${NC}"
    echo "Please configure either auth.secretName or auth.username/password"
    exit 1
fi

# Create or update registries ConfigMap if registries are specified
if [ ${#ADDITIONAL_REGISTRIES[@]} -gt 0 ]; then
    echo -e "${YELLOW}Creating/updating registries ConfigMap...${NC}"

    # Build the unqualified-search-registries array
    SEARCH_REGISTRIES="\"docker.io\", \"quay.io\""
    for registry in "${ADDITIONAL_REGISTRIES[@]}"; do
        SEARCH_REGISTRIES+=", \"${registry}\""
    done
    
    # Start with default registries
    REGISTRIES_CONF="unqualified-search-registries = [${SEARCH_REGISTRIES}]

"
    
    # Add docker.io and quay.io by default
    REGISTRIES_CONF+="[[registry]]
location = \"docker.io\"

[[registry]]
location = \"quay.io\"

"
    
    # Add additional registries
    for registry in "${ADDITIONAL_REGISTRIES[@]}"; do
        REGISTRIES_CONF+="[[registry]]
location = \"${registry}\"

"
    done
    
    # Create ConfigMap (will overwrite if exists)
    kubectl create configmap registries-conf \
        --from-literal=registries.conf="$REGISTRIES_CONF" \
        --namespace="$NAMESPACE" \
        --dry-run=client -o yaml | kubectl apply -f -
    
    echo -e "${GREEN}ConfigMap 'registries-conf' created/updated${NC}"
fi

# Find the index of --destination argument using yq
echo -e "${YELLOW}Finding destination argument index...${NC}"
DEST_INDEX=$(yq eval '.spec.template.spec.containers[0].args | to_entries | .[] | select(.value | test("^--destination=")) | .key' "$TMP_DIR/job.yaml")

if [ -z "$DEST_INDEX" ]; then
    echo -e "${RED}Error: Could not find --destination argument in template${NC}"
    exit 1
fi

echo "Found --destination at index: ${DEST_INDEX}"

# Check if template has initContainers and find their destination indices
INIT_CONTAINER_COUNT=$(yq eval '.spec.template.spec.initContainers | length' "$TMP_DIR/job.yaml" 2>/dev/null || echo "0")
declare -a INIT_DEST_INDICES
declare -a INIT_DEST_VALUES

if [ "$INIT_CONTAINER_COUNT" != "0" ] && [ "$INIT_CONTAINER_COUNT" != "null" ] && [ $INIT_CONTAINER_COUNT -gt 0 ]; then
    echo -e "${YELLOW}Finding destination arguments in ${INIT_CONTAINER_COUNT} initContainer(s)...${NC}"
    
    for ((init_idx=0; init_idx<INIT_CONTAINER_COUNT; init_idx++)); do
        # Find destination index for this initContainer
        INIT_DEST_IDX=$(yq eval ".spec.template.spec.initContainers[${init_idx}].args | to_entries | .[] | select(.value | test(\"^--destination=\")) | .key" "$TMP_DIR/job.yaml")
        
        if [ -n "$INIT_DEST_IDX" ]; then
            # Get the current destination value (without --destination= prefix)
            CURRENT_DEST=$(yq eval ".spec.template.spec.initContainers[${init_idx}].args[${INIT_DEST_IDX}]" "$TMP_DIR/job.yaml" | sed 's/^--destination=//')
            
            # Strip any existing registry prefix (anything before first /)
            # This handles cases like: REGISTRY_PLACEHOLDER/spark:tag or ghcr.io/org/spark:tag
            CLEAN_DEST=$(echo "$CURRENT_DEST" | sed 's|^[^/]*/||')
            
            # If CLEAN_DEST is same as CURRENT_DEST, it means there was no / so use as-is
            if [ "$CLEAN_DEST" = "$CURRENT_DEST" ]; then
                # No slash found, destination is like "spark:tag"
                NEW_DEST="${REGISTRY}/${CURRENT_DEST}"
            else
                # Slash found, use cleaned path like "dockerfiles/spark:tag"
                NEW_DEST="${REGISTRY}/${CLEAN_DEST}"
            fi
            
            INIT_DEST_INDICES[$init_idx]=$INIT_DEST_IDX
            INIT_DEST_VALUES[$init_idx]=$NEW_DEST
            
            INIT_NAME=$(yq eval ".spec.template.spec.initContainers[${init_idx}].name" "$TMP_DIR/job.yaml")
            echo "  InitContainer[$init_idx] '${INIT_NAME}': ${CURRENT_DEST} -> ${NEW_DEST}"
        fi
    done
fi

# Prepare patches array
PATCHES=""

# Always patch destination
PATCHES+="    - op: replace
      path: /spec/template/spec/containers/0/args/${DEST_INDEX}
      value: --destination=${DESTINATION}
"

# Patch initContainer destinations
for init_idx in "${!INIT_DEST_INDICES[@]}"; do
    INIT_DEST_IDX=${INIT_DEST_INDICES[$init_idx]}
    INIT_DEST_VAL=${INIT_DEST_VALUES[$init_idx]}
    
    PATCHES+="    - op: replace
      path: /spec/template/spec/initContainers/${init_idx}/args/${INIT_DEST_IDX}
      value: --destination=${INIT_DEST_VAL}
"
done

# Patch builder image if specified
if [ -n "$OVERRIDE_IMAGE" ]; then
    PATCHES+="    - op: replace
      path: /spec/template/spec/containers/0/image
      value: ${OVERRIDE_IMAGE}
"
fi

# Add primary registry secret volume
PATCHES+="    - op: add
      path: /spec/template/spec/volumes
      value:
      - name: docker-config
        secret:
          secretName: ${SECRET_NAME}
"

# Add registries ConfigMap volume if registries were specified
if [ ${#ADDITIONAL_REGISTRIES[@]} -gt 0 ]; then
    PATCHES+="    - op: add
      path: /spec/template/spec/volumes/-
      value:
        name: registries-conf
        configMap:
          name: registries-conf
"
fi

# Add primary registry volumeMount
PATCHES+="    - op: add
      path: /spec/template/spec/containers/0/volumeMounts
      value:
      - name: docker-config
        mountPath: /home/kimia/.docker/config.json
        subPath: .dockerconfigjson
        readOnly: true
"

# Add registries ConfigMap volumeMount if registries were specified
if [ ${#ADDITIONAL_REGISTRIES[@]} -gt 0 ]; then
    PATCHES+="    - op: add
      path: /spec/template/spec/containers/0/volumeMounts/-
      value:
        name: registries-conf
        mountPath: /home/kimia/.config/containers/registries.conf
        subPath: registries.conf
        readOnly: true
"
fi

# Add volume mounts to initContainers if they exist
INIT_CONTAINER_COUNT=$(yq eval '.spec.template.spec.initContainers | length' "$TMP_DIR/job.yaml" 2>/dev/null || echo "0")
if [ "$INIT_CONTAINER_COUNT" != "0" ] && [ "$INIT_CONTAINER_COUNT" != "null" ] && [ $INIT_CONTAINER_COUNT -gt 0 ]; then
    echo -e "${YELLOW}Adding volume mounts to ${INIT_CONTAINER_COUNT} initContainer(s)...${NC}"
    
    # Loop through each initContainer
    for ((init_idx=0; init_idx<INIT_CONTAINER_COUNT; init_idx++)); do
        # Add primary registry volumeMount
        PATCHES+="    - op: add
      path: /spec/template/spec/initContainers/${init_idx}/volumeMounts
      value:
      - name: docker-config
        mountPath: /home/kimia/.docker/config.json
        subPath: .dockerconfigjson
        readOnly: true
"
        
        # Add registries ConfigMap volumeMount if registries were specified
        if [ ${#ADDITIONAL_REGISTRIES[@]} -gt 0 ]; then
            PATCHES+="    - op: add
      path: /spec/template/spec/initContainers/${init_idx}/volumeMounts/-
      value:
        name: registries-conf
        mountPath: /home/kimia/.config/containers/registries.conf
        subPath: registries.conf
        readOnly: true
"
        fi
    done
fi

# Get the job name from template for tracking
TEMPLATE_JOB_NAME=$(yq eval '.metadata.name' "$TMP_DIR/job.yaml")

# Create kustomization.yaml
cat > "$TMP_DIR/kustomization.yaml" <<EOF
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: ${NAMESPACE}

resources:
- job.yaml

# Add common labels to track jobs
commonLabels:
  app: kimia-builder
  image: ${IMAGE_NAME}

patches:
- patch: |-
${PATCHES}
  target:
    kind: Job
EOF

# Build and apply
if [ "$DRY_RUN" = true ]; then
    echo -e "${YELLOW}Generated manifest:${NC}"
    kubectl kustomize "$TMP_DIR"
else
    echo -e "${GREEN}Applying manifest...${NC}"
    kubectl apply -k "$TMP_DIR"
    
    # Get the job name (it will have a generated suffix)
    sleep 1
    JOB_NAME=$(kubectl get jobs -n "$NAMESPACE" -l "image=${IMAGE_NAME}" --sort-by=.metadata.creationTimestamp -o jsonpath='{.items[-1].metadata.name}' 2>/dev/null || echo "")
    
    if [ -n "$JOB_NAME" ]; then
        echo -e "${GREEN}Job created: ${JOB_NAME}${NC}"
        echo ""
        echo "Monitor progress with:"
        echo "  kubectl logs -f job/${JOB_NAME} -n ${NAMESPACE}"
        echo "  kubectl get job ${JOB_NAME} -n ${NAMESPACE} -w"
    fi
fi