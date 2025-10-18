# Stage 1: Build smithy binary
ARG VERSION="0.0.0-dev"
ARG BUILD_DATE="0"
ARG COMMIT="unknown"
ARG BRANCH="unknown"
ARG RELEASE="0"

ARG GOLANG_VERSION=1.25.2
ARG SMITHY_UID=1000
ARG SMITHY_USER=smithy
ARG BUILDAH_VERSION=1.41.5
ARG TARGETARCH

FROM golang:${GOLANG_VERSION}-alpine AS smithy-builder
ARG VERSION
ARG BUILD_DATE
ARG COMMIT
ARG BRANCH

WORKDIR /app

# Copy the entire src directory
COPY src/ .

# The go.mod already exists in src/, so just tidy dependencies
RUN go mod tidy

# Build from cmd/smithy
# Note: ldflags use 'main' because cmd/smithy is the main package
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w \
        -X main.Version=${VERSION} \
        -X main.BuildDate=${BUILD_DATE} \
        -X main.CommitSHA=${COMMIT} \
        -X main.Branch=${BRANCH}" \
    -o smithy ./cmd/smithy

# Stage 2: Build buildah v1.41.x with ONE LINE patched
FROM golang:${GOLANG_VERSION}-alpine AS buildah-builder
ARG BUILDAH_VERSION

USER 0

# Install build dependencies
RUN apk add --no-cache \
    bash \
    btrfs-progs-dev \
    build-base \
    git \
    go-md2man \
    gpgme-dev \
    libassuan-dev \
    libseccomp-dev \
    libselinux-dev \
    lvm2-dev \
    make \
    ostree-dev

WORKDIR /go/src/github.com/containers

RUN git clone https://github.com/containers/buildah.git && \
    cd buildah && \
    git checkout v${BUILDAH_VERSION}

# THE SINGLE LINE PATCH!
WORKDIR /go/src/github.com/containers/buildah
RUN sed -i 's|const defaultRootLockPath = "/run/lock/netavark.lock"|const defaultRootLockPath = "/tmp/lock/netavark.lock"|' \
    vendor/github.com/containers/common/libnetwork/netavark/const.go

# Verify the patch
RUN grep "defaultRootLockPath" vendor/github.com/containers/common/libnetwork/netavark/const.go

# Configure git (required for commit)
RUN git config --global user.email "dev@rapidfort.com" && \
    git config --global user.name "RapidFort Build"

# Commit the patch to make it "clean"
RUN git add -A && \
    git commit -m "RapidFort patch: Use /tmp/lock for netavark" && \
    git tag -f v${BUILDAH_VERSION}-rf

# Build patched buildah with RF version
RUN make BUILDTAGS="seccomp selinux" GIT_COMMIT="$(git rev-parse HEAD)" VERSION="${BUILDAH_VERSION}-rf" && \
    make install

# Build running image binary
FROM alpine AS run-prep-image
ARG SMITHY_UID
ARG SMITHY_USER
ARG TARGETARCH

RUN apk update && apk add gnutls

# Install runtime dependencies including fuse-overlayfs
RUN apk add --no-cache \
    bash \
    ca-certificates \
    crun \
    curl \
    device-mapper-libs \
    git \
    gpgme \
    iptables \
    ip6tables \
    libseccomp \
    ostree \
    shadow \
    shadow-uidmap \
    slirp4netns \
    netavark \
    aardvark-dns \
    fuse-overlayfs \
    xz && \
    update-ca-certificates && \
    chmod u+s /usr/bin/newuidmap /usr/bin/newgidmap

# Copy patched buildah
COPY --from=buildah-builder /usr/local/bin/buildah /usr/local/bin/buildah

# Copy smithy binary
COPY --from=smithy-builder /app/smithy /usr/local/bin/smithy
RUN chmod +x /usr/local/bin/smithy

# Create smithy user
RUN addgroup -g ${SMITHY_UID} ${SMITHY_USER} && \
    adduser -D -G ${SMITHY_USER} -u ${SMITHY_UID} ${SMITHY_USER}

# Setup subuid/subgid for smithy user (for rootless mode)
RUN echo "${SMITHY_USER}:100000:65536" >> /etc/subuid && \
    echo "${SMITHY_USER}:100000:65536" >> /etc/subgid

# System-wide policy (for root - UID 0)
RUN mkdir -p /etc/containers && \
    cat > /etc/containers/policy.json <<'EOF'
{
    "default": [{"type": "insecureAcceptAnything"}]
}
EOF

# Also create for root's home directory
RUN mkdir -p /root/.config/containers && \
    cat > /root/.config/containers/policy.json <<'EOF'
{
    "default": [{"type": "insecureAcceptAnything"}]
}
EOF

# ============================================================================
# Storage and Configuration Setup for BOTH root and rootless modes
# ============================================================================

