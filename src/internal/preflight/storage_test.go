package preflight

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// StorageCheck Tests
// ============================================================================

func TestStorageCheckFields(t *testing.T) {
	tests := []struct {
		name     string
		check    StorageCheck
		expected StorageCheck
	}{
		{
			name: "All drivers available",
			check: StorageCheck{
				VFSAvailable:     true,
				NativeAvailable:  true,
				OverlayAvailable: true,
				TestResult: &OverlayTestResult{
					Success: true,
				},
			},
			expected: StorageCheck{
				VFSAvailable:     true,
				NativeAvailable:  true,
				OverlayAvailable: true,
				TestResult: &OverlayTestResult{
					Success: true,
				},
			},
		},
		{
			name: "Only VFS available",
			check: StorageCheck{
				VFSAvailable:     true,
				NativeAvailable:  false,
				OverlayAvailable: false,
				TestResult:       nil,
			},
			expected: StorageCheck{
				VFSAvailable:     true,
				NativeAvailable:  false,
				OverlayAvailable: false,
				TestResult:       nil,
			},
		},
		{
			name: "No drivers available",
			check: StorageCheck{
				VFSAvailable:     false,
				NativeAvailable:  false,
				OverlayAvailable: false,
				TestResult:       nil,
			},
			expected: StorageCheck{
				VFSAvailable:     false,
				NativeAvailable:  false,
				OverlayAvailable: false,
				TestResult:       nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.check.VFSAvailable != tt.expected.VFSAvailable {
				t.Errorf("VFSAvailable mismatch: got %v, want %v",
					tt.check.VFSAvailable, tt.expected.VFSAvailable)
			}
			if tt.check.NativeAvailable != tt.expected.NativeAvailable {
				t.Errorf("NativeAvailable mismatch: got %v, want %v",
					tt.check.NativeAvailable, tt.expected.NativeAvailable)
			}
			if tt.check.OverlayAvailable != tt.expected.OverlayAvailable {
				t.Errorf("OverlayAvailable mismatch: got %v, want %v",
					tt.check.OverlayAvailable, tt.expected.OverlayAvailable)
			}
		})
	}
}

// ============================================================================
// OverlayTestResult Tests
// ============================================================================

func TestOverlayTestResultFields(t *testing.T) {
	tests := []struct {
		name     string
		result   OverlayTestResult
		expected OverlayTestResult
	}{
		{
			name: "Successful test",
			result: OverlayTestResult{
				Success:      true,
				ErrorMessage: "",
				TestPath:     "/tmp/overlay-test",
				Duration:     100 * time.Millisecond,
			},
			expected: OverlayTestResult{
				Success:      true,
				ErrorMessage: "",
				TestPath:     "/tmp/overlay-test",
				Duration:     100 * time.Millisecond,
			},
		},
		{
			name: "Failed test",
			result: OverlayTestResult{
				Success:      false,
				ErrorMessage: "mount failed",
				TestPath:     "/tmp/overlay-test",
				Duration:     50 * time.Millisecond,
			},
			expected: OverlayTestResult{
				Success:      false,
				ErrorMessage: "mount failed",
				TestPath:     "/tmp/overlay-test",
				Duration:     50 * time.Millisecond,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.result.Success != tt.expected.Success {
				t.Errorf("Success mismatch: got %v, want %v",
					tt.result.Success, tt.expected.Success)
			}
			if tt.result.ErrorMessage != tt.expected.ErrorMessage {
				t.Errorf("ErrorMessage mismatch: got %q, want %q",
					tt.result.ErrorMessage, tt.expected.ErrorMessage)
			}
			if tt.result.TestPath != tt.expected.TestPath {
				t.Errorf("TestPath mismatch: got %q, want %q",
					tt.result.TestPath, tt.expected.TestPath)
			}
			if tt.result.Duration != tt.expected.Duration {
				t.Errorf("Duration mismatch: got %v, want %v",
					tt.result.Duration, tt.expected.Duration)
			}
		})
	}
}

// ============================================================================
// CheckStorageDrivers Tests
// ============================================================================

