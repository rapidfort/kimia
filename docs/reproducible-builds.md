# Reproducible Builds

Reproducible builds enable you to build identical container images from the same source code, regardless of when or where the build occurs. This is critical for supply chain security, compliance, and build verification.

---

## What Are Reproducible Builds?

A **reproducible build** produces byte-for-byte identical outputs when given the same inputs. This means:
- Same source code → Same image digest
- Builds are verifiable and auditable
- Tampering can be detected
- Supply chain integrity is maintained

---

## Shared Responsibility Model

Reproducible builds require **collaboration** between your build configuration and Kimia:

### Your Responsibilities 📌

You must ensure your Dockerfile and build inputs are deterministic:

#### 1. **Pin Base Image Digests**

❌ **Bad - Tags can change:**
```dockerfile
FROM alpine:3.18
FROM node:18
```

✅ **Good - Digests are immutable:**
```dockerfile
FROM alpine@sha256:82d1e9d7ed48a7523bdebc18cf6290bdb97b82302a8a9c27d4fe885949ea94d1
FROM node@sha256:a6385a6bb2fdcb7c48fc871e35e32af8daaa82c518f508e72c67a0e7f7b7d4e5
```

**Get image digest:**
```bash
docker pull alpine:3.18
docker inspect alpine:3.18 --format='{{.RepoDigests}}'
```

#### 2. **Pin Package Versions**

❌ **Bad - Versions can change:**
```dockerfile
RUN apt-get update && apt-get install -y \
    curl \
    git \
    vim
```

✅ **Good - Specific versions:**
```dockerfile
RUN apt-get update && apt-get install -y \
    curl=7.88.1-10+deb12u5 \
    git=1:2.39.2-1.1 \
    vim=2:9.0.1378-2
```

**Find exact versions:**
```bash
docker run --rm debian:12 bash -c "apt-cache policy curl git vim"
```

#### 3. **Pin Language Package Versions**

**Python:**
```dockerfile
# Use pinned requirements.txt
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# requirements.txt with pinned versions:
# flask==2.3.3
# requests==2.31.0
# gunicorn==21.2.0
```

**Node.js:**
```dockerfile
# Use package-lock.json for npm or yarn.lock for yarn
COPY package.json package-lock.json ./
RUN npm ci --only=production
```

**Go:**
```dockerfile
# Use go.mod and go.sum
COPY go.mod go.sum ./
RUN go mod download
RUN go mod verify
```

#### 4. **Avoid Network-Dependent Operations**

❌ **Bad - Downloads can change:**
```dockerfile
RUN curl -O https://example.com/latest/tool.tar.gz
RUN git clone https://github.com/org/repo.git
```

✅ **Good - Verify checksums:**
```dockerfile
RUN curl -O https://example.com/v1.2.3/tool.tar.gz && \
    echo "abc123... tool.tar.gz" | sha256sum -c -
```

#### 5. **Use ARG for SOURCE_DATE_EPOCH**

```dockerfile
ARG SOURCE_DATE_EPOCH
ENV SOURCE_DATE_EPOCH=${SOURCE_DATE_EPOCH}

# Some build tools respect this variable
RUN make build
```

### Kimia's Responsibilities 🔧

When you use `--reproducible` or `--timestamp`, Kimia automatically:

✅ **Normalizes File Timestamps**
- All files in the image use the same timestamp
- Defaults to epoch 0 (1970-01-01) or your specified timestamp

✅ **Sorts Build Arguments**
- Build args are sorted alphabetically
- Ensures consistent build metadata

✅ **Sorts Labels**
- Image labels are sorted alphabetically
- Consistent image annotations

✅ **Uses Deterministic Metadata**
- Image creation time uses specified timestamp
- Consistent layer metadata

✅ **Disables Caching**
- Cache can introduce non-determinism
- Fresh build from scratch guaranteed

---

## Using Reproducible Builds

### Basic Usage

#### Option 1: Use `--reproducible` Flag

```bash
# Uses timestamp 0 (epoch: 1970-01-01 00:00:00 UTC)
kimia --context=. \
      --destination=myregistry.io/myapp:v1.0.0 \
      --reproducible
```

#### Option 2: Use `SOURCE_DATE_EPOCH` Environment Variable

```bash
# Set timestamp via environment
export SOURCE_DATE_EPOCH=1609459200  # 2021-01-01 00:00:00 UTC
kimia --context=. \
      --destination=myregistry.io/myapp:v1.0.0 \
      --reproducible
```

#### Option 3: Use `--timestamp` Flag

```bash
# Explicitly specify timestamp (auto-enables reproducible mode)
kimia --context=. \
      --destination=myregistry.io/myapp:v1.0.0 \
      --timestamp=1609459200
```

---

## Practical Examples

### Example 1: Git Commit Timestamp

Use the git commit timestamp for versioned builds:

