#!/bin/bash
# Attestation Content Inspector
# Downloads and inspects SBOM and Provenance content from container images

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Check arguments
if [ $# -lt 1 ]; then
    echo "Usage: $0 <image> [output-dir]"
    echo ""
    echo "Example:"
    echo "  $0 100.92.16.57:5000/buildkit-rootless-attest-max-overlay:latest"
    echo "  $0 myregistry.io/myapp:v1 ./attestation-data"
    echo ""
    echo "Note: If output-dir is not provided, a temporary directory will be created and preserved."
    exit 1
fi

IMAGE="$1"

# Create output directory or use provided one
if [ -n "$2" ]; then
    OUTPUT_DIR="$2"
    mkdir -p "${OUTPUT_DIR}"
    TEMP_DIR_CREATED=false
else
    # Create temp directory that persists after script
    OUTPUT_DIR=$(mktemp -d -t attestation-inspect-XXXXXXXXXX)
    TEMP_DIR_CREATED=true
fi

echo -e "${CYAN}════════════════════════════════════════════════════════${NC}"
echo -e "${CYAN}  ATTESTATION CONTENT INSPECTOR${NC}"
echo -e "${CYAN}════════════════════════════════════════════════════════${NC}"
echo ""
echo "Image: ${IMAGE}"
echo "Output: ${OUTPUT_DIR}"
if [ "${TEMP_DIR_CREATED}" = true ]; then
    echo -e "${YELLOW}(Temporary directory created - will be preserved after inspection)${NC}"
fi
echo ""

# Check for required tools
check_tools() {
    local missing_tools=()
    
    # Check for jq (required)
    if ! command -v jq &> /dev/null; then
        missing_tools+=("jq")
    fi
    
    # Check for image inspection tools (need at least one)
    HAS_CRANE=false
    HAS_DOCKER=false
    HAS_SKOPEO=false
    
    if command -v crane &> /dev/null; then
        HAS_CRANE=true
    fi
    
    if command -v docker &> /dev/null; then
        HAS_DOCKER=true
    fi
    
    if command -v skopeo &> /dev/null; then
        HAS_SKOPEO=true
    fi
    
    if [ "$HAS_CRANE" = false ] && [ "$HAS_DOCKER" = false ] && [ "$HAS_SKOPEO" = false ]; then
        missing_tools+=("crane OR docker OR skopeo")
    fi
    
    if ! command -v cosign &> /dev/null; then
        echo -e "${YELLOW}Warning: cosign not found (optional)${NC}"
        echo ""
    fi
    
    if [ ${#missing_tools[@]} -gt 0 ]; then
        echo -e "${RED}Error: Missing required tools: ${missing_tools[*]}${NC}"
        exit 1
    fi
    
    # Set preferred tool
    if [ "$HAS_CRANE" = true ]; then
        TOOL="crane"
    elif [ "$HAS_DOCKER" = true ]; then
        TOOL="docker"
    else
        TOOL="skopeo"
    fi
    
    echo "Using tool: ${TOOL}"
    echo ""
}

# Helper: Check if registry is insecure (no TLS)
is_insecure_registry() {
    local image="$1"
    # Check if it's localhost or private IP
    if [[ "$image" =~ ^(localhost|127\.|10\.|172\.(1[6-9]|2[0-9]|3[01])\.|192\.168\.) ]]; then
        return 0
    fi
    return 1
}

# Helper: Fetch manifest using available tool
fetch_manifest() {
    local image="$1"
    local output="$2"
    
    local insecure_flag=""
    if is_insecure_registry "$image"; then
        insecure_flag="--insecure"
    fi
    
    case "$TOOL" in
        crane)
            crane manifest "$image" $insecure_flag > "$output" 2>/dev/null
            ;;
        docker)
            docker buildx imagetools inspect "$image" --raw > "$output" 2>/dev/null
            ;;
        skopeo)
            skopeo inspect --raw "docker://$image" --tls-verify=false > "$output" 2>/dev/null
            ;;
    esac
}