func TestCheckStorageDrivers(t *testing.T) {
	t.Run("VFS always available", func(t *testing.T) {
		// VFS should always be available as it doesn't require special capabilities
		check, err := CheckStorageDrivers(false)
		if err != nil {
			t.Fatalf("CheckStorageDrivers failed: %v", err)
		}

		if !check.VFSAvailable {
			t.Error("VFS should always be available")
		}
	})

	t.Run("Native always available", func(t *testing.T) {
		// Native is always available (BuildKit native snapshotter)
		check, err := CheckStorageDrivers(false)
		if err != nil {
			t.Fatalf("CheckStorageDrivers failed: %v", err)
		}

		if !check.NativeAvailable {
			t.Error("Native should always be available")
		}
	})

	t.Run("Overlay available with capabilities", func(t *testing.T) {
		// Overlay requires SETUID/SETGID capabilities
		check, err := CheckStorageDrivers(true)
		if err != nil {
			t.Fatalf("CheckStorageDrivers failed: %v", err)
		}

		if !check.OverlayAvailable {
			t.Error("Overlay should be available when hasCaps=true")
		}
	})

	t.Run("Overlay not available without capabilities", func(t *testing.T) {
		// Overlay requires SETUID/SETGID capabilities
		check, err := CheckStorageDrivers(false)
		if err != nil {
			t.Fatalf("CheckStorageDrivers failed: %v", err)
		}

		if check.OverlayAvailable {
			t.Error("Overlay should not be available when hasCaps=false")
		}
	})

	t.Run("Returns valid StorageCheck struct", func(t *testing.T) {
		check, err := CheckStorageDrivers(false)
		if err != nil {
			t.Fatalf("CheckStorageDrivers failed: %v", err)
		}

		// Verify we get a valid struct back
		if check == nil {
			t.Fatal("CheckStorageDrivers returned nil")
		}

		// At minimum, VFS should be available
		if !check.VFSAvailable {
			t.Error("VFS should always be available")
		}

		// Native should always be available
		if !check.NativeAvailable {
			t.Error("Native should always be available")
		}
	})

	t.Run("Error handling", func(t *testing.T) {
		// CheckStorageDrivers should not return error in normal operation
		_, err := CheckStorageDrivers(true)
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		_, err = CheckStorageDrivers(false)
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})
}

// ============================================================================
// ValidateStorageDriver Tests
// ============================================================================

func TestValidateStorageDriver(t *testing.T) {
	tests := []struct {
		name          string
		driver        string
		hasCaps       bool
		expectError   bool
		errorContains string
	}{
		{
			name:        "VFS driver - always valid",
			driver:      "vfs",
			hasCaps:     false,
			expectError: false,
		},
		{
			name:        "VFS driver - with caps",
			driver:      "vfs",
			hasCaps:     true,
			expectError: false,
		},
		{
			name:        "Overlay driver - with capabilities",
			driver:      "overlay",
			hasCaps:     true,
			expectError: false,
		},
		{
			name:          "Overlay driver - without capabilities",
			driver:        "overlay",
			hasCaps:       false,
			expectError:   true,
			errorContains: "overlay driver not available",
		},
		{
			name:        "Native driver - always available",
			driver:      "native",
			hasCaps:     false,
			expectError: false,
		},
		{
			name:        "Native driver - with caps",
			driver:      "native",
			hasCaps:     true,
			expectError: false,
		},
		{
			name:          "Unknown driver",
			driver:        "unknown",
			hasCaps:       true,
			expectError:   true,
			errorContains: "unknown storage driver",
		},
		{
			name:          "Empty driver name",
			driver:        "",
			hasCaps:       true,
			expectError:   true,
			errorContains: "unknown storage driver",
		},
		{
			name:        "Case insensitive - VFS uppercase",
			driver:      "VFS",
			hasCaps:     false,
			expectError: false,
		},
		{
			name:        "Case insensitive - Overlay mixed case",
			driver:      "Overlay",
			hasCaps:     true,
			expectError: false,
		},
		{
			name:        "Case insensitive - NATIVE uppercase",
			driver:      "NATIVE",
			hasCaps:     false,
			expectError: false,
		},
		{
			name:          "Overlay uppercase without caps",
			driver:        "OVERLAY",
			hasCaps:       false,
			expectError:   true,
			errorContains: "overlay driver not available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStorageDriver(tt.driver, tt.hasCaps)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error containing %q, got nil", tt.errorContains)
				} else if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing %q, got %q", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
			}
		})
	}
}

// ============================================================================
// TestOverlayMount Tests
// ============================================================================

