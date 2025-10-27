# Installation Guide

Complete installation instructions for Kimia across different Kubernetes platforms.

---

## Prerequisites

- **Kubernetes:** 1.21 or higher
- **User Namespaces:** Must be enabled on cluster nodes
- **Registry Credentials:** For pushing images

---

## Quick Check

Before installing, verify your cluster supports user namespaces:

```bash
# SSH to a node or use a debug pod
kubectl run test-userns --rm -it --image=busybox --restart=Never -- cat /proc/sys/user/max_user_namespaces

# Should return a number > 0 (typically 15000 or higher)
```

---

## Enable User Namespaces

User namespaces are **required** for Kimia's security model. Enable them on your cluster nodes:

### Check Current Status

```bash
# On cluster node
cat /proc/sys/user/max_user_namespaces

# 0 = disabled
# >0 = enabled (usually 15000 or higher)
```

### Enable User Namespaces

#### Temporary (Until Reboot)

```bash
sudo sysctl -w user.max_user_namespaces=15000
```

#### Permanent

```bash
# Add to sysctl configuration
echo "user.max_user_namespaces=15000" | sudo tee -a /etc/sysctl.conf

# Apply changes
sudo sysctl -p
```

---

## Platform-Specific Setup

### AWS EKS

User namespaces are **enabled by default** on standard Amazon Linux 2023 nodes.

#### Verification

```bash
# Deploy a test pod to verify
kubectl run userns-test --rm -it --image=busybox --restart=Never -- \
  cat /proc/sys/user/max_user_namespaces
```

#### Bottlerocket Nodes

If using Bottlerocket AMI, enable via user data:

```toml
[settings.kernel.sysctl]
"user.max_user_namespaces" = "15000"
```

#### DaemonSet Method (Alternative)

Enable on all nodes using a DaemonSet:

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
          echo "User namespaces enabled: $(cat /proc/sys/user/max_user_namespaces)"
          sleep infinity
```

---

### Google GKE

User namespaces are **enabled by default** on GKE nodes.

#### Verification

```bash
# Verify user namespace support
kubectl run userns-test --rm -it --image=busybox --restart=Never -- \
  cat /proc/sys/user/max_user_namespaces

# Should return a value > 0
```

No additional configuration required! ✅

---

### Azure AKS

Enable user namespaces on Ubuntu-based node pools:

#### New Node Pool

```bash
az aks nodepool add \
  --resource-group myResourceGroup \
  --cluster-name myAKSCluster \
  --name kimiabuild \
  --node-count 3 \
  --enable-user-namespaces
```

#### Existing Node Pool

```bash
az aks nodepool update \
  --resource-group myResourceGroup \
  --cluster-name myAKSCluster \
  --name nodepool1 \
  --enable-user-namespaces
```

#### Verification

```bash
kubectl run userns-test --rm -it --image=busybox --restart=Never -- \
  cat /proc/sys/user/max_user_namespaces
```

---

### Red Hat OpenShift

User namespaces are available on **OpenShift 4.7+**.

#### Enable via MachineConfig

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

**Base64 decoded content:**
```
user.max_user_namespaces=15000
```

#### Apply Configuration

```bash
oc apply -f machine-config-user-namespaces.yaml

# Wait for nodes to reboot and apply changes
oc get machineconfigpool -w
```

#### Verification

```bash
oc run userns-test --rm -it --image=busybox --restart=Never -- \
  cat /proc/sys/user/max_user_namespaces
```

---

### Other Kubernetes Distributions

#### k3s

User namespaces are typically enabled by default on k3s.

```bash
# Verify
kubectl run userns-test --rm -it --image=busybox --restart=Never -- \
  cat /proc/sys/user/max_user_namespaces
```

#### Rancher/RKE2

Enable on cluster nodes via cloud-init or node configuration:

```yaml
# cloud-config.yaml
runcmd:
  - echo "user.max_user_namespaces=15000" >> /etc/sysctl.conf
  - sysctl -p
```

#### MicroK8s

```bash
# On host running MicroK8s
sudo sysctl -w user.max_user_namespaces=15000
echo "user.max_user_namespaces=15000" | sudo tee -a /etc/sysctl.conf
```

#### kind (Local Development)

User namespaces may not work in kind due to nested containerization. Use for testing only:

```bash
# Create kind cluster
kind create cluster --name kimia-test

# User namespaces might be limited
# Test builds may work but production features might be limited
```

---

## Registry Credentials Setup

Kimia needs credentials to push images to your container registry.

### Option 1: From Existing Docker Config

```bash
kubectl create secret generic registry-credentials \
  --from-file=.dockerconfigjson=$HOME/.docker/config.json \
  --type=kubernetes.io/dockerconfigjson
