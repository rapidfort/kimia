# Examples

Common build scenarios and usage patterns for Kimia.

---

## Basic Examples

### 1. Simple Build from Local Context

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: basic-build
spec:
  ttlSecondsAfterFinished: 3600
  template:
    spec:
      restartPolicy: Never
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
```

### 2. Build from Git Repository

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: git-build
spec:
  ttlSecondsAfterFinished: 3600
  template:
    spec:
      restartPolicy: Never
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
          - --label=git.commit=abc123
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
```

### 3. Multi-Platform Build

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: multi-arch-build
spec:
  ttlSecondsAfterFinished: 3600
  template:
    spec:
      restartPolicy: Never
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
          - --destination=myregistry.io/myapp:v1.0.0-arm64
          - --custom-platform=linux/arm64
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
```

---

## Advanced Examples

### Build with Caching

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: cached-build
spec:
  template:
    spec:
      restartPolicy: Never
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000
      containers:
      - name: kimia
        image: ghcr.io/rapidfort/kimia:latest
        args:
          - --context=.
          - --destination=myregistry.io/myapp:latest
          - --cache
          - --cache-dir=/cache
        securityContext:
          allowPrivilegeEscalation: true
          capabilities:
            drop: [ALL]
            add: [SETUID, SETGID]
        volumeMounts:
        - name: docker-config
          mountPath: /home/kimia/.docker
        - name: build-cache
          mountPath: /cache
      volumes:
      - name: docker-config
        secret:
          secretName: registry-credentials
      - name: build-cache
        persistentVolumeClaim:
          claimName: kimia-cache
```

### Reproducible Build

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: reproducible-build
spec:
  template:
    spec:
      restartPolicy: Never
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000
      containers:
      - name: kimia
        image: ghcr.io/rapidfort/kimia:latest
        args:
          - --context=https://github.com/myorg/myapp.git
          - --git-revision=abc123
          - --destination=myregistry.io/myapp:v1.0.0
          - --reproducible
        env:
        - name: SOURCE_DATE_EPOCH
          value: "1609459200"  # 2021-01-01 00:00:00 UTC
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
```

### Build with Private Git Repository

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: private-git-build
spec:
  template:
    spec:
      restartPolicy: Never
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000
      containers:
      - name: kimia
        image: ghcr.io/rapidfort/kimia:latest
        args:
          - --context=https://github.com/myorg/private-repo.git
          - --git-branch=main
          - --git-token-file=/secrets/github-token
          - --git-token-user=oauth2
          - --destination=myregistry.io/myapp:latest
        securityContext:
          allowPrivilegeEscalation: true
          capabilities:
            drop: [ALL]
            add: [SETUID, SETGID]
        volumeMounts:
        - name: docker-config
          mountPath: /home/kimia/.docker
        - name: git-token
          mountPath: /secrets
      volumes:
      - name: docker-config
        secret:
          secretName: registry-credentials
      - name: git-token
        secret:
          secretName: github-token
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
      restartPolicy: Never
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000
      containers:
      - name: kimia
        image: ghcr.io/rapidfort/kimia:latest
        env:
        - name: BUILDAH_FORMAT
          value: "docker"
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
```

**Dockerfile with HEALTHCHECK:**
```dockerfile
FROM nginx:alpine

# Docker-specific instruction (requires BUILDAH_FORMAT=docker)
HEALTHCHECK --interval=30s --timeout=3s \
  CMD curl -f http://localhost/ || exit 1

SHELL ["/bin/bash", "-c"]
COPY index.html /usr/share/nginx/html/
```

### Export to TAR Archive

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: tar-export-build
spec:
  template:
    spec:
      restartPolicy: Never
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000
      containers:
      - name: kimia
        image: ghcr.io/rapidfort/kimia:latest
        args:
          - --context=.
          - --destination=myapp:latest
          - --tar-path=/output/myapp.tar
          - --storage-driver=native
          - --no-push
        securityContext:
          allowPrivilegeEscalation: true
          capabilities:
            drop: [ALL]
            add: [SETUID, SETGID]
        volumeMounts:
        - name: output
          mountPath: /output
      volumes:
      - name: output
        emptyDir: {}