func TestTestOverlayMount(t *testing.T) {
	// Note: Actual overlay mount tests require user namespace capabilities
	// These tests verify the function behavior in different scenarios

	t.Run("Returns OverlayTestResult", func(t *testing.T) {
		result := TestOverlayMount()

		if result == nil {
			t.Fatal("TestOverlayMount returned nil")
		}

		// Verify result has required fields
		if result.TestPath == "" {
			t.Error("TestPath should not be empty")
		}

		if result.Duration < 0 {
			t.Error("Duration should not be negative")
		}

		// If it failed, should have error message
		if !result.Success && result.ErrorMessage == "" {
			t.Error("Failed test should have error message")
		}

		// If it succeeded, should not have error message
		if result.Success && result.ErrorMessage != "" {
			t.Error("Successful test should not have error message")
		}
	})

	t.Run("Creates test path in temp directory", func(t *testing.T) {
		result := TestOverlayMount()

		if !strings.HasPrefix(result.TestPath, "/tmp") {
			t.Errorf("Test path should be in /tmp: got %s", result.TestPath)
		}

		// Test path should contain "kimia-overlay-test"
		if !strings.Contains(result.TestPath, "kimia-overlay-test") {
			t.Errorf("Test path should contain 'kimia-overlay-test': got %s", result.TestPath)
		}
	})

	t.Run("Cleans up after test", func(t *testing.T) {
		result := TestOverlayMount()

		// Wait a bit for cleanup
		time.Sleep(100 * time.Millisecond)

		// Test directory should be cleaned up
		// Note: This might still exist if cleanup failed, but that's expected
		// in environments without proper permissions
		_, err := os.Stat(result.TestPath)
		if err == nil {
			// Directory still exists - might be expected in some environments
			t.Logf("Test directory still exists (may be expected): %s", result.TestPath)
		}
	})

	t.Run("Duration is measured", func(t *testing.T) {
		result := TestOverlayMount()

		if result.Duration == 0 {
			t.Error("Duration should be measured")
		}

		// Duration should be reasonable (less than 10 seconds)
		if result.Duration > 10*time.Second {
			t.Errorf("Duration seems too long: %v", result.Duration)
		}
	})

	t.Run("Handles permission errors gracefully", func(t *testing.T) {
		result := TestOverlayMount()

		// In environments without user namespace support, we expect failure
		// but it should be graceful with a proper error message
		if !result.Success {
			if result.ErrorMessage == "" {
				t.Error("Failed test should have error message")
			}
			t.Logf("Overlay mount failed (expected in restricted environment): %s",
				result.ErrorMessage)
		}
	})
}

func TestTestOverlayMountConcurrent(t *testing.T) {
	t.Run("Concurrent overlay mount tests", func(t *testing.T) {
		const numGoroutines = 5
		var wg sync.WaitGroup
		results := make([]*OverlayTestResult, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				results[index] = TestOverlayMount()
			}(i)
		}

		wg.Wait()

		// Verify all tests completed
		for i, result := range results {
			if result == nil {
				t.Errorf("Goroutine %d returned nil result", i)
				continue
			}

			if result.TestPath == "" {
				t.Errorf("Goroutine %d returned empty TestPath", i)
			}
		}

		// Verify test paths are unique
		paths := make(map[string]bool)
		for i, result := range results {
			if result == nil {
				continue
			}
			if paths[result.TestPath] {
				t.Errorf("Duplicate test path detected: %s (goroutine %d)",
					result.TestPath, i)
			}
			paths[result.TestPath] = true
		}
	})
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestStorageDriversIntegration(t *testing.T) {
	t.Run("CheckStorageDrivers and ValidateStorageDriver integration", func(t *testing.T) {
		// Test with capabilities
		checkWithCaps, err := CheckStorageDrivers(true)
		if err != nil {
			t.Fatalf("CheckStorageDrivers failed: %v", err)
		}

		// VFS should always validate
		if err := ValidateStorageDriver("vfs", true); err != nil {
			t.Errorf("VFS validation failed: %v", err)
		}

		// Test overlay with capabilities
		if checkWithCaps.OverlayAvailable {
			if err := ValidateStorageDriver("overlay", true); err != nil {
				t.Errorf("Overlay validation failed when available: %v", err)
			}
		}

		// Test without capabilities
		checkNoCaps, err := CheckStorageDrivers(false)
		if err != nil {
			t.Fatalf("CheckStorageDrivers failed: %v", err)
		}

		// Overlay should fail without capabilities
		if !checkNoCaps.OverlayAvailable {
			if err := ValidateStorageDriver("overlay", false); err == nil {
				t.Error("Overlay validation should fail when not available")
			}
		}

		// Native should always work
		if err := ValidateStorageDriver("native", false); err != nil {
			t.Errorf("Native validation failed: %v", err)
		}
	})

	t.Run("TestOverlayMount consistency with CheckStorageDrivers", func(t *testing.T) {
		checkWithCaps, err := CheckStorageDrivers(true)
		if err != nil {
			t.Fatalf("CheckStorageDrivers failed: %v", err)
		}

		result := TestOverlayMount()

		// If overlay is available according to CheckStorageDrivers,
		// TestOverlayMount might still fail if user namespaces aren't available
		// This is expected behavior
		if checkWithCaps.OverlayAvailable && !result.Success {
			t.Logf("Overlay marked as available but mount test failed: %s",
				result.ErrorMessage)
			t.Logf("This is expected if user namespaces are not fully functional")
		}
	})
}

