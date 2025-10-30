# Attestation & Signing

Complete guide to generating attestations and signing container images with Kimia.

> **📌 Important**: Kimia runs as a Kubernetes Pod or Job, NOT as a standalone CLI binary. All examples in this document show the `args` array that should be used in your Kubernetes Pod/Job specifications. When running outside Kubernetes, use `docker run ghcr.io/rapidfort/kimia:latest [args]`.

---

## Table of Contents

- [Overview](#overview)
- [Attestation Modes](#attestation-modes)
  - [Level 1: Simple Mode](#level-1-simple-mode)
  - [Level 2: Advanced Mode](#level-2-advanced-mode)
  - [Level 3: Pass-Through Mode](#level-3-pass-through-mode)
- [Signing with Cosign](#signing-with-cosign)
- [Complete Workflow](#complete-workflow)
- [Verification](#verification)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)

---

## Overview

Kimia is a Kubernetes-native container image builder that runs as a Pod or Job. It supports two major security features:

### **Attestations**
Cryptographically-signed metadata about your build:
- **SBOM (Software Bill of Materials)**: Complete inventory of all packages and dependencies in your image
- **Provenance (Build Information)**: Details about how, when, and where the image was built

### **Signing**
Cryptographic signatures on the entire image artifact using [Sigstore Cosign](https://github.com/sigstore/cosign).

**Key Point**: When you sign an image that contains attestations, the signature protects both the image layers AND the attestations, ensuring end-to-end integrity.

---

## Running Kimia

Kimia runs as a Kubernetes Pod or Job. All build parameters are passed as container `args`:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: kimia-build
spec:
  restartPolicy: Never
  containers:
    - name: kimia
      image: ghcr.io/rapidfort/kimia:latest
      args:
        - --context=https://github.com/myorg/myapp.git
        - --destination=registry.io/myapp:v1
        - --attestation=max
```

**Note**: There is no `kimia` binary to execute locally. All examples in this documentation show the `args` array that should be used in your Pod/Job specification.

---

## Attestation Modes

Kimia provides three levels of attestation configuration, from simple to advanced:

### Level 1: Simple Mode

Use `--attestation` flag for quick, predefined configurations.

#### Options

| Mode | SBOM | Provenance | Details |
|------|------|------------|---------|
| `off` | ❌ | ❌ | No attestations (default) |
| `min` | ❌ | ✅ | Provenance only, minimal information |
| `max` | ✅ | ✅ | Full SBOM + detailed provenance |

#### Examples

```yaml
# No attestations (default)
apiVersion: v1
kind: Pod
metadata:
  name: build-no-attest
spec:
  restartPolicy: Never
  containers:
    - name: kimia
      image: ghcr.io/rapidfort/kimia:latest
      args:
        - --context=https://github.com/myorg/myapp.git
        - --destination=myapp:v1

---
# Minimal provenance only
apiVersion: v1
kind: Pod
metadata:
  name: build-min-attest
spec:
  restartPolicy: Never
  containers:
    - name: kimia
      image: ghcr.io/rapidfort/kimia:latest
      args:
        - --context=https://github.com/myorg/myapp.git
        - --destination=myapp:v1
        - --attestation=min

---
# Full attestations (recommended)
apiVersion: v1
kind: Pod
metadata:
  name: build-max-attest
spec:
  restartPolicy: Never
  containers:
    - name: kimia
      image: ghcr.io/rapidfort/kimia:latest
      args:
        - --context=https://github.com/myorg/myapp.git
        - --destination=myapp:v1
        - --attestation=max
```

---

### Level 2: Advanced Mode

Use `--attest` flag for fine-grained control over attestation generation.

#### SBOM Attestations

```bash
--attest type=sbom[,param=value...]
```

**Parameters:**

| Parameter | Description | Default |
|-----------|-------------|---------|
| `generator=IMAGE` | Custom SBOM scanner image | BuildKit default |
| `scan-context=true` | Include build context in scan | `false` |
| `scan-stage=true` | Scan all build stages | `false` |

**Examples:**

```yaml
# Basic SBOM
apiVersion: v1
kind: Pod
metadata:
  name: build-with-sbom
spec:
  restartPolicy: Never
  containers:
    - name: kimia
      image: ghcr.io/rapidfort/kimia:latest
      args:
        - --context=https://github.com/myorg/myapp.git
        - --destination=myapp:v1
        - --attest
        - type=sbom

---
# SBOM with custom scanner
apiVersion: v1
kind: Pod
metadata:
  name: build-custom-scanner
spec:
  restartPolicy: Never
  containers:
    - name: kimia
      image: ghcr.io/rapidfort/kimia:latest
      args:
        - --context=https://github.com/myorg/myapp.git
        - --destination=myapp:v1
        - --attest
        - type=sbom,generator=aquasec/trivy:latest

---
# Comprehensive SBOM scan
apiVersion: v1
kind: Pod
metadata:
  name: build-comprehensive-sbom
spec:
  restartPolicy: Never
  containers:
    - name: kimia
      image: ghcr.io/rapidfort/kimia:latest
      args:
        - --context=https://github.com/myorg/myapp.git
        - --destination=myapp:v1
        - --attest
        - type=sbom,scan-context=true,scan-stage=true
```

#### Provenance Attestations

```bash
--attest type=provenance[,param=value...]
```

**Parameters:**

| Parameter | Description | Default |
|-----------|-------------|---------|
| `mode=min\|max` | Detail level | `max` |
| `builder-id=ID` | SLSA Builder ID (URL) | Auto-generated |
| `reproducible=true` | Mark build as reproducible | `false` |
| `version=v0.2\|v1` | SLSA provenance version | `v0.2` |
| `inline-only=true` | Only inline exporters | `false` |
| `filename=NAME` | Output filename | Auto-generated |

**Examples:**

```yaml
# Basic provenance
apiVersion: v1
kind: Pod
metadata:
  name: build-with-provenance
spec:
  restartPolicy: Never
  containers:
    - name: kimia
      image: ghcr.io/rapidfort/kimia:latest
      args:
        - --context=https://github.com/myorg/myapp.git
        - --destination=myapp:v1
        - --attest
        - type=provenance

---
# Provenance with builder ID
apiVersion: v1
kind: Pod
metadata:
  name: build-provenance-builder-id
spec:
  restartPolicy: Never
  containers:
    - name: kimia
      image: ghcr.io/rapidfort/kimia:latest
      args:
        - --context=https://github.com/myorg/myapp.git
        - --destination=myapp:v1
        - --attest
        - type=provenance,mode=max,builder-id=https://github.com/org/repo

---
# Minimal provenance
apiVersion: v1
kind: Pod
metadata:
  name: build-minimal-provenance
spec:
  restartPolicy: Never
  containers:
    - name: kimia
      image: ghcr.io/rapidfort/kimia:latest
      args:
        - --context=https://github.com/myorg/myapp.git
        - --destination=myapp:v1
        - --attest
        - type=provenance,mode=min

---
# Both SBOM and provenance
apiVersion: v1
kind: Pod
metadata:
  name: build-full-attestations
spec:
  restartPolicy: Never
  containers:
    - name: kimia
      image: ghcr.io/rapidfort/kimia:latest
      args:
        - --context=https://github.com/myorg/myapp.git
        - --destination=myapp:v1
        - --attest
        - type=sbom,scan-context=true
        - --attest
        - type=provenance,mode=max
```

---

### Level 3: Pass-Through Mode

Use `--buildkit-opt` to pass raw BuildKit attestation options directly.

```yaml
# Direct BuildKit options
apiVersion: v1
kind: Pod
metadata:
  name: build-buildkit-opts
spec:
  restartPolicy: Never
  containers:
    - name: kimia
      image: ghcr.io/rapidfort/kimia:latest
      args:
        - --context=https://github.com/myorg/myapp.git
        - --destination=myapp:v1
        - --buildkit-opt
        - attest:sbom=true
        - --buildkit-opt
        - attest:provenance=mode=max

---
# Custom attestation format
apiVersion: v1
kind: Pod
metadata:
  name: build-custom-attestation
spec:
  restartPolicy: Never
  containers:
    - name: kimia
      image: ghcr.io/rapidfort/kimia:latest
      args:
        - --context=https://github.com/myorg/myapp.git
        - --destination=myapp:v1
        - --buildkit-opt
        - attest:sbom=generator=custom-scanner:v1,scan-context=true
```

**Note**: Cannot mix `--attestation` with `--attest`. If both are specified, `--attest` takes precedence.

---

## Signing with Cosign

Kimia integrates with [Sigstore Cosign](https://github.com/sigstore/cosign) to sign container images and attestations.

### Prerequisites

1. **Generate a cosign key pair** (one-time setup):

```bash
# Generate keys
cosign generate-key-pair

# Output:
# Private key written to cosign.key
# Public key written to cosign.pub
```

2. **Create Kubernetes Secret with the cosign key and password**:

```bash
# Create secret from files
kubectl create secret generic cosign-keys \
  --from-file=cosign.key=./cosign.key \
  --from-literal=password=your-secret-password
```

### Signing Images

Mount the cosign key and password as volumes/environment variables in your Pod:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: build-and-sign
spec:
  restartPolicy: Never
  containers:
    - name: kimia
      image: ghcr.io/rapidfort/kimia:latest
      args:
        - --context=https://github.com/myorg/myapp.git
        - --destination=registry.io/myapp:v1
        - --sign
        - --cosign-key=/secrets/cosign.key
        - --cosign-password-env=COSIGN_PASSWORD
      env:
        - name: COSIGN_PASSWORD
          valueFrom:
            secretKeyRef:
              name: cosign-keys
              key: password
      volumeMounts:
        - name: cosign-key
          mountPath: /secrets
          readOnly: true
  volumes:
    - name: cosign-key
      secret:
        secretName: cosign-keys
        items:
          - key: cosign.key
            path: cosign.key
```

### Signing with Attestations

When you sign an image that includes attestations, the signature protects the entire artifact:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: build-attest-and-sign
spec:
  restartPolicy: Never
  containers:
    - name: kimia
      image: ghcr.io/rapidfort/kimia:latest
      args:
        - --context=https://github.com/myorg/myapp.git
        - --destination=registry.io/myapp:v1
        - --attestation=max
        - --sign
        - --cosign-key=/secrets/cosign.key
        - --cosign-password-env=COSIGN_PASSWORD
      env:
        - name: COSIGN_PASSWORD
          valueFrom:
            secretKeyRef:
              name: cosign-keys
              key: password
      volumeMounts:
        - name: cosign-key
          mountPath: /secrets
          readOnly: true
  volumes:
    - name: cosign-key
      secret:
        secretName: cosign-keys
```

**What gets signed:**
- ✅ Image manifest list
- ✅ Image layers
- ✅ Attestation manifests (SBOM + Provenance)
- ✅ All metadata

### Signing Options

| Flag | Description |
|------|-------------|
| `--sign` | Enable cosign signing |
| `--cosign-key PATH` | Path to cosign private key |
| `--cosign-password-env VAR` | Environment variable with key password |

### Insecure Registries

For development/testing with insecure registries:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: build-insecure-registry
spec:
  restartPolicy: Never
  containers:
    - name: kimia
      image: ghcr.io/rapidfort/kimia:latest
      args:
        - --context=https://github.com/myorg/myapp.git
        - --destination=localhost:5000/myapp:v1
        - --insecure
        - --attestation=max
        - --sign
        - --cosign-key=/secrets/cosign.key
        - --cosign-password-env=COSIGN_PASSWORD
      env:
        - name: COSIGN_PASSWORD
          valueFrom:
            secretKeyRef:
              name: cosign-keys
              key: password
      volumeMounts:
        - name: cosign-key
          mountPath: /secrets
          readOnly: true
  volumes:
    - name: cosign-key
      secret:
        secretName: cosign-keys
```

---

## Complete Workflow

### Example 1: Local Development (Kubernetes Job)

```yaml
# 1. Create cosign secret (one-time setup)
# kubectl create secret generic cosign-keys \
#   --from-file=cosign.key=./cosign.key \
#   --from-file=cosign.pub=./cosign.pub \
#   --from-literal=password=dev-password

# 2. Create Job to build with attestations and signing
apiVersion: batch/v1
kind: Job
metadata:
  name: kimia-build-dev
spec:
  template:
    spec:
      restartPolicy: Never
      containers:
        - name: kimia
          image: ghcr.io/rapidfort/kimia:latest
          args:
            - --context=https://github.com/myorg/myapp.git
            - --destination=localhost:5000/myapp:latest
            - --attestation=max
            - --sign
            - --cosign-key=/secrets/cosign.key
            - --cosign-password-env=COSIGN_PASSWORD
            - --insecure
          env:
            - name: COSIGN_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: cosign-keys
                  key: password
          volumeMounts:
            - name: cosign-key
              mountPath: /secrets
              readOnly: true
      volumes:
        - name: cosign-key
          secret:
            secretName: cosign-keys

# 3. Verify signature (run from your local machine)
# cosign verify --key cosign.pub \
#   --insecure-ignore-tlog \
#   --allow-insecure-registry \
#   localhost:5000/myapp:latest

# 4. Inspect attestations (run from your local machine)
# crane manifest localhost:5000/myapp:latest | jq .
```

### Example 2: CI/CD Pipeline (Creating Kubernetes Job)

```yaml
# kimia-job-template.yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: kimia-build-${CI_COMMIT_SHA}
  labels:
    app: kimia-builder
    commit: ${CI_COMMIT_SHA}
spec:
  backoffLimit: 0
  template:
    spec:
      restartPolicy: Never
      containers:
        - name: kimia
          image: ghcr.io/rapidfort/kimia:latest
          args:
            - --context=https://github.com/myorg/myapp.git
            - --git-branch=${GIT_BRANCH}
            - --destination=registry.io/myapp:${CI_COMMIT_SHA}
            - --reproducible
            - --attestation=max
            - --sign
            - --cosign-key=/secrets/cosign.key
            - --cosign-password-env=COSIGN_PASSWORD
            - --build-arg
            - VERSION=${CI_COMMIT_SHA}
            - --label
            - org.opencontainers.image.source=${CI_PROJECT_URL}
            - --label
            - org.opencontainers.image.revision=${CI_COMMIT_SHA}
          env:
            - name: COSIGN_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: cosign-keys
                  key: password
            - name: SOURCE_DATE_EPOCH
              value: "${SOURCE_DATE_EPOCH}"
          volumeMounts:
            - name: cosign-key
              mountPath: /secrets
              readOnly: true
      volumes:
        - name: cosign-key
          secret:
            secretName: cosign-keys
```

**CI/CD Script (GitLab CI example):**

```yaml
build-and-sign:
  stage: build
  image: bitnami/kubectl:latest
  script:
    # Get commit timestamp for reproducible builds
    - export SOURCE_DATE_EPOCH=$(git log -1 --format=%ct)
    - export GIT_BRANCH=${CI_COMMIT_REF_NAME}
    
    # Create job from template
    - envsubst < kimia-job-template.yaml | kubectl apply -f -
    
    # Wait for job to complete
    - kubectl wait --for=condition=complete --timeout=600s job/kimia-build-${CI_COMMIT_SHA}
    
    # Verify signature
    - cosign verify --key cosign.pub registry.io/myapp:${CI_COMMIT_SHA}
    
    - echo "✅ Built and signed: registry.io/myapp:${CI_COMMIT_SHA}"
  only:
    - main
```

### Example 3: Custom SBOM Scanner

```yaml
# Use Trivy for SBOM generation
apiVersion: v1
kind: Pod
metadata:
  name: build-with-trivy
spec:
  restartPolicy: Never
  containers:
    - name: kimia
      image: ghcr.io/rapidfort/kimia:latest
      args:
        - --context=https://github.com/myorg/myapp.git
        - --destination=registry.io/myapp:v1
        - --attest
        - type=sbom,generator=aquasec/trivy:latest,scan-context=true
        - --attest
        - type=provenance,mode=max
        - --sign
        - --cosign-key=/secrets/cosign.key
        - --cosign-password-env=COSIGN_PASSWORD
      env:
        - name: COSIGN_PASSWORD
          valueFrom:
            secretKeyRef:
              name: cosign-keys
              key: password
      volumeMounts:
        - name: cosign-key
          mountPath: /secrets
          readOnly: true
  volumes:
    - name: cosign-key
      secret:
        secretName: cosign-keys
```

---

## Verification

### Verify Image Signature

```bash
# Verify with cosign
cosign verify --key cosign.pub registry.io/myapp:v1

# For insecure registries
cosign verify --key cosign.pub \
  --insecure-ignore-tlog \
  --allow-insecure-registry \
  localhost:5000/myapp:v1
```

**Successful verification output:**
```json
{
  "critical": {
    "identity": {
      "docker-reference": "registry.io/myapp:v1"
    },
    "image": {
      "docker-manifest-digest": "sha256:abc123..."
    },
    "type": "https://sigstore.dev/cosign/sign/v1"
  }
}
```

### Inspect Attestation Content

Kimia uses **BuildKit OCI attestations** (not cosign attestations), so you need OCI-aware tools:

#### Using crane (recommended)

```bash
# View manifest list
crane manifest registry.io/myapp:v1

# Extract attestation manifest digest
ATTEST_DIGEST=$(crane manifest registry.io/myapp:v1 | \
  jq -r '.manifests[] | select(.annotations."vnd.docker.reference.type" == "attestation-manifest") | .digest')

# View attestation details
crane manifest "registry.io/myapp@${ATTEST_DIGEST}"
```

#### Using docker buildx

```bash
# View image attestations
docker buildx imagetools inspect registry.io/myapp:v1 --format '{{json .}}'
```

#### Manual extraction

```bash
# Pull attestation layers
crane pull registry.io/myapp:v1 /tmp/image.tar

# Extract and view SBOM
tar -xf /tmp/image.tar -C /tmp/extracted
jq . /tmp/extracted/sbom.json

# Extract and view provenance
jq . /tmp/extracted/provenance.json
```

### Attestation Structure

BuildKit attestations are stored as OCI manifest references:

```json
{
  "manifests": [
    {
      "digest": "sha256:abc123...",
      "platform": { "architecture": "amd64", "os": "linux" }
    },
    {
      "digest": "sha256:def456...",
      "annotations": {
        "vnd.docker.reference.type": "attestation-manifest"
      },
      "platform": { "architecture": "unknown", "os": "unknown" }
    }
  ]
}
```

The attestation manifest contains layers with:
- **SBOM**: `in-toto.io/predicate-type: https://spdx.dev/Document`
- **Provenance**: `in-toto.io/predicate-type: https://slsa.dev/provenance/v0.2`

---

## Best Practices

### ✅ DO

1. **Always use `--attestation=max` in production**
   ```yaml
   args:
     - --context=https://github.com/myorg/myapp.git
     - --destination=myapp:v1
     - --attestation=max
   ```

2. **Sign all production images**
   ```yaml
   args:
     - --context=https://github.com/myorg/myapp.git
     - --destination=myapp:v1
     - --attestation=max
     - --sign
     - --cosign-key=/secrets/cosign.key
   ```

3. **Use reproducible builds with attestations**
   ```yaml
   env:
     - name: SOURCE_DATE_EPOCH
       value: "1609459200"  # From git log -1 --format=%ct
   args:
     - --context=https://github.com/myorg/myapp.git
     - --destination=myapp:v1
     - --reproducible
     - --attestation=max
   ```

4. **Store cosign keys securely**
   - Use Kubernetes Secrets for keys
   - Use secret management systems (Vault, AWS Secrets Manager, etc.)
   - Never commit keys to version control
   - Rotate keys periodically

5. **Include builder identity in provenance**
   ```yaml
   args:
     - --context=https://github.com/myorg/myapp.git
     - --destination=myapp:v1
     - --attest
     - type=provenance,builder-id=https://github.com/org/repo
   ```

6. **Tag images with digests for verification**
   ```yaml
   args:
     - --context=https://github.com/myorg/myapp.git
     - --destination=myapp:v1
     - --image-name-with-digest-file=/output/image-ref.txt
   ```
   ```bash
   # Use digest reference for verification
   IMAGE=$(cat image-ref.txt)
   cosign verify --key cosign.pub "${IMAGE}"
   ```

### ❌ DON'T

1. **Don't use `--attestation=off` in production**
   - Always generate attestations for supply chain security

2. **Don't skip signing in production**
   - Unsigned images cannot prove authenticity

3. **Don't use tags for verification**
   ```bash
   # ❌ BAD: Tags are mutable
   cosign verify --key cosign.pub myapp:latest
   
   # ✅ GOOD: Use digests
   cosign verify --key cosign.pub myapp@sha256:abc123...
   ```

4. **Don't mix attestation modes**
   ```yaml
   # ❌ BAD: Don't use both --attestation and --attest
   args:
     - --attestation=max
     - --attest
     - type=sbom
   
   # ✅ GOOD: Use one or the other
   args:
     - --attestation=max
   ```

5. **Don't hardcode passwords**
   ```yaml
   # ❌ BAD: Hardcoded password
   env:
     - name: COSIGN_PASSWORD
       value: "hardcoded-password"
   
   # ✅ GOOD: Use Kubernetes Secrets
   env:
     - name: COSIGN_PASSWORD
       valueFrom:
         secretKeyRef:
           name: cosign-keys
           key: password
   ```

---

## Troubleshooting

### Issue: "No signatures found"

**Problem:**
```bash
$ cosign verify --key cosign.pub myapp:latest
Error: no signatures found
```

**Solutions:**

1. **Check if you actually signed the image:**
   ```yaml
   args:
     - --context=https://github.com/myorg/myapp.git
     - --destination=myapp:latest
     - --sign
     - --cosign-key=/secrets/cosign.key
   ```

2. **Verify you're using the correct reference:**
   ```bash
   # Try with digest instead of tag
   cosign verify --key cosign.pub myapp@sha256:abc123...
   ```

3. **Check for insecure registry:**
   ```bash
   cosign verify --key cosign.pub \
     --allow-insecure-registry \
     localhost:5000/myapp:latest
   ```

---

### Issue: "No matching attestations"

**Problem:**
```bash
$ cosign verify-attestation --key cosign.pub --type slsaprovenance myapp:latest
Error: none of the attestations matched the predicate type: slsaprovenance
```

**Explanation:**

This is **expected behavior**. Kimia uses **BuildKit OCI attestations**, not **cosign attestations**.

`cosign verify-attestation` only works with attestations created by `cosign attest`, not BuildKit's native attestations.

**Solution:**

Use OCI-aware tools to inspect BuildKit attestations:

```bash
# View attestations with crane
crane manifest myapp:latest | jq '.manifests[] | select(.annotations."vnd.docker.reference.type" == "attestation-manifest")'

# Or use docker buildx
docker buildx imagetools inspect myapp:latest
```

**Verification that works:**

When you sign an image with BuildKit attestations, the signature protects the entire manifest list (including attestations):

```bash
# This verifies the image AND attestations are signed
cosign verify --key cosign.pub myapp:latest
```

---

### Issue: "signing with tag, not digest"

**Problem:**
```bash
WARNING: Image reference uses a tag, not a digest
```

**Solution:**

This warning is informational. Kimia automatically:
1. Extracts the manifest list digest after build
2. Signs using the digest reference (not the tag)

You can verify this by checking the logs:
```
[INFO] Signing with digest reference: registry.io/myapp@sha256:abc123...
```

No action needed - Kimia handles this correctly.

---

### Issue: Cosign password not found

**Problem:**
```bash
[WARN] Cosign password environment variable COSIGN_PASSWORD is not set or empty
Error: reading key: password required
```

**Solution:**

Set the password environment variable:

```bash
export COSIGN_PASSWORD="your-password"
kimia --sign --cosign-key=cosign.key --cosign-password-env=COSIGN_PASSWORD
```

Or use a custom variable name:

```bash
export MY_COSIGN_PASS="your-password"
kimia --sign --cosign-key=cosign.key --cosign-password-env=MY_COSIGN_PASS
```

---

### Issue: Cannot find attestations in registry

**Problem:**
Attestations are missing when inspecting the image.

**Checklist:**

1. **Verify you built with attestations:**
   ```bash
   kimia --context=. --destination=myapp:v1 --attestation=max
   ```

2. **Check you didn't use `--no-push`:**
   ```bash
   # ❌ Attestations not pushed
   kimia --context=. --destination=myapp:v1 --attestation=max --no-push
   
   # ✅ Attestations pushed
   kimia --context=. --destination=myapp:v1 --attestation=max
   ```

3. **Inspect using the TAG, not the digest:**
   ```bash
   # ✅ Shows full manifest list with attestations
   crane manifest myapp:latest
   
   # ❌ Shows only platform manifest
   crane manifest myapp@sha256:abc123...
   ```

4. **Verify OCI manifest support:**
   Some older registries don't support OCI manifest lists. Ensure your registry supports OCI 1.0+.

---

## Understanding Attestation Types

### SBOM (Software Bill of Materials)

**Format**: SPDX 2.3
**Purpose**: Complete inventory of software components

**Contains:**
- All packages and their versions
- Dependency relationships
- License information
- File checksums
- Package origins (source URLs)

**Use cases:**
- Vulnerability scanning
- License compliance
- Software composition analysis
- Supply chain risk assessment

**Example content:**
```json
{
  "predicate": {
    "packages": [
      {
        "name": "alpine-baselayout",
        "versionInfo": "3.4.3-r0",
        "supplier": "Organization: Alpine Linux"
      }
    ]
  }
}
```

---

### Provenance (Build Information)

**Format**: SLSA Provenance v0.2 or v1
**Purpose**: Verifiable record of how the image was built

**Contains:**
- Builder information (ID, version)
- Build timestamp
- Source repository and commit
- Build arguments and environment
- Materials (base images, dependencies)
- Build process metadata

**Use cases:**
- Build verification
- Reproducibility validation
- Supply chain security (SLSA compliance)
- Audit trails

**Example content:**
```json
{
  "predicate": {
    "builder": {
      "id": "https://github.com/org/repo"
    },
    "metadata": {
      "buildStartedOn": "2024-01-15T10:30:00Z",
      "buildFinishedOn": "2024-01-15T10:35:00Z",
      "reproducible": true
    },
    "materials": [
      {
        "uri": "pkg:docker/alpine@3.18"
      }
    ]
  }
}
```

---

## Integration Examples

### GitHub Actions

```yaml
name: Build and Sign

on:
  push:
    branches: [main]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup Cosign
        uses: sigstore/cosign-installer@v3
      
      - name: Login to Registry
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      
      - name: Build and Sign with Kimia
        env:
          COSIGN_PASSWORD: ${{ secrets.COSIGN_PASSWORD }}
          SOURCE_DATE_EPOCH: ${{ github.event.head_commit.timestamp }}
        run: |
          # Run Kimia as a Docker container
          docker run --rm \
            -v $PWD:/workspace \
            -e COSIGN_PASSWORD \
            -e SOURCE_DATE_EPOCH \
            ghcr.io/rapidfort/kimia:latest \
            --context=/workspace \
            --destination=ghcr.io/${{ github.repository }}:${{ github.sha }} \
            --reproducible \
            --attestation=max \
            --sign \
            --cosign-key=/workspace/cosign.key \
            --cosign-password-env=COSIGN_PASSWORD
      
      - name: Verify Signature
        run: |
          cosign verify --key cosign.pub \
            ghcr.io/${{ github.repository }}:${{ github.sha }}
```

---

### GitLab CI

**Option 1: Using Docker-in-Docker**

```yaml
build-and-sign:
  stage: build
  image: docker:latest
  services:
    - docker:dind
  
  variables:
    IMAGE: $CI_REGISTRY_IMAGE:$CI_COMMIT_SHA
    DOCKER_HOST: tcp://docker:2375
    DOCKER_TLS_CERTDIR: ""
  
  before_script:
    - export SOURCE_DATE_EPOCH=$(git log -1 --format=%ct)
  
  script:
    # Run Kimia as a Docker container
    - docker run --rm
        -v $PWD:/workspace
        -e COSIGN_PASSWORD=$COSIGN_PASSWORD
        -e SOURCE_DATE_EPOCH
        ghcr.io/rapidfort/kimia:latest
        --context=/workspace
        --destination=$IMAGE
        --reproducible
        --attestation=max
        --sign
        --cosign-key=/workspace/cosign.key
        --cosign-password-env=COSIGN_PASSWORD
        --build-arg CI_COMMIT_SHA=$CI_COMMIT_SHA
    
    - docker run --rm ghcr.io/sigstore/cosign:latest verify --key cosign.pub $IMAGE
  
  only:
    - main
```

**Option 2: Using Kubernetes Executor**

```yaml
build-and-sign:
  stage: build
  image: bitnami/kubectl:latest
  
  variables:
    IMAGE: $CI_REGISTRY_IMAGE:$CI_COMMIT_SHA
  
  script:
    - export SOURCE_DATE_EPOCH=$(git log -1 --format=%ct)
    
    # Create Kubernetes Job
    - |
      cat <<EOF | kubectl apply -f -
      apiVersion: batch/v1
      kind: Job
      metadata:
        name: kimia-build-$CI_COMMIT_SHORT_SHA
      spec:
        backoffLimit: 0
        template:
          spec:
            restartPolicy: Never
            containers:
              - name: kimia
                image: ghcr.io/rapidfort/kimia:latest
                args:
                  - --context=https://gitlab.com/$CI_PROJECT_PATH.git
                  - --git-branch=$CI_COMMIT_REF_NAME
                  - --destination=$IMAGE
                  - --reproducible
                  - --attestation=max
                  - --sign
                  - --cosign-key=/secrets/cosign.key
                  - --cosign-password-env=COSIGN_PASSWORD
                  - --build-arg
                  - CI_COMMIT_SHA=$CI_COMMIT_SHA
                env:
                  - name: COSIGN_PASSWORD
                    valueFrom:
                      secretKeyRef:
                        name: cosign-keys
                        key: password
                  - name: SOURCE_DATE_EPOCH
                    value: "$SOURCE_DATE_EPOCH"
                volumeMounts:
                  - name: cosign-key
                    mountPath: /secrets
                    readOnly: true
            volumes:
              - name: cosign-key
                secret:
                  secretName: cosign-keys
      EOF
    
    - kubectl wait --for=condition=complete --timeout=600s job/kimia-build-$CI_COMMIT_SHORT_SHA
    - cosign verify --key cosign.pub $IMAGE
  
  only:
    - main
```

---

## Additional Resources

- **Cosign Documentation**: https://docs.sigstore.dev/cosign/overview
- **SLSA Framework**: https://slsa.dev/
- **SPDX Specification**: https://spdx.dev/
- **BuildKit Attestations**: https://docs.docker.com/build/attestations/
- **OCI Image Spec**: https://github.com/opencontainers/image-spec

---

## Summary

| Feature | Flag | Output |
|---------|------|--------|
| **No Attestations** | (none) | Image only |
| **Minimal Provenance** | `--attestation=min` | Image + Provenance (minimal) |
| **Full Attestations** | `--attestation=max` | Image + SBOM + Provenance |
| **Custom SBOM** | `--attest type=sbom,generator=...` | Image + Custom SBOM |
| **Image Signing** | `--sign --cosign-key=...` | Signed image |
| **Full Security** | `--attestation=max --sign` | Signed image with attestations |

**Recommended production configuration:**

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: kimia-build-production
spec:
  template:
    spec:
      restartPolicy: Never
      containers:
        - name: kimia
          image: ghcr.io/rapidfort/kimia:latest
          args:
            - --context=https://github.com/myorg/myapp.git
            - --destination=registry.io/myapp:v1
            - --reproducible
            - --attestation=max
            - --sign
            - --cosign-key=/secrets/cosign.key
            - --cosign-password-env=COSIGN_PASSWORD
          env:
            - name: COSIGN_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: cosign-keys
                  key: password
            - name: SOURCE_DATE_EPOCH
              value: "1609459200"  # From: git log -1 --format=%ct
          volumeMounts:
            - name: cosign-key
              mountPath: /secrets
              readOnly: true
      volumes:
        - name: cosign-key
          secret:
            secretName: cosign-keys
```

This gives you:
- ✅ Reproducible builds
- ✅ Complete SBOM
- ✅ Detailed provenance
- ✅ Cryptographic signature
- ✅ Supply chain security
- ✅ Detailed provenance
- ✅ Cryptographic signature
- ✅ Supply chain security