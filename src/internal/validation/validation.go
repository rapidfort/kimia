package validation

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Git reference validation patterns
var (
	// gitRefPattern matches valid git branch/tag/ref names
	// Allows: alphanumeric, dash, underscore, dot, forward slash
	// Disallows: .., consecutive slashes, leading/trailing slashes
	gitRefPattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9/_.-]*[a-zA-Z0-9])?$`)

	// OCI image name pattern (simplified)
	// Format: [registry/]repository[:tag][@digest]
	imageNamePattern = regexp.MustCompile(`^[a-z0-9]+(([._-]|__|[-]*)[a-z0-9]+)*(\/[a-z0-9]+(([._-]|__|[-]*)[a-z0-9]+)*)*$`)

	// Docker tag pattern
	tagPattern = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}$`)
)

// ValidateGitRef validates a git reference (branch, tag, or commit SHA)
// Returns error if the ref contains potentially dangerous characters
func ValidateGitRef(ref string) error {
	if ref == "" {
		return fmt.Errorf("git reference cannot be empty")
	}

	if len(ref) > 256 {
		return fmt.Errorf("git reference too long: %d characters (max 256)", len(ref))
	}

	// Check for null bytes
	if strings.Contains(ref, "\x00") {
		return fmt.Errorf("git reference contains null byte")
	}

	// Check for dangerous sequences
	if strings.Contains(ref, "..") {
		return fmt.Errorf("git reference contains '..' sequence")
	}

	// Disallow refs starting or ending with slash
	if strings.HasPrefix(ref, "/") || strings.HasSuffix(ref, "/") {
		return fmt.Errorf("git reference cannot start or end with '/'")
	}

	// Disallow consecutive slashes
	if strings.Contains(ref, "//") {
		return fmt.Errorf("git reference contains consecutive slashes")
	}

	// Validate against pattern
	if !gitRefPattern.MatchString(ref) {
		return fmt.Errorf("git reference contains invalid characters: %s", ref)
	}

	return nil
}

// ValidateImageName validates an OCI/Docker image name
// Does not validate tag or digest - use ValidateImageTag for those
func ValidateImageName(name string) error {
	if name == "" {
		return fmt.Errorf("image name cannot be empty")
	}

	if len(name) > 255 {
		return fmt.Errorf("image name too long: %d characters (max 255)", len(name))
	}

	// Check for null bytes
	if strings.Contains(name, "\x00") {
		return fmt.Errorf("image name contains null byte")
	}

	// Split on tag/digest separators
	nameOnly := name
	if idx := strings.IndexAny(name, ":@"); idx != -1 {
		nameOnly = name[:idx]
	}

	// Basic validation for image name
	if !imageNamePattern.MatchString(nameOnly) {
		return fmt.Errorf("invalid image name format: %s", nameOnly)
	}

	return nil
}

// ValidateImageTag validates a Docker image tag
func ValidateImageTag(tag string) error {
	if tag == "" {
		return fmt.Errorf("image tag cannot be empty")
	}

	if len(tag) > 128 {
		return fmt.Errorf("image tag too long: %d characters (max 128)", len(tag))
	}

	// Check for null bytes
	if strings.Contains(tag, "\x00") {
		return fmt.Errorf("image tag contains null byte")
	}

	if !tagPattern.MatchString(tag) {
		return fmt.Errorf("invalid image tag format: %s", tag)
	}

	return nil
}

// ValidatePathWithinBase ensures a path is within a base directory
// Prevents directory traversal attacks
func ValidatePathWithinBase(path, base string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	if base == "" {
		return fmt.Errorf("base path cannot be empty")
	}

	// Check for null bytes
	if strings.Contains(path, "\x00") || strings.Contains(base, "\x00") {
		return fmt.Errorf("path contains null byte")
	}

	// Clean both paths
	cleanPath := filepath.Clean(path)
	cleanBase := filepath.Clean(base)

	// Convert to absolute paths
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %v", err)
	}

	absBase, err := filepath.Abs(cleanBase)
	if err != nil {
		return fmt.Errorf("failed to get absolute base path: %v", err)
	}

	// Ensure path is within base
	rel, err := filepath.Rel(absBase, absPath)
	if err != nil {
		return fmt.Errorf("failed to compute relative path: %v", err)
	}

	// Check if relative path tries to escape base
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return fmt.Errorf("path escapes base directory: %s not within %s", path, base)
	}

	return nil
}

