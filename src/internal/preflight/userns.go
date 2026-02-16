package preflight

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/rapidfort/kimia/pkg/logger"
)

// UserNamespaceCheck holds the result of user namespace validation
type UserNamespaceCheck struct {
	Supported       bool
	MaxUserNS       int
	SubuidConfigured bool
	SubgidConfigured bool
	SubuidRange     string
	SubgidRange     string
	CanCreate       bool
	ErrorMessage    string
}

// CheckUserNamespaces validates user namespace support
func CheckUserNamespaces() (*UserNamespaceCheck, error) {
	logger.Debug("Checking user namespace support")
	
	result := &UserNamespaceCheck{}
	
	// Check kernel support
	maxUserNS, err := readMaxUserNamespaces()
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("Failed to read max_user_namespaces: %v", err)
		return result, nil
	}
	
	result.MaxUserNS = maxUserNS
	result.Supported = maxUserNS > 0
	
	logger.Debug("max_user_namespaces: %d", maxUserNS)
	
	if !result.Supported {
		result.ErrorMessage = "User namespaces not enabled (max_user_namespaces is 0)"
		return result, nil
	}
	
	// Check subuid/subgid configuration
	uid := os.Getuid()
	username := os.Getenv("USER")
	if username == "" {
		username = fmt.Sprintf("%d", uid)
	}
	
	// Check /etc/subuid
	subuidRange, err := checkSubIDFile("/etc/subuid", username, uid)
	if err == nil && subuidRange != "" {
		result.SubuidConfigured = true
		result.SubuidRange = subuidRange
		logger.Debug("subuid configured: %s", subuidRange)
	} else {
		logger.Debug("subuid not configured or error: %v", err)
	}
	
	// Check /etc/subgid
	subgidRange, err := checkSubIDFile("/etc/subgid", username, uid)
	if err == nil && subgidRange != "" {
		result.SubgidConfigured = true
		result.SubgidRange = subgidRange
		logger.Debug("subgid configured: %s", subgidRange)
	} else {
		logger.Debug("subgid not configured or error: %v", err)
	}
	
	// Try to create a user namespace
	canCreate, err := testUserNamespaceCreation()
	if err != nil {
		result.CanCreate = false
		result.ErrorMessage = fmt.Sprintf("Cannot create user namespace: %v", err)
		logger.Debug("User namespace creation test failed: %v", err)
	} else {
		result.CanCreate = canCreate
		logger.Debug("User namespace creation test: %v", canCreate)
	}
	
	return result, nil
}

// readMaxUserNamespaces reads /proc/sys/user/max_user_namespaces
func readMaxUserNamespaces() (int, error) {
	data, err := os.ReadFile("/proc/sys/user/max_user_namespaces")
	if err != nil {
		return 0, err
	}
	
	value := strings.TrimSpace(string(data))
	maxNS, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid max_user_namespaces value: %s", value)
	}
	
	return maxNS, nil
}

// checkSubIDFile checks /etc/subuid or /etc/subgid for user configuration
func checkSubIDFile(filename, username string, uid int) (string, error) {
	// Validate filename is one of the expected system subid files
	if filename != "/etc/subuid" && filename != "/etc/subgid" {
		return "", fmt.Errorf("unexpected subid file: %s (expected /etc/subuid or /etc/subgid)", filename)
	}

	// #nosec G304 -- filename validated to be /etc/subuid or /etc/subgid only
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		
		parts := strings.Split(line, ":")
		if len(parts) != 3 {
			continue
		}
		
		// Check if this line matches username or UID
		if parts[0] == username || parts[0] == fmt.Sprintf("%d", uid) {
			// Found matching entry: username:start:count
			return fmt.Sprintf("%s:%s:%s", parts[0], parts[1], parts[2]), nil
		}
	}
	
	if err := scanner.Err(); err != nil {
		return "", err
	}
	
	return "", fmt.Errorf("no entry found for user %s (UID %d)", username, uid)
}

// testUserNamespaceCreation attempts to create a user namespace
func testUserNamespaceCreation() (bool, error) {
	// Use unshare command to test user namespace creation
	cmd := exec.Command("unshare", "--user", "--map-root-user", "true")
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("%v: %s", err, string(output))
	}
	
	return true, nil
}

// IsUserNamespaceReady checks if user namespaces are ready for rootless builds
func (u *UserNamespaceCheck) IsUserNamespaceReady() bool {
	return u.Supported && u.CanCreate
}

// GetIssues returns a list of user namespace issues
func (u *UserNamespaceCheck) GetIssues() []string {
	var issues []string
	
	if !u.Supported {
		issues = append(issues, "User namespaces not enabled in kernel")
	}
	
	if u.Supported && !u.SubuidConfigured {
		issues = append(issues, "/etc/subuid not configured")
	}
	
	if u.Supported && !u.SubgidConfigured {
		issues = append(issues, "/etc/subgid not configured")
	}
	
	if u.Supported && !u.CanCreate {
		issues = append(issues, fmt.Sprintf("Cannot create user namespace: %s", u.ErrorMessage))
	}
	
	return issues
}