# Helper: Fetch manifest by digest
fetch_manifest_by_digest() {
    local image="$1"
    local digest="$2"
    local output="$3"
    
    local insecure_flag=""
    if is_insecure_registry "$image"; then
        insecure_flag="--insecure"
    fi
    
    # Extract registry and repo
    local registry=$(echo "$image" | cut -d/ -f1)
    local repo=$(echo "$image" | cut -d/ -f2- | cut -d: -f1 | cut -d@ -f1)
    
    case "$TOOL" in
        crane)
            crane manifest "${registry}/${repo}@${digest}" $insecure_flag > "$output" 2>/dev/null
            ;;
        docker)
            docker buildx imagetools inspect "${registry}/${repo}@${digest}" --raw > "$output" 2>/dev/null
            ;;
        skopeo)
            skopeo inspect --raw "docker://${registry}/${repo}@${digest}" --tls-verify=false > "$output" 2>/dev/null
            ;;
    esac
}

# Helper: Fetch blob using available tool
fetch_blob() {
    local image="$1"
    local digest="$2"
    local output="$3"
    
    local insecure_flag=""
    if is_insecure_registry "$image"; then
        insecure_flag="--insecure"
    fi
    
    # Extract registry and repo
    local registry=$(echo "$image" | cut -d/ -f1)
    local repo=$(echo "$image" | cut -d/ -f2- | cut -d: -f1 | cut -d@ -f1)
    
    case "$TOOL" in
        crane)
            crane blob "${registry}/${repo}@${digest}" $insecure_flag > "$output" 2>/dev/null
            ;;
        docker)
            docker buildx imagetools inspect "${registry}/${repo}@${digest}" --raw > "$output" 2>/dev/null
            ;;
        skopeo)
            skopeo inspect --raw "docker://${registry}/${repo}@${digest}" --tls-verify=false > "$output" 2>/dev/null
            ;;
    esac
}

echo "Inspecting attestation content for: ${IMAGE}"
echo ""

# Check required tools
check_tools

# Step 1: Fetch Image Manifest
echo -e "${CYAN}──────────────────────────────────────────────────────${NC}"
echo -e "${CYAN}1. Fetching Image Manifest${NC}"
echo -e "${CYAN}──────────────────────────────────────────────────────${NC}"

if fetch_manifest "${IMAGE}" "${OUTPUT_DIR}/manifest.json"; then
    echo -e "${GREEN}✓${NC} Manifest saved to: ${OUTPUT_DIR}/manifest.json"
    
    # Pretty print
    jq . "${OUTPUT_DIR}/manifest.json" > "${OUTPUT_DIR}/manifest-pretty.json" 2>/dev/null || true
    echo -e "${GREEN}✓${NC} Pretty manifest: ${OUTPUT_DIR}/manifest-pretty.json"
    echo ""
    
    # Count manifests
    MANIFEST_COUNT=$(jq '.manifests | length' "${OUTPUT_DIR}/manifest.json" 2>/dev/null || echo "0")
    echo "Total manifests: ${MANIFEST_COUNT}"
    echo ""
    
    # Show manifest types
    echo "Manifest types:"
    jq -r '.manifests[]? | "  - \(.mediaType // .MediaType)"' "${OUTPUT_DIR}/manifest.json" 2>/dev/null | head -10
    if [ -n "$(jq -r '.manifests[]? | select(.annotations."vnd.docker.reference.type" == "attestation-manifest")' "${OUTPUT_DIR}/manifest.json" 2>/dev/null)" ]; then
        jq -r '.manifests[]? | select(.annotations."vnd.docker.reference.type" == "attestation-manifest") | "  - \(.mediaType // .MediaType) [attestation-manifest]"' "${OUTPUT_DIR}/manifest.json" 2>/dev/null
    fi
else
    echo -e "${RED}✗${NC} Failed to fetch manifest"
    exit 1
fi
echo ""