# Create root storage directories (for rootful mode)
RUN mkdir -p /var/lib/containers/storage \
    /var/run/containers/storage \
    /etc/containers \
    /tmp/lock && \
    chmod 1777 /tmp/lock

# System-wide storage config (for root - UID 0)
RUN cat > /etc/containers/storage.conf <<'EOF'
[storage]
driver="vfs"
runroot="/var/run/containers/storage"
graphroot="/var/lib/containers/storage"

[storage.options]
vfs.ignore_chown_errors="false"

[storage.options.overlay]
mount_program="/usr/bin/fuse-overlayfs"
mountopt="nodev,metacopy=on"
EOF

# System-wide containers config
RUN cat > /etc/containers/containers.conf <<'EOF'
[engine]
events_logger="file"
network_backend="netavark"

[engine.runtimes]
runc = ["/usr/bin/runc"]
crun = ["/usr/bin/crun"]
EOF

# Create user directories for smithy (for rootless mode)
RUN mkdir -p /home/${SMITHY_USER}/.local/share/containers/storage && \
    mkdir -p /home/${SMITHY_USER}/.config/containers && \
    mkdir -p /home/${SMITHY_USER}/.docker

# User-specific storage config (for rootless - UID 1000)
RUN cat > /home/${SMITHY_USER}/.config/containers/storage.conf <<'EOF'
[storage]
driver="vfs"
runroot="/tmp/containers/run"
graphroot="/home/smithy/.local/share/containers/storage"

[storage.options]
vfs.ignore_chown_errors="true"

[storage.options.overlay]
mount_program="/usr/bin/fuse-overlayfs"
mountopt="nodev,metacopy=on"
EOF

# Registries config (user-specific)
RUN cat > /home/${SMITHY_USER}/.config/containers/registries.conf <<'EOF'
unqualified-search-registries = ['docker.io', 'quay.io']

[[registry]]
location = "docker.io"

[[registry]]
location = "quay.io"
EOF

# Policy (user-specific)
RUN cat > /home/${SMITHY_USER}/.config/containers/policy.json <<'EOF'
{
    "default": [{"type": "insecureAcceptAnything"}]
}
EOF

# ECR helper
RUN ECR_VERSION=$(curl -s https://api.github.com/repos/awslabs/amazon-ecr-credential-helper/releases/latest | grep '"tag_name"' | cut -d'"' -f4 | sed 's/^v//') && \
    ARCH=$(case ${TARGETARCH} in \
        "amd64") echo "amd64" ;; \
        "arm64") echo "arm64" ;; \
        *) echo "amd64" ;; \
    esac) && \
    curl -fsSL "https://amazon-ecr-credential-helper-releases.s3.us-east-2.amazonaws.com/${ECR_VERSION}/linux-${ARCH}/docker-credential-ecr-login" \
    -o /usr/local/bin/docker-credential-ecr-login && \
    chmod +x /usr/local/bin/docker-credential-ecr-login

# GCR helper
RUN GCR_VERSION=$(curl -s https://api.github.com/repos/GoogleCloudPlatform/docker-credential-gcr/releases/latest | grep '"tag_name"' | cut -d'"' -f4 | sed 's/^v//') && \
    ARCH=$(case ${TARGETARCH} in \
        "amd64") echo "amd64" ;; \
        "arm64") echo "arm64" ;; \
        *) echo "amd64" ;; \
    esac) && \
    curl -fsSL "https://github.com/GoogleCloudPlatform/docker-credential-gcr/releases/download/v${GCR_VERSION}/docker-credential-gcr_linux_${ARCH}-${GCR_VERSION}.tar.gz" \
    | tar xz -C /usr/local/bin/ docker-credential-gcr && \
    chmod +x /usr/local/bin/docker-credential-gcr

# Set ownership for smithy user directories
RUN chown -R ${SMITHY_USER}:${SMITHY_USER} /home/${SMITHY_USER}

# Environment variables
ENV BUILDAH_LOG_LEVEL=error
ENV HOME=/home/smithy
ENV DOCKER_CONFIG=/home/smithy/.docker

# BUILDAH_ISOLATION is set dynamically by smithy based on UID:
# - oci for root (UID 0) - doesn't need user namespaces
# - chroot for non-root (UID 1000+) - uses user namespaces

# Default to non-root user (rootless mode - recommended)
USER ${SMITHY_UID}:${SMITHY_UID}
WORKDIR /home/${SMITHY_USER}

ENV PATH="/home/${SMITHY_USER}/rapidfort:${PATH}"

LABEL org.opencontainers.image.source="https://github.com/rapidfort/smithy"
LABEL org.opencontainers.image.description="Smithy - Kubernetes-Native OCI Image Builder"

# Set smithy as the entrypoint
ENTRYPOINT ["/usr/local/bin/smithy"]

# Default command shows help
CMD ["--help"]