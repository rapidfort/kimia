package preflight

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rapidfort/kimia/internal/build"
	"github.com/rapidfort/kimia/pkg/logger"
)

// Environment represents the runtime environment
type Environment int

const (
	EnvStandalone Environment = iota
	EnvDocker
	EnvKubernetes
)

// DetectEnvironment determines if running in Kubernetes, Docker, or standalone
func DetectEnvironment() Environment {
	// Check Kubernetes first (most specific)
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return EnvKubernetes
	}

	// Check Docker via .dockerenv file
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return EnvDocker
	}

	// Check Docker via cgroups
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		content := string(data)
		if strings.Contains(content, "docker") ||
			strings.Contains(content, "containerd") {
			return EnvDocker
		}
	}

	return EnvStandalone
}

// CheckEnvironment performs comprehensive environment check
func CheckEnvironment() int {
	// Get storage driver from environment or default to vfs
	var storageDriver string
	if build.DetectBuilder() == "buildah" {
		storageDriver = os.Getenv("STORAGE_DRIVER")
		if storageDriver == "" {
			storageDriver = "vfs"
		}
	} else {
		storageDriver = os.Getenv("STORAGE_DRIVER")
		if storageDriver == "" {
			storageDriver = "native"
		}
	}

	return CheckEnvironmentWithDriver(storageDriver)
}