# Step 2: Extract Attestation References
echo -e "${CYAN}──────────────────────────────────────────────────────${NC}"
echo -e "${CYAN}2. Extracting Attestation References${NC}"
echo -e "${CYAN}──────────────────────────────────────────────────────${NC}"

# Find attestation manifests
ATTESTATION_DIGESTS=$(jq -r '.manifests[]? | select(.annotations."vnd.docker.reference.type" == "attestation-manifest") | .digest' "${OUTPUT_DIR}/manifest.json" 2>/dev/null || echo "")

if [ -z "${ATTESTATION_DIGESTS}" ]; then
    echo -e "${YELLOW}⚠${NC} No attestation manifests found"
    echo ""
    echo "This could mean:"
    echo "  - Image was built without --attestation flag"
    echo "  - Attestations weren't pushed to registry"
    echo "  - Image doesn't support OCI attestations"
    exit 0
else
    ATTESTATION_COUNT=$(echo "${ATTESTATION_DIGESTS}" | wc -l)
    echo -e "${GREEN}✓${NC} Found ${ATTESTATION_COUNT} attestation manifest(s)"
    echo ""
    
    echo "Attestation digests:"
    echo "${ATTESTATION_DIGESTS}" | sed 's/^/  - /'
    echo "${ATTESTATION_DIGESTS}" > "${OUTPUT_DIR}/attestation-digests.txt"
fi
echo ""

# Step 3: Extract Attestation Content
echo -e "${CYAN}──────────────────────────────────────────────────────${NC}"
echo -e "${CYAN}3. Extracting Attestation Manifests and Layers${NC}"
echo -e "${CYAN}──────────────────────────────────────────────────────${NC}"
echo ""

# Download each attestation manifest
ATTESTATION_NUM=1
SBOM_FOUND=false
PROVENANCE_FOUND=false

for digest in ${ATTESTATION_DIGESTS}; do
    echo "Attestation #${ATTESTATION_NUM}: ${digest}"
    
    # Download attestation manifest
    echo "  Downloading manifest: ${digest}"
    
    # Fetch the attestation manifest (this is a manifest, not a blob!)
    if fetch_manifest_by_digest "${IMAGE}" "${digest}" "${OUTPUT_DIR}/attestation-${ATTESTATION_NUM}.json"; then
        echo -e "  ${GREEN}✓${NC} Attestation manifest downloaded"
        jq . "${OUTPUT_DIR}/attestation-${ATTESTATION_NUM}.json" > "${OUTPUT_DIR}/attestation-${ATTESTATION_NUM}-pretty.json" 2>/dev/null || true
        
        # Now extract the layer blobs from this attestation manifest
        echo "  Extracting attestation layers..."
        
        # Check if this manifest has layers
        LAYER_COUNT=$(jq '.layers // [] | length' "${OUTPUT_DIR}/attestation-${ATTESTATION_NUM}.json" 2>/dev/null || echo "0")
        
        if [ "$LAYER_COUNT" -gt 0 ]; then
            echo "  Found ${LAYER_COUNT} attestation layer(s)"
            
            # Extract each layer
            LAYER_NUM=1
            jq -c '.layers[]' "${OUTPUT_DIR}/attestation-${ATTESTATION_NUM}.json" 2>/dev/null | while read -r layer; do
                LAYER_DIGEST=$(echo "$layer" | jq -r '.digest')
                LAYER_SIZE=$(echo "$layer" | jq -r '.size')
                PREDICATE_TYPE=$(echo "$layer" | jq -r '.annotations."in-toto.io/predicate-type" // "unknown"')
                
                echo "    Layer ${LAYER_NUM}: ${LAYER_DIGEST}"
                echo "      Type: ${PREDICATE_TYPE}"
                echo "      Size: ${LAYER_SIZE} bytes"
                
                # Determine output filename based on predicate type
                case "$PREDICATE_TYPE" in
                    *"spdx.dev/Document"*)
                        OUTPUT_FILE="${OUTPUT_DIR}/sbom-layer-${ATTESTATION_NUM}-${LAYER_NUM}.json"
                        SBOM_FOUND=true
                        ;;
                    *"slsa.dev/provenance"*)
                        OUTPUT_FILE="${OUTPUT_DIR}/provenance-layer-${ATTESTATION_NUM}-${LAYER_NUM}.json"
                        PROVENANCE_FOUND=true
                        ;;
                    *)
                        OUTPUT_FILE="${OUTPUT_DIR}/attestation-${ATTESTATION_NUM}-layer-${LAYER_NUM}.json"
                        ;;
                esac
                
                # Download the layer blob
                if fetch_blob "${IMAGE}" "${LAYER_DIGEST}" "${OUTPUT_FILE}"; then
                    echo -e "      ${GREEN}✓${NC} Downloaded to: $(basename ${OUTPUT_FILE})"
                    
                    # Pretty print
                    jq . "${OUTPUT_FILE}" > "${OUTPUT_FILE%.json}-pretty.json" 2>/dev/null || true
                else
                    echo -e "      ${RED}✗${NC} Failed to download layer"
                fi
                
                LAYER_NUM=$((LAYER_NUM + 1))
            done
        else
            echo "  No layers found in this attestation manifest"
        fi
    else
        echo -e "  ${RED}✗${NC} Failed to download attestation manifest"
        touch "${OUTPUT_DIR}/attestation-${ATTESTATION_NUM}.json"
    fi
    
    ATTESTATION_NUM=$((ATTESTATION_NUM + 1))
    echo ""
