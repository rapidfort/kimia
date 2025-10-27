# Troubleshooting Guide

Common issues and solutions when using Kimia.

---

## User Namespace Issues

### Error: Failed to Create User Namespace

**Error message:**
```
failed to create user namespace: operation not permitted
```

**Cause:** User namespaces are not enabled on the cluster nodes.

**Solution:**

1. Check current status:
```bash
cat /proc/sys/user/max_user_namespaces
# Should return > 0 (typically 15000)
```

2. Enable user namespaces:
```bash
# Temporary
sudo sysctl -w user.max_user_namespaces=15000

# Permanent
echo "user.max_user_namespaces=15000" | sudo tee -a /etc/sysctl.conf
sudo sysctl -p
```

3. Verify in Kubernetes:
```bash
kubectl run test --rm -it --image=busybox --restart=Never -- \
  cat /proc/sys/user/max_user_namespaces
```

---

## Permission Issues

### Error: Permission Denied

**Error message:**
```
permission denied while trying to connect
```

**Cause:** Incorrect security context configuration.

**Solution:**

Verify your security context includes all required settings:

```yaml
spec:
  securityContext:
    runAsNonRoot: true
    runAsUser: 1000
    runAsGroup: 1000
    fsGroup: 1000  # Important!
  
  containers:
  - name: kimia
    securityContext:
      runAsUser: 1000
      allowPrivilegeEscalation: true  # Required!
      capabilities:
        drop: [ALL]
        add: [SETUID, SETGID]  # Required!
```

---

## Cache Permission Issues

### Error: Cache Directory Permission Denied

**Error message:**
```
error: failed to write to cache directory: permission denied
unable to save layer to cache: operation not permitted
```

**Cause:** Cache directory is not writable by UID 1000.

**Solutions:**

**Option 1: Use fsGroup (Recommended)**
```yaml
spec:
  securityContext:
    fsGroup: 1000  # Automatically sets ownership
```

**Option 2: Init Container**
```yaml
spec:
  initContainers:
  - name: fix-permissions
    image: busybox
    command: ['sh', '-c', 'chown -R 1000:1000 /cache && chmod -R 755 /cache']
    securityContext:
      runAsUser: 0
    volumeMounts:
    - name: build-cache
      mountPath: /cache
```

**Option 3: Use emptyDir**
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

---

## Registry Authentication Issues

### Error: Unauthorized

**Error message:**
```
unauthorized: authentication required
```

**Solution:**

1. Verify secret exists:
```bash
kubectl get secret registry-credentials
```

2. Check secret contents:
```bash
kubectl get secret registry-credentials -o jsonpath='{.data.\.dockerconfigjson}' | base64 -d
```

3. Recreate secret:
```bash
kubectl delete secret registry-credentials
kubectl create secret docker-registry registry-credentials \
  --docker-server=myregistry.io \
  --docker-username=myuser \
  --docker-password=mypassword
```

4. Verify mount path:
```yaml
volumeMounts:
- name: docker-config
  mountPath: /home/kimia/.docker  # Must be this path
```

---

## Build Failures

### Error: No Space Left on Device

**Error message:**
```
error building: no space left on device
```

**Cause:** Insufficient ephemeral storage.

**Solution:**

Increase ephemeral storage limits:

```yaml
resources:
  limits:
    ephemeral-storage: "20Gi"  # Increase as needed
```

---

### Error: Git Clone Failed

**Error message:**
```
fatal: could not read Username
```

**Cause:** Git authentication not configured.

**Solution:**

Add Git token:

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

---

### Error: HEALTHCHECK Instruction Ignored

**Error message:**
```
Warning: HEALTHCHECK instruction ignored
```

**Cause:** Using OCI format (default) with Docker-specific instructions.

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

---

## Image Format Issues

### How to Check Image Format

```bash
kubectl exec <kimia-pod> -- buildah inspect myapp:latest | grep -i format
```

### OCI vs Docker Format

| Format | Use Case | Trade-offs |
|--------|----------|------------|
| OCI (default) | Modern standard | May not support HEALTHCHECK, SHELL |
| Docker | Legacy compatibility | Slightly larger metadata |

---

## Debugging Commands

### Check Pod Status

```bash
kubectl describe pod <kimia-pod-name>
```

### View Logs

```bash
kubectl logs <kimia-pod-name> -f
```

### Check User Namespace Support

```bash
kubectl exec <kimia-pod-name> -- cat /proc/self/uid_map
```

### Verify Storage

```bash
kubectl exec <kimia-pod-name> -- df -h
```

### Check Cache Permissions

```bash
kubectl exec <kimia-pod-name> -- ls -la /cache
```

### Test Network Connectivity

```bash
kubectl exec <kimia-pod-name> -- ping -c 3 myregistry.io
```

### Inspect Built Image

```bash
kubectl exec <kimia-pod-name> -- buildah images
```

### Check fsGroup

```bash
kubectl get pod <kimia-pod-name> -o jsonpath='{.spec.securityContext.fsGroup}'
```

---

## Performance Issues

### Slow Builds

**Solutions:**

1. **Enable caching:**
```yaml
args:
  - --cache
  - --cache-dir=/cache
```

2. **Use overlay storage driver:**
```yaml
args:
  - --storage-driver=overlay
```

3. **Increase resources:**
```yaml
resources:
  requests:
    memory: "4Gi"
    cpu: "2"
  limits:
    memory: "16Gi"
    cpu: "8"
```

---

## Network Issues

### Cannot Reach Registry

**Error:**
```
dial tcp: lookup myregistry.io: no such host
```

**Solutions:**

1. Check DNS:
```bash
kubectl exec <kimia-pod> -- nslookup myregistry.io
```

2. Check NetworkPolicy:
```bash
kubectl get networkpolicy -n <namespace>
```

3. Verify egress rules allow registry access

---

## Common Mistakes

### ❌ Wrong Volume Mount Path

```yaml
# Wrong
volumeMounts:
- name: docker-config
  mountPath: /kaniko/.docker  # Kaniko path, not Kimia!

# Correct
volumeMounts:
- name: docker-config
  mountPath: /home/kimia/.docker  # Kimia path
```

### ❌ Missing fsGroup

```yaml
# Wrong - cache permissions will fail
spec:
  securityContext:
    runAsUser: 1000

# Correct - cache will work
spec:
  securityContext:
    runAsUser: 1000
    fsGroup: 1000  # Required for volume permissions!
```

### ❌ Missing Capabilities

```yaml
# Wrong - user namespaces won't work
securityContext:
  runAsUser: 1000

# Correct
securityContext:
  runAsUser: 1000
  allowPrivilegeEscalation: true
  capabilities:
    drop: [ALL]
    add: [SETUID, SETGID]
```

---

## Still Having Issues?

1. 📖 Check [Installation Guide](installation.md)
2. 🎯 Review [Examples](examples.md)
3. 📝 Read [FAQ](faq.md)
4. 🐛 [Open an Issue](https://github.com/rapidfort/kimia/issues)

---

[Back to Main README](../README.md) | [Security Guide](security.md) | [Performance](performance.md)
