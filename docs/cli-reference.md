# Command Line Reference

Complete reference for Kimia command-line arguments.

## Usage

```bash
kimia --context=<path|url> --destination=<image:tag> [options]
kimia check-environment              # Validate build environment
kimia --help                         # Show help message
kimia --version                      # Show version info
```

---

## Core Arguments

### `--context`, `-c`

**Required.** Specifies the build context directory or Git URL.

```bash
# Local directory
kimia --context=. --destination=myapp:latest

# Git repository
kimia --context=https://github.com/org/repo.git --destination=myapp:latest
```

### `--context-sub-path`

Sub-directory within the build context.

```bash
# Build from sub-directory in Git repo
kimia --context=https://github.com/org/repo.git \
      --context-sub-path=docker/app \
      --destination=myapp:latest
```

### `--dockerfile`, `-f`

Path to Dockerfile relative to context. **Default:** `Dockerfile`

```bash
kimia --context=. --dockerfile=docker/Dockerfile.prod --destination=myapp:latest
```

### `--destination`, `-d`

**Required.** Destination image with tag. Can be specified multiple times.

```bash
# Single destination
kimia --context=. --destination=myregistry.io/myapp:latest

# Multiple destinations
kimia --context=. \
      --destination=myregistry.io/myapp:latest \
      --destination=myregistry.io/myapp:v1.0.0 \
      --destination=myregistry.io/myapp:stable
```

### `--target`, `-t`

Target stage in multi-stage Dockerfile.

```bash
kimia --context=. --target=production --destination=myapp:latest
```

---

## Build Options

### `--build-arg`

Build-time variables. Can be specified multiple times.

```bash
kimia --context=. \
      --destination=myapp:latest \
      --build-arg VERSION=1.0.0 \
      --build-arg ENVIRONMENT=production
```

**In Dockerfile:**
```dockerfile
ARG VERSION
ARG ENVIRONMENT
RUN echo "Building ${VERSION} for ${ENVIRONMENT}"
```

### `--label`

Image metadata labels. Can be specified multiple times.

```bash
kimia --context=. \
      --destination=myapp:latest \
      --label maintainer=team@company.com \
      --label version=1.0.0 \
      --label git.commit=abc123
```

### `--no-push`

Build image but skip pushing to registry. Useful for testing.

```bash
kimia --context=. --destination=myapp:latest --no-push
```

### `--cache`

Enable layer caching for faster rebuilds.

```bash
kimia --context=. --destination=myapp:latest --cache --cache-dir=/cache
```

### `--cache-dir`

Directory path for build cache. Requires `--cache` flag.

```bash
kimia --context=. \
      --destination=myapp:latest \
      --cache \
      --cache-dir=/workspace/cache
```

### `--custom-platform`

Target platform for the build.

```bash
# Build for ARM64
kimia --context=. --destination=myapp:latest --custom-platform=linux/arm64

# Build for AMD64
kimia --context=. --destination=myapp:latest --custom-platform=linux/amd64
```

### `--storage-driver`

Storage driver to use: `native` (VFS) or `overlay`.

```bash
# Native driver (default, maximum compatibility)
kimia --context=. --destination=myapp:latest --storage-driver=native

# Overlay driver (better performance)
kimia --context=. --destination=myapp:latest --storage-driver=overlay
```

**Driver Comparison:**

| Driver | Speed | Compatibility | TAR Export | Use Case |
|--------|-------|---------------|------------|----------|
| native | Good | ✅ Maximum | ✅ Reliable | Default, TAR exports |
| overlay | ✅ Faster | Requires kernel support | ⚠️ May have issues | Production builds |

---

## Reproducible Build Options

### `--reproducible`

Enable reproducible builds for supply chain security.

```bash
# Uses timestamp 0 by default
kimia --context=. --destination=myapp:v1 --reproducible

# Respects SOURCE_DATE_EPOCH environment variable
export SOURCE_DATE_EPOCH=1609459200
kimia --context=. --destination=myapp:v1 --reproducible
```

**What it does:**
- Uses timestamp 0 by default (or SOURCE_DATE_EPOCH if set)
- Disables caching
- Sorts build args and labels alphabetically
- Rewrites all file timestamps in the image

### `--timestamp`

Custom timestamp for reproducible builds (Unix epoch seconds). Automatically enables reproducible mode.