done

# Step 4: Download with cosign (if available)
echo -e "${CYAN}──────────────────────────────────────────────────────${NC}"
echo -e "${CYAN}4. Downloading Attestation Content (using cosign)${NC}"
echo -e "${CYAN}──────────────────────────────────────────────────────${NC}"
echo ""

if command -v cosign &> /dev/null; then
    # Try to download SBOM
    echo "Attempting to download SBOM..."
    if cosign download sbom "${IMAGE}" > "${OUTPUT_DIR}/sbom-from-cosign.json" 2>/dev/null; then
        echo -e "${GREEN}✓${NC} SBOM downloaded"
        jq . "${OUTPUT_DIR}/sbom-from-cosign.json" > "${OUTPUT_DIR}/sbom-from-cosign-pretty.json" 2>/dev/null || true
    else
        echo -e "${YELLOW}⚠${NC} Could not download SBOM"
        touch "${OUTPUT_DIR}/sbom-from-cosign.json"
    fi
    echo ""
    
    # Try to download attestations
    echo "Attempting to download Attestations..."
    if cosign download attestation "${IMAGE}" > "${OUTPUT_DIR}/attestations-from-cosign.json" 2>/dev/null; then
        echo -e "${GREEN}✓${NC} Attestations downloaded"
        jq . "${OUTPUT_DIR}/attestations-from-cosign.json" > "${OUTPUT_DIR}/attestations-from-cosign-pretty.json" 2>/dev/null || true
    else
        echo -e "${YELLOW}⚠${NC} Could not download attestations"
        touch "${OUTPUT_DIR}/attestations-from-cosign.json"
    fi
else
    echo -e "${YELLOW}Cosign not available - skipping cosign download${NC}"
    touch "${OUTPUT_DIR}/sbom-from-cosign.json"
    touch "${OUTPUT_DIR}/attestations-from-cosign.json"
fi
echo ""

# Step 5: Show summary
echo -e "${CYAN}──────────────────────────────────────────────────────${NC}"
echo -e "${CYAN}5. Content Summary${NC}"
echo -e "${CYAN}──────────────────────────────────────────────────────${NC}"
echo ""
echo "Files generated in: ${OUTPUT_DIR}"
echo ""

# List all files with sizes
ls -lh "${OUTPUT_DIR}" | tail -n +2 | awk '{printf "  %-40s %s\n", $9, $5}'

echo ""
echo "To view content:"
echo "  cat ${OUTPUT_DIR}/manifest-pretty.json"
echo "  cat ${OUTPUT_DIR}/sbom-from-cosign-pretty.json"
echo "  cat ${OUTPUT_DIR}/attestations-from-cosign-pretty.json"
echo ""

