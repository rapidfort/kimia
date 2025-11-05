# Kimia CLI Reference

Complete command-line reference for Kimia container image builder.

## Table of Contents

- [Core Arguments](#core-arguments)
- [Build Options](#build-options)
- [Registry Authentication](#registry-authentication)
- [Registry Options](#registry-options)
- [Output Options](#output-options)
- [Attestation & Signing](#attestation--signing)
- [Git Options](#git-options)
- [Reproducible Builds](#reproducible-builds)
- [Logging & Debug](#logging--debug)
- [Advanced Options](#advanced-options)

---

## Core Arguments

| Argument | Description | Example | Required |
|----------|-------------|---------|----------|
| `-c, --context` | Build context (directory or Git URL) | `--context=.` | Yes |
| `-f, --dockerfile` | Path to Dockerfile | `--dockerfile=Dockerfile` | No (default: Dockerfile) |
| `-d, --destination` | Target image (repeatable for multiple tags) | `--destination=myapp:latest` | Yes (unless `--no-push`) |
| `-t, --target` | Multi-stage build target | `--target=builder` | No |
| `--context-sub-path` | Subdirectory within context | `--context-sub-path=app` | No |

### Examples

```bash
# Basic build
kimia --context=. --destination=myregistry.io/myapp:v1.0

# Multi-stage build with specific target
kimia --context=. --dockerfile=Dockerfile.prod --target=production --destination=myapp:prod

# Build from subdirectory
kimia --context=. --context-sub-path=backend --destination=myapp:backend

# Multiple destinations (tags)
kimia --context=. \
  --destination=myapp:latest \
  --destination=myapp:v1.0 \
  --destination=myapp:stable
```

---

## Build Options

| Argument | Description | Default | Example |
|----------|-------------|---------|---------|
| `--build-arg` | Build-time variables (repeatable) | - | `--build-arg VERSION=1.0` |
| `--cache` | Enable layer caching | `false` | `--cache` |
| `--cache-dir` | Custom cache directory | - | `--cache-dir=/cache` |
| `--storage-driver` | Storage backend (native\|overlay) | `native` | `--storage-driver=overlay` |
| `--label` | Image labels (repeatable) | - | `--label version=1.0` |

### Examples

```bash
# Build with arguments
kimia --context=. \
  --build-arg NODE_VERSION=18 \
  --build-arg APP_ENV=production \
  --destination=myapp:latest

# Enable caching for faster rebuilds
kimia --context=. \
  --cache \
  --cache-dir=/workspace/cache \
  --destination=myapp:latest

# Use overlay storage driver for better performance
kimia --context=. \
  --storage-driver=overlay \
  --destination=myapp:latest

# Add labels to image
kimia --context=. \
  --label version=1.0.0 \
  --label build-date=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  --label git-commit=$(git rev-parse HEAD) \
  --destination=myapp:v1.0
```

---

## Registry Authentication

Kimia has simplified registry authentication that works seamlessly with both Buildah and BuildKit.

### Simple Authentication (Single Registry)

**If `DOCKER_USERNAME`, `DOCKER_PASSWORD`, and optionally `DOCKER_REGISTRY` environment variables are defined, Kimia automatically generates a `config.json` that works with both Buildah and BuildKit.**

This simplified approach is ideal when pulling and pushing from the same registry.

#### Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `DOCKER_USERNAME` | Registry username | Yes |
| `DOCKER_PASSWORD` | Registry password or token | Yes |
| `DOCKER_REGISTRY` | Registry hostname (e.g., `ghcr.io`, `myregistry.io`) | No* |

*If `DOCKER_REGISTRY` is not specified, credentials will be applied to common registries (docker.io, quay.io, ghcr.io)

#### Examples

```bash
# Authenticate to specific registry
export DOCKER_USERNAME=myuser
export DOCKER_PASSWORD=mytoken
export DOCKER_REGISTRY=ghcr.io

kimia --context=. --destination=ghcr.io/myorg/myapp:latest

# Authenticate to Docker Hub (no DOCKER_REGISTRY needed)
export DOCKER_USERNAME=mydockerhubuser
export DOCKER_PASSWORD=mydockerhubtoken

kimia --context=. --destination=mydockerhubuser/myapp:latest
```

#### Kubernetes Example

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: kimia-build
spec:
  template:
    spec:
      containers:
      - name: kimia
        image: ghcr.io/rapidfort/kimia:latest
        env:
        - name: DOCKER_USERNAME
          valueFrom:
            secretKeyRef:
              name: registry-credentials
              key: username
        - name: DOCKER_PASSWORD
          valueFrom:
            secretKeyRef:
              name: registry-credentials
              key: password
        - name: DOCKER_REGISTRY
          value: "ghcr.io"
        args:
        - --context=git://github.com/myorg/myapp
        - --destination=ghcr.io/myorg/myapp:latest
```

### Multiple Registry Authentication

**For multiple registries, mounting a Docker config.json or using Kubernetes secrets is recommended.**

#### Option 1: Mount Docker Config

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: kimia-build
spec:
  template:
    spec:
      containers:
      - name: kimia
        image: ghcr.io/rapidfort/kimia:latest
        args:
        - --context=.
        - --destination=registry1.io/myapp:latest
        - --destination=registry2.io/myapp:latest
        volumeMounts:
        - name: docker-config
          mountPath: /home/kimia/.docker
          readOnly: true
      volumes:
      - name: docker-config
        secret:
          secretName: registry-credentials
          items:
          - key: .dockerconfigjson
            path: config.json
```

Create the secret:

```bash
# From existing Docker config
kubectl create secret generic registry-credentials \
  --from-file=.dockerconfigjson=$HOME/.docker/config.json \
  --type=kubernetes.io/dockerconfigjson

# Or create manually with multiple registries
kubectl create secret docker-registry registry-credentials \
  --docker-server=registry1.io \
  --docker-username=user1 \
  --docker-password=pass1
```

#### Option 2: Mount as Generic Secret

```yaml
volumeMounts:
- name: docker-config
  mountPath: /workspace/.docker
  readOnly: true
volumes:
- name: docker-config
  secret:
    secretName: docker-config
    items:
    - key: config.json
      path: config.json
```

### Cloud Registry Authentication

Kimia automatically handles authentication for cloud registries when appropriate credentials are available:

- **AWS ECR**: Uses AWS credentials or IAM roles
- **Google GCR/GAR**: Uses Google Cloud credentials or Workload Identity
- **Azure ACR**: Uses Azure credentials or managed identities

Example for AWS ECR:

```yaml
env:
- name: AWS_ACCESS_KEY_ID
  valueFrom:
    secretKeyRef:
      name: aws-credentials
      key: access-key-id
- name: AWS_SECRET_ACCESS_KEY
  valueFrom:
    secretKeyRef:
      name: aws-credentials
      key: secret-access-key
- name: AWS_REGION
  value: us-east-1
```

### Authentication Priority

Kimia checks for authentication in the following order:

1. `DOCKER_USERNAME` / `DOCKER_PASSWORD` / `DOCKER_REGISTRY` environment variables
2. Mounted config.json at:
   - `/home/kimia/.docker/config.json`
3. Cloud-specific credentials (AWS, GCP, Azure)
4. Credential helpers (if configured in config.json)

---

## Registry Options

| Argument | Description | Example |
|----------|-------------|---------|
| `--insecure` | Allow insecure connections to all registries | `--insecure` |
| `--insecure-pull` | Allow insecure base image pulls | `--insecure-pull` |
| `--insecure-registry` | Skip TLS for specific registry (repeatable) | `--insecure-registry=myregistry:5000` |
| `--push-retry` | Number of push retry attempts | `--push-retry=3` |
| `--image-download-retry` | Number of image download retries | `--image-download-retry=3` |
| `--registry-certificate` | Custom registry certificate directory | `--registry-certificate=/certs` |

### Examples

```bash
# Use insecure local registry
kimia --context=. \
  --destination=localhost:5000/myapp:latest \
  --insecure-registry=localhost:5000

# Multiple insecure registries
kimia --context=. \
  --destination=registry1:5000/myapp:latest \
  --insecure-registry=registry1:5000 \
  --insecure-registry=registry2:5000

# Configure retry attempts
kimia --context=. \
  --destination=myregistry.io/myapp:latest \
  --push-retry=5 \
  --image-download-retry=3

# Use custom certificates
kimia --context=. \
  --destination=private-registry.io/myapp:latest \
  --registry-certificate=/etc/docker/certs.d
```

---

## Output Options

| Argument | Description | Example |
|----------|-------------|---------|
| `--no-push` | Build without pushing to registry | `--no-push` |
| `--tar-path` | Export image to TAR file | `--tar-path=/output/image.tar` |
| `--digest-file` | Write image digest to file | `--digest-file=/output/digest.txt` |
| `--image-name-with-digest-file` | Write full image reference with digest | `--image-name-with-digest-file=/output/image-ref.txt` |

### Examples

```bash
# Build without pushing
kimia --context=. \
  --destination=myapp:latest \
  --no-push

# Export to TAR file
kimia --context=. \
  --destination=myapp:latest \
  --tar-path=/workspace/myapp.tar \
  --no-push

# Save digest for later use
kimia --context=. \
  --destination=myregistry.io/myapp:latest \
  --digest-file=/workspace/digest.txt

# Save full image reference with digest
kimia --context=. \
  --destination=myregistry.io/myapp:latest \
  --image-name-with-digest-file=/workspace/image-ref.txt
```

---

## Attestation & Signing

| Argument | Description | Example |
|----------|-------------|---------|
| `--attestation` | Simple attestation mode (off\|min\|max) | `--attestation=min` |
| `--attest` | Docker-style attestations (repeatable) | `--attest type=sbom` |
| `--sign` | Sign image with Cosign | `--sign` |
| `--cosign-key` | Cosign private key path | `--cosign-key=/keys/cosign.key` |

### Attestation Modes

- `off`: No attestations
- `min`: Basic provenance only
- `max`: Full SBOM + provenance + attestations

### Examples

```bash
# Simple attestation
kimia --context=. \
  --destination=myapp:latest \
  --attestation=min

# Docker-style SBOM
kimia --context=. \
  --destination=myapp:latest \
  --attest type=sbom

# Docker-style provenance
kimia --context=. \
  --destination=myapp:latest \
  --attest type=provenance,mode=max

# Sign image with Cosign
kimia --context=. \
  --destination=myapp:latest \
  --sign \
  --cosign-key=/secrets/cosign.key

# Combined: SBOM, provenance, and signing
kimia --context=. \
  --destination=myapp:latest \
  --attestation=max \
  --sign \
  --cosign-key=/secrets/cosign.key
```

**See [Attestation & Signing Guide](attestation-signing.md) for detailed documentation.**

---

## Git Options

| Argument | Description | Example |
|----------|-------------|---------|
| `--git-branch` | Git branch to checkout | `--git-branch=main` |
| `--git-revision` | Git commit SHA | `--git-revision=abc123` |
| `--git-token-file` | Git token for private repos | `--git-token-file=/secrets/git-token` |
| `--git-token-user` | Git token username | `--git-token-user=oauth2` |

### Examples

```bash
# Build from Git repository
kimia --context=git://github.com/myorg/myapp \
  --destination=myapp:latest

# Build specific branch
kimia --context=git://github.com/myorg/myapp \
  --git-branch=develop \
  --destination=myapp:dev

# Build specific commit
kimia --context=git://github.com/myorg/myapp \
  --git-revision=abc123def456 \
  --destination=myapp:abc123

# Private repository with token
kimia --context=git://github.com/myorg/private-app \
  --git-token-file=/secrets/github-token \
  --git-token-user=oauth2 \
  --destination=myapp:latest
```

---

## Reproducible Builds

| Argument | Description | Example |
|----------|-------------|---------|
| `--reproducible` | Enable reproducible builds | `--reproducible` |
| `--timestamp` | Set build timestamp (Unix epoch seconds) | `--timestamp=1609459200` |

### Environment Variables

- `SOURCE_DATE_EPOCH`: Unix timestamp for reproducible builds (automatically recognized)

### Examples

```bash
# Enable reproducible builds
kimia --context=. \
  --destination=myapp:latest \
  --reproducible

# Set specific timestamp
kimia --context=. \
  --destination=myapp:latest \
  --timestamp=1609459200

# Use SOURCE_DATE_EPOCH
export SOURCE_DATE_EPOCH=$(git log -1 --format=%ct)
kimia --context=. \
  --destination=myapp:latest \
  --reproducible
```

**Note:** Using `--timestamp` automatically enables `--reproducible`.

**See [Reproducible Builds Guide](reproducible-builds.md) for detailed documentation.**

---

## Logging & Debug

| Argument | Description | Default | Values |
|----------|-------------|---------|--------|
| `-v, --verbosity` | Log level | `info` | `debug`, `info`, `warn`, `error` |
| `--log-timestamp` | Add timestamps to logs | `false` | - |

### Examples

```bash
# Debug logging
kimia --context=. \
  --destination=myapp:latest \
  --verbosity=debug

# Quiet logging (errors only)
kimia --context=. \
  --destination=myapp:latest \
  --verbosity=error

# Add timestamps to logs
kimia --context=. \
  --destination=myapp:latest \
  --verbosity=debug \
  --log-timestamp
```

---

## Advanced Options

### BuildKit Options

| Argument | Description | Example |
|----------|-------------|---------|
| `--buildkit-opt` | Pass options directly to BuildKit | `--buildkit-opt=network=host` |

```bash
# Use host network
kimia --context=. \
  --destination=myapp:latest \
  --buildkit-opt=network=host

# Multiple BuildKit options
kimia --context=. \
  --destination=myapp:latest \
  --buildkit-opt=network=host \
  --buildkit-opt=platform=linux/amd64,linux/arm64
```

### Storage Driver

Kimia supports two storage drivers:

| Driver | Description | Best For | Requirements |
|--------|-------------|----------|--------------|
| `VFS` | VFS-based storage (default for Buildah) | Maximum compatibility, TAR exports | None |
| `native` | VFS-based storage (default for Buildkit) | Maximum compatibility, TAR exports | None |
| `overlay` | OverlayFS-based | Performance, production builds | Kernel OverlayFS support |

```bash
# Use overlay driver for better performance
kimia --context=. \
  --destination=myapp:latest \
  --storage-driver=overlay

# Use native (default) for TAR exports (BuildKit)
kimia --context=. \
  --tar-path=/output/image.tar \
  --no-push

# Use vfs (default) for TAR exports (Buildah)
kimia --context=. \
  --tar-path=/output/image.tar \
  --no-push
```

---

## Complete Examples

### Basic Build and Push

```bash
kimia \
  --context=. \
  --dockerfile=Dockerfile \
  --destination=myregistry.io/myapp:latest
```

### Production Build with All Features

```bash
export DOCKER_USERNAME=myuser
export DOCKER_PASSWORD=mytoken
export DOCKER_REGISTRY=ghcr.io

kimia \
  --context=. \
  --dockerfile=Dockerfile.prod \
  --target=production \
  --destination=ghcr.io/myorg/myapp:v1.0 \
  --destination=ghcr.io/myorg/myapp:latest \
  --build-arg VERSION=1.0 \
  --build-arg ENV=production \
  --label version=1.0 \
  --label git-commit=$(git rev-parse HEAD) \
  --cache \
  --cache-dir=/workspace/cache \
  --storage-driver=overlay \
  --attestation=max \
  --sign \
  --cosign-key=/secrets/cosign.key \
  --reproducible \
  --timestamp=$(git log -1 --format=%ct) \
  --digest-file=/workspace/digest.txt \
  --push-retry=3 \
  --verbosity=info
```

### Multi-Registry Build

```bash
# With mounted config.json for multiple registries
kimia \
  --context=. \
  --destination=ghcr.io/myorg/myapp:latest \
  --destination=docker.io/myuser/myapp:latest \
  --destination=quay.io/myorg/myapp:latest
```

### Git Repository Build

```bash
kimia \
  --context=git://github.com/myorg/myapp \
  --git-branch=main \
  --git-token-file=/secrets/github-token \
  --git-token-user=oauth2 \
  --destination=myregistry.io/myapp:$(git rev-parse --short HEAD) \
  --cache \
  --attestation=min
```

---

## Environment Variables

Kimia recognizes the following environment variables:

| Variable | Description | Example |
|----------|-------------|---------|
| `DOCKER_USERNAME` | Registry username | `myuser` |
| `DOCKER_PASSWORD` | Registry password/token | `mytoken123` |
| `DOCKER_REGISTRY` | Registry hostname | `ghcr.io` |
| `DOCKER_CONFIG` | Docker config directory | `/home/kimia/.docker` |
| `SOURCE_DATE_EPOCH` | Unix timestamp for reproducible builds | `1609459200` |
| `AWS_ACCESS_KEY_ID` | AWS credentials for ECR | - |
| `AWS_SECRET_ACCESS_KEY` | AWS credentials for ECR | - |
| `AWS_REGION` | AWS region for ECR | `us-east-1` |

---

## Exit Codes

| Code | Description |
|------|-------------|
| `0` | Success |
| `1` | General error |
| `2` | Configuration error |
| `3` | Build error |
| `4` | Push error |
| `5` | Authentication error |

---

## See Also

- [Installation Guide](installation.md)
- [Security Guide](security.md)
- [Attestation & Signing](attestation-signing.md)
- [Reproducible Builds](reproducible-builds.md)
- [Examples](examples.md)
- [Troubleshooting](troubleshooting.md)
- [Comparison with Kaniko](comparison.md)