```bash
#!/bin/bash

# Get git commit timestamp
export SOURCE_DATE_EPOCH=$(git log -1 --format=%ct)

# Build with that timestamp
kimia --context=. \
      --destination=myregistry.io/myapp:$(git rev-parse --short HEAD) \
      --reproducible

echo "Built reproducible image with commit timestamp: $SOURCE_DATE_EPOCH"
```

**Or with `--timestamp`:**
```bash
kimia --context=. \
      --destination=myregistry.io/myapp:$(git rev-parse --short HEAD) \
      --timestamp=$(git log -1 --format=%ct)
```

### Example 2: Release Builds

For tagged releases with pinned timestamp:

```bash
#!/bin/bash

# Get version from git tag
VERSION=$(git describe --tags --abbrev=0)

# Use release date as timestamp
RELEASE_DATE="2024-01-15 00:00:00 UTC"
TIMESTAMP=$(date -d "$RELEASE_DATE" +%s)

# Build reproducible release
kimia --context=. \
      --destination=myregistry.io/myapp:${VERSION} \
      --timestamp=$TIMESTAMP
```

### Example 3: CI/CD Pipeline

**GitHub Actions:**
```yaml
- name: Build reproducible image
  run: |
    # Use git commit timestamp
    export SOURCE_DATE_EPOCH=$(git log -1 --format=%ct)
    
    kimia --context=. \
          --destination=ghcr.io/${{ github.repository }}:${{ github.sha }} \
          --reproducible
```

**GitLab CI:**
```yaml
build:
  script:
    - export SOURCE_DATE_EPOCH=$(git log -1 --format=%ct)
    - kimia --context=. 
            --destination=$CI_REGISTRY_IMAGE:$CI_COMMIT_SHA 
            --reproducible
```

### Example 4: Complete Reproducible Dockerfile

```dockerfile
# Pin base image by digest
FROM alpine@sha256:82d1e9d7ed48a7523bdebc18cf6290bdb97b82302a8a9c27d4fe885949ea94d1

# Accept SOURCE_DATE_EPOCH as build arg
ARG SOURCE_DATE_EPOCH
ENV SOURCE_DATE_EPOCH=${SOURCE_DATE_EPOCH}

# Pin package versions
RUN apk add --no-cache \
    ca-certificates=20230506-r0 \
    curl=8.4.0-r0 \
    tzdata=2023c-r1

# Copy application
COPY --chown=1000:1000 app /app

# Set consistent user
USER 1000:1000

WORKDIR /app
ENTRYPOINT ["/app/myapp"]
```

**Build command:**
```bash
kimia --context=. \
      --destination=myregistry.io/myapp:v1.0.0 \
      --build-arg SOURCE_DATE_EPOCH=$(date +%s) \
      --reproducible
```

---

## Verification

### Verify Reproducibility

Build the same image twice and compare digests:

```bash
# First build
kimia --context=. \
      --destination=myapp:test1 \
      --timestamp=1609459200 \
      --no-push

# Second build (same timestamp)
kimia --context=. \
      --destination=myapp:test2 \
      --timestamp=1609459200 \
      --no-push

# Compare digests
DIGEST1=$(docker inspect myapp:test1 --format='{{.Id}}')
DIGEST2=$(docker inspect myapp:test2 --format='{{.Id}}')

if [ "$DIGEST1" = "$DIGEST2" ]; then
    echo "✅ Build is reproducible!"
    echo "Digest: $DIGEST1"
else
    echo "❌ Build is NOT reproducible"
    echo "Digest 1: $DIGEST1"
    echo "Digest 2: $DIGEST2"
fi
```

### Inspect Image Timestamps

Check that all files have the same timestamp:

```bash
# Extract tar and check timestamps
docker save myapp:test1 -o /tmp/image.tar
tar -xf /tmp/image.tar -C /tmp/image
find /tmp/image -type f -exec stat -c '%Y %n' {} \; | sort | uniq -c

# All files should have the same timestamp
```

---

## Best Practices

### ✅ DO

1. **Always pin base images by digest**
   ```dockerfile
   FROM alpine@sha256:...
   ```

2. **Pin all package versions explicitly**
   ```dockerfile
   RUN apt-get install package=1.2.3-4
   ```

3. **Use SOURCE_DATE_EPOCH in your build process**
   ```dockerfile
   ARG SOURCE_DATE_EPOCH
   ENV SOURCE_DATE_EPOCH=${SOURCE_DATE_EPOCH}
   ```

4. **Use git commit timestamps for versioned builds**
   ```bash
   export SOURCE_DATE_EPOCH=$(git log -1 --format=%ct)
   ```

5. **Verify reproducibility in CI/CD**
   ```bash
   # Build twice, compare digests
   ```

6. **Document your build process**
   ```
   # Include REPRODUCIBLE_BUILD.md in your repo
   ```

