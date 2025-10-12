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

# Build patched buildah
RUN make BUILDTAGS="seccomp selinux" && \
    make install

FROM alpine AS run-prep-image
ARG SMITHY_UID
ARG SMITHY_USER

RUN apk update && apk add gnutls

# Install runtime dependencies
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

# Setup subuid/subgid for smithy user
RUN echo "${SMITHY_USER}:100000:65536" >> /etc/subuid && \
    echo "${SMITHY_USER}:100000:65536" >> /etc/subgid

# Create directories including /tmp/lock (writable!)
RUN mkdir -p /home/${SMITHY_USER}/.local/share/containers/storage && \
    mkdir -p /home/${SMITHY_USER}/.config/containers && \
    mkdir -p /home/${SMITHY_USER}/.docker && \
    mkdir -p /etc/containers && \
    mkdir -p /tmp/lock && \
    chmod 1777 /tmp/lock

# Containers config
RUN cat > /etc/containers/containers.conf <<'EOF'
[engine]
events_logger="file"
network_backend="netavark"
userns="keep-id"
EOF

# Storage config
RUN cat > /home/${SMITHY_USER}/.config/containers/storage.conf <<'EOF'
[storage]
driver="vfs"
runroot="/tmp/containers/run"
graphroot="/home/smithy/.local/share/containers/storage"

[storage.options]
vfs.ignore_chown_errors="true"
EOF

# Registries config
RUN cat > /home/${SMITHY_USER}/.config/containers/registries.conf <<'EOF'
unqualified-search-registries = ['docker.io', 'quay.io']

[[registry]]
location = "docker.io"

[[registry]]
location = "quay.io"
EOF

# Policy
RUN cat > /home/${SMITHY_USER}/.config/containers/policy.json <<'EOF'
{
    "default": [{"type": "insecureAcceptAnything"}]
}
EOF

# Fetch latest ECR helper version dynamically
RUN ECR_VERSION=$(curl -s https://api.github.com/repos/awslabs/amazon-ecr-credential-helper/releases/latest | grep '"tag_name"' | cut -d'"' -f4 | sed 's/^v//') && \
    curl -fsSL "https://amazon-ecr-credential-helper-releases.s3.us-east-2.amazonaws.com/${ECR_VERSION}/linux-amd64/docker-credential-ecr-login" \
    -o /usr/local/bin/docker-credential-ecr-login && \
    chmod +x /usr/local/bin/docker-credential-ecr-login

# Fetch latest GCR helper version dynamically
RUN GCR_VERSION=$(curl -s https://api.github.com/repos/GoogleCloudPlatform/docker-credential-gcr/releases/latest | grep '"tag_name"' | cut -d'"' -f4 | sed 's/^v//') && \
    curl -fsSL "https://github.com/GoogleCloudPlatform/docker-credential-gcr/releases/download/v${GCR_VERSION}/docker-credential-gcr_linux_amd64-${GCR_VERSION}.tar.gz" \
    | tar xz -C /usr/local/bin/ docker-credential-gcr && \
    chmod +x /usr/local/bin/docker-credential-gcr

# Set ownership
RUN chown -R ${SMITHY_USER}:${SMITHY_USER} /home/${SMITHY_USER}

ENV BUILDAH_LOG_LEVEL=error
ENV BUILDAH_ISOLATION=chroot
ENV HOME=/home/smithy
ENV DOCKER_CONFIG=/home/smithy/.docker

USER ${SMITHY_UID}:${SMITHY_UID}
WORKDIR /home/${SMITHY_USER}

ENV PATH="/home/${SMITHY_USER}/rapidfort:${PATH}"

# Set smithy as the entrypoint
ENTRYPOINT ["/usr/local/bin/smithy"]

# Default command shows help
CMD ["--help"]

