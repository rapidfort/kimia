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
func CheckEnvironment() int {
	logger.Info("")
	logger.Info("Smithy Environment Check")
	logger.Info("═══════════════════════════════════════════════════════════")
	logger.Info("")

	allGood := true

	// Runtime Context
	logger.Info("RUNTIME CONTEXT")
	uid := os.Getuid()
	isRoot := uid == 0

	checkmark := getCheckmark(true)
	logger.Info("  User ID:                 %d (%s) %s", uid, getUIDDescription(isRoot), checkmark)

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
		logger.Info("  Effective Caps:          %s", caps.FormatCapabilities())

		// DON'T set allGood=false yet - check SETUID binaries first!
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

			// Don't fail yet - check if we can build via other means
		}
		logger.Info("")
	}

	// Storage Drivers
	logger.Info("STORAGE DRIVERS")

	hasRequiredCaps := false
	if caps != nil {
		hasRequiredCaps = caps.HasRequiredCapabilities()
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
			logger.Info("  Overlay (native):        Not available (non-root)")
		}

		if !isRoot {
			if storage.OverlayAvailable {
				logger.Info("  Overlay (fuse):          Testing...")
				logger.Info("    - /dev/fuse:           %s %s", getPresence(storage.FuseAvailable), getCheckmark(storage.FuseAvailable))

				if storage.FuseOverlayFS != "" {
					logger.Info("    - fuse-overlayfs:      %s %s", storage.FuseOverlayFS, getCheckmark(true))
				} else {
					logger.Info("    - fuse-overlayfs:      Not found %s", getCheckmark(false))
				}

				// Perform mount test
				testResult := TestOverlayMount(isRoot)
				if testResult.Success {
					logger.Info("    - Mount test:          Success %s", getCheckmark(true))
					logger.Info("    - Write test:          Success %s", getCheckmark(true))
				} else {
					logger.Info("    - Mount test:          Failed %s", getCheckmark(false))
					logger.Error("      Error: %s", testResult.ErrorMessage)
					// Don't set allGood=false, VFS is still available
				}
			} else {
				logger.Info("  Overlay (fuse):          Not available %s", getCheckmark(false))
				if !storage.FuseAvailable {
					logger.Info("    - /dev/fuse missing")
				}
				if storage.FuseOverlayFS == "" {
					logger.Info("    - fuse-overlayfs not installed")
				}
			}
		}
	}
	logger.Info("")

	// BUILD MODE - Check if we can actually build
	fmt.Println("BUILD MODE")

	// For non-root, check BOTH capabilities AND SETUID binaries
	var buildModeAvailable bool
	var buildModeMethod string

	if isRoot {
		buildModeAvailable = true
		buildModeMethod = "Root"
	} else {
		// Check capabilities
		hasRequiredCaps := caps != nil && caps.HasRequiredCapabilities()

		// Check SETUID binaries
		setuidBins, _ := CheckSetuidBinaries()
		hasSetuidBins := setuidBins != nil && setuidBins.HasSetuidBinaries()
		setuidCanWork := CanSetuidBinariesWork()

		// Check if user namespaces work
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
			allGood = false // NOW we set allGood = false
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

		if isRoot {
			logger.Info("✓ Root mode available (with security warnings)")
			logger.Info("")
			logger.Warning("⚠  SECURITY WARNING: Running as root")
			logger.Warning("   Consider rootless mode for production")
		} else {
			logger.Info("✓ %s", buildModeMethod)
			if storage != nil && storage.OverlayAvailable {
				logger.Info("✓ Overlay storage available for better performance")
			} else {
				logger.Info("✓ VFS storage available (overlay recommended for speed)")
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

		if !isRoot {
			hasRequiredCaps := caps != nil && caps.HasRequiredCapabilities()
			setuidBins, _ := CheckSetuidBinaries()
			hasSetuidBins := setuidBins != nil && setuidBins.HasSetuidBinaries()

			if !hasRequiredCaps && !hasSetuidBins {
				logger.Info("Enable Rootless Mode (Recommended):")
				logger.Info("  Add to Kubernetes SecurityContext:")
				logger.Info("    runAsUser: 1000")
				logger.Info("    runAsNonRoot: true")
				logger.Info("    allowPrivilegeEscalation: true")
				logger.Info("    capabilities:")
				logger.Info("      drop: [ALL]")
				logger.Info("      add: [SETUID, SETGID]")
				logger.Info("")
				logger.Info("  Or for Docker:")
				logger.Info("    docker run --user 1000:1000 \\")
				logger.Info("      --cap-drop ALL \\")
				logger.Info("      --cap-add SETUID \\")
				logger.Info("      --cap-add SETGID \\")
				logger.Info("      --security-opt seccomp=unconfined \\")
				logger.Info("      --security-opt apparmor=unconfined")
				logger.Info("")
			}
		}

		logger.Info("Alternative: Use Root Mode (Less Secure):")
		logger.Info("  Add to Kubernetes SecurityContext:")
		logger.Info("    runAsUser: 0")
		logger.Info("")

		return 1
	}
}

// Helper functions for formatted output
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
		// Take first line only
		if lines := strings.Split(version, "\n"); len(lines) > 0 {
			version = strings.TrimSpace(lines[0])
		}
		logger.Info("  %s version:%-*s %s %s", name, 12-len(name), "", version, getCheckmark(true))
	}
}