# Step 6: Parse and show key findings
echo -e "${CYAN}──────────────────────────────────────────────────────${NC}"
echo -e "${CYAN}6. Key Findings${NC}"
echo -e "${CYAN}──────────────────────────────────────────────────────${NC}"
echo ""

# Check for SBOM layers
SBOM_FILES=$(find "${OUTPUT_DIR}" -name "sbom-layer-*.json" 2>/dev/null | head -1)
if [ -n "${SBOM_FILES}" ] && [ -f "${SBOM_FILES}" ]; then
    echo -e "${GREEN}✓${NC} SBOM (Software Bill of Materials)"
    
    # Extract package count
    PACKAGE_COUNT=$(jq '[.predicate.packages[]? // empty] | length' "${SBOM_FILES}" 2>/dev/null || echo "0")
    
    if [ "${PACKAGE_COUNT}" -gt 0 ]; then
        echo "  - Contains ${PACKAGE_COUNT} packages"
        echo "  - Top packages:"
        jq -r '.predicate.packages[]? | "    • \(.name) (\(.versionInfo // .version // "n/a"))"' "${SBOM_FILES}" 2>/dev/null | head -10
    else
        echo "  - No packages found in SBOM"
    fi
    echo ""
elif [ -s "${OUTPUT_DIR}/sbom-from-cosign.json" ]; then
    echo -e "${GREEN}✓${NC} SBOM (from cosign)"
    
    PACKAGE_COUNT=$(jq '[.packages[]? // empty] | length' "${OUTPUT_DIR}/sbom-from-cosign.json" 2>/dev/null || echo "")
    
    if [ -z "${PACKAGE_COUNT}" ] || ! [[ "${PACKAGE_COUNT}" =~ ^[0-9]+$ ]]; then
        echo "  - Contains unknown number of packages"
    else
        if [ "${PACKAGE_COUNT}" -gt 0 ]; then
            echo "  - Contains ${PACKAGE_COUNT} packages"
            echo "  - Top packages:"
            jq -r '.packages[]? | "    • \(.name // "unknown") (\(.version // "n/a"))"' "${OUTPUT_DIR}/sbom-from-cosign.json" 2>/dev/null | head -5
        else
            echo "  - No packages found in SBOM"
        fi
    fi
    echo ""
fi

# Check for Provenance layers
PROV_FILES=$(find "${OUTPUT_DIR}" -name "provenance-layer-*.json" 2>/dev/null | head -1)
if [ -n "${PROV_FILES}" ] && [ -f "${PROV_FILES}" ]; then
    echo -e "${GREEN}✓${NC} Provenance (Build Information)"
    
    # Extract build information
    BUILDER_ID=$(jq -r '.predicate.builder.id // "unknown"' "${PROV_FILES}" 2>/dev/null || echo "unknown")
    BUILD_TIME=$(jq -r '.predicate.metadata.buildFinishedOn // .predicate.metadata.buildStartedOn // "unknown"' "${PROV_FILES}" 2>/dev/null || echo "unknown")
    REPRODUCIBLE=$(jq -r '.predicate.metadata.reproducible // "unknown"' "${PROV_FILES}" 2>/dev/null || echo "unknown")
    
    echo "  - Builder: ${BUILDER_ID}"
    echo "  - Build time: ${BUILD_TIME}"
    echo "  - Reproducible: ${REPRODUCIBLE}"
    
    # Show build args if present
    BUILD_ARGS=$(jq -r '.predicate.buildConfig.buildArgs // empty | to_entries[] | "    • \(.key)=\(.value)"' "${PROV_FILES}" 2>/dev/null)
    if [ -n "${BUILD_ARGS}" ]; then
        echo "  - Build args:"
        echo "${BUILD_ARGS}"
    fi
    echo ""
