# Build stage
FROM golang AS builder

ARG VERSION="dev"
ARG BUILD_DATE="0"
ARG COMMIT="unknown"
ARG BRANCH="unknown"

WORKDIR /app

# Copy source
COPY go.mod go.sum* ./
RUN go mod download || true

COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w \
        -X main.Version=${VERSION} \
        -X main.BuildDate=${BUILD_DATE} \
        -X main.CommitSHA=${COMMIT} \
        -X main.Branch=${BRANCH}" \
    -o smithy ./cmd/smithy

# Runtime stage
FROM alpine

# Install runtime dependencies
RUN apk add --no-cache \
    bash \
    ca-certificates \
    git \
    buildah \
    crun \
    slirp4netns \
    fuse-overlayfs \
    && update-ca-certificates

# Create smithy user
ARG SMITHY_UID=1000
ARG SMITHY_USER=smithy

RUN addgroup -g ${SMITHY_UID} ${SMITHY_USER} && \
    adduser -D -G ${SMITHY_USER} -u ${SMITHY_UID} ${SMITHY_USER}

# Setup subuid/subgid for rootless buildah
RUN echo "${SMITHY_USER}:100000:65536" >> /etc/subuid && \
    echo "${SMITHY_USER}:100000:65536" >> /etc/subgid

# Create directories
RUN mkdir -p /home/${SMITHY_USER}/.local/share/containers/storage && \
    mkdir -p /home/${SMITHY_USER}/.config/containers && \
    mkdir -p /tmp/lock && \
    chmod 1777 /tmp/lock

# Copy binary
COPY --from=builder /app/smithy /usr/local/bin/

# Set ownership
RUN chown -R ${SMITHY_USER}:${SMITHY_USER} /home/${SMITHY_USER}

# Container configuration
RUN cat > /etc/containers/containers.conf <<'EOF'
[engine]
events_logger="file"
network_backend="netavark"
userns="keep-id"
EOF

# Storage configuration
RUN cat > /home/${SMITHY_USER}/.config/containers/storage.conf <<'EOF'
[storage]
driver="vfs"
runroot="/tmp/containers/run"
graphroot="/home/smithy/.local/share/containers/storage"

[storage.options]
vfs.ignore_chown_errors="true"
EOF

# Environment
ENV BUILDAH_ISOLATION=chroot
ENV HOME=/home/smithy
ENV DOCKER_CONFIG=/home/smithy/.docker

USER ${SMITHY_UID}:${SMITHY_UID}
WORKDIR /home/${SMITHY_USER}

ENTRYPOINT ["/usr/local/bin/smithy"]
CMD ["--help"]

