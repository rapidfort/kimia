# Security Guide

Comprehensive security guide for running Kimia in production environments.

---

## Security Architecture

Kimia provides defense-in-depth security through multiple layers:

### 1. Rootless Operation

Kimia runs as a **non-root user (UID 1000)**, providing the first line of defense:

```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 1000
  runAsGroup: 1000
```

**Benefit:** Even if the container is compromised, attackers only have unprivileged user access.

### 2. User Namespace Isolation

User namespaces create an additional security boundary:

```
Container View          Host Reality
─────────────          ────────────
UID 0 (root)     →     UID 1000 (unprivileged kimia user)
UID 1            →     UID 100000
UID 2            →     UID 100001
...                    ...
UID 65535        →     UID 165535
```

**Benefit:** Container processes that appear to run as root are actually unprivileged on the host.

### 3. Minimal Capabilities

Kimia requires only two Linux capabilities:

```yaml
capabilities:
  drop: [ALL]
  add: [SETUID, SETGID]  # Only for user namespace operations
```

**Why these capabilities?**
- `SETUID` - Required to create user namespace UID mappings
- `SETGID` - Required to create user namespace GID mappings

**Benefit:** Minimal attack surface compared to privileged containers or containers with CAP_SYS_ADMIN.

### 4. No Privileged Mode

Kimia **does not require** `privileged: true`, unlike some container builders.

```yaml
# ❌ NOT needed
securityContext:
  privileged: true

# ✅ Kimia works without privileged mode
securityContext:
  allowPrivilegeEscalation: true  # Only this is needed
```

### 5. Daemonless Architecture

No Docker or Podman daemon required:
- Reduces attack surface
- No daemon socket exposure
- No shared daemon state
- Isolated build processes

---

## Pod Security Standards

Kimia is compatible with Kubernetes **Pod Security Standards at the Restricted level** (with `allowPrivilegeEscalation: true`).

### Full Restricted Pod Configuration

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
    args:
      - --context=.
      - --destination=myregistry.io/myapp:latest
    securityContext:
      runAsNonRoot: true
      runAsUser: 1000
      allowPrivilegeEscalation: true  # Required for user namespaces
      capabilities:
        drop: [ALL]
        add: [SETUID, SETGID]       # Minimal capabilities
      seccompProfile:
        type: RuntimeDefault
    volumeMounts:
    - name: docker-config
      mountPath: /home/kimia/.docker
      readOnly: true
  
  volumes:
  - name: docker-config
    secret:
      secretName: registry-credentials
```

### Pod Security Standard Compliance

| Restriction | Requirement | Kimia Status |
|-------------|-------------|--------------|
| `runAsNonRoot` | Must be true | ✅ Required (UID 1000) |
| `allowPrivilegeEscalation` | Must be false* | ⚠️ True (for user namespaces) |
| `capabilities` | Can only add SETUID/SETGID | ✅ Only SETUID & SETGID |
| `seccompProfile` | Must be set | ✅ RuntimeDefault |
| `privileged` | Must be false | ✅ Not required |
| `hostNetwork` | Must be false | ✅ Not required |
| `hostPID` | Must be false | ✅ Not required |

*`allowPrivilegeEscalation: true` is needed specifically for user namespace operations, which is safer than alternatives.

---

## Network Policies

Restrict Kimia's network access using Kubernetes NetworkPolicies:

### Basic Network Policy

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: kimia-network-policy
  namespace: builds
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

### Strict Network Policy (Registry-Only)

For maximum security, allow only specific registry access:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: kimia-strict-policy
  namespace: builds
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
  
  # Allow only specific registry
  - to:
    - podSelector: {}
      namespaceSelector:
        matchLabels:
          name: registry-namespace
    ports:
    - protocol: TCP
      port: 443
  
  # Explicitly allow ghcr.io (example)
  - to:
    - namespaceSelector: {}
    ports:
    - protocol: TCP
      port: 443
    # Add IP blocks for specific registries if needed
```

---

## Resource Limits

Always configure resource limits to prevent resource exhaustion attacks:

### Recommended Resource Configuration

