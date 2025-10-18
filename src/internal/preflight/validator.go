package preflight

import (
	"fmt"
	"os"

	"github.com/rapidfort/smithy/pkg/logger"
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
	BuildModeRoot
)

func (m BuildMode) String() string {
	switch m {
	case BuildModeRootless:
		return "Rootless"
	case BuildModeRoot:
		return "Root"
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
	IsRoot         bool
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
	result.IsRoot = result.UID == 0

	logger.Info("Current UID: %d (%s)", result.UID, getUIDDescription(result.IsRoot))

	// 2. Check capabilities
	caps, err := CheckCapabilities()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to check capabilities: %v", err))
		result.Status = StatusError
		return result, nil
	}
	result.Capabilities = caps

	// 2b. Check SETUID binaries (NEW)
	setuidBins, err := CheckSetuidBinaries()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to check SETUID binaries: %v", err))
		result.Status = StatusError
		return result, nil
	}
	result.SetuidBinaries = setuidBins

	// 3. Determine build mode and validate
	if result.IsRoot {
		result.BuildMode = BuildModeRoot
		result.Status = validateRootMode(result)
	} else {
		result.BuildMode = BuildModeRootless

		// Check user namespaces for rootless mode
		userns, err := CheckUserNamespaces()
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Failed to check user namespaces: %v", err))
			result.Status = StatusError
			return result, nil
		}
		result.UserNamespace = userns

		result.Status = validateRootlessMode(result)
	}

	// Rest of validation...
	return result, nil
}

// validateRootMode validates root mode configuration
func validateRootMode(result *ValidationResult) ValidationStatus {
	logger.Debug("Validating root mode configuration")

	// Root mode always works, but has security warnings
	result.Warnings = append(result.Warnings,
		"Running in ROOT mode",
		"Security implications:",
		"  • Container escapes grant root access to host",
		"  • No user namespace isolation",
		"  • Violates Pod Security Standards",
		"For production, use rootless mode:",
		"  runAsUser: 1000",
		"  capabilities: [SETUID, SETGID]",
	)

	// Check if capabilities are unnecessarily configured
	if result.Capabilities.HasSetUID || result.Capabilities.HasSetGID {
		result.Warnings = append(result.Warnings,
			"",
			"Unnecessary capabilities detected",
			"Root already has all privileges. The SETUID/SETGID",
			"capabilities have no effect and can be removed.",
		)
	}

	// Root mode can proceed
	return StatusWarning
}

func validateRootlessMode(result *ValidationResult) ValidationStatus {
	logger.Debug("Validating rootless mode configuration")

	var issues []string

	// Detect environment
	isK8s := IsInKubernetes()
	hasCapabilities := result.Capabilities.HasRequiredCapabilities()
	hasSetuidBinaries := result.SetuidBinaries.HasSetuidBinaries()
	setuidCanWork := CanSetuidBinariesWork()

	logger.Debug("Environment: K8s=%v, HasCaps=%v, HasSetuid=%v, SetuidCanWork=%v",
		isK8s, hasCapabilities, hasSetuidBinaries, setuidCanWork)

	// Determine if we can create user namespaces
	canCreateUserNS := false

	if isK8s {
		// In Kubernetes: Need capabilities AND allowPrivilegeEscalation
		// We detect APE by checking if SETUID binaries can work
		if hasCapabilities && setuidCanWork {
			canCreateUserNS = true
		} else {
			if !hasCapabilities {
				issues = append(issues, "Missing required capabilities: SETUID, SETGID")
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
			issues = append(issues, "      add: [SETUID, SETGID]")
		}
	} else {
		// In Docker: Either capabilities OR SETUID binaries work
		if hasCapabilities {
			canCreateUserNS = true
			logger.Debug("User namespace creation possible via capabilities")
		} else if hasSetuidBinaries && setuidCanWork {
			canCreateUserNS = true
			logger.Debug("User namespace creation possible via SETUID binaries")
		} else {
			issues = append(issues, "Cannot create user namespaces")
			issues = append(issues, "Need one of:")
			issues = append(issues, "  1. Capabilities: --cap-add SETUID --cap-add SETGID")
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
		logger.Info("Rootless mode: Using capabilities (SETUID, SETGID)")
	} else if hasSetuidBinaries {
		logger.Info("Rootless mode: Using SETUID binaries (%s, %s)",
			result.SetuidBinaries.NewuidmapPath, result.SetuidBinaries.NewgidmapPath)
	}

	return StatusSuccess
}

// getUIDDescription returns a human-readable description of UID
func getUIDDescription(isRoot bool) string {
	if isRoot {
		return "root"
	}
	return "non-root"
}

// PrintValidationResult prints the validation result to console
func PrintValidationResult(result *ValidationResult) {
	switch result.Status {
	case StatusSuccess:
		logger.Info("✓ Pre-flight validation passed")
		logger.Info("Build Mode: %s", result.BuildMode)
		logger.Info("Storage Driver: %s", result.StorageDriver)

	case StatusWarning:
		logger.Info("⚠ Pre-flight validation passed with warnings")
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
