# Kimia Community Image Builder

Build container images using Kimia in Kubernetes.

## Quick Start

### 1. Install Prerequisites
```bash
# Install yq (YAML processor)
brew install yq  # macOS
# or
sudo wget -qO /usr/local/bin/yq https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64
sudo chmod +x /usr/local/bin/yq
```

### 2. Configure Your Registry
```bash
# Copy example config
cp user-config.yaml.example user-config.yaml

# Edit with your registry details
vim user-config.yaml
```

### 3. Build an Image
```bash
# Make script executable
chmod +x build.sh

# Build alpine image
./build.sh alpine

# Build nginx image
./build.sh nginx

# Dry run (see what will be applied)
./build.sh postgres --dry-run
```

## Authentication Options

### Option 1: Use Existing Kubernetes Secret
```yaml
auth:
  secretName: my-registry-secret
```

### Option 2: Let Script Create Secret
```yaml
auth:
  username: myuser
  password: ghp_xxxxxxxxxxxxx
  email: user@example.com
  server: ghcr.io
```

## Available Images

List available image templates:
```bash
ls templates/
```

## Advanced Usage

### Override Destination for Specific Images
```yaml
overrides:
  alpine:
    destination: ghcr.io/custom-org/alpine-special
```

### Monitor Build Progress
```bash
# After running build.sh, use the job name shown
kubectl logs -f job/kimia-alpine-build-xxxxx
```

### Clean Up
```bash
./build.sh alpine --delete
```

## Adding New Image Templates

1. Create new directory in `templates/`:
```bash
mkdir -p templates/myimage
```

2. Add job.yaml with REGISTRY_PLACEHOLDER:
```yaml
args:
  - --destination=REGISTRY_PLACEHOLDER/myimage
```

3. Build:
```bash
./build.sh myimage
```