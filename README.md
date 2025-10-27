# Kimia - Kubernetes-Native OCI Image Builder
### Daemonless. Rootless. Privilege-free. Fully OCI-compliant.
<div align="center">
<p>
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License: MIT"></a>
  <a href="https://kubernetes.io/"><img src="https://img.shields.io/badge/Kubernetes-1.21%2B-326CE5?logo=kubernetes" alt="Kubernetes"></a>
  <a href="https://golang.org/"><img src="https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go" alt="Go Version"></a>
  <a href="https://ghcr.io/rapidfort/kimia"><img src="https://img.shields.io/badge/Registry-ghcr.io-blue" alt="Container Registry"></a>
</p>

**[Quick Start](#quick-start)** • **[Documentation](#command-line-reference)** • **[Examples](#examples)** • **[Contributing](#contributing)**

</div>

---

## Table of Contents

- [Overview](#overview)
  - [Why Kimia?](#why-kimia)
  - [Architecture](#architecture)
- [Comparison with Kaniko](#comparison-with-kaniko)
  - [Key Security Advantages](#key-security-advantages)
  - [Compatibility Advantages](#compatibility-advantages)
  - [When to Choose Kimia](#when-to-choose-kimia)
  - [Migration Path](#migration-path)
- [Quick Start](#quick-start)
  - [Prerequisites](#prerequisites)
  - [Enable User Namespaces](#1-enable-user-namespaces)
  - [Deploy a Build Job](#2-deploy-a-build-job)
  - [Check Build Progress](#3-check-build-progress)
- [Installation](#installation)
  - [Docker Registry Credentials](#docker-registry-credentials)
  - [Platform-Specific Setup](#platform-specific-setup)
    - [AWS EKS](#aws-eks)
    - [Google GKE](#google-gke)
    - [Azure AKS](#azure-aks)
    - [Red Hat OpenShift](#red-hat-openshift)
- [Command Line Reference](#command-line-reference)
  - [Core Arguments](#core-arguments)
  - [Build Options](#build-options)
  - [Git Options](#git-options)
  - [Registry Options](#registry-options)
  - [Output Options](#output-options)
  - [Logging](#logging)
- [Docker Format Compatibility](#docker-format-compatibility)
  - [Understanding OCI vs Docker Formats](#understanding-oci-vs-docker-formats)
  - [Enabling Docker Format](#enabling-docker-format)
  - [When to Use Docker Format](#when-to-use-docker-format)
  - [Format Comparison](#format-comparison)
  - [Troubleshooting Format Issues](#troubleshooting-format-issues)
- [Examples](#examples)
  - [Basic Build](#basic-build)
  - [Build from Git Repository](#build-from-git-repository)
  - [Multi-Platform Build](#multi-platform-build)
  - [Build with Docker-Specific Instructions](#build-with-docker-specific-instructions)
- [GitOps Integrations](#gitops-integrations)
  - [ArgoCD Workflows](#argocd-workflows)
  - [Flux Integration](#flux-integration)
  - [Tekton Pipeline](#tekton-pipeline)
  - [Jenkins Pipeline](#jenkins-pipeline)
- [Kaniko Compatibility Guide](#kaniko-compatibility-guide)
  - [Argument Mapping](#argument-mapping)
  - [Migration Example](#migration-example)
- [Security Best Practices](#security-best-practices)
  - [Pod Security Standards](#pod-security-standards)
  - [Network Policies](#network-policies)
  - [Resource Limits](#resource-limits)
  - [Secrets Management](#secrets-management)
- [Environment Variables](#environment-variables)
  - [Format Selection Guide](#format-selection-guide)
- [Performance Optimization](#performance-optimization)
  - [Build Caching](#build-caching)
  - [Parallel Builds](#parallel-builds)
  - [Resource Tuning](#resource-tuning)
- [Troubleshooting](#troubleshooting)
  - [Common Issues](#common-issues)
  - [Debugging Commands](#debugging-commands)
- [Contributing](#contributing)
  - [Reporting Issues](#reporting-issues)
  - [Development Setup](#development-setup)
  - [Pull Request Process](#pull-request-process)
  - [Code Style](#code-style)
- [FAQ](#faq)
  - [General Questions](#general-questions)
  - [Technical Questions](#technical-questions)
  - [Operational Questions](#operational-questions)
- [License](#license)
- [Support](#support)
- [Acknowledgments](#acknowledgments)

---

## Overview

Kimia is a **Kubernetes-native, OCI-compliant container builder** designed for secure, daemonless builds in cloud environments. Built on [Buildah](https://buildah.io/), Kimia provides a safer alternative to traditional Docker-based builders and addresses key security limitations found in other solutions.

### Why Kimia?

**Security First**
- **Rootless by Design** - Runs as non-root user (UID 1000)
- **Privilege-Free** - No privileged mode required
- **Minimal Capabilities** - Only SETUID & SETGID required
- **User Namespace Isolation** - Complete separation from host

**Cloud Native**
- **Kubernetes Native** - Designed for K8s from the ground up
- **GitOps Ready** - Works seamlessly with ArgoCD, Flux, Tekton
- **Multi-Platform** - Supports AWS, GCP, Azure, OpenShift
- **Fully OCI-Compliant** - Standards-based image building

**Developer Friendly**
- **Kaniko Compatible** - Drop-in replacement for Kaniko
- **Git Integration** - Build directly from repositories
- **Layer Caching** - Fast, efficient rebuilds
- **Standard Dockerfiles** - No special syntax needed

### Architecture

![Kimia Architecture](./assets/kimia_architecture.svg)

Kimia uses Linux user namespaces to provide true rootless operation:

```
Host System (Real)          User Namespace (Mapped)
─────────────────          ───────────────────────
UID 1000 (kimia)    ───►  UID 0 (appears as root)
UID 100000           ───►  UID 1
UID 100001           ───►  UID 2
     ...                        ...
UID 165535           ───►  UID 65535
```

Even if a container escapes, it only has unprivileged user access on the host.

---

## Comparison with Kaniko

| Feature | Kimia | Kaniko | Advantage |
|---------|--------|--------|-----------|
| **User Context** | Non-root (UID 1000) | Root (UID 0) | ✅ Kimia: Reduced privilege escalation risk |
| **Capabilities Required** | SETUID, SETGID only | None | ⚖️ Kimia: Explicit minimal caps for user namespaces |
| **Docker Daemon** | Not required | Not required | ✅ Equal: No daemon dependencies |
| **Privileged Mode** | Not required | Not required | ✅ Equal: No privileged containers |
| **User Namespaces** | Required & utilized | Not used | ✅ Kimia: Additional isolation layer |
| **Complex Dockerfiles** | Full support | Limited (chown issues) | ✅ Kimia: Better compatibility with ownership changes |
| **Storage Driver** | VFS (isolated) | Various | ✅ Kimia: Consistent, secure, portable |
| **Build Cache** | Layer caching | Layer caching | ✅ Equal: Efficient rebuilds |
| **Registry Authentication** | Multiple methods | Multiple methods | ✅ Equal: Flexible auth options |
| **Multi-stage Builds** | Full support | Full support | ✅ Equal: Modern Dockerfile features |
| **Git Integration** | Built-in (via args) | Built-in (via executor) | ✅ Equal: Both support Git directly |
| **Attack Surface** | Minimal (rootless) | Larger (root) | ✅ Kimia: Significantly reduced |
| **Pod Security Standards** | Restricted-compliant* | Baseline only | ✅ Kimia: Higher security standard |
| **Build Performance** | Fast (native) | Fast (native) | ✅ Equal: Both performant |
| **Cross-platform Builds** | ✅ Supported | ✅ Supported | ✅ Equal: Multi-arch capable |
| **Secrets Handling** | Buildah secrets | Kaniko secrets | ✅ Equal: Secure secret management |
| **Resource Efficiency** | Lightweight | Lightweight | ✅ Equal: Minimal overhead |

*With `allowPrivilegeEscalation: true` for user namespace operations

### Key Security Advantages

#### 1. **Rootless Architecture**
```yaml
# Kimia - Non-root by design
securityContext:
  runAsUser: 1000
  runAsNonRoot: true

# Kaniko - Runs as root
securityContext:
  runAsUser: 0
```

**Impact:** Even if container is compromised, attacker only has unprivileged user access.

#### 2. **User Namespace Isolation**
```
Kimia:
  Container UID 0 → Host UID 1000 (unprivileged)
  Container UID 1 → Host UID 100000
  
Kaniko:
  Container UID 0 → Host UID 0 (if escaped)
```

**Impact:** Additional security boundary that Kaniko lacks.

#### 3. **Explicit Capability Model**
```yaml
# Kimia - Minimal explicit capabilities
capabilities:
  drop: [ALL]
  add: [SETUID, SETGID]  # Only for user namespaces

# Kaniko - No capabilities needed
capabilities:
  drop: [ALL]
```

**Impact:** Kimia's approach is more transparent about security requirements.


### When to Choose Kimia

✅ **Choose Kimia when:**
- Security is paramount (defense in depth)
- Complex ownership operations needed
- Compliance requires rootless containers
- Need Pod Security Standard "Restricted" compliance

⚠️ **Consider Kaniko when:**
- User namespaces cannot be enabled on nodes
- Existing Kaniko pipelines work well
- Minimal change to existing infrastructure

### Migration Path

1. **Test in Development**
   ```bash
   # Run side-by-side comparison
   kubectl apply -f kimia-test-build.yaml
   kubectl apply -f kaniko-test-build.yaml
   ```

2. **Update SecurityContext**
   ```yaml
   # Add to existing Kaniko manifests
   securityContext:
     runAsUser: 1000
     fsGroup: 1000  # Important for cache directory permissions
     allowPrivilegeEscalation: true
     capabilities:
       add: [SETUID, SETGID]
   ```

3. **Add Docker Format if Needed**
   ```yaml
   env:
   - name: BUILDAH_FORMAT
     value: "docker"
   ```

4. **Verify User Namespace Support**
   ```bash
   sysctl user.max_user_namespaces
   ```

5. **Update Volume Mounts**
   ```yaml
   # Change from /kaniko/.docker to /home/kimia/.docker
   volumeMounts:
   - name: docker-config
     mountPath: /home/kimia/.docker
   ```

6. **Test and Rollout**
   - Deploy to dev/staging first
   - Monitor build times and success rates
   - Gradual rollout to production

---

## Quick Start

### Prerequisites

- Kubernetes 1.21+
- User namespaces enabled on nodes
- Container registry credentials

### 1. Enable User Namespaces

```bash
# Check if enabled
cat /proc/sys/user/max_user_namespaces

# Enable if needed (value should be > 0)
sudo sysctl -w user.max_user_namespaces=15000

# Make persistent
echo "user.max_user_namespaces=15000" | sudo tee -a /etc/sysctl.conf
```

### 2. Deploy a Build Job

```bash
kubectl apply -f - <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: kimia-nginx-build
spec:
  ttlSecondsAfterFinished: 60
  template:
    spec:
      restartPolicy: Never
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000
      containers:
      - name: kimia
        image: ghcr.io/rapidfort/kimia
        args:
          - --context=https://github.com/nginx/docker-nginx.git
          - --dockerfile=mainline/alpine/Dockerfile
          - --destination=mynginx
          - --no-push
        securityContext:
          allowPrivilegeEscalation: true
          capabilities:
            drop: [ALL]
            add: [SETUID, SETGID]
EOF
```

### 3. Check Build Progress

```bash
# Watch job status
kubectl get jobs -w

# View logs
kubectl logs job/kimia-nginx-build -f
```

---

## Installation

### Docker Registry Credentials

Create a secret with your registry credentials:

```bash
kubectl create secret docker-registry registry-credentials \
  --docker-server=myregistry.io \
  --docker-username=myuser \
  --docker-password=mypassword \
  --docker-email=myemail@example.com
```

Or from existing Docker config:

```bash
kubectl create secret generic registry-credentials \
  --from-file=.dockerconfigjson=$HOME/.docker/config.json \
  --type=kubernetes.io/dockerconfigjson
```

### Platform-Specific Setup

#### AWS EKS

Standard EKS supports Kimia out of the box. For Bottlerocket nodes:

```toml
# Bottlerocket user data
[settings.kernel.sysctl]
"user.max_user_namespaces" = "15000"
```

Or use a DaemonSet:

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: enable-user-namespaces
  namespace: kube-system
spec:
  selector:
    matchLabels:
      app: enable-user-namespaces
  template:
    metadata:
      labels:
        app: enable-user-namespaces
    spec:
      hostPID: true
      hostNetwork: true
      containers:
      - name: sysctl
        image: busybox
        securityContext:
          privileged: true
        command:
        - sh
        - -c
        - |
          sysctl -w user.max_user_namespaces=15000
          sleep infinity
```

#### Google GKE

User namespaces are enabled by default. No additional configuration needed.

#### Azure AKS

Enable user namespaces on Ubuntu node pools:

```bash
az aks nodepool update \
  --resource-group myResourceGroup \
  --cluster-name myAKSCluster \
  --name nodepool1 \
  --enable-user-namespaces
```

#### Red Hat OpenShift

User namespaces are available on OpenShift 4.7+. Configure via MachineConfig:

```yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  labels:
    machineconfiguration.openshift.io/role: worker
  name: 99-worker-enable-user-namespaces
spec:
  config:
    ignition:
      version: 3.2.0
    storage:
      files:
      - contents:
          source: data:text/plain;charset=utf-8;base64,dXNlci5tYXhfdXNlcl9uYW1lc3BhY2VzPTE1MDAw
        mode: 0644
        path: /etc/sysctl.d/99-user-namespaces.conf
```

---

## Command Line Reference

### Core Arguments

| Argument | Short | Description | Example |
|----------|-------|-------------|---------|
| `--context` | `-c` | Build context (directory or Git URL) | `--context=.` or `--context=git://github.com/org/repo.git` |
| `--dockerfile` | `-f` | Path to Dockerfile | `--dockerfile=Dockerfile` |
| `--destination` | `-d` | Target image with tag (repeatable) | `--destination=registry.io/app:v1` |
| `--target` | `-t` | Multi-stage build target | `--target=production` |

### Build Options

| Argument | Description | Example |
|----------|-------------|---------|
| `--build-arg` | Build-time variables (repeatable) | `--build-arg=VERSION=1.0.0` |
| `--label` | Image metadata labels (repeatable) | `--label=maintainer=team@company.com` |
| `--cache` | Enable layer caching | `--cache=true` |
| `--cache-dir` | Cache directory path | `--cache-dir=/cache` |
| `--custom-platform` | Target platform | `--custom-platform=linux/arm64` |
| `--no-push` | Build only, skip push | `--no-push` |
| `--reproducible` | Enable reproducible builds by setting consistent timestamps and disabling layer caching | `--reproducible` |

### Git Options

| Argument | Description | Example |
|----------|-------------|---------|
| `--git-branch` | Git branch to checkout | `--git-branch=develop` |
| `--git-revision` | Git commit SHA | `--git-revision=abc123` |
| `--git-token-file` | File containing Git token | `--git-token-file=/secrets/token` |
| `--git-token-user` | Git auth username | `--git-token-user=oauth2` |

### Registry Options

| Argument | Description | Example |
|----------|-------------|---------|
| `--insecure` | Allow insecure connections | Flag only |
| `--insecure-registry` | Specific insecure registry (repeatable) | `--insecure-registry=localhost:5000` |
| `--skip-tls-verify` | Skip TLS verification | Flag only |
| `--push-retry` | Push retry attempts | `--push-retry=3` |
| `--image-download-retry` | Image pull retry attempts during build | `--image-download-retry=5` |
| `--registry-certificate` | Registry certificate directory | `--registry-certificate=/certs` |

### Output Options

| Argument | Description | Example |
|----------|-------------|---------|
| `--tar-path` | Export to TAR archive | `--tar-path=/output/image.tar` |
| `--digest-file` | Save image digest | `--digest-file=/output/digest.txt` |
| `--image-name-with-digest-file` | Save image reference with digest | `--image-name-with-digest-file=/output/ref.txt` |

### Logging

| Argument | Short | Description | Values |
|----------|-------|-------------|--------|
| `--verbosity` | `-v` | Log level | `debug`, `info`, `warn`, `error` |
| `--log-timestamp` | | Add timestamps | Flag only |

---

## Docker Format Compatibility

### Understanding OCI vs Docker Formats

Kimia (via Buildah) defaults to **OCI format** for image manifests, which is the industry standard. However, some Dockerfiles use Docker-specific instructions that may not be fully supported in OCI format:

**Docker-specific instructions:**
- `SHELL` - Custom shell configuration
- `HEALTHCHECK` - Container health monitoring
- `STOPSIGNAL` - Custom stop signal

### Enabling Docker Format

To ensure full compatibility with Docker-specific Dockerfile commands, set the `BUILDAH_FORMAT` environment variable:

```yaml
containers:
- name: kimia
  image: ghcr.io/rapidfort/kimia:latest
  env:
  - name: BUILDAH_FORMAT
    value: "docker"  # Use Docker format instead of OCI
  args:
    - --context=.
    - --dockerfile=Dockerfile
    - --destination=myregistry.io/myapp:latest
```

### When to Use Docker Format

| Use Case | Recommended Format | Reason |
|----------|-------------------|--------|
| Standard Dockerfile | OCI (default) | Better compatibility, industry standard |
| Dockerfile with `HEALTHCHECK` | Docker | Full instruction support |
| Dockerfile with `SHELL` | Docker | Custom shell preservation |
| Legacy Dockerfiles | Docker | Maximum compatibility |
| Multi-stage builds | Either | Both work equally |
| Scratch/distroless images | OCI (default) | Minimal, standard format |

### Format Comparison

```yaml
# OCI Format (Default - Recommended)
apiVersion: batch/v1
kind: Job
metadata:
  name: kimia-oci-build
spec:
  template:
    spec:
      containers:
      - name: kimia
        image: ghcr.io/rapidfort/kimia:latest
        # No BUILDAH_FORMAT needed - OCI is default
        args:
          - --context=.
          - --destination=myregistry.io/app:latest

---
# Docker Format (For Docker-specific instructions)
apiVersion: batch/v1
kind: Job
metadata:
  name: kimia-docker-build
spec:
  template:
    spec:
      containers:
      - name: kimia
        image: ghcr.io/rapidfort/kimia:latest
        env:
        - name: BUILDAH_FORMAT
          value: "docker"
        args:
          - --context=.
          - --destination=myregistry.io/app:latest
```

### Troubleshooting Format Issues

**Issue: HEALTHCHECK not preserved in image**

```
Warning: HEALTHCHECK instruction ignored
```

**Solution:**
```yaml
env:
- name: BUILDAH_FORMAT
  value: "docker"
```

**Issue: SHELL instruction not working**

```
Error: SHELL instruction not supported in OCI format
```

**Solution:**
```yaml
env:
- name: BUILDAH_FORMAT
  value: "docker"
```

**Verify image format:**

```bash
# Check manifest format
kubectl exec <kimia-pod> -- buildah inspect myapp:latest | grep -i format
```

---

## Examples

### Basic Build

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: basic-build
spec:
  template:
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000
      containers:
      - name: kimia
        image: ghcr.io/rapidfort/kimia:latest
        args:
          - --context=.
          - --dockerfile=Dockerfile
          - --destination=myregistry.io/myapp:latest
        securityContext:
          allowPrivilegeEscalation: true
          capabilities:
            drop: [ALL]
            add: [SETUID, SETGID]
        volumeMounts:
        - name: source
          mountPath: /workspace
        - name: docker-config
          mountPath: /home/kimia/.docker
      volumes:
      - name: source
        emptyDir: {}
      - name: docker-config
        secret:
          secretName: registry-credentials
      restartPolicy: Never
```

### Build from Git Repository

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: git-build
spec:
  template:
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000
      containers:
      - name: kimia
        image: ghcr.io/rapidfort/kimia:latest
        args:
          - --context=https://github.com/myorg/myapp.git
          - --git-branch=main
          - --dockerfile=Dockerfile
          - --destination=myregistry.io/myapp:v1.0.0
          - --build-arg=VERSION=1.0.0
          - --label=git.commit=$(GIT_COMMIT)
        env:
        - name: GIT_COMMIT
          value: "abc123def456"
        securityContext:
          allowPrivilegeEscalation: true
          capabilities:
            drop: [ALL]
            add: [SETUID, SETGID]
        volumeMounts:
        - name: docker-config
          mountPath: /home/kimia/.docker
      volumes:
      - name: docker-config
        secret:
          secretName: registry-credentials
      restartPolicy: Never
```

### Multi-Platform Build

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: multi-arch-build
spec:
  template:
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000
      containers:
      - name: kimia
        image: ghcr.io/rapidfort/kimia:latest
        args:
          - --context=.
          - --dockerfile=Dockerfile
          - --destination=myregistry.io/myapp:v1.0.0-amd64
          - --custom-platform=linux/amd64
        securityContext:
          allowPrivilegeEscalation: true
          capabilities:
            drop: [ALL]
            add: [SETUID, SETGID]
        volumeMounts:
        - name: docker-config
          mountPath: /home/kimia/.docker
      volumes:
      - name: docker-config
        secret:
          secretName: registry-credentials
      restartPolicy: Never
```

### Build with Docker-Specific Instructions

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: docker-format-build
spec:
  template:
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000
      containers:
      - name: kimia
        image: ghcr.io/rapidfort/kimia:latest
        env:
        - name: BUILDAH_FORMAT
          value: "docker"  # Enable Docker format for HEALTHCHECK support
        args:
          - --context=.
          - --dockerfile=Dockerfile
          - --destination=myregistry.io/nginx-custom:latest
        securityContext:
          allowPrivilegeEscalation: true
          capabilities:
            drop: [ALL]
            add: [SETUID, SETGID]
        volumeMounts:
        - name: docker-config
          mountPath: /home/kimia/.docker
      volumes:
      - name: docker-config
        secret:
          secretName: registry-credentials
      restartPolicy: Never
```

**Dockerfile example:**
```dockerfile
FROM nginx:alpine

# Docker-specific instruction
HEALTHCHECK --interval=30s --timeout=3s \
  CMD curl -f http://localhost/ || exit 1

# Custom shell
SHELL ["/bin/bash", "-c"]

COPY index.html /usr/share/nginx/html/
```

---

## Customizing Kimia

### Building Custom Kimia Images

You can extend the base Kimia image to include additional build tools and dependencies by creating your own Dockerfile.

#### Example: Adding Bazel to Kimia

Create a custom Dockerfile that extends the base Kimia image:

```dockerfile
# Dockerfile.kimia-bazel
FROM ghcr.io/rapidfort/kimia:latest

# Switch to root to install packages
USER root

# Install Bazel dependencies
RUN apk add --no-cache \
    bash \
    zip \
    unzip \
    g++ \
    linux-headers

# Install Bazel
ARG BAZEL_VERSION=7.0.0
RUN wget -O /tmp/bazel-installer.sh \
    "https://github.com/bazelbuild/bazel/releases/download/${BAZEL_VERSION}/bazel-${BAZEL_VERSION}-installer-linux-x86_64.sh" && \
    chmod +x /tmp/bazel-installer.sh && \
    /tmp/bazel-installer.sh && \
    rm /tmp/bazel-installer.sh

# Verify Bazel installation
RUN bazel --version

# Switch back to kimia user
USER 1000:1000

# Bazel cache directory (owned by kimia user)
RUN mkdir -p /home/kimia/.cache/bazel && \
    chown -R 1000:1000 /home/kimia/.cache

WORKDIR /home/kimia
```

#### Build Your Custom Image

```bash
# Build the custom image
docker build -f Dockerfile.kimia-bazel -t myregistry.io/kimia-bazel:latest .

# Push to your registry
docker push myregistry.io/kimia-bazel:latest
```

#### Use Custom Image in Kubernetes

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: kimia-bazel-build
spec:
  template:
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000
      containers:
      - name: kimia
        image: myregistry.io/kimia-bazel:latest  # Your custom image
        args:
          - --context=https://github.com/myorg/bazel-project.git
          - --dockerfile=Dockerfile
          - --destination=myregistry.io/myapp:latest
        securityContext:
          allowPrivilegeEscalation: true
          capabilities:
            drop: [ALL]
            add: [SETUID, SETGID]
        volumeMounts:
        - name: docker-config
          mountPath: /home/kimia/.docker
        - name: bazel-cache
          mountPath: /home/kimia/.cache/bazel
      volumes:
      - name: docker-config
        secret:
          secretName: registry-credentials
      - name: bazel-cache
        emptyDir: {}
      restartPolicy: Never
```

### Common Customization Examples

#### Adding Node.js and npm

```dockerfile
FROM ghcr.io/rapidfort/kimia:latest

USER root

# Install Node.js and npm
RUN apk add --no-cache nodejs npm

# Verify installation
RUN node --version && npm --version

USER 1000:1000
```

#### Adding Python and pip

```dockerfile
FROM ghcr.io/rapidfort/kimia:latest

USER root

# Install Python and pip
RUN apk add --no-cache \
    python3 \
    py3-pip \
    python3-dev

# Verify installation
RUN python3 --version && pip3 --version

USER 1000:1000
```

#### Adding Java and Maven

```dockerfile
FROM ghcr.io/rapidfort/kimia:latest

USER root

# Install Java and Maven
RUN apk add --no-cache \
    openjdk17 \
    maven

# Verify installation
RUN java -version && mvn --version

USER 1000:1000
```

#### Adding Go

```dockerfile
FROM ghcr.io/rapidfort/kimia:latest

USER root

# Install Go
ARG GO_VERSION=1.21.5
RUN apk add --no-cache wget && \
    wget -O /tmp/go.tar.gz \
    "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" && \
    tar -C /usr/local -xzf /tmp/go.tar.gz && \
    rm /tmp/go.tar.gz

# Add Go to PATH
ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOPATH="/home/kimia/go"

# Verify installation
RUN go version

USER 1000:1000
```

#### Multi-Tool Image (Node.js + Python + Go)

```dockerfile
FROM ghcr.io/rapidfort/kimia:latest

USER root

# Install multiple language runtimes
RUN apk add --no-cache \
    nodejs \
    npm \
    python3 \
    py3-pip \
    wget

# Install Go
ARG GO_VERSION=1.21.5
RUN wget -O /tmp/go.tar.gz \
    "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" && \
    tar -C /usr/local -xzf /tmp/go.tar.gz && \
    rm /tmp/go.tar.gz

# Add Go to PATH
ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOPATH="/home/kimia/go"

# Verify installations
RUN node --version && \
    python3 --version && \
    go version

USER 1000:1000
```

### Best Practices for Custom Images

1. **Always Switch Back to Non-Root User**
   ```dockerfile
   USER root
   # Install packages...
   USER 1000:1000  # Always switch back!
   ```

2. **Keep Permissions Correct**
   ```dockerfile
   RUN mkdir -p /home/kimia/.cache && \
       chown -R 1000:1000 /home/kimia/.cache
   ```

3. **Use Specific Version Tags**
   ```dockerfile
   FROM ghcr.io/rapidfort/kimia:1.0.13  # Not :latest in production
   ```

4. **Minimize Layer Size**
   ```dockerfile
   # Bad - Multiple layers
   RUN apk add git
   RUN apk add curl
   RUN apk add wget
   
   # Good - Single layer
   RUN apk add --no-cache \
       git \
       curl \
       wget
   ```

5. **Clean Up After Installation**
   ```dockerfile
   RUN apk add --no-cache build-base && \
       # Build something... && \
       apk del build-base  # Remove build dependencies
   ```

6. **Document Your Image**
   ```dockerfile
   LABEL maintainer="team@company.com"
   LABEL version="1.0.0"
   LABEL description="Kimia with Bazel and Node.js"
   ```

### Multi-Architecture Custom Images

Build custom images for multiple architectures:

```bash
# Build for AMD64 and ARM64
docker buildx create --use
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t myregistry.io/kimia-bazel:latest \
  -f Dockerfile.kimia-bazel \
  --push \
  .
```

### Testing Custom Images

Before deploying, test your custom image:

```bash
# Test locally
docker run --rm \
  --cap-drop ALL \
  --cap-add SETUID \
  --cap-add SETGID \
  --security-opt seccomp=unconfined \
  --security-opt apparmor=unconfined \
  myregistry.io/kimia-bazel:latest \
  --version

# Test Bazel availability
docker run --rm \
  --cap-drop ALL \
  --cap-add SETUID \
  --cap-add SETGID \
  --security-opt seccomp=unconfined \
  --security-opt apparmor=unconfined \
  myregistry.io/kimia-bazel:latest \
  bash -c "bazel --version"
```

### Example: Complete Bazel Build Workflow

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: bazel-monorepo-build
spec:
  template:
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000
      
      containers:
      - name: kimia
        image: myregistry.io/kimia-bazel:latest
        
        env:
        - name: BUILDAH_FORMAT
          value: "docker"
        
        args:
          - --context=https://github.com/myorg/bazel-monorepo.git
          - --git-branch=main
          - --dockerfile=Dockerfile
          - --destination=myregistry.io/myapp:v1.0.0
          - --build-arg=BAZEL_VERSION=7.0.0
          - --cache=true
          - --cache-dir=/cache
        
        securityContext:
          allowPrivilegeEscalation: true
          capabilities:
            drop: [ALL]
            add: [SETUID, SETGID]
        
        volumeMounts:
        - name: docker-config
          mountPath: /home/kimia/.docker
        - name: bazel-cache
          mountPath: /home/kimia/.cache/bazel
        - name: build-cache
          mountPath: /cache
        
        resources:
          requests:
            memory: "4Gi"
            cpu: "2"
          limits:
            memory: "16Gi"
            cpu: "8"
            ephemeral-storage: "20Gi"
      
      volumes:
      - name: docker-config
        secret:
          secretName: registry-credentials
      - name: bazel-cache
        emptyDir: {}
      - name: build-cache
        emptyDir: {}
      
      restartPolicy: Never
```

**Example Dockerfile for Bazel project:**

```dockerfile
# Use the custom kimia-bazel base
FROM myregistry.io/kimia-bazel:latest as builder

USER root

WORKDIR /workspace

# Copy source code
COPY . .

# Build with Bazel
RUN bazel build //... && \
    bazel-bin/app/myapp /tmp/myapp

# Final runtime image
FROM alpine:latest

COPY --from=builder /tmp/myapp /usr/local/bin/myapp

ENTRYPOINT ["/usr/local/bin/myapp"]
```

---

## GitOps Integrations

### ArgoCD Workflows

Complete ArgoCD Workflow example with Kimia:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Workflow
metadata:
  generateName: kimia-build-
  namespace: argo
spec:
  entrypoint: build-and-deploy
  serviceAccountName: argo-workflow
  
  securityContext:
    runAsNonRoot: true
    runAsUser: 1000
    fsGroup: 1000

  templates:
  - name: build-and-deploy
    steps:
    - - name: build
        template: kimia-build
    - - name: deploy
        template: deploy-image

  - name: kimia-build
    inputs:
      artifacts:
      - name: source
        path: /workspace
        git:
          repo: "https://github.com/myorg/myapp.git"
          revision: "{{workflow.parameters.git-revision}}"
    
    container:
      image: ghcr.io/rapidfort/kimia:latest
      
      args:
        - "--context=/workspace"
        - "--dockerfile=Dockerfile"
        - "--destination=myregistry.io/myapp:{{workflow.uid}}"
        - "--build-arg=BUILD_DATE={{workflow.creationTimestamp}}"
        - "--build-arg=GIT_COMMIT={{workflow.parameters.git-commit}}"
        - "--label=workflow.id={{workflow.name}}"
        - "--cache=true"
        - "--push-retry=3"
      
      volumeMounts:
        - name: docker-config
          mountPath: /home/kimia/.docker
        - name: cache
          mountPath: /home/kimia/.local/share/containers
      
      securityContext:
        runAsUser: 1000
        allowPrivilegeEscalation: true
        capabilities:
          drop: [ALL]
          add: [SETUID, SETGID]
      
      resources:
        requests:
          memory: "2Gi"
          cpu: "1"
        limits:
          memory: "8Gi"
          cpu: "4"
  
  - name: deploy-image
    container:
      image: bitnami/kubectl:latest
      command: [sh, -c]
      args:
        - |
          kubectl set image deployment/myapp \
            myapp=myregistry.io/myapp:{{workflow.uid}} \
            -n production

  volumes:
  - name: docker-config
    secret:
      secretName: registry-credentials
  - name: cache
    emptyDir: {}
```

### Flux Integration

```yaml
apiVersion: image.toolkit.fluxcd.io/v1beta1
kind: ImageUpdateAutomation
metadata:
  name: kimia-automation
  namespace: flux-system
spec:
  interval: 5m
  sourceRef:
    kind: GitRepository
    name: myapp-repo
  git:
    checkout:
      ref:
        branch: main
    commit:
      author:
        email: fluxcdbot@example.com
        name: fluxcdbot
      messageTemplate: |
        Automated image update
        
        Built with Kimia: {{ .AutomationObject }}
  update:
    path: ./config
    strategy: Setters

---
apiVersion: batch/v1
kind: CronJob
metadata:
  name: kimia-builder
  namespace: flux-system
spec:
  schedule: "0 2 * * *"  # Daily at 2 AM
  jobTemplate:
    spec:
      template:
        spec:
          securityContext:
            runAsNonRoot: true
            runAsUser: 1000
            fsGroup: 1000
          containers:
          - name: kimia
            image: ghcr.io/rapidfort/kimia:latest
            args:
              - --context=https://github.com/myorg/myapp.git
              - --git-branch=main
              - --dockerfile=Dockerfile
              - --destination=myregistry.io/myapp:$(date +%Y%m%d-%H%M%S)
              - --destination=myregistry.io/myapp:latest
            securityContext:
              allowPrivilegeEscalation: true
              capabilities:
                drop: [ALL]
                add: [SETUID, SETGID]
            volumeMounts:
            - name: docker-config
              mountPath: /home/kimia/.docker
          volumes:
          - name: docker-config
            secret:
              secretName: registry-credentials
          restartPolicy: OnFailure
```

### Tekton Pipeline

```yaml
apiVersion: tekton.dev/v1beta1
kind: Pipeline
metadata:
  name: kimia-build-pipeline
spec:
  params:
    - name: git-url
      type: string
      description: Git repository URL
    - name: git-revision
      type: string
      description: Git revision to build
      default: main
    - name: image-name
      type: string
      description: Target image name
    - name: image-tag
      type: string
      description: Target image tag
      default: latest

  workspaces:
    - name: shared-workspace
    - name: docker-config

  tasks:
    - name: fetch-repository
      taskRef:
        name: git-clone
      workspaces:
        - name: output
          workspace: shared-workspace
      params:
        - name: url
          value: $(params.git-url)
        - name: revision
          value: $(params.git-revision)

    - name: build-push
      taskRef:
        name: kimia-build
      runAfter:
        - fetch-repository
      workspaces:
        - name: source
          workspace: shared-workspace
        - name: dockerconfig
          workspace: docker-config
      params:
        - name: IMAGE
          value: "$(params.image-name):$(params.image-tag)"
        - name: DOCKERFILE
          value: ./Dockerfile
        - name: CONTEXT
          value: .

---
apiVersion: tekton.dev/v1beta1
kind: Task
metadata:
  name: kimia-build
spec:
  params:
    - name: IMAGE
      description: Reference of the image to build
    - name: DOCKERFILE
      default: ./Dockerfile
    - name: CONTEXT
      default: .
    - name: EXTRA_ARGS
      default: ""

  workspaces:
    - name: source
    - name: dockerconfig
      optional: true

  steps:
    - name: build-and-push
      image: ghcr.io/rapidfort/kimia:latest
      workingDir: $(workspaces.source.path)
      
      securityContext:
        runAsUser: 1000
        allowPrivilegeEscalation: true
        capabilities:
          drop: [ALL]
          add: [SETUID, SETGID]
      
      script: |
        #!/bin/sh
        set -e
        
        kimia \
          --context=$(params.CONTEXT) \
          --dockerfile=$(params.DOCKERFILE) \
          --destination=$(params.IMAGE) \
          --cache=true \
          $(params.EXTRA_ARGS)
      
      volumeMounts:
        - name: docker-config
          mountPath: /home/kimia/.docker
      
      env:
        - name: DOCKER_CONFIG
          value: /home/kimia/.docker

  volumes:
    - name: docker-config
      secret:
        secretName: $(workspaces.dockerconfig.bound ? workspaces.dockerconfig.claim.name : "empty-secret")
        optional: true
```

### Jenkins Pipeline

```groovy
pipeline {
    agent {
        kubernetes {
            yaml '''
apiVersion: v1
kind: Pod
metadata:
  labels:
    jenkins: agent
spec:
  securityContext:
    runAsNonRoot: true
    runAsUser: 1000
    fsGroup: 1000
  containers:
  - name: kimia
    image: ghcr.io/rapidfort/kimia:latest
    command:
    - cat
    tty: true
    securityContext:
      allowPrivilegeEscalation: true
      capabilities:
        drop: [ALL]
        add: [SETUID, SETGID]
    volumeMounts:
    - name: docker-config
      mountPath: /home/kimia/.docker
    resources:
      requests:
        memory: "2Gi"
        cpu: "1"
      limits:
        memory: "8Gi"
        cpu: "4"
  volumes:
  - name: docker-config
    secret:
      secretName: registry-credentials
'''
        }
    }
    
    environment {
        REGISTRY = 'myregistry.io'
        IMAGE_NAME = 'myapp'
        GIT_COMMIT_SHORT = sh(script: "git rev-parse --short HEAD", returnStdout: true).trim()
    }
    
    stages {
        stage('Checkout') {
            steps {
                checkout scm
            }
        }
        
        stage('Build and Push') {
            steps {
                container('kimia') {
                    sh '''
                        kimia \
                          --context=. \
                          --dockerfile=Dockerfile \
                          --destination=${REGISTRY}/${IMAGE_NAME}:${BUILD_NUMBER} \
                          --destination=${REGISTRY}/${IMAGE_NAME}:${GIT_COMMIT_SHORT} \
                          --destination=${REGISTRY}/${IMAGE_NAME}:latest \
                          --build-arg=VERSION=${BUILD_NUMBER} \
                          --build-arg=GIT_COMMIT=${GIT_COMMIT_SHORT} \
                          --label=jenkins.build=${BUILD_NUMBER} \
                          --label=git.commit=${GIT_COMMIT_SHORT} \
                          --cache=true \
                          --push-retry=3 \
                          --verbosity=info
                    '''
                }
            }
        }
        
        stage('Deploy') {
            when {
                branch 'main'
            }
            steps {
                sh """
                    kubectl set image deployment/${IMAGE_NAME} \
                      ${IMAGE_NAME}=${REGISTRY}/${IMAGE_NAME}:${BUILD_NUMBER} \
                      -n production
                    
                    kubectl rollout status deployment/${IMAGE_NAME} -n production
                """
            }
        }
    }
    
    post {
        success {
            echo "Build successful! Images pushed:"
            echo "  - ${REGISTRY}/${IMAGE_NAME}:${BUILD_NUMBER}"
            echo "  - ${REGISTRY}/${IMAGE_NAME}:${GIT_COMMIT_SHORT}"
            echo "  - ${REGISTRY}/${IMAGE_NAME}:latest"
        }
        failure {
            echo "Build failed. Check logs for details."
        }
    }
}
```

---

## Kaniko Compatibility Guide

Kimia supports most Kaniko arguments for easy migration.

### Argument Mapping

| Kaniko Argument | Kimia Equivalent | Notes |
|-----------------|-------------------|-------|
| `--context` | `--context` | ✅ Direct compatibility |
| `--dockerfile` | `--dockerfile` | ✅ Direct compatibility |
| `--destination` | `--destination` | ✅ Direct compatibility (repeatable) |
| `--build-arg` | `--build-arg` | ✅ Direct compatibility |
| `--target` | `--target` | ✅ Direct compatibility |
| `--cache` | `--cache` | ✅ Direct compatibility |
| `--cache-dir` | `--cache-dir` | ✅ Direct compatibility |
| `--insecure` | `--insecure` | ✅ Direct compatibility |
| `--skip-tls-verify` | `--skip-tls-verify` | ✅ Direct compatibility |
| `--verbosity` | `--verbosity` | ✅ Direct compatibility |
| `--label` | `--label` | ✅ Direct compatibility |
| `--reproducible` | `--reproducible` | ✅ Direct compatibility |
| `--image-download-retry` | `--image-download-retry` | ✅ Direct compatibility |
| `--git` | Built-in Git support | ℹ️ Use `--context=git://...` |
| `--snapshot-mode` | N/A | ℹ️ VFS storage handles this |
| `--use-new-run` | N/A | ℹ️ Buildah default behavior |

### Migration Example

**Before (Kaniko):**
```yaml
containers:
- name: kaniko
  image: gcr.io/kaniko-project/executor:latest
  args:
    - --context=git://github.com/myorg/myapp.git
    - --dockerfile=Dockerfile
    - --destination=myregistry.io/myapp:v1.0.0
    - --cache=true
    - --cache-dir=/cache
    - --build-arg=VERSION=1.0.0
  volumeMounts:
  - name: docker-config
    mountPath: /kaniko/.docker/
```

**After (Kimia):**
```yaml
containers:
- name: kimia
  image: ghcr.io/rapidfort/kimia:latest
  securityContext:
    runAsUser: 1000
    allowPrivilegeEscalation: true
    capabilities:
      drop: [ALL]
      add: [SETUID, SETGID]
  args:
    - --context=git://github.com/myorg/myapp.git
    - --dockerfile=Dockerfile
    - --destination=myregistry.io/myapp:v1.0.0
    - --cache=true
    - --cache-dir=/cache
    - --build-arg=VERSION=1.0.0
  volumeMounts:
  - name: docker-config
    mountPath: /home/kimia/.docker/
```

**Key Changes:**
1. Add securityContext with user 1000 and fsGroup 1000
2. Add capabilities (SETUID, SETGID)
3. Change volumeMount path to `/home/kimia/.docker/`
4. All arguments remain the same!

---

## Security Best Practices

### Pod Security Standards

Kimia is compatible with Kubernetes Pod Security Standards at the **Restricted** level (with allowPrivilegeEscalation):

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: kimia-build
  labels:
    pod-security.kubernetes.io/enforce: restricted
    pod-security.kubernetes.io/enforce-version: latest
spec:
  securityContext:
    runAsNonRoot: true
    runAsUser: 1000
    runAsGroup: 1000
    fsGroup: 1000
    seccompProfile:
      type: RuntimeDefault
  
  containers:
  - name: kimia
    image: ghcr.io/rapidfort/kimia:latest
    securityContext:
      runAsNonRoot: true
      runAsUser: 1000
      allowPrivilegeEscalation: true  # Required for user namespaces
      capabilities:
        drop: [ALL]
        add: [SETUID, SETGID]       # Minimal capabilities
      seccompProfile:
        type: RuntimeDefault
```

### Network Policies

Restrict Kimia's network access:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: kimia-network-policy
spec:
  podSelector:
    matchLabels:
      app: kimia
  policyTypes:
  - Egress
  egress:
  # Allow DNS
  - to:
    - namespaceSelector: {}
    ports:
    - protocol: UDP
      port: 53
  # Allow HTTPS to registries
  - to:
    - namespaceSelector: {}
    ports:
    - protocol: TCP
      port: 443
  # Allow HTTP for package downloads
  - to:
    - namespaceSelector: {}
    ports:
    - protocol: TCP
      port: 80
```

### Resource Limits

Always set resource limits:

```yaml
resources:
  requests:
    memory: "2Gi"
    cpu: "1"
  limits:
    memory: "8Gi"
    cpu: "4"
    ephemeral-storage: "10Gi"  # Important for builds!
```

### Secrets Management

Use Kubernetes secrets for credentials:

```bash
# Create from Docker config
kubectl create secret generic registry-credentials \
  --from-file=.dockerconfigjson=$HOME/.docker/config.json \
  --type=kubernetes.io/dockerconfigjson

# Or create manually
kubectl create secret docker-registry registry-credentials \
  --docker-server=myregistry.io \
  --docker-username=myuser \
  --docker-password=mypassword
```

Mount as read-only:

```yaml
volumeMounts:
- name: docker-config
  mountPath: /home/kimia/.docker
  readOnly: true
```

---

## Environment Variables

| Variable | Description | Example | Default |
|----------|-------------|---------|---------|
| `BUILDAH_FORMAT` | Image format (oci or docker) | `docker` | `oci` |
| `DOCKER_CONFIG` | Docker config directory | `/home/kimia/.docker` | `/home/kimia/.docker` |
| `REGISTRY_AUTH_FILE` | Buildah auth file location | `/home/kimia/.docker/auth.json` | Auto-detected |
| `BUILDAH_ISOLATION` | Buildah isolation mode | `chroot` | `chroot` |
| `STORAGE_DRIVER` | Storage driver for layers | `vfs` | `vfs` |
| `DOCKER_USERNAME` | Registry username for auth | `myuser` | - |
| `DOCKER_PASSWORD` | Registry password for auth | `secret` | - |
| `DOCKER_REGISTRY` | Default registry URL | `docker.io` | `docker.io` |

### Format Selection Guide

```bash
# Default OCI format (recommended for modern images)
# No environment variable needed

# Docker format (for legacy compatibility)
env:
- name: BUILDAH_FORMAT
  value: "docker"

# Verify format after build
buildah inspect myimage:latest | grep -i format
```

---

## Performance Optimization

### Build Caching

Enable and persist build cache with proper permissions:

**Option 1: Using fsGroup (Recommended - Simple)**

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: kimia-build-cached
spec:
  template:
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        runAsGroup: 1000
        fsGroup: 1000  # Automatically sets ownership of mounted volumes
      
      containers:
      - name: kimia
        image: ghcr.io/rapidfort/kimia:latest
        args:
          - --context=.
          - --dockerfile=Dockerfile
          - --destination=myregistry.io/myapp:latest
          - --cache=true
          - --cache-dir=/cache
        securityContext:
          allowPrivilegeEscalation: true
          capabilities:
            drop: [ALL]
            add: [SETUID, SETGID]
        volumeMounts:
        - name: build-cache
          mountPath: /cache
        - name: docker-config
          mountPath: /home/kimia/.docker
      
      volumes:
      - name: build-cache
        emptyDir: {}  # For ephemeral cache
      - name: docker-config
        secret:
          secretName: registry-credentials
      
      restartPolicy: Never
```

**Option 2: Using PersistentVolume with Init Container**

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: kimia-build-cache
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi

---
apiVersion: batch/v1
kind: Job
metadata:
  name: kimia-build-persistent-cache
spec:
  template:
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000
      
      initContainers:
      # Initialize cache directory with correct permissions
      - name: init-cache
        image: busybox
        command:
        - sh
        - -c
        - |
          chown -R 1000:1000 /cache
          chmod -R 755 /cache
        securityContext:
          runAsUser: 0  # Needs root to chown
        volumeMounts:
        - name: build-cache
          mountPath: /cache
      
      containers:
      - name: kimia
        image: ghcr.io/rapidfort/kimia:latest
        args:
          - --context=.
          - --dockerfile=Dockerfile
          - --destination=myregistry.io/myapp:latest
          - --cache=true
          - --cache-dir=/cache
        securityContext:
          runAsUser: 1000
          allowPrivilegeEscalation: true
          capabilities:
            drop: [ALL]
            add: [SETUID, SETGID]
        volumeMounts:
        - name: build-cache
          mountPath: /cache
        - name: docker-config
          mountPath: /home/kimia/.docker
      
      volumes:
      - name: build-cache
        persistentVolumeClaim:
          claimName: kimia-build-cache
      - name: docker-config
        secret:
          secretName: registry-credentials
      
      restartPolicy: Never
```

**Key Points:**
- **fsGroup: 1000** - Ensures mounted volumes are owned by kimia user (UID 1000)
- Cache directory must be writable by UID 1000
- For PersistentVolumes, use init container OR fsGroup
- For emptyDir, fsGroup alone is sufficient

### Parallel Builds

Run multiple builds concurrently:

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: parallel-builds
spec:
  parallelism: 3
  completions: 3
  template:
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000
      containers:
      - name: kimia
        image: ghcr.io/rapidfort/kimia:latest
        # ... kimia configuration
```

### Resource Tuning

Optimize based on build complexity:

```yaml
# Small builds (< 500MB)
resources:
  requests:
    memory: "1Gi"
    cpu: "500m"
  limits:
    memory: "4Gi"
    cpu: "2"
    ephemeral-storage: "5Gi"

# Medium builds (500MB - 2GB)
resources:
  requests:
    memory: "2Gi"
    cpu: "1"
  limits:
    memory: "8Gi"
    cpu: "4"
    ephemeral-storage: "10Gi"

# Large builds (> 2GB)
resources:
  requests:
    memory: "4Gi"
    cpu: "2"
  limits:
    memory: "16Gi"
    cpu: "8"
    ephemeral-storage: "20Gi"
```

---

## Troubleshooting

### Common Issues

#### 1. User Namespace Error

**Error:**
```
failed to create user namespace: operation not permitted
```

**Solution:**
```bash
# Check if user namespaces are enabled
cat /proc/sys/user/max_user_namespaces

# Enable on host
sudo sysctl -w user.max_user_namespaces=15000

# Make persistent
echo "user.max_user_namespaces=15000" | sudo tee -a /etc/sysctl.conf
```

#### 2. Permission Denied

**Error:**
```
permission denied while trying to connect
```

**Solution:**
Verify securityContext:
```yaml
securityContext:
  runAsUser: 1000
  fsGroup: 1000
  allowPrivilegeEscalation: true
  capabilities:
    drop: [ALL]
    add: [SETUID, SETGID]
```

#### 3. Registry Authentication Failed

**Error:**
```
unauthorized: authentication required
```

**Solution:**
```bash
# Verify secret exists
kubectl get secret registry-credentials

# Check secret content
kubectl get secret registry-credentials -o jsonpath='{.data.\.dockerconfigjson}' | base64 -d

# Recreate if needed
kubectl delete secret registry-credentials
kubectl create secret docker-registry registry-credentials \
  --docker-server=myregistry.io \
  --docker-username=myuser \
  --docker-password=mypassword
```

#### 4. Build Fails with "No Space Left"

**Error:**
```
error building: no space left on device
```

**Solution:**
Increase ephemeral storage:
```yaml
resources:
  limits:
    ephemeral-storage: "20Gi"
```

#### 5. Git Clone Fails

**Error:**
```
fatal: could not read Username
```

**Solution:**
Use Git token:
```yaml
args:
  - --context=https://github.com/org/repo.git
  - --git-token-file=/secrets/git-token
  - --git-token-user=oauth2

volumeMounts:
- name: git-token
  mountPath: /secrets
  readOnly: true

volumes:
- name: git-token
  secret:
    secretName: github-token
```

#### 6. Docker-Specific Instructions Not Working

**Error:**
```
Warning: HEALTHCHECK instruction ignored
Error: SHELL instruction not supported
```

**Solution:**
Enable Docker format:
```yaml
env:
- name: BUILDAH_FORMAT
  value: "docker"
```

**When to use Docker format:**
- Dockerfile contains `HEALTHCHECK`
- Dockerfile uses `SHELL` instruction
- Dockerfile has `STOPSIGNAL`
- Legacy Dockerfiles requiring maximum compatibility

**Trade-offs:**
- Docker format: Better compatibility, slightly larger metadata
- OCI format: Industry standard, better tooling support

#### 7. Cache Directory Permission Denied

**Error:**
```
error: failed to write to cache directory: permission denied
unable to save layer to cache: operation not permitted
```

**Solution:**

**Option 1: Use fsGroup (Recommended)**
```yaml
spec:
  securityContext:
    runAsUser: 1000
    runAsGroup: 1000
    fsGroup: 1000  # Sets group ownership of volumes to 1000
```

**Option 2: Init container to set permissions**
```yaml
spec:
  initContainers:
  - name: fix-cache-permissions
    image: busybox
    command: ['sh', '-c', 'chown -R 1000:1000 /cache && chmod -R 755 /cache']
    securityContext:
      runAsUser: 0
    volumeMounts:
    - name: build-cache
      mountPath: /cache
```

**Option 3: Use emptyDir for ephemeral cache**
```yaml
volumes:
- name: build-cache
  emptyDir: {}  # Always has correct permissions with fsGroup
```

**Verify permissions:**
```bash
kubectl exec <kimia-pod> -- ls -la /cache
# Should show: drwxr-xr-x kimia kimia
```

### Debugging Commands

```bash
# Check pod status
kubectl describe pod <kimia-pod-name>

# View logs
kubectl logs <kimia-pod-name> -f

# Check user namespace support
kubectl exec <kimia-pod-name> -- cat /proc/self/uid_map

# Verify storage
kubectl exec <kimia-pod-name> -- df -h

# Check cache directory permissions
kubectl exec <kimia-pod-name> -- ls -la /cache

# Check network connectivity
kubectl exec <kimia-pod-name> -- ping -c 3 myregistry.io

# Inspect image
kubectl exec <kimia-pod-name> -- buildah images

# Check image format
kubectl exec <kimia-pod-name> -- buildah inspect myapp:latest | grep -i format

# Verify fsGroup is applied
kubectl get pod <kimia-pod-name> -o jsonpath='{.spec.securityContext.fsGroup}'
```

---

## Contributing

We welcome contributions! Here's how you can help:

### Reporting Issues

- Use GitHub Issues for bug reports and feature requests
- Include detailed reproduction steps
- Provide relevant logs and configuration

### Development Setup

```bash
# Clone repository
git clone https://github.com/rapidfort/kimia.git
cd kimia

# Build locally
make build

# Run tests
make test

# Build multi-arch
make release RELEASE_TYPE=staging
```

### Pull Request Process

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Run tests (`make test`)
5. Commit your changes (`git commit -m 'Add amazing feature'`)
6. Push to the branch (`git push origin feature/amazing-feature`)
7. Open a Pull Request

### Code Style

- Follow Go best practices
- Add tests for new features
- Update documentation
- Use meaningful commit messages

---

## FAQ

### General Questions

**Q: How does Kimia differ from Kaniko?**

A: Kimia uses user namespaces for true rootless operation, while Kaniko runs as root. Kimia also has better support for complex Dockerfiles with ownership changes and Docker-specific instructions like HEALTHCHECK.

**Q: Can I use Kimia outside Kubernetes?**

A: Yes! Kimia can run as a standard container:
```bash
docker run \
  --cap-drop ALL \
  --cap-add SETUID \
  --cap-add SETGID \
  --security-opt seccomp=unconfined \
  --security-opt apparmor=unconfined \
  -v $(pwd):/workspace \
  ghcr.io/rapidfort/kimia:latest \
  --context=/workspace --destination=registry/image:tag
```

**Q: Does Kimia support multi-architecture builds?**

A: Yes, use the `--custom-platform` flag:
```bash
kimia --custom-platform=linux/arm64 ...
```

### Technical Questions

**Q: Why do I need user namespaces?**

A: User namespaces provide security isolation. Without them, container processes could potentially escape with host privileges.

**Q: What are SETUID and SETGID capabilities for?**

A: These minimal capabilities allow Kimia to create user namespaces. They're far safer than privileged mode or CAP_SYS_ADMIN.

**Q: Can I use Kimia with distroless images?**

A: Yes, Kimia supports building any OCI-compliant image, including distroless.

**Q: When should I use Docker format vs OCI format?**

A: Use Docker format (`BUILDAH_FORMAT=docker`) when your Dockerfile contains HEALTHCHECK, SHELL, or STOPSIGNAL instructions. Otherwise, use the default OCI format.

### Operational Questions

**Q: How much resource overhead does Kimia add?**

A: Minimal - typically 2-5% CPU and 256MB-2GB RAM depending on build complexity.

**Q: Can I run multiple Kimia builds simultaneously?**

A: Yes, Kimia is designed for concurrent builds. Each build is isolated in its own user namespace.

**Q: Does Kimia work with private registries?**

A: Yes, Kimia supports authentication with any OCI-compliant registry.

**Q: Will my existing Kaniko configurations work with Kimia?**

A: Most Kaniko arguments are directly compatible. You just need to add proper securityContext (including fsGroup: 1000) and change the volume mount path from `/kaniko/.docker` to `/home/kimia/.docker`.

**Q: Why is my cache directory giving permission errors?**

A: The cache directory must be writable by the kimia user (UID 1000). Use one of these solutions:

```yaml
# Solution 1: Use fsGroup (recommended)
securityContext:
  fsGroup: 1000

# Solution 2: Use emptyDir for ephemeral cache
volumes:
- name: cache
  emptyDir: {}

# Solution 3: Init container to fix permissions
initContainers:
- name: fix-perms
  image: busybox
  command: ['chown', '-R', '1000:1000', '/cache']
  securityContext:
    runAsUser: 0
```
---

## Acknowledgments

- Built on [Buildah](https://buildah.io/) - The backbone of Kimia
- Inspired by [Kaniko](https://github.com/GoogleContainerTools/kaniko) - Pioneering daemonless builds
- Container tools from the [Containers](https://github.com/containers) organization