```

---

## CI/CD Integration Examples

### GitHub Actions

```yaml
name: Build with Kimia
on:
  push:
    branches: [main]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Set up kubectl
        uses: azure/setup-kubectl@v3
      
      - name: Configure kubeconfig
        run: |
          echo "${{ secrets.KUBECONFIG }}" > $HOME/.kube/config
      
      - name: Create registry secret
        run: |
          kubectl create secret docker-registry registry-credentials \
            --docker-server=ghcr.io \
            --docker-username=${{ github.actor }} \
            --docker-password=${{ secrets.GITHUB_TOKEN }} \
            --dry-run=client -o yaml | kubectl apply -f -
      
      - name: Build with Kimia
        run: |
          cat <<EOF | kubectl apply -f -
          apiVersion: batch/v1
          kind: Job
          metadata:
            name: kimia-build-${{ github.run_number }}
          spec:
            ttlSecondsAfterFinished: 600
            template:
              spec:
                restartPolicy: Never
                securityContext:
                  runAsNonRoot: true
                  runAsUser: 1000
                  fsGroup: 1000
                containers:
                - name: kimia
                  image: ghcr.io/rapidfort/kimia:latest
                  args:
                    - --context=https://github.com/${{ github.repository }}.git
                    - --git-revision=${{ github.sha }}
                    - --destination=ghcr.io/${{ github.repository }}:${{ github.sha }}
                    - --destination=ghcr.io/${{ github.repository }}:latest
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
          EOF
          
          kubectl wait --for=condition=complete job/kimia-build-${{ github.run_number }} --timeout=600s
          kubectl logs job/kimia-build-${{ github.run_number }}
```

### GitLab CI

```yaml
build:
  stage: build
  image: bitnami/kubectl:latest
  script:
    - kubectl create secret docker-registry registry-credentials
        --docker-server=$CI_REGISTRY
        --docker-username=$CI_REGISTRY_USER
        --docker-password=$CI_REGISTRY_PASSWORD
        --dry-run=client -o yaml | kubectl apply -f -
    
    - |
      cat <<EOF | kubectl apply -f -
      apiVersion: batch/v1
      kind: Job
      metadata:
        name: kimia-build-$CI_PIPELINE_ID
      spec:
        ttlSecondsAfterFinished: 600
        template:
          spec:
            restartPolicy: Never
            securityContext:
              runAsNonRoot: true
              runAsUser: 1000
              fsGroup: 1000
            containers:
            - name: kimia
              image: ghcr.io/rapidfort/kimia:latest
              args:
                - --context=$CI_REPOSITORY_URL
                - --git-revision=$CI_COMMIT_SHA
                - --destination=$CI_REGISTRY_IMAGE:$CI_COMMIT_SHA
                - --destination=$CI_REGISTRY_IMAGE:latest
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
      EOF
    
    - kubectl wait --for=condition=complete job/kimia-build-$CI_PIPELINE_ID --timeout=600s
```

---

## Complete Example: Production Build Pipeline

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: production-build
  namespace: builds
  labels:
    app: kimia
    environment: production
  annotations:
    build.initiated-by: "ci-pipeline"
    build.source: "github.com/myorg/myapp"
spec:
  ttlSecondsAfterFinished: 3600
  template:
    metadata:
      labels:
        app: kimia
        build-id: "prod-12345"
    spec:
      restartPolicy: Never
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
        args:
          - --context=https://github.com/myorg/myapp.git
          - --git-branch=main
          - --git-revision=abc123
          - --dockerfile=Dockerfile
          - --destination=myregistry.io/myapp:v1.0.0
          - --destination=myregistry.io/myapp:latest
          - --build-arg=VERSION=1.0.0
          - --build-arg=BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
          - --label=version=1.0.0
          - --label=git.commit=abc123
          - --label=built-by=kimia
          - --cache
          - --cache-dir=/cache
          - --reproducible
          - --verbosity=info
          - --push-retry=3
        
        env:
        - name: SOURCE_DATE_EPOCH
          value: "1609459200"
        
        securityContext:
          runAsNonRoot: true
          runAsUser: 1000
          allowPrivilegeEscalation: true
          capabilities:
            drop: [ALL]
            add: [SETUID, SETGID]
          seccompProfile:
            type: RuntimeDefault
        
        resources:
          requests:
            memory: "2Gi"
            cpu: "1"
          limits:
            memory: "8Gi"
            cpu: "4"
            ephemeral-storage: "10Gi"
        
        volumeMounts:
        - name: docker-config
          mountPath: /home/kimia/.docker
          readOnly: true
        - name: build-cache
          mountPath: /cache
      
      volumes:
      - name: docker-config
        secret:
          secretName: registry-credentials
      - name: build-cache
        persistentVolumeClaim:
          claimName: kimia-build-cache
```

---

[Back to Main README](../README.md) | [GitOps Integration](gitops.md) | [CLI Reference](cli-reference.md)
