package preflight

import (
	"fmt"
	"os"

	"github.com/rapidfort/kimia/pkg/logger"
)

// ValidationStatus represents the validation result status
type ValidationStatus int

const (
	StatusSuccess ValidationStatus = iota
	StatusWarning
	StatusError
)

// BuildMode represents the build mode
type BuildMode int

const (
	BuildModeRootless BuildMode = iota
)

func (m BuildMode) String() string {
	switch m {
	case BuildModeRootless:
		return "Rootless"
	default:
		return "Unknown"
	}
}

// ValidationResult holds the result of pre-flight validation
type ValidationResult struct {
	Status         ValidationStatus
	BuildMode      BuildMode
	StorageDriver  string
	Errors         []string
	Warnings       []string
	UID            int
	Capabilities   *CapabilityCheck
	UserNamespace  *UserNamespaceCheck
	Storage        *StorageCheck
	SetuidBinaries *SetuidBinaryCheck
}

func Validate(storageDriver string) (*ValidationResult, error) {
	logger.Debug("Starting pre-flight validation")

	result := &ValidationResult{
		StorageDriver: storageDriver,
		Errors:        []string{},
		Warnings:      []string{},
	}

	// 1. Detect current user context
	result.UID = os.Getuid()

	logger.Info("Current UID: %d", result.UID)

	// CRITICAL: Kimia is rootless-only and does NOT support root mode
	if result.UID == 0 {
		result.Errors = append(result.Errors,
			"Kimia does not support root mode",
			"",
			"Kimia is designed to run as a non-root user with capabilities.",
			"Please run as a non-root user (UID > 0)",
			"",
			"For Kubernetes, set:",
			"  securityContext:",
			"    runAsUser: 1000",
			"    runAsNonRoot: true",
			"    allowPrivilegeEscalation: true",
			"    capabilities:",
			"      drop: [ALL]")

		// Add storage-specific capability requirements
		if storageDriver == "overlay" {
			result.Errors = append(result.Errors,
				"      add: [SETUID, SETGID, MKNOD]  # MKNOD required for overlay")
		} else {
			result.Errors = append(result.Errors,
				"      add: [SETUID, SETGID]  # MKNOD not needed for vfs")
		}

		result.Status = StatusError
		return result, nil
	}

	// Kimia always operates in rootless mode
	result.BuildMode = BuildModeRootless

	// 2. Check capabilities
	caps, err := CheckCapabilities()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to check capabilities: %v", err))
		result.Status = StatusError
		return result, nil
	}
	result.Capabilities = caps

	// 2b. Check SETUID binaries
	setuidBins, err := CheckSetuidBinaries()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to check SETUID binaries: %v", err))
		result.Status = StatusError
		return result, nil
	}
	result.SetuidBinaries = setuidBins

	// 3. Check user namespaces for rootless mode
	userns, err := CheckUserNamespaces()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to check user namespaces: %v", err))
		result.Status = StatusError
		return result, nil
	}
	result.UserNamespace = userns

	// 4. Validate rootless mode configuration
	result.Status = validateRootlessMode(result)

	// 5. Validate storage driver
	if result.Status != StatusError {
		storageStatus := validateStorageDriver(result)
		if storageStatus == StatusError {
			result.Status = StatusError
		} else if storageStatus == StatusWarning && result.Status != StatusError {
			result.Status = StatusWarning
		}
	}

	return result, nil
}