```yaml
resources:
  requests:
    memory: "2Gi"
    cpu: "1"
  limits:
    memory: "8Gi"
    cpu: "4"
    ephemeral-storage: "10Gi"  # Important for build artifacts!
```

### Resource Sizing Guidelines

| Build Size | Memory Request | Memory Limit | CPU Request | CPU Limit | Storage |
|------------|----------------|--------------|-------------|-----------|---------|
| Small (<500MB) | 1Gi | 4Gi | 500m | 2 | 5Gi |
| Medium (500MB-2GB) | 2Gi | 8Gi | 1 | 4 | 10Gi |
| Large (>2GB) | 4Gi | 16Gi | 2 | 8 | 20Gi |

### Why Ephemeral Storage Matters

Build processes create temporary files:
- Layer extraction
- Build context
- Intermediate artifacts

Without sufficient ephemeral storage, builds will fail with:
```
no space left on device
```

---

## Secrets Management

### Best Practices for Registry Credentials

#### Option 1: Kubernetes Secrets (Recommended)

```bash
# Create from Docker config
kubectl create secret generic registry-credentials \
  --from-file=.dockerconfigjson=$HOME/.docker/config.json \
  --type=kubernetes.io/dockerconfigjson
```

#### Mount Secrets Read-Only

```yaml
volumeMounts:
- name: docker-config
  mountPath: /home/kimia/.docker
  readOnly: true  # ✅ Prevent modification
```

#### Option 2: External Secrets Operator

For production environments, use External Secrets Operator:

```yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: registry-credentials
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: vault-backend
    kind: SecretStore
  target:
    name: registry-credentials
    template:
      type: kubernetes.io/dockerconfigjson
      data:
        .dockerconfigjson: "{{ .registryAuth | toString }}"
  data:
  - secretKey: registryAuth
    remoteRef:
      key: container-registry
      property: dockerconfigjson
```

#### Option 3: Workload Identity

Use cloud provider workload identity instead of static credentials:

**GKE Workload Identity:**
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kimia-builder
  annotations:
    iam.gke.io/gcp-service-account: kimia-builder@project.iam.gserviceaccount.com
---
apiVersion: batch/v1
kind: Job
spec:
  template:
    spec:
      serviceAccountName: kimia-builder
      # No secrets needed - uses workload identity
```

**EKS IRSA (IAM Roles for Service Accounts):**
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kimia-builder
  annotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::ACCOUNT:role/KimiaBuilderRole
```

---

## Image Scanning

Integrate security scanning into your build pipeline:

### Trivy Integration

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: kimia-build-and-scan
spec:
  template:
    spec:
      restartPolicy: Never
      containers:
      # Build container
      - name: kimia
        image: ghcr.io/rapidfort/kimia:latest
        args:
          - --context=.
          - --destination=myregistry.io/myapp:latest
        # ... security context ...
      
      # Scanning container (runs after build)
      - name: trivy-scan
        image: aquasec/trivy:latest
        command:
        - sh
        - -c
        - |
          trivy image \
            --severity HIGH,CRITICAL \
            --exit-code 1 \
            myregistry.io/myapp:latest
```

### Grype Integration

```yaml
- name: grype-scan
  image: anchore/grype:latest
  command:
  - grype
  - myregistry.io/myapp:latest
  - --fail-on
  - critical
```

---

## Audit Logging

Enable audit logging for compliance:

### Job Audit Annotations

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: kimia-build
  annotations:
    build.initiated-by: "john@company.com"
    build.source: "https://github.com/org/repo.git"
    build.commit: "abc123"
    build.timestamp: "2024-01-15T10:30:00Z"
spec:
  # ... job spec ...
```

### Pod Audit Labels

```yaml
spec:
  template:
    metadata:
      labels:
        app: kimia
        build-id: "build-12345"
        project: "myapp"
        environment: "production"
```

---

## Security Best Practices Checklist

### ✅ Container Security

- [ ] Run as non-root (UID 1000)
- [ ] Use minimal capabilities (SETUID, SETGID only)
- [ ] No privileged mode
- [ ] Set seccomp profile (RuntimeDefault)
- [ ] Enable read-only root filesystem where possible
- [ ] Mount secrets as read-only