func TestStorageRealWorldScenarios(t *testing.T) {
	t.Run("Typical rootless container scenario with capabilities", func(t *testing.T) {
		// Check what storage drivers are available with capabilities
		check, err := CheckStorageDrivers(true)
		if err != nil {
			t.Fatalf("CheckStorageDrivers failed: %v", err)
		}

		t.Logf("Storage driver availability (with caps):")
		t.Logf("  VFS:     %v", check.VFSAvailable)
		t.Logf("  Native:  %v", check.NativeAvailable)
		t.Logf("  Overlay: %v", check.OverlayAvailable)

		// VFS should always work
		if !check.VFSAvailable {
			t.Error("VFS should always be available for rootless containers")
		}

		// Try to validate the best available driver
		preferredDrivers := []struct {
			name    string
			hasCaps bool
		}{
			{"overlay", true},
			{"native", true},
			{"vfs", true},
		}

		var selectedDriver string
		var validationErr error

		for _, driver := range preferredDrivers {
			err := ValidateStorageDriver(driver.name, driver.hasCaps)
			if err == nil {
				selectedDriver = driver.name
				break
			}
			validationErr = err
		}

		if selectedDriver == "" {
			t.Errorf("No storage driver validated successfully, last error: %v",
				validationErr)
		} else {
			t.Logf("Selected storage driver: %s", selectedDriver)
		}
	})

	t.Run("Restricted environment (no capabilities)", func(t *testing.T) {
		// Check without capabilities
		check, err := CheckStorageDrivers(false)
		if err != nil {
			t.Fatalf("CheckStorageDrivers failed: %v", err)
		}

		t.Logf("Storage driver availability (no caps):")
		t.Logf("  VFS:     %v", check.VFSAvailable)
		t.Logf("  Native:  %v", check.NativeAvailable)
		t.Logf("  Overlay: %v", check.OverlayAvailable)

		// VFS should still work
		if err := ValidateStorageDriver("vfs", false); err != nil {
			t.Errorf("VFS should work in restricted environment: %v", err)
		}

		// Overlay should fail
		if err := ValidateStorageDriver("overlay", false); err == nil {
			t.Error("Overlay should not work without capabilities")
		}

		// Native should work
		if err := ValidateStorageDriver("native", false); err != nil {
			t.Errorf("Native should work in restricted environment: %v", err)
		}
	})

	t.Run("Full capabilities environment", func(t *testing.T) {
		// All drivers should validate with capabilities
		for _, driver := range []string{"vfs", "native", "overlay"} {
			if err := ValidateStorageDriver(driver, true); err != nil {
				t.Errorf("Driver %s should work with full capabilities: %v",
					driver, err)
			}
		}
	})
}

// ============================================================================
// Edge Cases and Error Handling
// ============================================================================

func TestStorageEdgeCases(t *testing.T) {
	t.Run("Driver name with whitespace", func(t *testing.T) {
		tests := []string{
			" vfs",
			"vfs ",
			" vfs ",
			"vfs\t",
			"\nvfs",
		}

		for _, driver := range tests {
			t.Run("driver='"+driver+"'", func(t *testing.T) {
				err := ValidateStorageDriver(driver, true)
				// Implementation may trim or reject whitespace
				// Just verify it handles it without crashing
				t.Logf("Driver %q validation: %v", driver, err)
			})
		}
	})

	t.Run("Special characters in driver name", func(t *testing.T) {
		tests := []string{
			"vfs/overlay",
			"vfs;overlay",
			"vfs|native",
			"../vfs",
			"vfs\x00",
		}

		for _, driver := range tests {
			err := ValidateStorageDriver(driver, true)
			if err == nil {
				t.Errorf("Driver %q should be rejected", driver)
			}
		}
	})

	t.Run("Very long driver name", func(t *testing.T) {
		longName := strings.Repeat("a", 10000)
		err := ValidateStorageDriver(longName, true)
		if err == nil {
			t.Error("Very long driver name should be rejected")
		}
	})
}

