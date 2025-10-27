# Performance Optimization

Optimize Kimia builds for speed and efficiency.

---

## Build Caching

### Enable Layer Caching

```yaml
args:
  - --context=.
  - --destination=myregistry.io/myapp:latest
  - --cache
  - --cache-dir=/cache
```

### Persistent Cache with PVC

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: kimia-cache
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 20Gi
---
apiVersion: batch/v1
kind: Job
metadata:
  name: cached-build
spec:
  template:
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000  # Important for cache permissions!
      containers:
      - name: kimia
        image: ghcr.io/rapidfort/kimia:latest
        args:
          - --context=.
          - --destination=myregistry.io/myapp:latest
          - --cache
          - --cache-dir=/cache
        volumeMounts:
        - name: build-cache
          mountPath: /cache
      volumes:
      - name: build-cache
        persistentVolumeClaim:
          claimName: kimia-cache
```

---

## Storage Driver Selection

### Native (VFS) vs Overlay

| Driver | Speed | Compatibility | TAR Export | Best For |
|--------|-------|---------------|------------|----------|
| native (default) | Good | ✅ Maximum | ✅ Reliable | Default, TAR exports |
| overlay | ✅ Faster | Requires kernel support | ⚠️ May have issues | Production builds |

### Using Overlay Driver

```yaml
args:
  - --context=.
  - --destination=myregistry.io/myapp:latest
  - --storage-driver=overlay
```

**Performance improvement:** ~20-30% faster builds

---

## Resource Optimization

### Size-Based Resource Allocation

```yaml
# Small builds (<500MB)
resources:
  requests:
    memory: "1Gi"
    cpu: "500m"
  limits:
    memory: "4Gi"
    cpu: "2"
    ephemeral-storage: "5Gi"

# Medium builds (500MB-2GB)
resources:
  requests:
    memory: "2Gi"
    cpu: "1"
  limits:
    memory: "8Gi"
    cpu: "4"
    ephemeral-storage: "10Gi"

# Large builds (>2GB)
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

## Parallel Builds

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: parallel-builds
spec:
  parallelism: 3  # Run 3 builds concurrently
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
        # ... configuration
```

---

## Dockerfile Optimization

### Multi-Stage Builds

```dockerfile
# Build stage
FROM golang:1.21 AS builder
WORKDIR /app
COPY go.* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o myapp

# Final stage - much smaller
FROM alpine:3.18
COPY --from=builder /app/myapp /myapp
ENTRYPOINT ["/myapp"]
```

### Layer Caching Best Practices

```dockerfile
# ❌ Bad - invalidates cache on any file change
COPY . /app
RUN npm install

# ✅ Good - cache dependencies separately
COPY package*.json /app/
RUN npm install
COPY . /app
```

---

## Build Time Comparison

| Optimization | Improvement | Notes |
|--------------|-------------|-------|
| Enable caching | 50-80% | First build slow, subsequent fast |
| Overlay driver | 20-30% | Requires kernel support |
| Multi-stage builds | 40-60% | Smaller final images |
| Optimized Dockerfile | 30-50% | Better layer caching |
| Parallel builds | 2-3x | For multiple images |

---

[Back to Main README](../README.md) | [Troubleshooting](troubleshooting.md) | [Examples](examples.md)