elif [ -s "${OUTPUT_DIR}/attestations-from-cosign.json" ]; then
    echo -e "${GREEN}✓${NC} Provenance (from cosign)"
    
    if jq -e '.payload' "${OUTPUT_DIR}/attestations-from-cosign.json" &>/dev/null; then
        DECODED_PROVENANCE=$(jq -r '.payload' "${OUTPUT_DIR}/attestations-from-cosign.json" | base64 -d 2>/dev/null || echo "")
        
        if [ -n "${DECODED_PROVENANCE}" ]; then
            echo "  - Build tool: $(echo "${DECODED_PROVENANCE}" | jq -r '.predicate.builder.id // "unknown"' 2>/dev/null || echo "unknown")"
            echo "  - Build time: $(echo "${DECODED_PROVENANCE}" | jq -r '.predicate.metadata.buildFinishedOn // "unknown"' 2>/dev/null || echo "unknown")"
            echo "  - Reproducible: $(echo "${DECODED_PROVENANCE}" | jq -r '.predicate.metadata.reproducible // "unknown"' 2>/dev/null || echo "unknown")"
        fi
    fi
    echo ""
fi

# Detect attestation mode
ATTESTATION_MODE="UNKNOWN"

if [ "$SBOM_FOUND" = true ] && [ "$PROVENANCE_FOUND" = true ]; then
    ATTESTATION_MODE="MAX"
elif [ "$PROVENANCE_FOUND" = true ]; then
    ATTESTATION_MODE="MIN"
elif [ "$SBOM_FOUND" = true ]; then
    ATTESTATION_MODE="SBOM-ONLY"
fi

echo "Attestation Mode Detected:"
case "${ATTESTATION_MODE}" in
    "MAX")
        echo -e "  ${GREEN}MAX${NC} - Both SBOM and Provenance present"
        ;;
    "MIN")
        echo -e "  ${YELLOW}MIN${NC} - Only Provenance present"
        ;;
    "SBOM-ONLY")
        echo -e "  ${YELLOW}SBOM-ONLY${NC} - Only SBOM present"
        ;;
    *)
        echo -e "  ${YELLOW}UNKNOWN${NC} - Could not determine attestation mode"
        ;;
esac
echo ""

echo -e "${CYAN}════════════════════════════════════════════════════════${NC}"
echo -e "${CYAN}  Inspection Complete!${NC}"
echo -e "${CYAN}════════════════════════════════════════════════════════${NC}"
echo ""

if [ "${TEMP_DIR_CREATED}" = true ]; then
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${GREEN}  Temporary directory created and preserved:${NC}"
    echo -e "${GREEN}  ${OUTPUT_DIR}${NC}"
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    echo -e "${YELLOW}Note: This directory will NOT be automatically deleted.${NC}"
    echo -e "${YELLOW}To remove it when done:${NC}"
    echo -e "${YELLOW}  rm -rf ${OUTPUT_DIR}${NC}"
    echo ""
else
    echo "All content saved to: ${OUTPUT_DIR}"
fi

echo ""
echo "To view full SBOM:"
SBOM_LAYER=$(find "${OUTPUT_DIR}" -name "sbom-layer-*-pretty.json" 2>/dev/null | head -1)
if [ -n "${SBOM_LAYER}" ]; then
    echo "  jq . ${SBOM_LAYER} | less"
    echo "  # Or view packages:"
    echo "  jq '.predicate.packages[] | {name: .name, version: .versionInfo}' ${SBOM_LAYER} | less"
else
    echo "  jq . ${OUTPUT_DIR}/sbom-from-cosign-pretty.json | less"
fi
echo ""
echo "To view full Provenance:"
PROV_LAYER=$(find "${OUTPUT_DIR}" -name "provenance-layer-*-pretty.json" 2>/dev/null | head -1)
if [ -n "${PROV_LAYER}" ]; then
    echo "  jq . ${PROV_LAYER} | less"
else
    echo "  jq -r '.payload' ${OUTPUT_DIR}/attestations-from-cosign.json | base64 -d | jq . | less"
fi
echo ""