// CheckEnvironmentWithDriver performs comprehensive environment check with storage driver context
func CheckEnvironmentWithDriver(storageDriver string) int {
	builder := build.DetectBuilder()
	logger.Info("")
	logger.Info("Kimia Environment Check (%s)", builder)
	logger.Info("═══════════════════════════════════════════════════════")
	logger.Info("")

	allGood := true

	// Runtime Context
	logger.Info("RUNTIME CONTEXT")
	uid := os.Getuid()

	// CRITICAL: Kimia is rootless-only and does NOT support root mode
	if uid == 0 {
		logger.Error("  User ID:                 %d ✗", uid)
		logger.Error("")
		logger.Error("╔══════════════════════════════════════════════════════╗")
		logger.Error("║              KIMIA DOES NOT SUPPORT ROOT             ║")
		logger.Error("╠══════════════════════════════════════════════════════╣")
		logger.Error("║ Kimia is designed to run as a non-root user with     ║")
		logger.Error("║ capabilities, not as root.                           ║")
		logger.Error("║                                                      ║")
		logger.Error("║ Please run as non-root user (UID > 0)                ║")
		logger.Error("║                                                      ║")
		logger.Error("║ For Kubernetes, set:                                 ║")
		logger.Error("║   securityContext:                                   ║")
		logger.Error("║     runAsUser: 1000                                  ║")
		logger.Error("║     runAsNonRoot: true                               ║")
		logger.Error("║     allowPrivilegeEscalation: true                   ║")
		logger.Error("║     capabilities:                                    ║")
		logger.Error("║       drop: [ALL]                                    ║")
		logger.Error("║       add: [SETUID, SETGID]                          ║")
		logger.Error("╚══════════════════════════════════════════════════════╝")
		logger.Error("")
		return 1
	}

	env := DetectEnvironment()

	checkmark := getCheckmark(true)
	logger.Info("  User ID:                 %d %s", uid, checkmark)
	logger.Info("  Environment:             %s", getEnvironment(env))
	logger.Info("  Storage Driver:          %s", storageDriver)

	if username := os.Getenv("USER"); username != "" {
		logger.Info("  User Name:               %s", username)
	}

	if home := os.Getenv("HOME"); home != "" {
		logger.Info("  Home Directory:          %s", home)
	}

	if wd, err := os.Getwd(); err == nil {
		logger.Info("  Working Directory:       %s", wd)
	}

	logger.Info("")

	// Capabilities
	logger.Info("CAPABILITIES")
	caps, err := CheckCapabilities()
	if err != nil {
		logger.Error("  Error: %v", err)
		allGood = false
	} else {
		logger.Info("  CAP_SETUID:              %s %s", getPresence(caps.HasSetUID), getCheckmark(caps.HasSetUID))
		logger.Info("  CAP_SETGID:              %s %s", getPresence(caps.HasSetGID), getCheckmark(caps.HasSetGID))

		// Additional capabilities for overlay storage
		switch storageDriver {
		case "overlay":
			hasMknod := caps.HasCapability("CAP_MKNOD")
			hasDACOverride := caps.HasCapability("CAP_DAC_OVERRIDE")

			logger.Info("  CAP_MKNOD:               %s %s (required for overlay)",
				getPresence(hasMknod), getCheckmark(hasMknod))
			logger.Info("  CAP_DAC_OVERRIDE:        %s %s (required for overlay)",
				getPresence(hasDACOverride), getCheckmark(hasDACOverride))

			if !hasMknod {
				logger.Warning("  Warning: MKNOD capability missing (required for overlay storage)")
				// Note: Don't set allGood=false yet - SETUID binaries may still work
			}
			if !hasDACOverride {
				logger.Warning("  Warning: DAC_OVERRIDE capability missing (required for overlay storage)")
				// Note: Don't set allGood=false yet - SETUID binaries may still work
			}
		case "vfs", "native":
			logger.Info("  CAP_MKNOD:               Not required (%s storage)", storageDriver)
			logger.Info("  CAP_DAC_OVERRIDE:        Not required (%s storage)", storageDriver)
		}

		logger.Info("  Effective Caps:          %s", caps.FormatCapabilities())
	}

	logger.Info("")

	// SETUID Binaries
	logger.Info("SETUID BINARIES")
	setuidBins, err := CheckSetuidBinaries()
	if err != nil {
		logger.Error("  Error: %v", err)
	} else {
		if setuidBins.NewuidmapPresent {
			if setuidBins.NewuidmapSetuid {
				logger.Info("  newuidmap:               %s (SETUID enabled) %s",
					setuidBins.NewuidmapPath, getCheckmark(true))
			} else {
				logger.Info("  newuidmap:               %s (SETUID disabled) %s",
					setuidBins.NewuidmapPath, getCheckmark(false))
			}
		} else {
			logger.Info("  newuidmap:               Not found %s", getCheckmark(false))
		}

		if setuidBins.NewgidmapPresent {
			if setuidBins.NewgidmapSetuid {
				logger.Info("  newgidmap:               %s (SETUID enabled) %s",
					setuidBins.NewgidmapPath, getCheckmark(true))
			} else {
				logger.Info("  newgidmap:               %s (SETUID disabled) %s",
					setuidBins.NewgidmapPath, getCheckmark(false))
			}
		} else {
			logger.Info("  newgidmap:               Not found %s", getCheckmark(false))
		}

		setuidCanWork := CanSetuidBinariesWork()
		logger.Info("  Privilege Escalation:    %s %s", getEnabled(setuidCanWork), getCheckmark(setuidCanWork))
	}

	logger.Info("")

	// User Namespaces (Kimia is rootless-only, always check)
	logger.Info("USER NAMESPACES")
	userns, err := CheckUserNamespaces()
	if err != nil {
		logger.Error("  Error: %v", err)
		allGood = false
	} else {
		logger.Info("  Kernel Support:          %s %s", getEnabled(userns.Supported), getCheckmark(userns.Supported))
		if userns.Supported {
			logger.Info("  Max User Namespaces:     %d", userns.MaxUserNS)
		}

		if userns.SubuidConfigured {
			logger.Info("  Subuid Mapping:          %s %s", userns.SubuidRange, getCheckmark(true))
		} else {
			logger.Info("  Subuid Mapping:          Not configured %s", getCheckmark(false))
		}

		if userns.SubgidConfigured {
			logger.Info("  Subgid Mapping:          %s %s", userns.SubgidRange, getCheckmark(true))
		} else {
			logger.Info("  Subgid Mapping:          Not configured %s", getCheckmark(false))
		}

		logger.Info("  Namespace Creation:      %s %s", getSuccess(userns.CanCreate), getCheckmark(userns.CanCreate))
	}
	logger.Info("")

	// Storage Drivers
	logger.Info("STORAGE DRIVERS")

	hasRequiredCaps := false
	if caps != nil {
		hasRequiredCaps = caps.HasRequiredCapabilities()
		// For overlay, also check MKNOD and DAC_OVERRIDE
		if storageDriver == "overlay" {
			hasRequiredCaps = hasRequiredCaps &&
				caps.HasCapability("CAP_MKNOD") &&
				caps.HasCapability("CAP_DAC_OVERRIDE")
		}
	}

	storage, err := CheckStorageDrivers(hasRequiredCaps)
	if err != nil {
		logger.Error("  Error: %v", err)
		allGood = false
	} else {
		if builder != "buildah" {
			logger.Info("  Native:                  %s", getCheckmark(storage.NativeAvailable))
		} else {
			logger.Info("  VFS:                     %s", getCheckmark(storage.VFSAvailable))
		}

		if storage.OverlayAvailable {
			logger.Info("  Overlay:                 Available %s", getCheckmark(true))
		} else {
			logger.Info("  Overlay:                 Not available (requires MKNOD + DAC_OVERRIDE capabilities)")
		}
	}
	logger.Info("")

	// BUILD MODE (Kimia is rootless-only)
	fmt.Println("BUILD MODE")

	var buildModeAvailable bool
	var buildModeMethod string

	// Kimia only supports rootless mode
	hasRequiredCaps = caps != nil && caps.HasRequiredCapabilities()

	// Note: For overlay storage with capabilities mode, we also need MKNOD and DAC_OVERRIDE
	// However, if capabilities are missing, SETUID binaries may still work with native/vfs storage
	hasOverlayCaps := false
	if storageDriver == "overlay" && hasRequiredCaps {
		hasOverlayCaps = caps.HasCapability("CAP_MKNOD") && caps.HasCapability("CAP_DAC_OVERRIDE")
	}

	setuidBins, _ = CheckSetuidBinaries()
	hasSetuidBins := setuidBins != nil && setuidBins.HasSetuidBinaries()
	setuidCanWork := CanSetuidBinariesWork()

	usernsOK := userns != nil && userns.IsUserNamespaceReady()

	// Determine if building is possible:
	// 1. With capabilities (including overlay caps if overlay storage is requested)
	// 2. With SETUID binaries (can work even without overlay caps, using native/vfs)
	if hasRequiredCaps && usernsOK {
		if storageDriver == "overlay" && !hasOverlayCaps {
			// Has basic caps but not overlay caps - will fall back to native/vfs
			buildModeAvailable = true
			buildModeMethod = "Rootless (via capabilities, native storage)"
		} else {
			buildModeAvailable = true
			buildModeMethod = "Rootless (via capabilities)"
		}
	} else if hasSetuidBins && setuidCanWork && usernsOK {
		buildModeAvailable = true
		if storageDriver == "overlay" {
			buildModeMethod = "Rootless (via SETUID binaries, native storage)"
		} else {
			buildModeMethod = "Rootless (via SETUID binaries)"
		}
		fmt.Printf("    newuidmap:             %s\n", setuidBins.NewuidmapPath)
		fmt.Printf("    newgidmap:             %s\n", setuidBins.NewgidmapPath)
	} else {
		buildModeAvailable = false
		buildModeMethod = "None"
		allGood = false
	}

	if buildModeAvailable {
		fmt.Printf("  Available Modes:         %s ✓\n", buildModeMethod)
	} else {
		fmt.Printf("  Available Modes:         %s ✗\n", buildModeMethod)
	}

	logger.Info("")

	// Dependencies
	logger.Info("DEPENDENCIES")
	if builder == "buildah" {
		checkDependency("buildah", "/usr/local/bin/buildah")
		checkDependencyVersion("buildah", "buildah", "--version")
	} else {
		checkDependency("buildctl", "/usr/local/bin/buildctl")
		checkDependencyVersion("buildctl", "buildctl", "--version")
	}
	checkDependency("git", "/usr/bin/git")
	checkDependencyVersion("git", "git", "--version")
	logger.Info("")

	// Authentication
	logger.Info("AUTHENTICATION")

	// Get DOCKER_CONFIG, sanitize and warn if suspicious
	dockerConfig := os.Getenv("DOCKER_CONFIG")
	if dockerConfig == "" {
		homeDir := os.Getenv("HOME")
		if homeDir != "" {
			dockerConfig = filepath.Join(homeDir, ".docker")
		}
	}

	// Sanitize the path - resolve any .. or . components
	if dockerConfig != "" {
		cleanDockerConfig := filepath.Clean(dockerConfig)

		// Warn if path looks suspicious (contains .. traversal attempts)
		if strings.Contains(dockerConfig, "..") {
			logger.Warning("  Docker Config path contains '..' - this may be suspicious: %s", dockerConfig)
		}

		// Warn if path contains null bytes
		if strings.Contains(dockerConfig, "\x00") {
			logger.Warning("  Docker Config path contains null bytes - this is suspicious")
			dockerConfig = "" // Reject paths with null bytes
		} else {
			dockerConfig = cleanDockerConfig
		}
	}

	if dockerConfig == "" {
		logger.Info("  Docker Config:           Not configured")
		logger.Info("    Note: Set DOCKER_CONFIG or HOME environment variable")
	} else {
		// Safely join the config file path
		configFile := filepath.Join(dockerConfig, "config.json")
		configFile = filepath.Clean(configFile)

		// #nosec G703 -- configFile is constructed from sanitized dockerConfig (cleaned with filepath.Clean and validated for null bytes)
		if _, err := os.Stat(configFile); err == nil {
			logger.Info("  Docker Config:           %s", configFile)
			logger.Info("  Auth File Readable:      Yes %s", getCheckmark(true))
		} else {
			logger.Info("  Docker Config:           %s", configFile)
			logger.Info("  Auth File Readable:      No (file not found)")
			logger.Info("    Note: Authentication may be configured via environment variables")
		}
	}
	logger.Info("")

	// Verdict
	logger.Info("VERDICT")
	logger.Info("═══════════════════════════════════════════════════════")
	logger.Info("")

	if allGood {
		logger.Info("✓ Environment is properly configured for building images")
		logger.Info("✓ %s", buildModeMethod)

		if storage != nil && storage.OverlayAvailable {
			logger.Info("✓ Overlay storage available for better performance")
		} else {
			if builder != "buildah" {
				logger.Info("✓ Native storage available")
			} else {
				logger.Info("✓ VFS storage available")
			}
		}

		logger.Info("")
		logger.Info("Ready to build!")
		return 0
	} else {
		logger.Error("✗ Environment is NOT configured for building")
		logger.Error("Cannot proceed with build until environment is fixed.")
		logger.Info("")
		logger.Info("REQUIRED ACTIONS:")
		logger.Info("")

		hasRequiredCaps = caps != nil && caps.HasRequiredCapabilities()
		setuidBins, _ = CheckSetuidBinaries()
		hasSetuidBins = setuidBins != nil && setuidBins.HasSetuidBinaries()
		setuidCanWork = CanSetuidBinariesWork()

		// Check all failure scenarios
		needsHelp := false

		// Scenario 1: Missing both capabilities and setuid binaries
		if !hasRequiredCaps && !hasSetuidBins {
			needsHelp = true
		}

		// Scenario 2: Have capabilities granted but they're not working
		if caps != nil && (!caps.HasSetUID || !caps.HasSetGID) {
			needsHelp = true
		}

		// Scenario 3: Have setuid binaries but they can't escalate privileges
		if hasSetuidBins && !setuidCanWork {
			needsHelp = true
		}

		// Scenario 4: Overlay storage missing required capabilities
		if storageDriver == "overlay" && caps != nil {
			if !caps.HasCapability("CAP_MKNOD") || !caps.HasCapability("CAP_DAC_OVERRIDE") {
				needsHelp = true
			}
		}

		if needsHelp {
			// Provide environment-specific guidance
			switch env {
			case EnvKubernetes:
				logger.Info("Kubernetes Configuration Required:")
				logger.Info("")
				logger.Info("Some Kubernetes clusters require unconfined seccomp and AppArmor profiles")
				logger.Info("for user namespace operations to work properly.")
				logger.Info("")
				logger.Info("Complete Pod/Job specification:")
				logger.Info("")
				logger.Info("---")
				logger.Info("apiVersion: batch/v1")
				logger.Info("kind: Job")
				logger.Info("metadata:")
				logger.Info("  name: kimia-build")
				logger.Info("spec:")
				logger.Info("  template:")
				logger.Info("    spec:")
				logger.Info("      restartPolicy: Never")
				logger.Info("      securityContext:")
				logger.Info("        runAsUser: 1000")
				logger.Info("        runAsGroup: 1000")
				logger.Info("        fsGroup: 1000")
				logger.Info("        seccompProfile:")
				logger.Info("          type: Unconfined  # May be required for user namespaces")
				logger.Info("      containers:")
				logger.Info("      - name: kimia")
				logger.Info("        image: ghcr.io/rapidfort/kimia:latest")
				logger.Info("        securityContext:")
				logger.Info("          runAsUser: 1000")
				logger.Info("          runAsGroup: 1000")
				logger.Info("          allowPrivilegeEscalation: true  # CRITICAL: Required!")
				logger.Info("          appArmorProfile:")
				logger.Info("            type: Unconfined  # May be required for user namespaces")
				logger.Info("          seccompProfile:")
				logger.Info("            type: Unconfined  # May be required for user namespaces")
				logger.Info("          capabilities:")
				logger.Info("            drop: [ALL]")

				if storageDriver == "overlay" {
					logger.Info("            add: [SETUID, SETGID, MKNOD, DAC_OVERRIDE]")
				} else {
					logger.Info("            add: [SETUID, SETGID]")
				}

			case EnvDocker:
				logger.Info("Docker Configuration Required:")
				logger.Info("")
				logger.Info("Run Kimia with the following Docker options:")
				logger.Info("")
				if storageDriver == "overlay" {
					logger.Info("  docker run --cap-add SETUID --cap-add SETGID --cap-add MKNOD \\")
					logger.Info("             --user 1000:1000 \\")
					logger.Info("             ghcr.io/rapidfort/kimia:latest")
				} else {
					logger.Info("  docker run --cap-add SETUID --cap-add SETGID \\")
					logger.Info("             --user 1000:1000 \\")
					logger.Info("             ghcr.io/rapidfort/kimia:latest")
				}
				logger.Info("")
				logger.Info("If capabilities don't work, try with SETUID binaries:")
				logger.Info("  docker run --security-opt seccomp=unconfined \\")
				logger.Info("             --user 1000:1000 \\")
				logger.Info("             ghcr.io/rapidfort/kimia:latest")

			case EnvStandalone:
				logger.Info("Standalone/VM Configuration Required:")
				logger.Info("")
				logger.Info("Ensure the following are available:")
				logger.Info("  1. User namespaces enabled in kernel")
				logger.Info("  2. Subuid/subgid mappings configured in /etc/subuid and /etc/subgid")
				logger.Info("  3. Either:")
				logger.Info("     - Run with capabilities (CAP_SETUID, CAP_SETGID)")
				logger.Info("     - Have newuidmap/newgidmap SETUID binaries available")
			}

			logger.Info("")
			logger.Info("NOTE: seccompProfile and appArmorProfile set to Unconfined may be required")
			logger.Info("      depending on your environment's security policies.")
			logger.Info("")

			// Provide specific guidance based on what's wrong
			if caps != nil && (!caps.HasSetUID || !caps.HasSetGID) {
				logger.Error("DIAGNOSIS: Required capabilities (SETUID, SETGID) are missing")
				logger.Error("  - Capabilities may be granted in YAML but not effective")
				logger.Error("  - Check: kubectl describe pod <pod-name>")
				logger.Error("  - Verify capabilities in container securityContext")
				logger.Error("")
			}

			if storageDriver == "overlay" && caps != nil {
				if !caps.HasCapability("CAP_MKNOD") {
					logger.Error("DIAGNOSIS: CAP_MKNOD capability missing (required for overlay)")
					logger.Error("  - Add CAP_MKNOD to capabilities.add list")
					logger.Error("")
				}
				if !caps.HasCapability("CAP_DAC_OVERRIDE") {
					logger.Error("DIAGNOSIS: CAP_DAC_OVERRIDE capability missing (required for overlay)")
					logger.Error("  - Add CAP_DAC_OVERRIDE to capabilities.add list")
					logger.Error("")
				}
			}

			if hasSetuidBins && !setuidCanWork {
				logger.Error("DIAGNOSIS: SETUID binaries found but cannot escalate privileges")
				logger.Error("  - This indicates allowPrivilegeEscalation is set to false")
				logger.Error("  - OR seccomp/AppArmor profiles are blocking privilege escalation")
				logger.Error("")
				logger.Error("FIXES:")
				logger.Error("  1. Set allowPrivilegeEscalation: true in container securityContext")
				logger.Error("  2. Add seccompProfile: type: Unconfined (pod and/or container level)")
				logger.Error("  3. Add appArmorProfile: type: Unconfined (container level)")
				logger.Error("")
			}

			if !hasSetuidBins && !hasRequiredCaps {
				logger.Error("DIAGNOSIS: Neither capabilities nor SETUID binaries available")
				logger.Error("  - Add capabilities: SETUID, SETGID to container")
				if storageDriver == "overlay" {
					logger.Error("  - Add capabilities: MKNOD, DAC_OVERRIDE for overlay storage")
				}
				logger.Error("  - Set allowPrivilegeEscalation: true")
				logger.Error("  - Consider adding seccompProfile: Unconfined if still failing")
				logger.Error("")
			}

			logger.Info("Troubleshooting Steps:")
			if env == EnvKubernetes {
				logger.Info("  1. Apply the YAML configuration above")
				logger.Info("  2. Check pod status: kubectl get pod <pod-name>")
				logger.Info("  3. Describe pod: kubectl describe pod <pod-name>")
				logger.Info("  4. View logs: kubectl logs <pod-name>")
				logger.Info("  5. Run preflight check: kubectl exec <pod-name> -- kimia check-environment")
			} else if env == EnvDocker {
				logger.Info("  1. Run with the recommended Docker options above")
				logger.Info("  2. Check container status: docker ps")
				logger.Info("  3. View logs: docker logs <container-name>")
				logger.Info("  4. Run preflight check: docker exec <container-name> kimia check-environment")
			} else {
				logger.Info("  1. Verify user namespace support: unshare --user --pid --map-root-user echo OK")
				logger.Info("  2. Check subuid/subgid: cat /etc/subuid /etc/subgid")
				logger.Info("  3. Run preflight check: kimia check-environment")
			}
			logger.Info("")
		}

		return 1
	}
}

