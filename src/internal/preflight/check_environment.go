package preflight

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rapidfort/smithy/pkg/logger"
)

// CheckEnvironment performs comprehensive environment check
// Now accepts storage driver parameter to provide storage-specific guidance
func CheckEnvironment() int {
	// Get storage driver from environment or default to vfs
	storageDriver := os.Getenv("STORAGE_DRIVER")
	if storageDriver == "" {
		storageDriver = "vfs"
	}

	return CheckEnvironmentWithDriver(storageDriver)
}

// CheckEnvironmentWithDriver performs comprehensive environment check with storage driver context
func CheckEnvironmentWithDriver(storageDriver string) int {
	logger.Info("")
	logger.Info("Smithy Environment Check")
	logger.Info("═══════════════════════════════════════════════════════════")
	logger.Info("")

	allGood := true

	// Runtime Context
	logger.Info("RUNTIME CONTEXT")
	uid := os.Getuid()
	isRoot := uid == 0
	isK8s := IsInKubernetes()

	checkmark := getCheckmark(true)
	logger.Info("  User ID:                 %d %s", uid, checkmark)
	logger.Info("  Environment:             %s", getEnvironment(isK8s))
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

		// MKNOD only needed for overlay storage
		if storageDriver == "overlay" {
			hasMknod := caps.HasCapability("CAP_MKNOD")
			logger.Info("  CAP_MKNOD:               %s %s (required for overlay)",
				getPresence(hasMknod), getCheckmark(hasMknod))
			if !hasMknod && !isRoot {
				logger.Warning("  Warning: MKNOD capability missing (required for overlay storage)")
				if !isK8s {
					allGood = false
				}
			}
		} else if storageDriver == "vfs" {
			logger.Info("  CAP_MKNOD:               Not required (vfs storage)")
		}

		logger.Info("  Effective Caps:          %s", caps.FormatCapabilities())
	}

	logger.Info("")

	// SETUID Binaries
	logger.Info("SETUID BINARIES")
	setuidBins, err := CheckSetuidBinaries()
	if err != nil {
		logger.Error("  Error: %v", err)
		// Don't fail yet - capabilities might work
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

	// User Namespaces (only for non-root)
	var userns *UserNamespaceCheck
	if !isRoot {
		logger.Info("USER NAMESPACES")
		var err error
		userns, err = CheckUserNamespaces()
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
	}

	// Storage Drivers
	logger.Info("STORAGE DRIVERS")

	hasRequiredCaps := false
	if caps != nil {
		hasRequiredCaps = caps.HasRequiredCapabilities()
		// For overlay, also check MKNOD
		if storageDriver == "overlay" {
			hasRequiredCaps = hasRequiredCaps && caps.HasCapability("CAP_MKNOD")
		}
	}

	storage, err := CheckStorageDrivers(isRoot, hasRequiredCaps)
	if err != nil {
		logger.Error("  Error: %v", err)
		allGood = false
	} else {
		logger.Info("  VFS:                     Available %s", getCheckmark(true))

		if isRoot {
			logger.Info("  Overlay (native):        Available %s", getCheckmark(true))
		} else {
			logger.Info("  Overlay (native):        Not available (rootless mode)")
			logger.Info("    Note: Overlay requires root privileges")
		}
	}
	logger.Info("")

	// BUILD MODE
	fmt.Println("BUILD MODE")

	var buildModeAvailable bool
	var buildModeMethod string

	if isRoot {
		// Check if in Kubernetes
		if isK8s {
			buildModeAvailable = false
			buildModeMethod = "Root (NOT supported in Kubernetes)"
			allGood = false
			logger.Error("  ERROR: Rootful mode is NOT supported in Kubernetes")
		} else {
			buildModeAvailable = true
			buildModeMethod = "Root"
		}
	} else {
		// Rootless mode checks
		hasRequiredCaps := caps != nil && caps.HasRequiredCapabilities()

		// For overlay, also need MKNOD
		if storageDriver == "overlay" && hasRequiredCaps {
			hasRequiredCaps = caps.HasCapability("CAP_MKNOD")
		}

		setuidBins, _ := CheckSetuidBinaries()
		hasSetuidBins := setuidBins != nil && setuidBins.HasSetuidBinaries()
		setuidCanWork := CanSetuidBinariesWork()

		usernsOK := userns != nil && userns.IsUserNamespaceReady()

		if hasRequiredCaps && usernsOK {
			buildModeAvailable = true
			buildModeMethod = "Rootless (via capabilities)"
		} else if hasSetuidBins && setuidCanWork && usernsOK {
			buildModeAvailable = true
			buildModeMethod = "Rootless (via SETUID binaries)"
			fmt.Printf("    newuidmap:             %s\n", setuidBins.NewuidmapPath)
			fmt.Printf("    newgidmap:             %s\n", setuidBins.NewgidmapPath)
		} else {
			buildModeAvailable = false
			buildModeMethod = "None"
			allGood = false
		}
	}

	if buildModeAvailable {
		fmt.Printf("  Available Modes:         %s ✓\n", buildModeMethod)
	} else {
		fmt.Printf("  Available Modes:         %s ✗\n", buildModeMethod)
	}

	logger.Info("")

	// Dependencies
	logger.Info("DEPENDENCIES")
	checkDependency("buildah", "/usr/local/bin/buildah")
	checkDependencyVersion("buildah", "buildah", "--version")
	checkDependency("git", "/usr/bin/git")
	checkDependencyVersion("git", "git", "--version")
	logger.Info("")

	// Authentication
	logger.Info("AUTHENTICATION")
	dockerConfig := os.Getenv("DOCKER_CONFIG")
	if dockerConfig == "" {
		dockerConfig = filepath.Join(os.Getenv("HOME"), ".docker")
	}

	configFile := filepath.Join(dockerConfig, "config.json")
	if _, err := os.Stat(configFile); err == nil {
		logger.Info("  Docker Config:           %s", configFile)
		logger.Info("  Auth File Readable:      Yes %s", getCheckmark(true))
	} else {
		logger.Info("  Docker Config:           %s", configFile)
		logger.Info("  Auth File Readable:      No (file not found)")
		logger.Info("    Note: Authentication may be configured via environment variables")
	}
	logger.Info("")

	// Verdict
	logger.Info("VERDICT")
	logger.Info("═══════════════════════════════════════════════════════════")
	logger.Info("")

	if allGood {
		logger.Info("✓ Environment is properly configured for building images")

		if isRoot && !isK8s {
			logger.Info("✓ Root mode available (with security warnings)")
			logger.Info("")
			logger.Warning("⚠  SECURITY WARNING: Running as root")
			logger.Warning("   Consider rootless mode for production")
		} else {
			logger.Info("✓ %s", buildModeMethod)
			if storage != nil && storage.OverlayAvailable && isRoot {
				logger.Info("✓ Overlay storage available for better performance")
			} else {
				logger.Info("✓ VFS storage available (recommended for rootless)")
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

		if isRoot && isK8s {
			logger.Error("ERROR: Rootful mode is NOT supported in Kubernetes")
			logger.Error("")
			logger.Error("Kubernetes requires rootless mode with capabilities:")
			logger.Error("  securityContext:")
			logger.Error("    runAsUser: 1000")
			logger.Error("    runAsNonRoot: true")
			logger.Error("    allowPrivilegeEscalation: true")
			logger.Error("    capabilities:")
			logger.Error("      drop: [ALL]")

			if storageDriver == "overlay" {
				logger.Error("      add: [SETUID, SETGID, MKNOD]  # MKNOD required for overlay")
			} else {
				logger.Error("      add: [SETUID, SETGID]  # MKNOD not needed for vfs")
			}
			logger.Error("")
		} else if !isRoot {
			hasRequiredCaps := caps != nil && caps.HasRequiredCapabilities()
			setuidBins, _ := CheckSetuidBinaries()
			hasSetuidBins := setuidBins != nil && setuidBins.HasSetuidBinaries()
			setuidCanWork := CanSetuidBinariesWork()

			// FIXED: Show help if we can't build - check all failure scenarios
			needsHelp := false

			// Scenario 1: Missing both capabilities and setuid binaries
			if !hasRequiredCaps && !hasSetuidBins {
				needsHelp = true
			}

			// Scenario 2: Have capabilities granted but they're not working (missing from /proc/self/status)
			if caps != nil && (!caps.HasSetUID || !caps.HasSetGID) {
				needsHelp = true
			}

			// Scenario 3: Have setuid binaries but they can't escalate privileges
			if hasSetuidBins && !setuidCanWork {
				needsHelp = true
			}

			if needsHelp {
				if isK8s {
					logger.Info("Kubernetes Configuration Required:")
					logger.Info("  securityContext:")
					logger.Info("    runAsUser: 1000")
					logger.Info("    runAsNonRoot: true")
					logger.Info("    allowPrivilegeEscalation: true  # CRITICAL: Must be true!")
					logger.Info("    capabilities:")
					logger.Info("      drop: [ALL]")

					if storageDriver == "overlay" {
						logger.Info("      add: [SETUID, SETGID, MKNOD]  # MKNOD required for overlay")
					} else {
						logger.Info("      add: [SETUID, SETGID]  # MKNOD not needed for vfs")
					}
					logger.Info("")

					// Provide specific guidance based on what's wrong
					if caps != nil && (!caps.HasSetUID || !caps.HasSetGID) {
						logger.Error("DIAGNOSIS: Required capabilities (SETUID, SETGID) are missing")
						logger.Error("  - Capabilities may be granted in YAML but not effective")
						logger.Error("  - Check: kubectl describe pod <pod-name>")
						logger.Error("  - Verify capabilities in container securityContext")
					}

					if hasSetuidBins && !setuidCanWork {
						logger.Error("DIAGNOSIS: SETUID binaries found but cannot escalate privileges")
						logger.Error("  - This indicates allowPrivilegeEscalation is set to false")
						logger.Error("  - NoNewPrivs flag is set, blocking privilege escalation")
						logger.Error("  - FIX: Set allowPrivilegeEscalation: true in securityContext")
					}

					if !hasSetuidBins && !hasRequiredCaps {
						logger.Error("DIAGNOSIS: Neither capabilities nor SETUID binaries available")
						logger.Error("  - Add capabilities: SETUID, SETGID to container")
						logger.Error("  - Set allowPrivilegeEscalation: true")
					}
				} else {
					logger.Info("Enable Rootless Mode (Recommended):")
					logger.Info("  For Docker:")
					logger.Info("    docker run --user 1000:1000 \\")
					logger.Info("      --cap-drop ALL \\")
					logger.Info("      --cap-add SETUID \\")
					logger.Info("      --cap-add SETGID \\")

					if storageDriver == "overlay" {
						logger.Info("      --cap-add MKNOD \\  # Required for overlay storage")
					}

					logger.Info("      --security-opt seccomp=unconfined \\")
					logger.Info("      --security-opt apparmor=unconfined")
					logger.Info("")

					if hasSetuidBins && !setuidCanWork {
						logger.Error("Note: SETUID binaries found but blocked by seccomp/apparmor")
						logger.Error("      Add: --security-opt seccomp=unconfined")
					}
				}
			}
		}

		if !isK8s && !isRoot {
			logger.Info("Alternative: Use Root Mode (Less Secure, Docker only):")
			logger.Info("  docker run --user 0:0 --privileged")
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

func getEnvironment(isK8s bool) string {
	if isK8s {
		return "Kubernetes"
	}
	return "Docker/Standalone"
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