// ValidateSocketPath validates a Unix socket path
func ValidateSocketPath(socketPath string) error {
	if socketPath == "" {
		return fmt.Errorf("socket path cannot be empty")
	}

	// Check for null bytes
	if strings.Contains(socketPath, "\x00") {
		return fmt.Errorf("socket path contains null byte")
	}

	// Clean the path
	cleanPath := filepath.Clean(socketPath)

	// Unix socket paths have length limits (typically 108 bytes)
	if len(cleanPath) > 108 {
		return fmt.Errorf("socket path too long: %d bytes (max 108)", len(cleanPath))
	}

	// Ensure it's an absolute path
	if !filepath.IsAbs(cleanPath) {
		return fmt.Errorf("socket path must be absolute: %s", cleanPath)
	}

	// Check for path traversal attempts
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("socket path contains '..' sequence")
	}

	return nil
}

// ValidateBuildArg validates a build argument key
// Build arg values are not validated as they may contain any content
func ValidateBuildArg(key string) error {
	if key == "" {
		return fmt.Errorf("build arg key cannot be empty")
	}

	if len(key) > 128 {
		return fmt.Errorf("build arg key too long: %d characters (max 128)", len(key))
	}

	// Check for null bytes
	if strings.Contains(key, "\x00") {
		return fmt.Errorf("build arg key contains null byte")
	}

	// Build arg keys should be simple identifiers
	matched, err := regexp.MatchString(`^[A-Z_][A-Z0-9_]*$`, key)
	if err != nil {
		return fmt.Errorf("failed to validate build arg key: %v", err)
	}

	if !matched {
		return fmt.Errorf("invalid build arg key format: %s (must be uppercase with underscores)", key)
	}

	return nil
}

// SanitizeFilename removes or replaces dangerous characters from a filename
func SanitizeFilename(filename string) (string, error) {
	if filename == "" {
		return "", fmt.Errorf("filename cannot be empty")
	}

	// Check for null bytes
	if strings.Contains(filename, "\x00") {
		return "", fmt.Errorf("filename contains null byte")
	}

	// Remove path separators
	cleaned := strings.ReplaceAll(filename, "/", "_")
	cleaned = strings.ReplaceAll(cleaned, "\\", "_")

	// Remove or replace other dangerous characters
	cleaned = strings.ReplaceAll(cleaned, "..", "_")
	cleaned = strings.TrimPrefix(cleaned, ".")

	if cleaned == "" || cleaned == "." || cleaned == ".." {
		return "", fmt.Errorf("filename invalid after sanitization: %s", filename)
	}

	if len(cleaned) > 255 {
		return "", fmt.Errorf("filename too long after sanitization: %d characters", len(cleaned))
	}

	return cleaned, nil
}

// ValidateRegistryHost validates a container registry hostname
func ValidateRegistryHost(host string) error {
	if host == "" {
		return fmt.Errorf("registry host cannot be empty")
	}

	if len(host) > 253 {
		return fmt.Errorf("registry host too long: %d characters (max 253)", len(host))
	}

	// Check for null bytes
	if strings.Contains(host, "\x00") {
		return fmt.Errorf("registry host contains null byte")
	}

	// Basic hostname validation (simplified)
	// Full DNS validation is complex; this catches obvious issues
	hostPattern := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)*$`)

	// Check for port
	hostOnly := host
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		hostOnly = host[:idx]
		port := host[idx+1:]
		
		// Validate port is numeric and in valid range
		portPattern := regexp.MustCompile(`^[0-9]{1,5}$`)
		if !portPattern.MatchString(port) {
			return fmt.Errorf("invalid port in registry host: %s", port)
		}
	}

	if !hostPattern.MatchString(hostOnly) {
		return fmt.Errorf("invalid registry host format: %s", hostOnly)
	}

	return nil
}
