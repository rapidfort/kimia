package preflight

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rapidfort/kimia/internal/security"
	"github.com/rapidfort/kimia/pkg/logger"
)

// StorageCheck holds the result of storage driver validation
type StorageCheck struct {
	VFSAvailable     bool
	NativeAvailable bool
	OverlayAvailable bool
	TestResult       *OverlayTestResult
}

// OverlayTestResult holds the result of overlay mount test
type OverlayTestResult struct {
	Success      bool
	ErrorMessage string
	TestPath     string
	Duration     time.Duration
}

// CheckStorageDrivers validates available storage drivers
func CheckStorageDrivers(hasCaps bool) (*StorageCheck, error) {
	logger.Debug("Checking storage driver availability")

	result := &StorageCheck{
		VFSAvailable:    true, // VFS is always available
		NativeAvailable: true, // Native (BuildKit) is always available
	}

	if hasCaps {
		// With SETUID/SETGID caps, user namespaces work, giving mount capability
		result.OverlayAvailable = true
		logger.Debug("Overlay available (rootless mode - native kernel overlay via user namespaces)")
	} else {
		result.OverlayAvailable = false
		logger.Debug("Overlay not available (missing SETUID/SETGID capabilities)")
	}

	return result, nil
}

// TestOverlayMount performs an actual overlay mount test
// Note: In rootless mode, this must be called from within a user namespace
// (e.g., via buildah unshare or similar) to have mount capability
func TestOverlayMount() *OverlayTestResult {
	logger.Debug("Testing overlay mount capability")

	startTime := time.Now()

	// Create temporary test directory
	testBase := filepath.Join("/tmp", fmt.Sprintf("kimia-overlay-test-%d", time.Now().UnixNano()))

	result := &OverlayTestResult{
		TestPath: testBase,
	}

	// Ensure cleanup
	defer func() {
		if testBase != "" {
			cleanupOverlayTest(testBase)
		}
	}()

	// Create directory structure
	lowerDir := filepath.Join(testBase, "lower")
	upperDir := filepath.Join(testBase, "upper")
	workDir := filepath.Join(testBase, "work")
	mergedDir := filepath.Join(testBase, "merged")

	for _, dir := range []string{lowerDir, upperDir, workDir, mergedDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			result.ErrorMessage = fmt.Sprintf("Failed to create test directory: %v", err)
			result.Duration = time.Since(startTime)
			return result
		}
	}

	logger.Debug("Created test directories at: %s", testBase)

	// Create a test file in lower layer
	testFile := filepath.Join(lowerDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		result.ErrorMessage = fmt.Sprintf("Failed to create test file: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}

	// Attempt native overlay mount (works in rootless mode with user namespace)
	logger.Debug("Testing native kernel overlay mount")

	// Validate all directory paths
	if err := security.ValidateDirectoryPath(lowerDir); err != nil {
		return result
	}
	if err := security.ValidateDirectoryPath(upperDir); err != nil {
		return result
	}
	if err := security.ValidateDirectoryPath(workDir); err != nil {
		return result
	}
	if err := security.ValidateDirectoryPath(mergedDir); err != nil {
		return result
	}

	// Clean paths
	lowerDir = filepath.Clean(lowerDir)
	upperDir = filepath.Clean(upperDir)
	workDir = filepath.Clean(workDir)
	mergedDir = filepath.Clean(mergedDir)

	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerDir, upperDir, workDir)
	// #nosec G204 -- all directory paths validated and cleaned above
	cmd := exec.Command("mount", "-t", "overlay", "overlay", "-o", opts, mergedDir)

	if output, err := cmd.CombinedOutput(); err != nil {
		result.ErrorMessage = fmt.Sprintf("Native overlay mount failed: %v\nOutput: %s", err, string(output))
		result.Duration = time.Since(startTime)

		// Kimia is rootless-only, provide helpful error message
		result.ErrorMessage += "\nNote: Rootless overlay requires user namespace. Ensure SETUID/SETGID capabilities are available."
		return result
	}

	logger.Debug("Native overlay mount successful")

	// Test write to merged directory
	writeTestFile := filepath.Join(mergedDir, "write-test.txt")
	if err := os.WriteFile(writeTestFile, []byte("write test"), 0644); err != nil {
		// Try to unmount before returning error
		unmountOverlay(mergedDir)
		result.ErrorMessage = fmt.Sprintf("Write test to overlay failed: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}
	logger.Debug("Write test successful")

	// Verify file appears in upper layer
	upperTestFile := filepath.Join(upperDir, "write-test.txt")
	if _, err := os.Stat(upperTestFile); err != nil {
		unmountOverlay(mergedDir)
		result.ErrorMessage = fmt.Sprintf("File did not appear in upper layer: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}
	logger.Debug("File correctly appeared in upper layer")

	// Unmount
	if err := unmountOverlay(mergedDir); err != nil {
		result.ErrorMessage = fmt.Sprintf("Unmount failed: %v", err)
		result.Duration = time.Since(startTime)
		return result
	}
	logger.Debug("Unmount successful")

	result.Success = true
	result.Duration = time.Since(startTime)
	logger.Debug("Overlay mount test completed successfully in %v", result.Duration)

	return result
}

// unmountOverlay unmounts an overlay filesystem
func unmountOverlay(mountPoint string) error {
	// Use umount for native kernel overlay in rootless mode
	cmd := exec.Command("umount", mountPoint)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("umount failed: %v\nOutput: %s", err, string(output))
	}
	return nil
}

// cleanupOverlayTest removes test directories
func cleanupOverlayTest(testBase string) {
	logger.Debug("Cleaning up overlay test directory: %s", testBase)

	// Try to unmount merged directory if it exists
	mergedDir := filepath.Join(testBase, "merged")
	if _, err := os.Stat(mergedDir); err == nil {
		// Attempt unmount, ignore errors (might not be mounted)
		unmountOverlay(mergedDir)
	}

	// Remove test directory
	if err := os.RemoveAll(testBase); err != nil {
		logger.Debug("Failed to cleanup test directory: %v", err)
	} else {
		logger.Debug("Test directory cleaned up successfully")
	}
}

// ValidateStorageDriver validates if the requested storage driver is available
// Kimia is rootless-only, so this function assumes non-root user
func ValidateStorageDriver(driver string, hasCaps bool) error {
	driver = strings.ToLower(driver)

	logger.Debug("Validating storage driver: %s", driver)

	switch driver {
	case "vfs":
		// VFS is always available
		logger.Debug("VFS storage driver selected - always available")
		return nil

	case "overlay":
		// Check overlay requirements
		check, err := CheckStorageDrivers(hasCaps)
		if err != nil {
			return fmt.Errorf("failed to check storage drivers: %v", err)
		}

		if !check.OverlayAvailable {
			if !hasCaps {
				return fmt.Errorf("overlay driver not available: missing SETUID/SETGID capabilities for user namespaces")
			}
			return fmt.Errorf("overlay driver not available: unknown reason")
		}

		// Note: We skip the actual mount test here because:
		// 1. In Docker/K8s, the build environment will have proper namespaces set up
		// 2. The test would need to be run inside a user namespace (buildah unshare)
		// 3. BuildKit/Buildah handle this internally when they start
		logger.Info("Overlay storage driver validated (native kernel overlay via user namespaces)")
		return nil

	case "native":
		// Native is BuildKit-specific, treated same as overlay
		logger.Debug("Native storage driver (BuildKit native snapshotter)")
		return nil

	default:
		return fmt.Errorf("unknown storage driver: %s (valid options: vfs, overlay, native)", driver)
	}
}