func validateRootlessMode(result *ValidationResult) ValidationStatus {
	logger.Debug("Validating rootless mode configuration")

	var issues []string

	// Detect environment
	isK8s := IsInKubernetes()
	hasCapabilities := result.Capabilities.HasRequiredCapabilities()
	hasSetuidBinaries := result.SetuidBinaries.HasSetuidBinaries()
	setuidCanWork := CanSetuidBinariesWork()

	// For overlay storage, also check MKNOD capability
	needsMknod := result.StorageDriver == "overlay"
	hasMknod := result.Capabilities.HasCapability("CAP_MKNOD")

	logger.Debug("Environment: K8s=%v, HasCaps=%v, HasSetuid=%v, SetuidCanWork=%v, StorageDriver=%s, NeedsMknod=%v, HasMknod=%v",
		isK8s, hasCapabilities, hasSetuidBinaries, setuidCanWork, result.StorageDriver, needsMknod, hasMknod)

	// Determine if we can create user namespaces
	canCreateUserNS := false

	if isK8s {
		// In Kubernetes: Need capabilities AND allowPrivilegeEscalation
		if hasCapabilities && setuidCanWork {
			// Check MKNOD for overlay
			if needsMknod && !hasMknod {
				issues = append(issues, "Missing CAP_MKNOD capability (required for overlay storage)")
			} else {
				canCreateUserNS = true
			}
		} else {
			if !hasCapabilities {
				issues = append(issues, "Missing required capabilities: SETUID, SETGID")
				if needsMknod {
					issues = append(issues, "Missing required capability: MKNOD (for overlay storage)")
				}
			}
			if !setuidCanWork {
				issues = append(issues, "allowPrivilegeEscalation is not enabled (NoNewPrivs is set)")
				issues = append(issues, "SETUID binaries cannot escalate privileges")
			}
			issues = append(issues, "")
			issues = append(issues, "Kubernetes requires:")
			issues = append(issues, "  securityContext:")
			issues = append(issues, "    allowPrivilegeEscalation: true")
			issues = append(issues, "    capabilities:")
			issues = append(issues, "      drop: [ALL]")

			if needsMknod {
				issues = append(issues, "      add: [SETUID, SETGID, MKNOD]  # MKNOD for overlay")
			} else {
				issues = append(issues, "      add: [SETUID, SETGID]  # No MKNOD needed for vfs")
			}
		}
	} else {
		// In Docker/Standalone: Either capabilities OR SETUID binaries work
		if hasCapabilities {
			// Check MKNOD for overlay
			if needsMknod && !hasMknod {
				issues = append(issues, "Missing CAP_MKNOD capability (required for overlay storage)")
			} else {
				canCreateUserNS = true
				logger.Debug("User namespace creation possible via capabilities")
			}
		} else if hasSetuidBinaries && setuidCanWork {
			canCreateUserNS = true
			logger.Debug("User namespace creation possible via SETUID binaries")
		} else {
			issues = append(issues, "Cannot create user namespaces")
			issues = append(issues, "Need one of:")

			if needsMknod {
				issues = append(issues, "  1. Capabilities: --cap-add SETUID --cap-add SETGID --cap-add MKNOD")
			} else {
				issues = append(issues, "  1. Capabilities: --cap-add SETUID --cap-add SETGID")
			}

			issues = append(issues, "  2. SETUID binaries with: --security-opt seccomp=unconfined")

			if hasSetuidBinaries && !setuidCanWork {
				issues = append(issues, "")
				issues = append(issues, "Note: SETUID binaries found but cannot escalate privileges")
			}
		}
	}

	// Check user namespace support
	if canCreateUserNS && !result.UserNamespace.IsUserNamespaceReady() {
		nsIssues := result.UserNamespace.GetIssues()
		issues = append(issues, nsIssues...)
		canCreateUserNS = false
	}

	if !canCreateUserNS {
		result.Errors = append(result.Errors, "Cannot build in rootless mode:")
		result.Errors = append(result.Errors, issues...)
		return StatusError
	}

	// Success - add info about which method is being used
	if hasCapabilities {
		if needsMknod {
			logger.Info("Rootless mode: Using capabilities (SETUID, SETGID, MKNOD)")
		} else {
			logger.Info("Rootless mode: Using capabilities (SETUID, SETGID)")
		}
	} else if hasSetuidBinaries {
		logger.Info("Rootless mode: Using SETUID binaries (%s, %s)",
			result.SetuidBinaries.NewuidmapPath, result.SetuidBinaries.NewgidmapPath)
	}

	return StatusSuccess
}

// validateStorageDriver validates storage-specific requirements
func validateStorageDriver(result *ValidationResult) ValidationStatus {
	logger.Debug("Validating storage driver: %s", result.StorageDriver)

	if result.StorageDriver == "overlay" {
		// Check if MKNOD capability is available for overlay (rootless mode)
		if !result.Capabilities.HasCapability("CAP_MKNOD") {
			result.Warnings = append(result.Warnings,
				"",
				"Overlay storage requires CAP_MKNOD capability",
				"Consider using VFS storage if MKNOD cannot be granted")
			return StatusWarning
		}
	}

	return StatusSuccess
}

// PrintValidationResult prints the validation result to console
func PrintValidationResult(result *ValidationResult) {
	switch result.Status {
	case StatusSuccess:
		logger.Info("✓ Pre-flight validation passed")
		logger.Info("Build Mode: %s", result.BuildMode)
		logger.Info("Storage Driver: %s", result.StorageDriver)

	case StatusWarning:
		logger.Info("⚠  Pre-flight validation passed with warnings")
		logger.Info("Build Mode: %s", result.BuildMode)
		logger.Info("Storage Driver: %s", result.StorageDriver)
		logger.Warning("")
		printBox(result.Warnings, "WARNING")
		logger.Warning("")

	case StatusError:
		logger.Error("✗ Pre-flight validation failed")
		logger.Error("")
		printBox(result.Errors, "ERROR")
		logger.Error("")
	}
}

// printBox prints messages in a box using logger
func printBox(messages []string, title string) {
	width := 60

	// Determine which logger function to use based on title
	logFunc := logger.Warning
	if title == "ERROR" {
		logFunc = logger.Error
	}

	// Top border
	border := "╔"
	for i := 0; i < width; i++ {
		border += "═"
	}
	border += "╗"
	logFunc(border)

	// Title
	titlePadding := (width - len(title)) / 2
	titleLine := "║"
	for i := 0; i < titlePadding; i++ {
		titleLine += " "
	}
	titleLine += title
	for i := 0; i < width-titlePadding-len(title); i++ {
		titleLine += " "
	}
	titleLine += "║"
	logFunc(titleLine)

	// Separator
	separator := "╠"
	for i := 0; i < width; i++ {
		separator += "═"
	}
	separator += "╣"
	logFunc(separator)

	// Messages
	for _, msg := range messages {
		logFunc("║ %-*s ║", width-2, msg)
	}

	// Bottom border
	bottomBorder := "╚"
	for i := 0; i < width; i++ {
		bottomBorder += "═"
	}
	bottomBorder += "╝"
	logFunc(bottomBorder)
}

// ShouldProceed checks if build should proceed based on validation result
func (r *ValidationResult) ShouldProceed() bool {
	return r.Status != StatusError
}