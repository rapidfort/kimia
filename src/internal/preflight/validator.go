package preflight

import (
	"fmt"
	"os"
	"strings"

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
	Status          ValidationStatus
	BuildMode       BuildMode
	StorageDriver   string
	Errors          []string
	Warnings        []string
	IsRoot          bool
	UID             int
	Capabilities    *CapabilityCheck
	UserNamespace   *UserNamespaceCheck
	Storage         *StorageCheck
}

// Validate performs comprehensive pre-flight validation
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
	
	// 4. If validation passed so far, check storage driver
	if result.Status != StatusError {
		storage, err := CheckStorageDrivers(result.IsRoot, result.Capabilities.HasRequiredCapabilities())
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Failed to check storage drivers: %v", err))
			result.Status = StatusError
			return result, nil
		}
		result.Storage = storage
		
		// Validate requested storage driver
		if err := ValidateStorageDriver(storageDriver, result.IsRoot, result.Capabilities.HasRequiredCapabilities()); err != nil {
			result.Errors = append(result.Errors, err.Error())
			result.Status = StatusError
			return result, nil
		}
	}
	
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

// validateRootlessMode validates rootless mode configuration
func validateRootlessMode(result *ValidationResult) ValidationStatus {
	logger.Debug("Validating rootless mode configuration")
	
	var issues []string
	
	// Check capabilities
	if !result.Capabilities.HasRequiredCapabilities() {
		missing := result.Capabilities.GetMissingCapabilities()
		issues = append(issues, fmt.Sprintf("Missing required capabilities: %s", strings.Join(missing, ", ")))
	}
	
	// Check user namespace support
	if !result.UserNamespace.IsUserNamespaceReady() {
		nsIssues := result.UserNamespace.GetIssues()
		issues = append(issues, nsIssues...)
	}
	
	if len(issues) > 0 {
		result.Errors = append(result.Errors, "Cannot build in rootless mode:")
		result.Errors = append(result.Errors, issues...)
		result.Errors = append(result.Errors, "", "Required configuration:")
		result.Errors = append(result.Errors, 
			"  securityContext:",
			"    runAsUser: 1000",
			"    runAsNonRoot: true", 
			"    allowPrivilegeEscalation: true",
			"    capabilities:",
			"      drop: [ALL]",
			"      add: [SETUID, SETGID]",
			"",
			"Alternative: Use root mode (less secure)",
			"  securityContext:",
			"    runAsUser: 0",
		)
		return StatusError
	}
	
	// Rootless mode successfully validated
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
		fmt.Fprintln(os.Stderr)
		printBox(result.Warnings, "WARNING")
		fmt.Fprintln(os.Stderr)
		
	case StatusError:
		logger.Error("✗ Pre-flight validation failed")
		fmt.Fprintln(os.Stderr)
		printBox(result.Errors, "ERROR")
		fmt.Fprintln(os.Stderr)
	}
}

// printBox prints messages in a box
func printBox(messages []string, title string) {
	width := 60
	
	// Top border
	fmt.Fprintf(os.Stderr, "╔")
	for i := 0; i < width; i++ {
		fmt.Fprintf(os.Stderr, "═")
	}
	fmt.Fprintf(os.Stderr, "╗\n")
	
	// Title
	titlePadding := (width - len(title)) / 2
	fmt.Fprintf(os.Stderr, "║")
	for i := 0; i < titlePadding; i++ {
		fmt.Fprintf(os.Stderr, " ")
	}
	fmt.Fprintf(os.Stderr, "%s", title)
	for i := 0; i < width-titlePadding-len(title); i++ {
		fmt.Fprintf(os.Stderr, " ")
	}
	fmt.Fprintf(os.Stderr, "║\n")
	
	// Separator
	fmt.Fprintf(os.Stderr, "╠")
	for i := 0; i < width; i++ {
		fmt.Fprintf(os.Stderr, "═")
	}
	fmt.Fprintf(os.Stderr, "╣\n")
	
	// Messages
	for _, msg := range messages {
		fmt.Fprintf(os.Stderr, "║ %-*s ║\n", width-2, msg)
	}
	
	// Bottom border
	fmt.Fprintf(os.Stderr, "╚")
	for i := 0; i < width; i++ {
		fmt.Fprintf(os.Stderr, "═")
	}
	fmt.Fprintf(os.Stderr, "╝\n")
}

// ShouldProceed checks if build should proceed based on validation result
func (r *ValidationResult) ShouldProceed() bool {
	return r.Status != StatusError
}
