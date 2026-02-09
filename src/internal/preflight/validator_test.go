package preflight

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
)

// ============================================================================
// Enum and Constant Tests
// ============================================================================

func TestValidationStatusConstants(t *testing.T) {
	// Test that constants have expected values
	if StatusSuccess != 0 {
		t.Errorf("StatusSuccess = %d, want 0", StatusSuccess)
	}
	if StatusWarning != 1 {
		t.Errorf("StatusWarning = %d, want 1", StatusWarning)
	}
	if StatusError != 2 {
		t.Errorf("StatusError = %d, want 2", StatusError)
	}

	// Test that they are different
	if StatusSuccess == StatusWarning {
		t.Error("StatusSuccess should not equal StatusWarning")
	}
	if StatusWarning == StatusError {
		t.Error("StatusWarning should not equal StatusError")
	}
	if StatusSuccess == StatusError {
		t.Error("StatusSuccess should not equal StatusError")
	}
}

func TestBuildModeConstants(t *testing.T) {
	// Test that constants have expected values
	if BuildModeRootless != 0 {
		t.Errorf("BuildModeRootless = %d, want 0", BuildModeRootless)
	}
}

func TestBuildModeString(t *testing.T) {
	tests := []struct {
		name string
		mode BuildMode
		want string
	}{
		{
			name: "Rootless mode",
			mode: BuildModeRootless,
			want: "Rootless",
		},
		{
			name: "Unknown mode (1)",
			mode: BuildMode(1),
			want: "Unknown",
		},
		{
			name: "Unknown mode (99)",
			mode: BuildMode(99),
			want: "Unknown",
		},
		{
			name: "Unknown mode (-1)",
			mode: BuildMode(-1),
			want: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.mode.String()
			if got != tt.want {
				t.Errorf("BuildMode(%d).String() = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

// ============================================================================
// Struct Field Tests
// ============================================================================

func TestValidationResultFields(t *testing.T) {
	result := &ValidationResult{
		Status:        StatusSuccess,
		BuildMode:     BuildModeRootless,
		StorageDriver: "vfs",
		Errors:        []string{"error1"},
		Warnings:      []string{"warning1"},
		UID:           1000,
	}

	if result.Status != StatusSuccess {
		t.Errorf("Status = %v, want %v", result.Status, StatusSuccess)
	}
	if result.BuildMode != BuildModeRootless {
		t.Errorf("BuildMode = %v, want %v", result.BuildMode, BuildModeRootless)
	}
	if result.StorageDriver != "vfs" {
		t.Errorf("StorageDriver = %q, want %q", result.StorageDriver, "vfs")
	}
	if len(result.Errors) != 1 || result.Errors[0] != "error1" {
		t.Errorf("Errors = %v, want [\"error1\"]", result.Errors)
	}
	if len(result.Warnings) != 1 || result.Warnings[0] != "warning1" {
		t.Errorf("Warnings = %v, want [\"warning1\"]", result.Warnings)
	}
	if result.UID != 1000 {
		t.Errorf("UID = %d, want 1000", result.UID)
	}
}

func TestValidationResultNilFields(t *testing.T) {
	result := &ValidationResult{}

	// Test that nil pointers don't cause panics
	if result.Capabilities != nil {
		t.Errorf("Capabilities should be nil initially")
	}
	if result.UserNamespace != nil {
		t.Errorf("UserNamespace should be nil initially")
	}
	if result.Storage != nil {
		t.Errorf("Storage should be nil initially")
	}
	if result.SetuidBinaries != nil {
		t.Errorf("SetuidBinaries should be nil initially")
	}
}

// ============================================================================
// ShouldProceed Tests
// ============================================================================

func TestShouldProceed(t *testing.T) {
	tests := []struct {
		name   string
		status ValidationStatus
		want   bool
	}{
		{
			name:   "Success should proceed",
			status: StatusSuccess,
			want:   true,
		},
		{
			name:   "Warning should proceed",
			status: StatusWarning,
			want:   true,
		},
		{
			name:   "Error should not proceed",
			status: StatusError,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ValidationResult{Status: tt.status}
			got := result.ShouldProceed()
			if got != tt.want {
				t.Errorf("ShouldProceed() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ============================================================================
// printBox Tests
// ============================================================================

func TestPrintBox(t *testing.T) {
	tests := []struct {
		name     string
		messages []string
		title    string
	}{
		{
			name:     "Single message - ERROR",
			messages: []string{"Test error message"},
			title:    "ERROR",
		},
		{
			name:     "Multiple messages - ERROR",
			messages: []string{"Error 1", "Error 2", "Error 3"},
			title:    "ERROR",
		},
		{
			name:     "Single message - WARNING",
			messages: []string{"Test warning message"},
			title:    "WARNING",
		},
		{
			name:     "Multiple messages - WARNING",
			messages: []string{"Warning 1", "Warning 2"},
			title:    "WARNING",
		},
		{
			name:     "Empty messages",
			messages: []string{},
			title:    "ERROR",
		},
		{
			name:     "Messages with empty strings",
			messages: []string{"Message 1", "", "Message 2"},
			title:    "ERROR",
		},
		{
			name:     "Long message",
			messages: []string{"This is a very long message that should still be formatted correctly"},
			title:    "ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify it doesn't panic
			// We can't easily test output without capturing logger output
			printBox(tt.messages, tt.title)
		})
	}
}

func TestPrintBoxEdgeCases(t *testing.T) {
	t.Run("nil messages slice", func(t *testing.T) {
		// Should not panic
		printBox(nil, "ERROR")
	})

	t.Run("very long title", func(t *testing.T) {
		// Should not panic
		printBox([]string{"test"}, "THIS IS A VERY LONG TITLE THAT EXCEEDS WIDTH")
	})

	t.Run("empty title", func(t *testing.T) {
		// Should not panic
		printBox([]string{"test"}, "")
	})

	t.Run("special characters in messages", func(t *testing.T) {
		messages := []string{
			"Message with Unicode: ✓ ✗ ⚠",
			"Message with tabs:\t\ttabs",
			"Message with newline: first\nsecond",
		}
		// Should not panic
		printBox(messages, "TEST")
	})
}

// ============================================================================
// PrintValidationResult Tests
// ============================================================================

func TestPrintValidationResult(t *testing.T) {
	tests := []struct {
		name   string
		result *ValidationResult
	}{
		{
			name: "Success status",
			result: &ValidationResult{
				Status:        StatusSuccess,
				BuildMode:     BuildModeRootless,
				StorageDriver: "vfs",
			},
		},
		{
			name: "Warning status",
			result: &ValidationResult{
				Status:        StatusWarning,
				BuildMode:     BuildModeRootless,
				StorageDriver: "overlay",
				Warnings:      []string{"Warning 1", "Warning 2"},
			},
		},
		{
			name: "Error status",
			result: &ValidationResult{
				Status: StatusError,
				Errors: []string{"Error 1", "Error 2"},
			},
		},
		{
			name: "Success with empty warnings",
			result: &ValidationResult{
				Status:        StatusSuccess,
				BuildMode:     BuildModeRootless,
				StorageDriver: "vfs",
				Warnings:      []string{},
			},
		},
		{
			name: "Error with empty errors",
			result: &ValidationResult{
				Status: StatusError,
				Errors: []string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify it doesn't panic
			PrintValidationResult(tt.result)
		})
	}
}

// ============================================================================
// validateStorageDriver Tests
// ============================================================================

func TestValidatorStorageValidation(t *testing.T) {
	tests := []struct {
		name          string
		result        *ValidationResult
		wantStatus    ValidationStatus
		wantWarnings  int
		checkWarnings bool
	}{
		{
			name: "VFS driver - no MKNOD needed",
			result: &ValidationResult{
				StorageDriver: "vfs",
				Warnings:      []string{},
				Capabilities: &CapabilityCheck{
					HasSetUID: true,
					HasSetGID: true,
					// No MKNOD
				},
			},
			wantStatus:    StatusSuccess,
			wantWarnings:  0,
			checkWarnings: true,
		},
		{
			name: "Overlay driver - with MKNOD",
			result: &ValidationResult{
				StorageDriver: "overlay",
				Warnings:      []string{},
				Capabilities: &CapabilityCheck{
					HasSetUID:     true,
					HasSetGID:     true,
					EffectiveCaps: (1 << CAP_MKNOD),
					Capabilities: []Capability{
						{Name: "CAP_MKNOD", Bit: CAP_MKNOD, Present: true},
					},
				},
			},
			wantStatus:    StatusSuccess,
			wantWarnings:  0,
			checkWarnings: true,
		},
		{
			name: "Overlay driver - without MKNOD",
			result: &ValidationResult{
				StorageDriver: "overlay",
				Warnings:      []string{},
				Capabilities: &CapabilityCheck{
					HasSetUID:     true,
					HasSetGID:     true,
					EffectiveCaps: 0,
					Capabilities:  []Capability{},
				},
			},
			wantStatus:    StatusWarning,
			wantWarnings:  3, // Warning message has multiple lines
			checkWarnings: true,
		},
		{
			name: "Native driver",
			result: &ValidationResult{
				StorageDriver: "native",
				Warnings:      []string{},
				Capabilities: &CapabilityCheck{
					HasSetUID: true,
					HasSetGID: true,
				},
			},
			wantStatus:    StatusSuccess,
			wantWarnings:  0,
			checkWarnings: true,
		},
		{
			name: "Empty storage driver",
			result: &ValidationResult{
				StorageDriver: "",
				Warnings:      []string{},
				Capabilities: &CapabilityCheck{
					HasSetUID: true,
					HasSetGID: true,
				},
			},
			wantStatus:    StatusSuccess,
			wantWarnings:  0,
			checkWarnings: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := validateStorageDriver(tt.result)
			if status != tt.wantStatus {
				t.Errorf("validateStorageDriver() status = %v, want %v", status, tt.wantStatus)
			}
			if tt.checkWarnings && len(tt.result.Warnings) != tt.wantWarnings {
				t.Errorf("validateStorageDriver() warnings count = %d, want %d (warnings: %v)",
					len(tt.result.Warnings), tt.wantWarnings, tt.result.Warnings)
			}
		})
	}
}

// ============================================================================
// validateRootlessMode Tests
// ============================================================================

func TestValidateRootlessModeKubernetes(t *testing.T) {
	// Save and restore environment
	originalK8s := os.Getenv("KUBERNETES_SERVICE_HOST")
	defer func() {
		if originalK8s == "" {
			os.Unsetenv("KUBERNETES_SERVICE_HOST")
		} else {
			os.Setenv("KUBERNETES_SERVICE_HOST", originalK8s)
		}
	}()

	// Simulate Kubernetes environment
	os.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")

	tests := []struct {
		name         string
		result       *ValidationResult
		wantStatus   ValidationStatus
		wantErrors   bool
		errorKeyword string
	}{
		{
			name: "K8s - Success with capabilities",
			result: &ValidationResult{
				StorageDriver: "vfs",
				Errors:        []string{},
				Capabilities: &CapabilityCheck{
					HasSetUID: true,
					HasSetGID: true,
				},
				SetuidBinaries: &SetuidBinaryCheck{
					NewuidmapPresent: false,
					NewgidmapPresent: false,
				},
				UserNamespace: &UserNamespaceCheck{
					Supported: true,
					CanCreate: true,
				},
			},
			wantStatus: StatusSuccess,
			wantErrors: false,
		},
		{
			name: "K8s - Error missing capabilities",
			result: &ValidationResult{
				StorageDriver: "vfs",
				Errors:        []string{},
				Capabilities: &CapabilityCheck{
					HasSetUID: false,
					HasSetGID: false,
				},
				SetuidBinaries: &SetuidBinaryCheck{
					NewuidmapPresent: true,
					NewgidmapPresent: true,
					BothAvailable:    true,
				},
				UserNamespace: &UserNamespaceCheck{
					Supported: true,
					CanCreate: true,
				},
			},
			wantStatus:   StatusError,
			wantErrors:   true,
			errorKeyword: "Missing required capabilities",
		},
		{
			name: "K8s - Overlay without MKNOD",
			result: &ValidationResult{
				StorageDriver: "overlay",
				Errors:        []string{},
				Capabilities: &CapabilityCheck{
					HasSetUID:     true,
					HasSetGID:     true,
					EffectiveCaps: 0, // No MKNOD
					Capabilities:  []Capability{},
				},
				SetuidBinaries: &SetuidBinaryCheck{
					NewuidmapPresent: false,
					NewgidmapPresent: false,
				},
				UserNamespace: &UserNamespaceCheck{
					Supported: true,
					CanCreate: true,
				},
			},
			wantStatus:   StatusError,
			wantErrors:   true,
			errorKeyword: "CAP_MKNOD",
		},
		{
			name: "K8s - Overlay with MKNOD",
			result: &ValidationResult{
				StorageDriver: "overlay",
				Errors:        []string{},
				Capabilities: &CapabilityCheck{
					HasSetUID:     true,
					HasSetGID:     true,
					EffectiveCaps: (1 << CAP_MKNOD),
					Capabilities: []Capability{
						{Name: "CAP_MKNOD", Bit: CAP_MKNOD, Present: true},
					},
				},
				SetuidBinaries: &SetuidBinaryCheck{
					NewuidmapPresent: false,
					NewgidmapPresent: false,
				},
				UserNamespace: &UserNamespaceCheck{
					Supported: true,
					CanCreate: true,
				},
			},
			wantStatus: StatusSuccess,
			wantErrors: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip if NoNewPrivs prevents SETUID from working
			if !CanSetuidBinariesWork() && !tt.result.Capabilities.HasRequiredCapabilities() {
				t.Skip("Skipping: system doesn't allow privilege escalation")
			}

			status := validateRootlessMode(tt.result)
			if status != tt.wantStatus {
				t.Errorf("validateRootlessMode() status = %v, want %v", status, tt.wantStatus)
			}
			if tt.wantErrors {
				if len(tt.result.Errors) == 0 {
					t.Errorf("validateRootlessMode() expected errors but got none")
				}
				if tt.errorKeyword != "" {
					found := false
					for _, err := range tt.result.Errors {
						if strings.Contains(err, tt.errorKeyword) {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("validateRootlessMode() errors should contain %q, got: %v",
							tt.errorKeyword, tt.result.Errors)
					}
				}
			} else {
				if len(tt.result.Errors) > 0 {
					t.Errorf("validateRootlessMode() expected no errors but got: %v", tt.result.Errors)
				}
			}
		})
	}
}

func TestValidateRootlessMode(t *testing.T) {
	tests := []struct {
		name         string
		result       *ValidationResult
		wantStatus   ValidationStatus
		wantErrors   bool
		errorKeyword string // Check if errors contain this keyword
	}{
		{
			name: "Success - Has capabilities and user namespace ready",
			result: &ValidationResult{
				StorageDriver: "vfs",
				Errors:        []string{},
				Capabilities: &CapabilityCheck{
					HasSetUID: true,
					HasSetGID: true,
				},
				SetuidBinaries: &SetuidBinaryCheck{
					NewuidmapPresent: false,
					NewgidmapPresent: false,
				},
				UserNamespace: &UserNamespaceCheck{
					Supported: true,
					CanCreate: true,
				},
			},
			wantStatus: StatusSuccess,
			wantErrors: false,
		},
		{
			name: "Success - Has SETUID binaries (if system allows)",
			result: &ValidationResult{
				StorageDriver: "vfs",
				Errors:        []string{},
				Capabilities: &CapabilityCheck{
					HasSetUID: false,
					HasSetGID: false,
				},
				SetuidBinaries: &SetuidBinaryCheck{
					NewuidmapPresent: true,
					NewgidmapPresent: true,
					NewuidmapSetuid:  true,
					NewgidmapSetuid:  true,
					NewuidmapPath:    "/usr/bin/newuidmap",
					NewgidmapPath:    "/usr/bin/newgidmap",
					BothAvailable:    true, // This is what HasSetuidBinaries() checks
				},
				UserNamespace: &UserNamespaceCheck{
					Supported: true,
					CanCreate: true,
				},
			},
			// Note: This may fail if CanSetuidBinariesWork() returns false
			// (e.g., in containers with NoNewPrivs set). We'll check actual result.
			wantStatus: StatusSuccess,
			wantErrors: false,
		},
		{
			name: "Error - No capabilities and no SETUID binaries",
			result: &ValidationResult{
				StorageDriver: "vfs",
				Errors:        []string{},
				Capabilities: &CapabilityCheck{
					HasSetUID: false,
					HasSetGID: false,
				},
				SetuidBinaries: &SetuidBinaryCheck{
					NewuidmapPresent: false,
					NewgidmapPresent: false,
				},
				UserNamespace: &UserNamespaceCheck{
					Supported: true,
					CanCreate: false,
				},
			},
			wantStatus:   StatusError,
			wantErrors:   true,
			errorKeyword: "Cannot build in rootless mode",
		},
		{
			name: "Error - User namespace not supported",
			result: &ValidationResult{
				StorageDriver: "vfs",
				Errors:        []string{},
				Capabilities: &CapabilityCheck{
					HasSetUID: true,
					HasSetGID: true,
				},
				SetuidBinaries: &SetuidBinaryCheck{
					NewuidmapPresent: false,
					NewgidmapPresent: false,
				},
				UserNamespace: &UserNamespaceCheck{
					Supported: false,
					CanCreate: false,
				},
			},
			wantStatus:   StatusError,
			wantErrors:   true,
			errorKeyword: "Cannot build in rootless mode",
		},
		{
			name: "Error - Overlay driver without MKNOD capability",
			result: &ValidationResult{
				StorageDriver: "overlay",
				Errors:        []string{},
				Capabilities: &CapabilityCheck{
					HasSetUID:     true,
					HasSetGID:     true,
					EffectiveCaps: 0, // No MKNOD
					Capabilities:  []Capability{},
				},
				SetuidBinaries: &SetuidBinaryCheck{
					NewuidmapPresent: false,
					NewgidmapPresent: false,
				},
				UserNamespace: &UserNamespaceCheck{
					Supported: true,
					CanCreate: true,
				},
			},
			wantStatus:   StatusError,
			wantErrors:   true,
			errorKeyword: "CAP_MKNOD",
		},
		{
			name: "Success - Overlay driver with MKNOD capability",
			result: &ValidationResult{
				StorageDriver: "overlay",
				Errors:        []string{},
				Capabilities: &CapabilityCheck{
					HasSetUID:     true,
					HasSetGID:     true,
					EffectiveCaps: (1 << CAP_MKNOD),
					Capabilities: []Capability{
						{Name: "CAP_MKNOD", Bit: CAP_MKNOD, Present: true},
					},
				},
				SetuidBinaries: &SetuidBinaryCheck{
					NewuidmapPresent: false,
					NewgidmapPresent: false,
				},
				UserNamespace: &UserNamespaceCheck{
					Supported: true,
					CanCreate: true,
				},
			},
			wantStatus: StatusSuccess,
			wantErrors: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Special handling for SETUID binary test - depends on system
			if strings.Contains(tt.name, "SETUID binaries") && !tt.wantErrors {
				// Check if SETUID binaries can actually work on this system
				if !CanSetuidBinariesWork() {
					t.Skip("Skipping SETUID binary test: system doesn't allow privilege escalation (NoNewPrivs set)")
				}
			}

			status := validateRootlessMode(tt.result)
			if status != tt.wantStatus {
				t.Errorf("validateRootlessMode() status = %v, want %v", status, tt.wantStatus)
			}
			if tt.wantErrors {
				if len(tt.result.Errors) == 0 {
					t.Errorf("validateRootlessMode() expected errors but got none")
				}
				if tt.errorKeyword != "" {
					found := false
					for _, err := range tt.result.Errors {
						if strings.Contains(err, tt.errorKeyword) {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("validateRootlessMode() errors should contain %q, got: %v",
							tt.errorKeyword, tt.result.Errors)
					}
				}
			} else {
				if len(tt.result.Errors) > 0 {
					t.Errorf("validateRootlessMode() expected no errors but got: %v", tt.result.Errors)
				}
			}
		})
	}
}

// ============================================================================
// Validate Tests (Integration)
// ============================================================================

func TestValidate(t *testing.T) {
	// Skip if running as root
	if os.Getuid() == 0 {
		t.Skip("Skipping test: running as root user")
	}

	tests := []struct {
		name           string
		storageDriver  string
		wantStatus     ValidationStatus // We can't predict exact status, but can check it's set
		checkUID       bool
		checkBuildMode bool
	}{
		{
			name:           "VFS storage driver",
			storageDriver:  "vfs",
			checkUID:       true,
			checkBuildMode: true,
		},
		{
			name:           "Overlay storage driver",
			storageDriver:  "overlay",
			checkUID:       true,
			checkBuildMode: true,
		},
		{
			name:           "Native storage driver",
			storageDriver:  "native",
			checkUID:       true,
			checkBuildMode: true,
		},
		{
			name:           "Empty storage driver",
			storageDriver:  "",
			checkUID:       true,
			checkBuildMode: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Validate(tt.storageDriver)
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}

			if result == nil {
				t.Fatal("Validate() returned nil result")
			}

			// Check basic fields are populated
			if tt.checkUID && result.UID == 0 {
				t.Error("Validate() UID should be non-zero (not root)")
			}

			if tt.checkBuildMode && result.BuildMode != BuildModeRootless {
				t.Errorf("Validate() BuildMode = %v, want %v", result.BuildMode, BuildModeRootless)
			}

			if result.StorageDriver != tt.storageDriver {
				t.Errorf("Validate() StorageDriver = %q, want %q", result.StorageDriver, tt.storageDriver)
			}

			// Check that sub-checks were performed
			if result.Capabilities == nil {
				t.Error("Validate() Capabilities should be populated")
			}
			if result.SetuidBinaries == nil {
				t.Error("Validate() SetuidBinaries should be populated")
			}
			if result.UserNamespace == nil {
				t.Error("Validate() UserNamespace should be populated")
			}

			// Status should be one of the valid values
			if result.Status != StatusSuccess && result.Status != StatusWarning && result.Status != StatusError {
				t.Errorf("Validate() Status = %v, expected one of: Success, Warning, Error", result.Status)
			}

			t.Logf("Validation result: Status=%v, BuildMode=%v, StorageDriver=%s",
				result.Status, result.BuildMode, result.StorageDriver)
			t.Logf("  Capabilities: SETUID=%v, SETGID=%v",
				result.Capabilities.HasSetUID, result.Capabilities.HasSetGID)
			t.Logf("  UserNamespace: Supported=%v, CanCreate=%v",
				result.UserNamespace.Supported, result.UserNamespace.CanCreate)
			t.Logf("  Errors: %d, Warnings: %d", len(result.Errors), len(result.Warnings))
		})
	}
}

func TestValidateAsRoot(t *testing.T) {
	// Only run if we are root
	if os.Getuid() != 0 {
		t.Skip("Skipping test: not running as root")
	}

	tests := []struct {
		name          string
		storageDriver string
		wantStatus    ValidationStatus
		errorKeyword  string
	}{
		{
			name:          "Root user with VFS",
			storageDriver: "vfs",
			wantStatus:    StatusError,
			errorKeyword:  "does not support root mode",
		},
		{
			name:          "Root user with overlay",
			storageDriver: "overlay",
			wantStatus:    StatusError,
			errorKeyword:  "does not support root mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Validate(tt.storageDriver)
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}

			if result.Status != tt.wantStatus {
				t.Errorf("Validate() Status = %v, want %v", result.Status, tt.wantStatus)
			}

			if result.UID != 0 {
				t.Errorf("Validate() UID = %d, want 0", result.UID)
			}

			// Check error message
			if len(result.Errors) == 0 {
				t.Error("Validate() should have errors for root user")
			}

			found := false
			for _, err := range result.Errors {
				if strings.Contains(err, tt.errorKeyword) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Validate() errors should contain %q, got: %v", tt.errorKeyword, result.Errors)
			}
		})
	}
}

func TestValidateConsistency(t *testing.T) {
	// Skip if running as root
	if os.Getuid() == 0 {
		t.Skip("Skipping test: running as root user")
	}

	t.Run("Validate and ShouldProceed consistency", func(t *testing.T) {
		result, err := Validate("vfs")
		if err != nil {
			t.Fatalf("Validate() error = %v", err)
		}

		// If status is error, should not proceed
		if result.Status == StatusError && result.ShouldProceed() {
			t.Error("Status is Error but ShouldProceed() returns true")
		}

		// If status is success or warning, should proceed
		if result.Status != StatusError && !result.ShouldProceed() {
			t.Error("Status is not Error but ShouldProceed() returns false")
		}
	})

	t.Run("Validate error/warning lists consistency", func(t *testing.T) {
		result, err := Validate("vfs")
		if err != nil {
			t.Fatalf("Validate() error = %v", err)
		}

		// If status is error, should have errors
		if result.Status == StatusError && len(result.Errors) == 0 {
			t.Error("Status is Error but no errors in Errors list")
		}

		// If status is warning, should have warnings
		if result.Status == StatusWarning && len(result.Warnings) == 0 {
			t.Error("Status is Warning but no warnings in Warnings list")
		}
	})
}

// ============================================================================
// Real-World Scenario Tests
// ============================================================================

func TestValidatorRealWorldScenarios(t *testing.T) {
	// Skip if running as root
	if os.Getuid() == 0 {
		t.Skip("Skipping test: running as root user")
	}

	t.Run("typical docker container scenario", func(t *testing.T) {
		// In a typical Docker container, we'd have capabilities
		result, err := Validate("vfs")
		if err != nil {
			t.Fatalf("Validate() error = %v", err)
		}

		// Should have performed all checks
		if result.Capabilities == nil {
			t.Error("Should have checked capabilities")
		}
		if result.UserNamespace == nil {
			t.Error("Should have checked user namespaces")
		}
		if result.SetuidBinaries == nil {
			t.Error("Should have checked SETUID binaries")
		}

		t.Logf("Docker scenario result: Status=%v, Errors=%d, Warnings=%d",
			result.Status, len(result.Errors), len(result.Warnings))
	})

	t.Run("kubernetes pod scenario", func(t *testing.T) {
		// Save and restore environment
		originalK8s := os.Getenv("KUBERNETES_SERVICE_HOST")
		defer func() {
			if originalK8s == "" {
				os.Unsetenv("KUBERNETES_SERVICE_HOST")
			} else {
				os.Setenv("KUBERNETES_SERVICE_HOST", originalK8s)
			}
		}()

		// Simulate Kubernetes environment
		os.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")

		result, err := Validate("vfs")
		if err != nil {
			t.Fatalf("Validate() error = %v", err)
		}

		// Should have performed all checks
		if result.Capabilities == nil {
			t.Error("Should have checked capabilities")
		}

		t.Logf("K8s scenario result: Status=%v, Errors=%d, Warnings=%d",
			result.Status, len(result.Errors), len(result.Warnings))
	})

	t.Run("overlay storage scenario", func(t *testing.T) {
		result, err := Validate("overlay")
		if err != nil {
			t.Fatalf("Validate() error = %v", err)
		}

		// Check MKNOD capability was evaluated
		hasMknod := result.Capabilities.HasCapability("CAP_MKNOD")

		if result.Status == StatusError && hasMknod {
			t.Error("Should not error if MKNOD capability is present for overlay")
		}

		t.Logf("Overlay scenario result: Status=%v, HasMknod=%v, Errors=%d, Warnings=%d",
			result.Status, hasMknod, len(result.Errors), len(result.Warnings))
	})

	t.Run("vfs storage scenario", func(t *testing.T) {
		result, err := Validate("vfs")
		if err != nil {
			t.Fatalf("Validate() error = %v", err)
		}

		// VFS doesn't require MKNOD, so even without it should work
		// (as long as basic SETUID/SETGID requirements are met)

		t.Logf("VFS scenario result: Status=%v, Errors=%d, Warnings=%d",
			result.Status, len(result.Errors), len(result.Warnings))
	})
}

// ============================================================================
// Edge Case Tests
// ============================================================================

func TestValidatorEdgeCases(t *testing.T) {
	t.Run("validateStorageDriver with nil capabilities panics", func(t *testing.T) {
		result := &ValidationResult{
			StorageDriver: "overlay",
			Warnings:      []string{},
			Capabilities:  nil, // Nil capabilities
		}

		// This function requires non-nil Capabilities - it will panic
		// This documents the expected behavior
		defer func() {
			if r := recover(); r == nil {
				t.Error("validateStorageDriver should panic with nil capabilities for overlay driver")
			}
		}()

		validateStorageDriver(result)
	})

	t.Run("validateStorageDriver with nil capabilities non-overlay", func(t *testing.T) {
		result := &ValidationResult{
			StorageDriver: "vfs", // Non-overlay doesn't check MKNOD
			Warnings:      []string{},
			Capabilities:  nil, // Nil capabilities
		}

		// VFS doesn't check MKNOD, so nil capabilities is OK
		status := validateStorageDriver(result)

		if status != StatusSuccess {
			t.Errorf("validateStorageDriver() with vfs and nil capabilities = %v, want Success", status)
		}
	})

	t.Run("validateRootlessMode with nil fields panics", func(t *testing.T) {
		result := &ValidationResult{
			StorageDriver:  "vfs",
			Errors:         []string{},
			Capabilities:   nil,
			SetuidBinaries: nil,
			UserNamespace:  nil,
		}

		// This function requires non-nil fields - it will panic
		// This documents the expected behavior
		defer func() {
			if r := recover(); r == nil {
				t.Error("validateRootlessMode should panic with nil fields")
			}
		}()

		validateRootlessMode(result)
	})

	t.Run("ShouldProceed with zero value", func(t *testing.T) {
		result := &ValidationResult{}

		// Zero value of ValidationStatus is StatusSuccess, so should proceed
		if !result.ShouldProceed() {
			t.Error("Zero-value ValidationResult should proceed (default is Success)")
		}
	})

	t.Run("very long error messages", func(t *testing.T) {
		longMsg := strings.Repeat("This is a very long error message. ", 100)
		result := &ValidationResult{
			Status: StatusError,
			Errors: []string{longMsg},
		}

		// Should not panic
		PrintValidationResult(result)
	})

	t.Run("empty storage driver strings", func(t *testing.T) {
		result := &ValidationResult{
			StorageDriver: "",
			Warnings:      []string{},
			Capabilities: &CapabilityCheck{
				HasSetUID: true,
				HasSetGID: true,
			},
		}

		status := validateStorageDriver(result)
		if status != StatusSuccess {
			t.Errorf("validateStorageDriver with empty driver = %v, want Success", status)
		}
	})
}

// ============================================================================
// Concurrent Tests
// ============================================================================

func TestValidateConcurrent(t *testing.T) {
	// Skip if running as root
	if os.Getuid() == 0 {
		t.Skip("Skipping test: running as root user")
	}

	const numGoroutines = 10

	t.Run("concurrent Validate calls", func(t *testing.T) {
		var wg sync.WaitGroup
		results := make([]*ValidationResult, numGoroutines)
		errors := make([]error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				results[index], errors[index] = Validate("vfs")
			}(i)
		}

		wg.Wait()

		// All should succeed without error
		for i, err := range errors {
			if err != nil {
				t.Errorf("Goroutine %d: Validate() error = %v", i, err)
			}
			if results[i] == nil {
				t.Errorf("Goroutine %d: Validate() returned nil result", i)
			}
		}

		// All should have same UID
		for i := 1; i < numGoroutines; i++ {
			if results[i].UID != results[0].UID {
				t.Errorf("Inconsistent UID: results[%d].UID = %d, results[0].UID = %d",
					i, results[i].UID, results[0].UID)
			}
		}

		t.Logf("All %d concurrent Validate() calls completed successfully", numGoroutines)
	})

	t.Run("concurrent ShouldProceed calls", func(t *testing.T) {
		result := &ValidationResult{Status: StatusSuccess}

		var wg sync.WaitGroup
		results := make([]bool, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				results[index] = result.ShouldProceed()
			}(i)
		}

		wg.Wait()

		// All should return same result
		for i := 1; i < numGoroutines; i++ {
			if results[i] != results[0] {
				t.Errorf("Inconsistent ShouldProceed: results[%d] = %v, results[0] = %v",
					i, results[i], results[0])
			}
		}
	})

	t.Run("concurrent validateStorageDriver calls", func(t *testing.T) {
		var wg sync.WaitGroup
		statuses := make([]ValidationStatus, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				result := &ValidationResult{
					StorageDriver: "vfs",
					Warnings:      []string{},
					Capabilities: &CapabilityCheck{
						HasSetUID: true,
						HasSetGID: true,
					},
				}
				statuses[index] = validateStorageDriver(result)
			}(i)
		}

		wg.Wait()

		// All should return same status
		for i := 1; i < numGoroutines; i++ {
			if statuses[i] != statuses[0] {
				t.Errorf("Inconsistent status: statuses[%d] = %v, statuses[0] = %v",
					i, statuses[i], statuses[0])
			}
		}
	})
}

// ============================================================================
// Benchmarks
// ============================================================================

func BenchmarkValidate(b *testing.B) {
	// Skip if running as root
	if os.Getuid() == 0 {
		b.Skip("Skipping benchmark: running as root user")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Validate("vfs")
	}
}

func BenchmarkValidateOverlay(b *testing.B) {
	// Skip if running as root
	if os.Getuid() == 0 {
		b.Skip("Skipping benchmark: running as root user")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Validate("overlay")
	}
}

func BenchmarkValidatorStorageValidation(b *testing.B) {
	result := &ValidationResult{
		StorageDriver: "vfs",
		Warnings:      []string{},
		Capabilities: &CapabilityCheck{
			HasSetUID: true,
			HasSetGID: true,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validateStorageDriver(result)
	}
}

func BenchmarkValidateRootlessMode(b *testing.B) {
	result := &ValidationResult{
		StorageDriver: "vfs",
		Errors:        []string{},
		Capabilities: &CapabilityCheck{
			HasSetUID: true,
			HasSetGID: true,
		},
		SetuidBinaries: &SetuidBinaryCheck{
			NewuidmapPresent: false,
			NewgidmapPresent: false,
		},
		UserNamespace: &UserNamespaceCheck{
			Supported: true,
			CanCreate: true,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Create a fresh result for each iteration to avoid accumulated errors
		testResult := &ValidationResult{
			StorageDriver:  result.StorageDriver,
			Errors:         []string{},
			Capabilities:   result.Capabilities,
			SetuidBinaries: result.SetuidBinaries,
			UserNamespace:  result.UserNamespace,
		}
		validateRootlessMode(testResult)
	}
}

func BenchmarkShouldProceed(b *testing.B) {
	result := &ValidationResult{Status: StatusSuccess}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = result.ShouldProceed()
	}
}

func BenchmarkPrintValidationResult(b *testing.B) {
	result := &ValidationResult{
		Status:        StatusSuccess,
		BuildMode:     BuildModeRootless,
		StorageDriver: "vfs",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		PrintValidationResult(result)
	}
}

func BenchmarkBuildModeString(b *testing.B) {
	mode := BuildModeRootless

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mode.String()
	}
}

func BenchmarkValidateConcurrent(b *testing.B) {
	// Skip if running as root
	if os.Getuid() == 0 {
		b.Skip("Skipping benchmark: running as root user")
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = Validate("vfs")
		}
	})
}