```bash
# Fixed timestamp
kimia --context=. --destination=myapp:v1 --timestamp=1609459200

# Current timestamp
kimia --context=. --destination=myapp:v1 --timestamp=$(date +%s)

# Git commit timestamp
kimia --context=. --destination=myapp:v1 --timestamp=$(git log -1 --format=%ct)
```

**Note:** `--timestamp` overrides `SOURCE_DATE_EPOCH` environment variable.

---

## Git Options

### `--git-branch`

Git branch to checkout when using Git URL as context.

```bash
kimia --context=https://github.com/org/repo.git \
      --git-branch=develop \
      --destination=myapp:dev
```

### `--git-revision`

Git commit SHA to checkout. Takes precedence over `--git-branch`.

```bash
kimia --context=https://github.com/org/repo.git \
      --git-revision=abc123def456 \
      --destination=myapp:abc123
```

### `--git-token-file`

Path to file containing Git authentication token.

```bash
kimia --context=https://github.com/org/private-repo.git \
      --git-token-file=/secrets/github-token \
      --git-token-user=oauth2 \
      --destination=myapp:latest
```

**Token file format:**
```
ghp_YourGitHubPersonalAccessToken
```

### `--git-token-user`

Username for Git authentication. **Default:** `oauth2`

```bash
kimia --context=https://github.com/org/repo.git \
      --git-token-file=/secrets/token \
      --git-token-user=myusername \
      --destination=myapp:latest
```

---

## Registry Options

### `--insecure`

Allow insecure connections to all registries (HTTP instead of HTTPS).

```bash
kimia --context=. --destination=localhost:5000/myapp:latest --insecure
```

⚠️ **Use with caution:** Only for development/testing.

### `--insecure-registry`

Allow insecure connections to specific registry. Can be specified multiple times.

```bash
kimia --context=. \
      --destination=localhost:5000/myapp:latest \
      --insecure-registry=localhost:5000
```

### `--push-retry`

Number of retry attempts for pushing images. **Default:** 1

```bash
kimia --context=. \
      --destination=myregistry.io/myapp:latest \
      --push-retry=3
```

### `--image-download-retry`

Number of retry attempts when pulling base images during build. **Default:** 1

```bash
kimia --context=. \
      --destination=myapp:latest \
      --image-download-retry=5
```

### `--registry-certificate`

Directory containing registry TLS certificates.

```bash
kimia --context=. \
      --destination=registry.io/myapp:latest \
      --registry-certificate=/certs/registry
```

**Certificate directory structure:**
```
/certs/registry/
├── ca.crt
├── client.cert
└── client.key
```

---

## Output Options

### `--tar-path`

Export built image to TAR archive instead of pushing to registry.

```bash
kimia --context=. \
      --destination=myapp:latest \
      --tar-path=/output/myapp.tar \
      --no-push
```

**Recommended:** Use `--storage-driver=native` for reliable TAR exports.

### `--digest-file`

Save image digest to file after successful build.

```bash
kimia --context=. \
      --destination=myregistry.io/myapp:latest \
      --digest-file=/output/digest.txt
```

**Output format:**
```
sha256:1234567890abcdef...
```

### `--image-name-with-digest-file`

Save full image reference with digest to file.

```bash
kimia --context=. \
      --destination=myregistry.io/myapp:latest \
      --image-name-with-digest-file=/output/image-ref.txt
```

**Output format:**
```
myregistry.io/myapp@sha256:1234567890abcdef...
```

---

## Logging Options

### `--verbosity`, `-v`

Set log level. Options: `debug`, `info`, `warn`, `error`. **Default:** `info`

```bash
# Detailed debug output
kimia --context=. --destination=myapp:latest --verbosity=debug

# Minimal output
kimia --context=. --destination=myapp:latest --verbosity=error
```

### `--log-timestamp`

Add timestamps to log output.

```bash
kimia --context=. --destination=myapp:latest --log-timestamp
```

---

## Other Commands

### `check-environment`

Validate the build environment before attempting a build.

```bash
kimia check-environment
```

**Checks:**
- User namespace support
- Required capabilities
- Storage driver availability
- Buildah version

### `--version`

Display version information.

```bash
kimia --version
```

**Output:**
```
Kimia version: 1.0.13
Built: 2025-10-27 05:06:43 UTC
Commit: db272a8
```

### `--help`, `-h`

Display help message with all available options.

```bash
kimia --help
```

---

## Environment Variables

Kimia respects several environment variables:

### `SOURCE_DATE_EPOCH`

Timestamp for reproducible builds (Unix epoch seconds).

```bash
export SOURCE_DATE_EPOCH=1609459200
kimia --context=. --destination=myapp:v1 --reproducible
```

### `STORAGE_DRIVER`

Override default storage driver.

```bash
export STORAGE_DRIVER=overlay
kimia --context=. --destination=myapp:latest
```

### `BUILDAH_FORMAT`

Image format: `oci` (default) or `docker`.

```bash
export BUILDAH_FORMAT=docker
kimia --context=. --destination=myapp:latest
```

**When to use `docker` format:**
- Dockerfile contains `HEALTHCHECK` instruction
- Dockerfile uses `SHELL` instruction
- Dockerfile has `STOPSIGNAL`

### `DOCKER_CONFIG`

Docker config directory for registry authentication. **Default:** `/home/kimia/.docker`

```bash
export DOCKER_CONFIG=/custom/docker/config
kimia --context=. --destination=myregistry.io/myapp:latest
```

---

## Complete Examples

### Basic Local Build

```bash
kimia --context=. \
      --dockerfile=Dockerfile \
      --destination=myregistry.io/myapp:latest
```

### Build from Git with Branch

```bash
kimia --context=https://github.com/org/repo.git \
      --git-branch=main \
      --dockerfile=Dockerfile \
      --destination=myregistry.io/myapp:v1.0.0
```

### Multi-Destination Build

```bash
kimia --context=. \
      --destination=myregistry.io/myapp:latest \
      --destination=myregistry.io/myapp:v1.0.0 \
      --destination=myregistry.io/myapp:stable
```

### Build with Arguments and Labels

```bash
kimia --context=. \
      --destination=myregistry.io/myapp:latest \
      --build-arg VERSION=1.0.0 \
      --build-arg ENVIRONMENT=production \
      --label maintainer=team@company.com \
      --label version=1.0.0
```

### Build for ARM64

```bash
kimia --context=. \
      --destination=myregistry.io/myapp:arm64 \
      --custom-platform=linux/arm64
```

### Build with Cache

```bash
kimia --context=. \
      --destination=myregistry.io/myapp:latest \
      --cache \
      --cache-dir=/workspace/cache
```

### Export to TAR

```bash
kimia --context=. \
      --destination=myapp:latest \
      --tar-path=/output/myapp.tar \
      --storage-driver=native \
      --no-push
```

### Reproducible Build

```bash
# With git commit timestamp
export SOURCE_DATE_EPOCH=$(git log -1 --format=%ct)
kimia --context=. \
      --destination=myregistry.io/myapp:v1.0.0 \
      --reproducible
```

### Private Git Repository with Authentication

```bash
kimia --context=https://github.com/org/private-repo.git \
      --git-branch=main \
      --git-token-file=/secrets/github-token \
      --git-token-user=oauth2 \
      --destination=myregistry.io/myapp:latest
```

### Multi-Stage Build

```bash
kimia --context=. \
      --dockerfile=Dockerfile \
      --target=production \
      --destination=myregistry.io/myapp:prod
```

### Build with Custom Registry Certificates

```bash
kimia --context=. \
      --destination=secure-registry.io/myapp:latest \
      --registry-certificate=/certs/registry
```

---

## Kaniko Argument Compatibility

Most Kaniko arguments work directly with Kimia. Key mappings:

| Kaniko | Kimia | Notes |
|--------|-------|-------|
| `--context` | `--context` | ✅ Direct compatibility |
| `--dockerfile` | `--dockerfile` | ✅ Direct compatibility |
| `--destination` | `--destination` | ✅ Direct compatibility |
| `--build-arg` | `--build-arg` | ✅ Direct compatibility |
| `--target` | `--target` | ✅ Direct compatibility |
| `--cache` | `--cache` | ✅ Direct compatibility |
| `--cache-dir` | `--cache-dir` | ✅ Direct compatibility |
| `--insecure` | `--insecure` | ✅ Direct compatibility |
| `--verbosity` | `--verbosity` | ✅ Direct compatibility |

**See also:** [Kaniko Comparison Guide](comparison.md)

---

## Need Help?

- 📖 [Back to Main README](../README.md)
- 🎯 [Examples](examples.md)
- 🔧 [Troubleshooting](troubleshooting.md)
- ❓ [FAQ](faq.md)