// Helper functions
func getCheckmark(condition bool) string {
	if condition {
		return "✓"
	}
	return "✗"
}

func getPresence(present bool) string {
	if present {
		return "Present"
	}
	return "Missing"
}

func getEnabled(enabled bool) string {
	if enabled {
		return "Enabled"
	}
	return "Disabled"
}

func getSuccess(success bool) string {
	if success {
		return "Success"
	}
	return "Failed"
}

func getEnvironment(env Environment) string {
	switch env {
	case EnvKubernetes:
		return "Kubernetes"
	case EnvDocker:
		return "Docker"
	case EnvStandalone:
		return "Standalone"
	default:
		return "Unknown"
	}
}

func checkDependency(name, path string) {
	if _, err := os.Stat(path); err == nil {
		logger.Info("  %s:%-*s %s %s", name, 20-len(name), "", path, getCheckmark(true))
	} else if foundPath, err := exec.LookPath(name); err == nil {
		logger.Info("  %s:%-*s %s %s", name, 20-len(name), "", foundPath, getCheckmark(true))
	} else {
		logger.Info("  %s:%-*s Not found %s", name, 20-len(name), "", getCheckmark(false))
	}
}

func checkDependencyVersion(name, command string, versionArg string) {
	// Sanitize command - check for shell metacharacters
	if strings.ContainsAny(command, ";|&$`<>(){}\\'\"\n\r\x00") {
		logger.Warning("  %s version: Command contains suspicious characters, skipping", name)
		return
	}

	// Sanitize version argument - check for shell metacharacters
	if strings.ContainsAny(versionArg, ";|&$`<>(){}\\'\"\n\r\x00") {
		logger.Warning("  %s version: Argument contains suspicious characters, skipping", name)
		return
	}

	// Trim whitespace from inputs
	command = strings.TrimSpace(command)
	versionArg = strings.TrimSpace(versionArg)

	// Ensure neither is empty after trimming
	if command == "" || versionArg == "" {
		logger.Warning("  %s version: Empty command or argument, skipping", name)
		return
	}

	// #nosec G204 -- command and versionArg are validated above to reject shell metacharacters and null bytes
	cmd := exec.Command(command, versionArg)
	output, err := cmd.CombinedOutput()
	if err == nil {
		version := strings.TrimSpace(string(output))
		if lines := strings.Split(version, "\n"); len(lines) > 0 {
			version = strings.TrimSpace(lines[0])
		}
		logger.Info("  %s version:%-*s %s %s", name, 12-len(name), "", version, getCheckmark(true))
	}
}