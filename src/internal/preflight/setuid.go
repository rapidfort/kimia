package preflight

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/rapidfort/smithy/pkg/logger"
)

// SetuidBinaryCheck holds the result of SETUID binary validation
type SetuidBinaryCheck struct {
	NewuidmapPresent bool
	NewgidmapPresent bool
	NewuidmapSetuid  bool
	NewgidmapSetuid  bool
	NewuidmapPath    string
	NewgidmapPath    string
	BothAvailable    bool
}

// CheckSetuidBinaries checks if newuidmap/newgidmap exist with SETUID bit
func CheckSetuidBinaries() (*SetuidBinaryCheck, error) {
	logger.Debug("Checking SETUID binaries (newuidmap/newgidmap)")

	result := &SetuidBinaryCheck{}

	// Common paths for newuidmap/newgidmap
	paths := []string{
		"/usr/bin/newuidmap",
		"/bin/newuidmap",
		"/usr/local/bin/newuidmap",
	}

	// Check newuidmap
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil {
			result.NewuidmapPresent = true
			result.NewuidmapPath = path

			// Check if SETUID bit is set (mode & 04000)
			if info.Mode()&os.ModeSetuid != 0 {
				result.NewuidmapSetuid = true
				logger.Debug("newuidmap found with SETUID bit: %s (mode: %04o)",
					path, info.Mode().Perm())
			} else {
				logger.Debug("newuidmap found but missing SETUID bit: %s (mode: %04o)",
					path, info.Mode().Perm())
			}
			break
		}
	}

	// Check newgidmap
	paths = []string{
		"/usr/bin/newgidmap",
		"/bin/newgidmap",
		"/usr/local/bin/newgidmap",
	}

	for _, path := range paths {
		if info, err := os.Stat(path); err == nil {
			result.NewgidmapPresent = true
			result.NewgidmapPath = path

			// Check if SETUID bit is set
			if info.Mode()&os.ModeSetuid != 0 {
				result.NewgidmapSetuid = true
				logger.Debug("newgidmap found with SETUID bit: %s (mode: %04o)",
					path, info.Mode().Perm())
			} else {
				logger.Debug("newgidmap found but missing SETUID bit: %s (mode: %04o)",
					path, info.Mode().Perm())
			}
			break
		}
	}

	// Both must be available with SETUID bit
	// FIX: Check NewgidmapSetuid, not NewuidmapSetuid twice!
	result.BothAvailable = result.NewuidmapPresent &&
		result.NewgidmapPresent &&
		result.NewuidmapSetuid &&
		result.NewgidmapSetuid // ✅ FIXED: was NewuidmapSetuid

	if result.BothAvailable {
		logger.Debug("SETUID binaries available and properly configured")
	} else {
		logger.Debug("SETUID binaries not fully available or missing SETUID bit")
	}

	return result, nil
}

// HasSetuidBinaries returns true if both binaries exist with SETUID bit
func (s *SetuidBinaryCheck) HasSetuidBinaries() bool {
	return s.BothAvailable
}

// GetIssues returns a list of issues with SETUID binaries
func (s *SetuidBinaryCheck) GetIssues() []string {
	var issues []string

	if !s.NewuidmapPresent {
		issues = append(issues, "newuidmap binary not found")
	} else if !s.NewuidmapSetuid {
		issues = append(issues, fmt.Sprintf("newuidmap missing SETUID bit: %s", s.NewuidmapPath))
	}

	if !s.NewgidmapPresent {
		issues = append(issues, "newgidmap binary not found")
	} else if !s.NewgidmapSetuid {
		issues = append(issues, fmt.Sprintf("newgidmap missing SETUID bit: %s", s.NewgidmapPath))
	}

	return issues
}

// IsInKubernetes detects if running inside Kubernetes
func IsInKubernetes() bool {
	// Kubernetes sets this environment variable for all pods
	return os.Getenv("KUBERNETES_SERVICE_HOST") != ""
}

// CanSetuidBinariesWork checks if SETUID binaries can actually work
// In Kubernetes, this requires allowPrivilegeEscalation: true
// In Docker, this requires seccomp/apparmor=unconfined
func CanSetuidBinariesWork() bool {
	// First check if no_new_privs is set
	// When allowPrivilegeEscalation: false, NoNewPrivs = 1
	file, err := os.Open("/proc/self/status")
	if err != nil {
		logger.Debug("Cannot read /proc/self/status: %v", err)
		return false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 11 && line[:11] == "NoNewPrivs:" {
			value := strings.TrimSpace(line[11:])
			if value == "1" {
				logger.Debug("NoNewPrivs is set (allowPrivilegeEscalation: false)")
				return false
			}
			break
		}
	}

	// NoNewPrivs is not set, but we still need to test if we can actually
	// create a user namespace. This will fail if seccomp blocks the syscalls.
	logger.Debug("NoNewPrivs is not set, testing user namespace creation...")

	canCreate, err := testUserNamespaceCreation()
	if err != nil {
		logger.Debug("User namespace creation test failed: %v", err)
		return false
	}

	if canCreate {
		logger.Debug("User namespace creation successful - SETUID binaries can work")
		return true
	}

	logger.Debug("User namespace creation failed - SETUID binaries blocked by seccomp/apparmor")
	return false
}
