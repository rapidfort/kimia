package security

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// ValidateImageReference validates a container image reference
func ValidateImageReference(image string) error {
	if image == "" {
		return fmt.Errorf("empty image reference")
	}

	// Reject shell metacharacters
	if strings.ContainsAny(image, ";|&$`\n") {
		return fmt.Errorf("invalid characters in image reference")
	}

	// Basic format validation
	// [registry/]name[:tag|@digest]
	validImageRef := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]*[a-zA-Z0-9](:([a-zA-Z0-9._-]+))?(@sha256:[a-f0-9]{64})?$`)
	if !validImageRef.MatchString(image) {
		return fmt.Errorf("invalid image reference format: %s", image)
	}

	return nil
}

// ValidateFilePath validates a file path is within an expected base directory
func ValidateFilePath(path, baseDir string) error {
	if path == "" {
		return fmt.Errorf("empty path")
	}

	// Reject shell metacharacters
	if strings.ContainsAny(path, ";|&$`\n") {
		return fmt.Errorf("invalid characters in path")
	}

	// Clean the path
	cleanPath := filepath.Clean(path)

	// Get absolute paths
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %v", err)
	}

	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute base: %v", err)
	}

	// Ensure path is within base directory
	if !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) && absPath != absBase {
		return fmt.Errorf("path %s is outside base directory %s", path, baseDir)
	}

	return nil
}

// ValidateSocketPath validates a Unix socket path
func ValidateSocketPath(socketPath string) error {
	if socketPath == "" {
		return fmt.Errorf("empty socket path")
	}

	// Reject shell metacharacters
	if strings.ContainsAny(socketPath, ";|&$`\n") {
		return fmt.Errorf("invalid characters in socket path")
	}

	// Path should be absolute
	if !filepath.IsAbs(socketPath) {
		return fmt.Errorf("socket path must be absolute")
	}

	return nil
}

// ValidateCredentialHelper validates a credential helper name
func ValidateCredentialHelper(helper string) error {
	// Should be just the helper name, not a path
	if strings.Contains(helper, "/") || strings.Contains(helper, "\\") {
		return fmt.Errorf("invalid credential helper name (contains path separator)")
	}

	// Allowlist known helpers
	allowedHelpers := map[string]bool{
		"docker-credential-pass":          true,
		"docker-credential-secretservice": true,
		"docker-credential-osxkeychain":   true,
		"docker-credential-wincred":       true,
		"docker-credential-ecr-login":     true,
		"docker-credential-gcr":           true,
		"docker-credential-helper":        true,
	}

	baseName := filepath.Base(helper)
	if !allowedHelpers[baseName] {
		return fmt.Errorf("unknown credential helper: %s", baseName)
	}

	return nil
}

// ValidateDirectoryPath validates a directory path for creation
func ValidateDirectoryPath(path string) error {
	if path == "" {
		return fmt.Errorf("empty directory path")
	}

	// Reject shell metacharacters
	if strings.ContainsAny(path, ";|&$`\n") {
		return fmt.Errorf("invalid characters in directory path")
	}

	// Clean the path
	cleanPath := filepath.Clean(path)

	// Should not contain ..
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("directory path contains ..")
	}

	return nil
}
