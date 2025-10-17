
package preflight

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rapidfort/smithy/pkg/logger"
)

// StorageCheck holds the result of storage driver validation
type StorageCheck struct {
	VFSAvailable     bool
	OverlayAvailable bool
	FuseAvailable    bool
	FuseOverlayFS    string
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
func CheckStorageDrivers(isRoot bool, hasCaps bool) (*StorageCheck, error) {
	logger.Debug("Checking storage driver availability")

	result := &StorageCheck{
		VFSAvailable: true, // VFS is always available
	}

	// Check for /dev/fuse
	if _, err := os.Stat("/dev/fuse"); err == nil {
		result.FuseAvailable = true
		logger.Debug("/dev/fuse is available")
	} else {
		logger.Debug("/dev/fuse is not available: %v", err)
	}

	// Check for fuse-overlayfs binary
	if path, err := exec.LookPath("fuse-overlayfs"); err == nil {
		result.FuseOverlayFS = path
		logger.Debug("fuse-overlayfs found at: %s", path)
	} else {
		logger.Debug("fuse-overlayfs not found in PATH")
	}

	// Determine if overlay is potentially available
	if isRoot {
		// Root can use native overlay
		result.OverlayAvailable = true
		logger.Debug("Overlay available (root mode - native overlay)")
	} else if hasCaps && result.FuseAvailable && result.FuseOverlayFS != "" {
		// Non-root with caps needs fuse-overlayfs
		result.OverlayAvailable = true
		logger.Debug("Overlay potentially available (rootless mode - fuse-overlayfs)")
	} else {
		result.OverlayAvailable = false
		logger.Debug("Overlay not available (missing requirements)")
	}

	return result, nil
}

// TestOverlayMount performs an actual overlay mount test
func TestOverlayMount(isRoot bool) *OverlayTestResult {
	logger.Debug("Testing overlay mount capability")
	
	startTime := time.Now()
	
	// Create temporary test directory
	testBase := filepath.Join("/tmp", fmt.Sprintf("smithy-overlay-test-%d", time.Now().UnixNano()))
	
	result := &OverlayTestResult{
		TestPath: testBase,
	}
	
	// Ensure cleanup
	defer func() {
		if testBase != "" {
			cleanupOverlayTest(testBase, isRoot)
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
	
	// Attempt mount
	var cmd *exec.Cmd
	var mountSuccess bool
	
	if isRoot {
		// Try native overlay mount
		logger.Debug("Testing native overlay mount (root mode)")
		opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerDir, upperDir, workDir)
		cmd = exec.Command("mount", "-t", "overlay", "overlay", "-o", opts, mergedDir)
		
		if output, err := cmd.CombinedOutput(); err != nil {
			result.ErrorMessage = fmt.Sprintf("Native overlay mount failed: %v\nOutput: %s", err, string(output))
			result.Duration = time.Since(startTime)
			return result
		}
		mountSuccess = true
		logger.Debug("Native overlay mount successful")
	} else {
		// Try fuse-overlayfs
		logger.Debug("Testing fuse-overlayfs mount (rootless mode)")
		opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerDir, upperDir, workDir)
		cmd = exec.Command("fuse-overlayfs", "-o", opts, mergedDir)
		
		if output, err := cmd.CombinedOutput(); err != nil {
			result.ErrorMessage = fmt.Sprintf("fuse-overlayfs mount failed: %v\nOutput: %s", err, string(output))
			result.Duration = time.Since(startTime)
			return result
		}
		mountSuccess = true
		logger.Debug("fuse-overlayfs mount successful")
	}
	
	// Test write to merged directory
	if mountSuccess {
		writeTestFile := filepath.Join(mergedDir, "write-test.txt")
		if err := os.WriteFile(writeTestFile, []byte("write test"), 0644); err != nil {
			// Try to unmount before returning error
			unmountOverlay(mergedDir, isRoot)
			result.ErrorMessage = fmt.Sprintf("Write test to overlay failed: %v", err)
			result.Duration = time.Since(startTime)
			return result
		}
		logger.Debug("Write test successful")
		
		// Verify file appears in upper layer
		upperTestFile := filepath.Join(upperDir, "write-test.txt")
		if _, err := os.Stat(upperTestFile); err != nil {
			unmountOverlay(mergedDir, isRoot)
			result.ErrorMessage = fmt.Sprintf("File did not appear in upper layer: %v", err)
			result.Duration = time.Since(startTime)
			return result
		}
		logger.Debug("File correctly appeared in upper layer")
		
		// Unmount
		if err := unmountOverlay(mergedDir, isRoot); err != nil {
			result.ErrorMessage = fmt.Sprintf("Unmount failed: %v", err)
			result.Duration = time.Since(startTime)
			return result
		}
		logger.Debug("Unmount successful")
	}
	
	result.Success = true
	result.Duration = time.Since(startTime)
	logger.Debug("Overlay mount test completed successfully in %v", result.Duration)
	
	return result
}

// unmountOverlay unmounts an overlay filesystem
func unmountOverlay(mountPoint string, isRoot bool) error {
	if isRoot {
		// Use umount for native overlay
		cmd := exec.Command("umount", mountPoint)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("umount failed: %v\nOutput: %s", err, string(output))
		}
	} else {
		// fuse-overlayfs should be unmounted with fusermount
		cmd := exec.Command("fusermount", "-u", mountPoint)
		if output, err := cmd.CombinedOutput(); err != nil {
			// Try fusermount3 as fallback
			cmd = exec.Command("fusermount3", "-u", mountPoint)
			if output2, err2 := cmd.CombinedOutput(); err2 != nil {
				return fmt.Errorf("fusermount failed: %v\nOutput: %s\nfusermount3 also failed: %v\nOutput: %s", 
					err, string(output), err2, string(output2))
			}
		}
	}
	return nil
}