// ============================================================================
// Examples
// ============================================================================

func ExampleValidate() {
	// Perform pre-flight validation with VFS storage driver
	result, err := Validate("vfs")
	if err != nil {
		panic(err)
	}

	// Check if we should proceed with the build
	if !result.ShouldProceed() {
		// Print errors and exit
		PrintValidationResult(result)
		return
	}

	// Proceed with build
	_ = result
}

func ExampleValidate_overlay() {
	// Perform pre-flight validation with overlay storage driver
	result, err := Validate("overlay")
	if err != nil {
		panic(err)
	}

	// Print the result
	PrintValidationResult(result)

	// Check status
	switch result.Status {
	case StatusSuccess:
		// Proceed with build
	case StatusWarning:
		// Proceed but user should be aware of warnings
	case StatusError:
		// Cannot proceed
		return
	}
}

func ExampleValidationResult_ShouldProceed() {
	result := &ValidationResult{
		Status: StatusSuccess,
	}

	if result.ShouldProceed() {
		// Continue with build process
		_ = result
	}
}

func ExampleBuildMode_String() {
	mode := BuildModeRootless
	fmt.Println(mode.String())
	// Output: Rootless
}

func ExamplePrintValidationResult() {
	result := &ValidationResult{
		Status:        StatusSuccess,
		BuildMode:     BuildModeRootless,
		StorageDriver: "vfs",
	}

	// Print formatted validation result
	PrintValidationResult(result)
}

func ExamplePrintValidationResult_withWarnings() {
	result := &ValidationResult{
		Status:        StatusWarning,
		BuildMode:     BuildModeRootless,
		StorageDriver: "overlay",
		Warnings: []string{
			"Overlay storage requires CAP_MKNOD capability",
			"Consider using VFS storage if MKNOD cannot be granted",
		},
	}

	// Print formatted validation result with warnings
	PrintValidationResult(result)
}

func ExamplePrintValidationResult_withErrors() {
	result := &ValidationResult{
		Status: StatusError,
		Errors: []string{
			"Cannot create user namespaces",
			"Need one of:",
			"  1. Capabilities: --cap-add SETUID --cap-add SETGID",
			"  2. SETUID binaries with: --security-opt seccomp=unconfined",
		},
	}

	// Print formatted validation result with errors
	PrintValidationResult(result)
}