### ✅ Network Security

- [ ] Implement NetworkPolicies
- [ ] Restrict egress to known registries
- [ ] Use private registries when possible
- [ ] Enable TLS for all registry connections
- [ ] Consider using registry mirrors

### ✅ Resource Security

- [ ] Set resource requests and limits
- [ ] Configure ephemeral storage limits
- [ ] Use node selectors/taints for isolation
- [ ] Implement pod disruption budgets

### ✅ Secrets & Credentials

- [ ] Never hardcode credentials
- [ ] Use Kubernetes secrets or external secret managers
- [ ] Mount credentials as read-only
- [ ] Rotate credentials regularly
- [ ] Use workload identity when available

### ✅ Build Security

- [ ] Pin base images by digest
- [ ] Scan images for vulnerabilities
- [ ] Use reproducible builds
- [ ] Sign images with Cosign
- [ ] Generate SBOMs

### ✅ Compliance & Audit

- [ ] Enable audit logging
- [ ] Tag builds with metadata
- [ ] Implement RBAC for build jobs
- [ ] Monitor build failures
- [ ] Regular security reviews

---

## Security Incident Response

### If a Build Pod is Compromised

1. **Immediate Actions:**
   ```bash
   # Delete the compromised pod
   kubectl delete pod <kimia-pod>
   
   # Check for other suspicious pods
   kubectl get pods -l app=kimia --all-namespaces
   
   # Review audit logs
   kubectl logs <kimia-pod> --previous
   ```

2. **Investigation:**
   - Check what images were built/pushed
   - Review registry access logs
   - Verify no malicious images were created
   - Check for privilege escalation attempts

3. **Mitigation:**
   - Rotate registry credentials
   - Scan all recently built images
   - Review and tighten NetworkPolicies
   - Update RBAC policies if needed

---

## Advanced Security

### AppArmor Profile

Create custom AppArmor profile for additional hardening:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: kimia-build
  annotations:
    container.apparmor.security.beta.kubernetes.io/kimia: localhost/kimia-profile
spec:
  # ... pod spec ...
```

### Seccomp Profile

Use custom seccomp profile:

```yaml
securityContext:
  seccompProfile:
    type: Localhost
    localhostProfile: profiles/kimia-seccomp.json
```

### SELinux

On SELinux-enabled systems:

```yaml
securityContext:
  seLinuxOptions:
    level: "s0:c123,c456"
```

---

## Compliance

### NIST 800-190 Compliance

Kimia addresses NIST 800-190 container security recommendations:

- ✅ **Runtime Defense:** Rootless operation
- ✅ **Image Security:** Supports image scanning integration
- ✅ **Registry Security:** TLS enforcement
- ✅ **Orchestrator Security:** Kubernetes-native with Pod Security Standards
- ✅ **Host Security:** User namespace isolation

### CIS Kubernetes Benchmarks

Kimia aligns with CIS Kubernetes security benchmarks:

- ✅ 5.2.1 - Minimize admission of privileged containers
- ✅ 5.2.5 - Minimize admission of containers with capabilities
- ✅ 5.2.6 - Minimize admission of root containers
- ✅ 5.7.3 - Apply Security Context to Pods and Containers

---

## Summary

Kimia provides **defense-in-depth security** through:

1. 🔒 **Rootless operation** - UID 1000, not root
2. 🛡️ **User namespace isolation** - Container escape protection
3. ⚡ **Minimal capabilities** - Only SETUID & SETGID
4. 🚫 **No privileged mode** - Reduced attack surface
5. 🌐 **Network policies** - Restrict egress traffic
6. 📊 **Resource limits** - Prevent DoS attacks
7. 🔐 **Secrets management** - Secure credential handling
8. 📝 **Audit logging** - Compliance and monitoring

**Result:** Production-ready, secure container builds in Kubernetes! 🎉

---

[Back to Main README](../README.md) | [Installation](installation.md) | [Troubleshooting](troubleshooting.md)