// cleanupOverlayTest removes test directories
func cleanupOverlayTest(testBase string, isRoot bool) {
	logger.Debug("Cleaning up overlay test directory: %s", testBase)
	
	// Try to unmount merged directory if it exists
	mergedDir := filepath.Join(testBase, "merged")
	if _, err := os.Stat(mergedDir); err == nil {
		// Attempt unmount, ignore errors (might not be mounted)
		unmountOverlay(mergedDir, isRoot)
	}
	
	// Remove test directory
	if err := os.RemoveAll(testBase); err != nil {
		logger.Debug("Failed to cleanup test directory: %v", err)
	} else {
		logger.Debug("Test directory cleaned up successfully")
	}
}

// ValidateStorageDriver validates if the requested storage driver is available
func ValidateStorageDriver(driver string, isRoot bool, hasCaps bool) error {
	driver = strings.ToLower(driver)
	
	logger.Debug("Validating storage driver: %s", driver)
	
	switch driver {
	case "vfs":
		// VFS is always available
		return nil
		
	case "overlay":
		// Check overlay requirements
		check, err := CheckStorageDrivers(isRoot, hasCaps)
		if err != nil {
			return fmt.Errorf("failed to check storage drivers: %v", err)
		}
		
		if !check.OverlayAvailable {
			var reasons []string
			
			if !isRoot && !hasCaps {
				reasons = append(reasons, "missing SETUID/SETGID capabilities")
			}
			if !check.FuseAvailable {
				reasons = append(reasons, "/dev/fuse not available")
			}
			if check.FuseOverlayFS == "" && !isRoot {
				reasons = append(reasons, "fuse-overlayfs not installed")
			}
			
			return fmt.Errorf("overlay driver not available: %s", strings.Join(reasons, ", "))
		}
		
		// Perform actual mount test
		logger.Info("Testing overlay mount capability...")
		testResult := TestOverlayMount(isRoot)
		
		if !testResult.Success {
			return fmt.Errorf("overlay mount test failed: %s", testResult.ErrorMessage)
		}
		
		logger.Info("Overlay mount test successful (took %v)", testResult.Duration)
		return nil
		
	default:
		return fmt.Errorf("unknown storage driver: %s (valid options: vfs, overlay)", driver)
	}
}