```

### Option 2: Create Manually

```bash
kubectl create secret docker-registry registry-credentials \
  --docker-server=myregistry.io \
  --docker-username=myuser \
  --docker-password=mypassword \
  --docker-email=myemail@example.com
```

### Option 3: Multiple Registries

Create a Docker config JSON file:

```json
{
  "auths": {
    "https://index.docker.io/v1/": {
      "auth": "base64_encoded_username:password"
    },
    "ghcr.io": {
      "auth": "base64_encoded_token"
    },
    "myregistry.io": {
      "auth": "base64_encoded_username:password"
    }
  }
}
```

```bash
kubectl create secret generic registry-credentials \
  --from-file=.dockerconfigjson=config.json \
  --type=kubernetes.io/dockerconfigjson
```

### Verify Secret

```bash
kubectl get secret registry-credentials -o yaml
```

---

## Deploy Your First Build

### Basic Build Job

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: kimia-test-build
  namespace: default
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
        - --context=https://github.com/nginx/docker-nginx.git
        - --dockerfile=mainline/alpine/Dockerfile
        - --destination=myregistry.io/nginx:test
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

### Deploy and Monitor

```bash
# Deploy the job
kubectl apply -f kimia-test-build.yaml

# Watch job status
kubectl get jobs -w

# View logs
kubectl logs -f job/kimia-test-build

# Check completion
kubectl get job kimia-test-build -o jsonpath='{.status.succeeded}'
```

---

## Verify Installation

Run the environment check:

```bash
kubectl run kimia-check --rm -it --image=ghcr.io/rapidfort/kimia:latest \
  --restart=Never \
  --overrides='
{
  "spec": {
    "securityContext": {
      "runAsUser": 1000,
      "fsGroup": 1000
    },
    "containers": [
      {
        "name": "kimia-check",
        "image": "ghcr.io/rapidfort/kimia:latest",
        "command": ["kimia", "check-environment"],
        "securityContext": {
          "allowPrivilegeEscalation": true,
          "capabilities": {
            "drop": ["ALL"],
            "add": ["SETUID", "SETGID"]
          }
        }
      }
    ]
  }
}
'
```

**Expected output:**
```
✅ User namespaces: Enabled
✅ SETUID capability: Available
✅ SETGID capability: Available
✅ Storage driver: OK
✅ Buildah: v1.33.0

Kimia is ready to build! 🚀
```

---

## Troubleshooting Installation

### User Namespaces Not Available

**Error:**
```
failed to create user namespace: operation not permitted
```

**Solution:**
```bash
# Check current value
cat /proc/sys/user/max_user_namespaces

# If 0, enable user namespaces
sudo sysctl -w user.max_user_namespaces=15000
```

### Permission Denied Errors

**Error:**
```
permission denied while trying to connect
```

**Check security context:**
```yaml
securityContext:
  runAsUser: 1000
  fsGroup: 1000  # Important!
  allowPrivilegeEscalation: true
  capabilities:
    drop: [ALL]
    add: [SETUID, SETGID]
```

### Registry Authentication Failed

**Error:**
```
unauthorized: authentication required
```

**Verify secret:**
```bash
# Check secret exists
kubectl get secret registry-credentials

# Verify contents
kubectl get secret registry-credentials -o jsonpath='{.data.\.dockerconfigjson}' | base64 -d

# Recreate if needed
kubectl delete secret registry-credentials
kubectl create secret docker-registry registry-credentials \
  --docker-server=myregistry.io \
  --docker-username=myuser \
  --docker-password=mypassword
```

### Pod Security Standards

If your cluster enforces Pod Security Standards, Kimia requires **Restricted** level with `allowPrivilegeEscalation: true`:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: kimia-builds
  labels:
    pod-security.kubernetes.io/enforce: restricted
    pod-security.kubernetes.io/enforce-version: latest
    pod-security.kubernetes.io/audit: restricted
    pod-security.kubernetes.io/warn: restricted
```

---

## Next Steps

✅ Installation complete! Now you can:

1. 📖 [Read CLI Reference](cli-reference.md) - Learn all available options
2. 🎯 [Try Examples](examples.md) - Common build scenarios
3. 🚀 [Setup GitOps Integration](gitops.md) - Integrate with ArgoCD, Flux, Tekton
4. 🔒 [Review Security Best Practices](security.md) - Harden your builds

---

## Quick Reference

```bash
# Enable user namespaces (permanent)
echo "user.max_user_namespaces=15000" | sudo tee -a /etc/sysctl.conf
sudo sysctl -p

# Create registry secret
kubectl create secret docker-registry registry-credentials \
  --docker-server=myregistry.io \
  --docker-username=myuser \
  --docker-password=mypassword

# Deploy test build
kubectl apply -f kimia-test-build.yaml

# Check logs
kubectl logs -f job/kimia-test-build
```

---

[Back to Main README](../README.md) | [CLI Reference](cli-reference.md) | [Examples](examples.md)
