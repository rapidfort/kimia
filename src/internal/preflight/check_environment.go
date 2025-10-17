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
	fmt.Println()
	fmt.Println("Smithy Environment Check")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println()
	
	allGood := true
	
	// Runtime Context
	fmt.Println("RUNTIME CONTEXT")
	uid := os.Getuid()
	isRoot := uid == 0
	
	checkmark := getCheckmark(true)
	fmt.Printf("  User ID:                 %d (%s) %s\n", uid, getUIDDescription(isRoot), checkmark)
	
	if username := os.Getenv("USER"); username != "" {
		fmt.Printf("  User Name:               %s\n", username)
	}
	
	if home := os.Getenv("HOME"); home != "" {
		fmt.Printf("  Home Directory:          %s\n", home)
	}
	
	if wd, err := os.Getwd(); err == nil {
		fmt.Printf("  Working Directory:       %s\n", wd)
	}
	
	fmt.Println()
	
	// Capabilities
	fmt.Println("CAPABILITIES")
	caps, err := CheckCapabilities()
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
		allGood = false
	} else {
		fmt.Printf("  CAP_SETUID:              %s %s\n", getPresence(caps.HasSetUID), getCheckmark(caps.HasSetUID))
		fmt.Printf("  CAP_SETGID:              %s %s\n", getPresence(caps.HasSetGID), getCheckmark(caps.HasSetGID))
		fmt.Printf("  Effective Caps:          %s\n", caps.FormatCapabilities())
		
		if !isRoot && !caps.HasRequiredCapabilities() {
			allGood = false
		}
	}
	fmt.Println()
	
	// User Namespaces (only for non-root)
	if !isRoot {
		fmt.Println("USER NAMESPACES")
		userns, err := CheckUserNamespaces()
		if err != nil {
			fmt.Printf("  Error: %v\n", err)
			allGood = false
		} else {
			fmt.Printf("  Kernel Support:          %s %s\n", getEnabled(userns.Supported), getCheckmark(userns.Supported))
			if userns.Supported {
				fmt.Printf("  Max User Namespaces:     %d\n", userns.MaxUserNS)
			}
			
			if userns.SubuidConfigured {
				fmt.Printf("  Subuid Mapping:          %s %s\n", userns.SubuidRange, getCheckmark(true))
			} else {
				fmt.Printf("  Subuid Mapping:          Not configured %s\n", getCheckmark(false))
			}
			
			if userns.SubgidConfigured {
				fmt.Printf("  Subgid Mapping:          %s %s\n", userns.SubgidRange, getCheckmark(true))
			} else {
				fmt.Printf("  Subgid Mapping:          Not configured %s\n", getCheckmark(false))
			}
			
			fmt.Printf("  Namespace Creation:      %s %s\n", getSuccess(userns.CanCreate), getCheckmark(userns.CanCreate))
			
			if !userns.IsUserNamespaceReady() {
				allGood = false
			}
		}
		fmt.Println()
	}
	
	// Storage Drivers
	fmt.Println("STORAGE DRIVERS")
	
	hasRequiredCaps := false
	if caps != nil {
		hasRequiredCaps = caps.HasRequiredCapabilities()
	}
	
	storage, err := CheckStorageDrivers(isRoot, hasRequiredCaps)
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
		allGood = false
	} else {
		fmt.Printf("  VFS:                     Available %s\n", getCheckmark(true))
		
		if isRoot {
			fmt.Printf("  Overlay (native):        Available %s\n", getCheckmark(true))
		} else {
			fmt.Printf("  Overlay (native):        Not available (non-root)\n")
		}
		
		if !isRoot {
			if storage.OverlayAvailable {
				fmt.Printf("  Overlay (fuse):          Testing...\n")
				fmt.Printf("    - /dev/fuse:           %s %s\n", getPresence(storage.FuseAvailable), getCheckmark(storage.FuseAvailable))
				
				if storage.FuseOverlayFS != "" {
					fmt.Printf("    - fuse-overlayfs:      %s %s\n", storage.FuseOverlayFS, getCheckmark(true))
				} else {
					fmt.Printf("    - fuse-overlayfs:      Not found %s\n", getCheckmark(false))
				}
				
				// Perform mount test
				testResult := TestOverlayMount(isRoot)
				if testResult.Success {
					fmt.Printf("    - Mount test:          Success %s\n", getCheckmark(true))
					fmt.Printf("    - Write test:          Success %s\n", getCheckmark(true))
				} else {
					fmt.Printf("    - Mount test:          Failed %s\n", getCheckmark(false))
					fmt.Printf("      Error: %s\n", testResult.ErrorMessage)
					// Don't set allGood=false, VFS is still available
				}
			} else {
				fmt.Printf("  Overlay (fuse):          Not available %s\n", getCheckmark(false))
				if !storage.FuseAvailable {
					fmt.Printf("    - /dev/fuse missing\n")
				}
				if storage.FuseOverlayFS == "" {
					fmt.Printf("    - fuse-overlayfs not installed\n")
				}
			}
		}
	}
	fmt.Println()
	
	// Build Mode
	fmt.Println("BUILD MODE")
	if isRoot {
		fmt.Println("  Recommended Mode:        Root (with security warnings)")
		fmt.Println("  Storage Driver:          overlay (faster) or vfs")
	} else {
		if caps != nil && caps.HasRequiredCapabilities() && 
		   (storage != nil && storage.OverlayAvailable) {
			fmt.Printf("  Recommended Mode:        Rootless %s\n", getCheckmark(true))
			fmt.Println("  Recommended Driver:      overlay (via fuse-overlayfs)")
			fmt.Println("  Alternative Driver:      vfs (slower but always works)")
		} else if caps != nil && caps.HasRequiredCapabilities() {
			fmt.Printf("  Recommended Mode:        Rootless %s\n", getCheckmark(true))
			fmt.Println("  Recommended Driver:      vfs")
		} else {
			fmt.Printf("  Available Modes:         None %s\n", getCheckmark(false))
			allGood = false
		}
	}
	fmt.Println()
	
	// Dependencies
	fmt.Println("DEPENDENCIES")
	checkDependency("buildah", "/usr/local/bin/buildah")
	checkDependencyVersion("buildah", "buildah", "--version")
	checkDependency("git", "/usr/bin/git")
	checkDependencyVersion("git", "git", "--version")
	fmt.Println()
	
	// Authentication
	fmt.Println("AUTHENTICATION")
	dockerConfig := os.Getenv("DOCKER_CONFIG")
	if dockerConfig == "" {
		dockerConfig = filepath.Join(os.Getenv("HOME"), ".docker")
	}
	
	configFile := filepath.Join(dockerConfig, "config.json")
	if _, err := os.Stat(configFile); err == nil {
		fmt.Printf("  Docker Config:           %s\n", configFile)
		fmt.Printf("  Auth File Readable:      Yes %s\n", getCheckmark(true))
	} else {
		fmt.Printf("  Docker Config:           %s\n", configFile)
		fmt.Printf("  Auth File Readable:      No (file not found)\n")
		fmt.Println("    Note: Authentication may be configured via environment variables")
	}
	fmt.Println()
	
	// Verdict
	fmt.Println("VERDICT")
	fmt.Println("═══════════════════════════════════════════════════════════")
	
	if allGood {
		fmt.Println("✓ Environment is properly configured for building images")
		
		if isRoot {
			fmt.Println("✓ Root mode available (with security warnings)")
			fmt.Println()
			fmt.Println("⚠  SECURITY WARNING: Running as root")
			fmt.Println("   Consider rootless mode for production")
		} else {
			fmt.Println("✓ Rootless mode available (recommended)")
			if storage != nil && storage.OverlayAvailable {
				fmt.Println("✓ Overlay storage available for better performance")
			} else {
				fmt.Println("✓ VFS storage available (overlay recommended for speed)")
			}
		}
		
		fmt.Println()
		fmt.Println("Ready to build!")
		return 0
	} else {
		fmt.Println("✗ Environment is NOT configured for building")
		fmt.Println()
		fmt.Println("REQUIRED ACTIONS:")
		fmt.Println()
		
		if !isRoot {
			if caps == nil || !caps.HasRequiredCapabilities() {
				fmt.Println("Enable Rootless Mode (Recommended):")
				fmt.Println("  Add to Kubernetes SecurityContext:")
				fmt.Println("    runAsUser: 1000")
				fmt.Println("    runAsNonRoot: true")
				fmt.Println("    allowPrivilegeEscalation: true")
				fmt.Println("    capabilities:")
				fmt.Println("      drop: [ALL]")
				fmt.Println("      add: [SETUID, SETGID]")
				fmt.Println()
			}
		}
		
		fmt.Println("Alternative: Use Root Mode (Less Secure):")
		fmt.Println("  Add to Kubernetes SecurityContext:")
		fmt.Println("    runAsUser: 0")
		fmt.Println()
		fmt.Println("Cannot proceed with build until environment is fixed.")
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
		fmt.Printf("  %s:%-*s %s %s\n", name, 20-len(name), "", path, getCheckmark(true))
	} else if foundPath, err := exec.LookPath(name); err == nil {
		fmt.Printf("  %s:%-*s %s %s\n", name, 20-len(name), "", foundPath, getCheckmark(true))
	} else {
		fmt.Printf("  %s:%-*s Not found %s\n", name, 20-len(name), "", getCheckmark(false))
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
		fmt.Printf("  %s version:%-*s %s %s\n", name, 12-len(name), "", version, getCheckmark(true))
	}
}
