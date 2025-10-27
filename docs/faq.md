# Frequently Asked Questions (FAQ)

Common questions about Kimia.

---

## General Questions

### Q: How does Kimia differ from Kaniko?

**A:** Kimia uses user namespaces for true rootless operation (runs as UID 1000), while Kaniko runs as root (UID 0). Kimia also provides:
- Better support for complex Dockerfiles with ownership changes
- Native reproducible builds
- Pod Security Standards (Restricted) compliance

See the [Comparison Guide](comparison.md) for details.

---

### Q: Can I use Kimia outside Kubernetes?

**A:** Yes! Kimia can run as a standard container:

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

---

### Q: Does Kimia support multi-architecture builds?

**A:** Yes, use the `--custom-platform` flag:

```bash
kimia --custom-platform=linux/arm64 ...
```

---

### Q: What's the difference between kimia and kimia-bud?

**A:** Both provide the same functionality and security:
- **kimia**: Based on BuildKit
- **kimia-bud**: Based on Buildah

Choose based on your preference - both support the same Kimia CLI arguments.

---

## Technical Questions

### Q: Why do I need user namespaces?

**A:** User namespaces provide security isolation. They map container UIDs to unprivileged host UIDs, so even if a container escapes, the attacker only has unprivileged access.

---

### Q: What are SETUID and SETGID capabilities for?

**A:** These minimal capabilities allow Kimia to create user namespaces. They're far safer than:
- Privileged mode
- CAP_SYS_ADMIN
- Running as root

---

### Q: Can I use Kimia with distroless images?

**A:** Yes! Kimia supports building any OCI-compliant image, including distroless.

---

### Q: When should I use Docker format vs OCI format?

**A:** Use Docker format (`BUILDAH_FORMAT=docker`) when your Dockerfile contains:
- `HEALTHCHECK` instruction
- `SHELL` instruction  
- `STOPSIGNAL`

Otherwise, use the default OCI format.

---

## Operational Questions

### Q: How much resource overhead does Kimia add?

**A:** Minimal - typically:
- 2-5% CPU overhead
- 256MB-2GB RAM depending on build complexity
- Similar performance to Kaniko and other builders

---

### Q: Can I run multiple Kimia builds simultaneously?

**A:** Yes! Kimia is designed for concurrent builds. Each build is isolated in its own user namespace.

---

### Q: Does Kimia work with private registries?

**A:** Yes, Kimia supports authentication with any OCI-compliant registry.

---

### Q: Will my existing Kaniko configurations work?

**A:** Most Kaniko arguments are directly compatible. You need to:
1. Add proper securityContext (runAsUser: 1000, fsGroup: 1000)
2. Add capabilities (SETUID, SETGID)
3. Change volume mount path from `/kaniko/.docker` to `/home/kimia/.docker`

See the [Migration Guide](comparison.md#migration-from-kaniko-to-kimia).

---

### Q: Why is my cache directory giving permission errors?

**A:** The cache directory must be writable by UID 1000. Solutions:

```yaml
# Solution 1: Use fsGroup (recommended)
securityContext:
  fsGroup: 1000

# Solution 2: Use emptyDir
volumes:
- name: cache
  emptyDir: {}

# Solution 3: Init container
initContainers:
- name: fix-perms
  image: busybox
  command: ['chown', '-R', '1000:1000', '/cache']
  securityContext:
    runAsUser: 0
```

---

## Security Questions

### Q: Is Kimia secure for production?

**A:** Yes! Kimia provides defense-in-depth security:
- Rootless operation (UID 1000)
- User namespace isolation
- Minimal capabilities (SETUID, SETGID only)
- Pod Security Standards (Restricted) compliant
- No privileged mode required

---

### Q: How does Kimia compare to building with Docker?

**A:** Kimia is more secure than Docker:
- No Docker daemon required (reduced attack surface)
- Runs as non-root
- User namespace isolation
- Kubernetes-native

---

### Q: Can Kimia be used in regulated environments?

**A:** Yes! Kimia supports:
- Reproducible builds for supply chain security
- Pod Security Standards compliance
- Audit logging integration
- Image signing (via Cosign)
- SBOM generation

---

## Troubleshooting Questions

### Q: Error: "failed to create user namespace"

**A:** User namespaces are not enabled. Enable them:

```bash
sudo sysctl -w user.max_user_namespaces=15000
echo "user.max_user_namespaces=15000" | sudo tee -a /etc/sysctl.conf
```

---

### Q: Error: "permission denied"

**A:** Check your security context:

```yaml
securityContext:
  runAsUser: 1000
  fsGroup: 1000  # Important!
  allowPrivilegeEscalation: true
  capabilities:
    add: [SETUID, SETGID]
```

---

### Q: Build is slow, how do I speed it up?

**A:** Enable caching and optimize resources:

```yaml
args:
  - --cache
  - --cache-dir=/cache
  - --storage-driver=overlay  # Faster

resources:
  requests:
    memory: "4Gi"
    cpu: "2"
```

See [Performance Guide](performance.md).

---

## Migration Questions

### Q: How long does migration from Kaniko take?

**A:** Typically 1-2 days:
- Day 1: Test in development
- Day 2: Roll out to staging/production

See [Migration Guide](comparison.md#migration-from-kaniko-to-kimia).

---

### Q: Can I run Kimia and Kaniko side-by-side?

**A:** Yes! Many organizations use both:
- Kimia for production (security-critical)
- Kaniko for development (faster setup)

---

## Still Have Questions?

- 📖 [Documentation](../README.md)
- 🔧 [Troubleshooting](troubleshooting.md)
- 💬 [GitHub Discussions](https://github.com/rapidfort/kimia/discussions)
- 🐛 [Report an Issue](https://github.com/rapidfort/kimia/issues)

---

[Back to Main README](../README.md)