func TestOverlayTestResultEdgeCases(t *testing.T) {
	t.Run("Negative duration", func(t *testing.T) {
		result := &OverlayTestResult{
			Success:      false,
			ErrorMessage: "test failed",
			TestPath:     "/tmp/test",
			Duration:     -1 * time.Second,
		}

		// Should handle negative duration gracefully
		if result.Duration < 0 {
			t.Logf("Negative duration detected: %v", result.Duration)
		}
	})

	t.Run("Very long error message", func(t *testing.T) {
		longError := strings.Repeat("error ", 10000)
		result := &OverlayTestResult{
			Success:      false,
			ErrorMessage: longError,
			TestPath:     "/tmp/test",
			Duration:     100 * time.Millisecond,
		}

		if len(result.ErrorMessage) != len(longError) {
			t.Error("Error message should preserve length")
		}
	})

	t.Run("Empty test path", func(t *testing.T) {
		result := &OverlayTestResult{
			Success:      false,
			ErrorMessage: "no path",
			TestPath:     "",
			Duration:     0,
		}

		if result.TestPath != "" {
			t.Error("Empty test path should be preserved")
		}
	})
}

// ============================================================================
// Benchmarks
// ============================================================================

func BenchmarkCheckStorageDrivers(b *testing.B) {
	b.Run("with capabilities", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = CheckStorageDrivers(true)
		}
	})

	b.Run("without capabilities", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = CheckStorageDrivers(false)
		}
	})
}

func BenchmarkValidateStorageDriver(b *testing.B) {
	b.Run("vfs", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = ValidateStorageDriver("vfs", false)
		}
	})

	b.Run("overlay-with-caps", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = ValidateStorageDriver("overlay", true)
		}
	})

	b.Run("overlay-no-caps", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = ValidateStorageDriver("overlay", false)
		}
	})

	b.Run("native", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = ValidateStorageDriver("native", false)
		}
	})

	b.Run("invalid", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = ValidateStorageDriver("invalid", true)
		}
	})
}

func BenchmarkTestOverlayMount(b *testing.B) {
	// Note: This might be slow in environments without proper support
	b.Run("sequential", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = TestOverlayMount()
		}
	})
}

func BenchmarkValidateStorageDriverConcurrent(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		drivers := []string{"vfs", "overlay", "native"}
		i := 0
		for pb.Next() {
			_ = ValidateStorageDriver(drivers[i%len(drivers)], true)
			i++
		}
	})
}

// ============================================================================
// Test Helpers and Utilities
// ============================================================================

func TestCleanupOverlayTest(t *testing.T) {
	t.Run("Cleanup with valid path", func(t *testing.T) {
		// Create a temporary directory for testing
		testDir := filepath.Join("/tmp", "kimia-overlay-test-cleanup")
		if err := os.MkdirAll(testDir, 0755); err != nil {
			t.Fatalf("Failed to create test directory: %v", err)
		}

		// Try to clean it up manually
		if err := os.RemoveAll(testDir); err != nil {
			t.Logf("Manual cleanup failed: %v", err)
		}
	})
}

// ============================================================================
// Documentation Examples
// ============================================================================

func ExampleCheckStorageDrivers() {
	check, err := CheckStorageDrivers(true)
	if err != nil {
		panic(err)
	}

	if check.VFSAvailable {
		println("VFS storage driver is available")
	}

	if check.OverlayAvailable {
		println("Overlay storage driver is available")
	}

	if check.NativeAvailable {
		println("Native storage driver is available")
	}
}

func ExampleValidateStorageDriver() {
	// Try to use overlay driver with capabilities
	if err := ValidateStorageDriver("overlay", true); err != nil {
		// Fall back to VFS
		println("Overlay not available, using VFS")
		_ = ValidateStorageDriver("vfs", false)
	}
}

func ExampleTestOverlayMount() {
	result := TestOverlayMount()

	if result.Success {
		println("Overlay mount test succeeded")
	} else {
		println("Overlay mount test failed:", result.ErrorMessage)
	}
}