### ❌ DON'T

1. **Don't use floating tags**
   ```dockerfile
   FROM alpine:latest  # ❌ Not reproducible
   ```

2. **Don't use `apt-get upgrade` or similar**
   ```dockerfile
   RUN apt-get upgrade  # ❌ Introduces variability
   ```

3. **Don't download files without verification**
   ```dockerfile
   RUN curl -O https://example.com/latest.tar.gz  # ❌ No checksum
   ```

4. **Don't rely on build cache for reproducibility**
   ```bash
   # Cache can mask non-deterministic builds
   # Use --reproducible which disables cache
   ```

5. **Don't use `$(date)` or similar in Dockerfile**
   ```dockerfile
   RUN echo "Built on $(date)" > /version.txt  # ❌ Changes every build
   ```

---

## Troubleshooting

### Build is Not Reproducible

**Check these common issues:**

1. **Base image not pinned:**
   ```bash
   # Find current digest
   docker pull alpine:3.18
   docker inspect alpine:3.18 | grep -i digest
   ```

2. **Package versions not pinned:**
   ```bash
   # Check what versions are available
   docker run --rm alpine:3.18 apk search curl
   ```

3. **Timestamps differ:**
   ```bash
   # Ensure SOURCE_DATE_EPOCH is set consistently
   echo $SOURCE_DATE_EPOCH
   ```

4. **Network downloads in Dockerfile:**
   ```bash
   # Add checksum verification
   RUN curl -O ... && echo "sha256 ..." | sha256sum -c
   ```

### Different Digests on Different Platforms

If builds on different platforms (amd64 vs arm64) produce different digests, this is **expected**. Each platform has its own digest:

```bash
# AMD64 digest
sha256:abc123...

# ARM64 digest  
sha256:def456...

# Manifest digest (points to both)
sha256:789xyz...
```

**Solution:** Use multi-arch manifests and compare platform-specific digests.

---

## Integration with Supply Chain Security

### SLSA (Supply chain Levels for Software Artifacts)

Reproducible builds are a key requirement for SLSA Level 3:

```yaml
# Generate provenance
- name: Build with provenance
  run: |
    export SOURCE_DATE_EPOCH=$(git log -1 --format=%ct)
    kimia --context=. \
          --destination=myapp:${{ github.sha }} \
          --reproducible \
          --digest-file=digest.txt
    
    # Generate SLSA provenance
    # Use digest.txt for verification
```

### Sigstore/Cosign Integration

Sign reproducible builds for verification:

```bash
# Build reproducibly
kimia --context=. \
      --destination=myregistry.io/myapp:v1.0.0 \
      --reproducible \
      --digest-file=digest.txt

# Sign with cosign
IMAGE_DIGEST=$(cat digest.txt)
cosign sign myregistry.io/myapp@${IMAGE_DIGEST}
```

### Build Verification

Anyone can verify your build:

```bash
# Clone source
git clone https://github.com/org/repo.git
cd repo
git checkout v1.0.0

# Get commit timestamp
export SOURCE_DATE_EPOCH=$(git log -1 --format=%ct)

# Rebuild
kimia --context=. \
      --destination=myapp:verify \
      --reproducible \
      --no-push

# Compare with published image
VERIFY_DIGEST=$(docker inspect myapp:verify --format='{{.Id}}')
PUBLISHED_DIGEST=$(docker pull myregistry.io/myapp:v1.0.0 && docker inspect myregistry.io/myapp:v1.0.0 --format='{{.Id}}')

if [ "$VERIFY_DIGEST" = "$PUBLISHED_DIGEST" ]; then
    echo "✅ Build verified! Image is reproducible."
else
    echo "❌ Verification failed! Digests don't match."
fi
```

---

## Additional Resources

- 📖 [Reproducible Builds Project](https://reproducible-builds.org/)
- 🔒 [SLSA Framework](https://slsa.dev/)
- 📝 [SOURCE_DATE_EPOCH Specification](https://reproducible-builds.org/specs/source-date-epoch/)
- 🔗 [Sigstore](https://www.sigstore.dev/)

---

## Summary

**Reproducible builds with Kimia require:**

| Component | Your Responsibility | Kimia's Responsibility |
|-----------|-------------------|----------------------|
| Base Images | Pin by digest | - |
| Packages | Pin versions | - |
| Dependencies | Lock files | - |
| Network Downloads | Verify checksums | - |
| Timestamps | - | Normalize with `--reproducible` |
| Build Args | - | Sort alphabetically |
| Labels | - | Sort alphabetically |
| Metadata | - | Use deterministic values |
| Caching | - | Disable automatically |

**Result:** Byte-for-byte identical images every time! 🎉

---

[Back to Main README](../README.md) | [CLI Reference](cli-reference.md) | [Examples](examples.md)
