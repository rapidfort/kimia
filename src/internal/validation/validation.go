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
// Registry hostnames are case-insensitive (DNS); repository paths must be lowercase (OCI spec)
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

	// Strip tag/digest to get the name-only portion, but only treat ':'
	// as a tag separator if it comes after the last '/' (not a registry:port).
	nameOnly := name
	slashIdx := strings.LastIndex(name, "/")
	if atIdx := strings.Index(name, "@"); atIdx != -1 {
		nameOnly = name[:atIdx]
	} else if colonIdx := strings.Index(name, ":"); colonIdx != -1 {
		if slashIdx == -1 || colonIdx > slashIdx {
			nameOnly = name[:colonIdx]
		}
	}

	// Split into registry host and repository path.
	// A registry host is present if the first component contains a '.' or ':'
	// (hostname/IP) or is "localhost" — matching Docker's own heuristic.
	var registryHost, repoPath string
	firstSlash := strings.Index(nameOnly, "/")
	if firstSlash == -1 {
		// No slash: entire string is the repository name (e.g. "ubuntu")
		repoPath = nameOnly
	} else {
		first := nameOnly[:firstSlash]
		if strings.ContainsAny(first, ".:") || first == "localhost" {
			// First component looks like a registry host
			registryHost = first
			repoPath = nameOnly[firstSlash+1:]
		} else {
			// No registry host (e.g. "library/ubuntu")
			repoPath = nameOnly
		}
	}

	// Validate registry host if present (case-insensitive DNS name, optional port)
	if registryHost != "" {
		if err := ValidateRegistryHost(registryHost); err != nil {
			return fmt.Errorf("invalid registry host in image name: %v", err)
		}
	}

	// Validate repository path — must be lowercase per OCI spec
	if !imageNamePattern.MatchString(repoPath) {
		return fmt.Errorf("invalid image name format: %s", repoPath)
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

// ValidateBuildctlArg validates individual buildctl arguments to prevent injection
// This is a general-purpose validator for any buildctl argument
func ValidateBuildctlArg(arg string) error {
	// Check for null bytes (path traversal/injection vector)
	if strings.Contains(arg, "\x00") {
		return fmt.Errorf("argument contains null bytes")
	}

	// Check for shell metacharacters that could enable command injection
	// These should never appear in legitimate buildctl arguments
	dangerousChars := []string{";", "&", "|", "`", "$", "(", ")", "<", ">", "\n", "\r"}
	for _, char := range dangerousChars {
		if strings.Contains(arg, char) {
			return fmt.Errorf("argument contains dangerous character: %s", char)
		}
	}

	return nil
}

// ValidateBuildArgKeyValue validates build argument in key=value format
// Validates both the key and checks the value for dangerous characters
func ValidateBuildArgKeyValue(buildArg string) error {
	// Must be in key=value format
	if !strings.Contains(buildArg, "=") {
		return fmt.Errorf("build arg must be in key=value format")
	}

	parts := strings.SplitN(buildArg, "=", 2)
	key := parts[0]
	value := parts[1]

	// Validate key using existing function
	if err := ValidateBuildArg(key); err != nil {
		return fmt.Errorf("invalid build arg key: %v", err)
	}

	// Validate value for dangerous characters
	if err := ValidateBuildctlArg(value); err != nil {
		return fmt.Errorf("invalid build arg value for key %s: %v", key, err)
	}

	return nil
}

// ValidateExportType validates buildctl export type
func ValidateExportType(exportType string) error {
	// Allowlist of valid export types for buildctl
	validTypes := map[string]bool{
		"image":    true,
		"oci":      true,
		"docker":   true,
		"local":    true,
		"tar":      true,
		"registry": true,
	}

	if !validTypes[exportType] {
		return fmt.Errorf("invalid export type: %s (must be one of: image, oci, docker, local, tar, registry)", exportType)
	}

	return nil
}

// ValidateOutputPath validates output paths for export operations
// More permissive than ValidatePathWithinBase since it doesn't require a base
func ValidateOutputPath(path string) error {
	if path == "" {
		return fmt.Errorf("output path cannot be empty")
	}

	// Check for null bytes
	if strings.Contains(path, "\x00") {
		return fmt.Errorf("path contains null byte")
	}

	// Clean the path
	cleanPath := filepath.Clean(path)

	// Check for path traversal attempts in the original path
	if strings.Contains(path, "..") {
		return fmt.Errorf("path contains directory traversal: %s", path)
	}

	// Validate base argument rules
	if err := ValidateBuildctlArg(cleanPath); err != nil {
		return fmt.Errorf("invalid output path: %v", err)
	}

	return nil
}

// ValidatePlatform validates target platform strings for multi-arch builds
func ValidatePlatform(platform string) error {
	if platform == "" {
		return fmt.Errorf("platform cannot be empty")
	}

	// Format: os[/arch[/variant]]
	// Examples: linux/amd64, linux/arm64, linux/arm/v7

	if err := ValidateBuildctlArg(platform); err != nil {
		return fmt.Errorf("invalid platform: %v", err)
	}

	// Must contain at least os/arch
	parts := strings.Split(platform, "/")
	if len(parts) < 2 || len(parts) > 3 {
		return fmt.Errorf("platform must be in format os/arch[/variant], got: %s", platform)
	}

	// Validate OS (allowlist)
	validOS := map[string]bool{
		"linux":   true,
		"darwin":  true,
		"windows": true,
		"freebsd": true,
		"netbsd":  true,
		"openbsd": true,
		"solaris": true,
		"aix":     true,
	}
	if !validOS[parts[0]] {
		return fmt.Errorf("invalid OS in platform: %s", parts[0])
	}

	// Validate architecture (allowlist)
	validArch := map[string]bool{
		"amd64":    true,
		"arm64":    true,
		"arm":      true,
		"386":      true,
		"ppc64le":  true,
		"ppc64":    true,
		"s390x":    true,
		"mips64le": true,
		"mips64":   true,
		"riscv64":  true,
	}
	if !validArch[parts[1]] {
		return fmt.Errorf("invalid architecture in platform: %s", parts[1])
	}

	// Variant validation if present
	if len(parts) == 3 {
		variantPattern := regexp.MustCompile(`^v[0-9]+$`)
		if !variantPattern.MatchString(parts[2]) {
			return fmt.Errorf("invalid variant in platform: %s (must be v<number>)", parts[2])
		}
	}

	return nil
}

// ValidateCachePath validates cache directory paths
// Alias for ValidateOutputPath for semantic clarity
func ValidateCachePath(path string) error {
	return ValidateOutputPath(path)
}

// ValidateSecretID validates secret identifiers used in buildctl
func ValidateSecretID(secretID string) error {
	if secretID == "" {
		return fmt.Errorf("secret ID cannot be empty")
	}

	if len(secretID) > 128 {
		return fmt.Errorf("secret ID too long: %d characters (max 128)", len(secretID))
	}

	// Check for null bytes
	if strings.Contains(secretID, "\x00") {
		return fmt.Errorf("secret ID contains null byte")
	}

	// Secret IDs should be simple alphanumeric identifiers
	pattern := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)
	if !pattern.MatchString(secretID) {
		return fmt.Errorf("invalid secret ID: %s (must start with letter, contain only alphanumeric/underscore/hyphen)", secretID)
	}

	return nil
}

// ValidateSSHAgentSocket validates SSH agent socket paths
// More strict than ValidateSocketPath - must be absolute and validated
func ValidateSSHAgentSocket(socketPath string) error {
	// First use the general socket validation
	if err := ValidateSocketPath(socketPath); err != nil {
		return err
	}

	// Additional validation specific to SSH agent sockets
	// SSH agent sockets are typically in /tmp or user runtime dirs
	validPrefixes := []string{
		"/tmp/",
		"/var/run/",
		"/run/",
	}

	// Check if path starts with a valid prefix or is in user home
	hasValidPrefix := false
	for _, prefix := range validPrefixes {
		if strings.HasPrefix(socketPath, prefix) {
			hasValidPrefix = true
			break
		}
	}

	// Also allow paths in home directory runtime dirs
	if strings.Contains(socketPath, "/.local/") || strings.Contains(socketPath, "/run/user/") {
		hasValidPrefix = true
	}

	if !hasValidPrefix {
		return fmt.Errorf("SSH agent socket not in expected location: %s", socketPath)
	}

	return nil
}

// ValidateGitURL validates a git URL for buildctl context
// Supports https://, git://, ssh:// and git@ formats
func ValidateGitURL(url string) error {
	if url == "" {
		return fmt.Errorf("git URL cannot be empty")
	}

	if len(url) > 2048 {
		return fmt.Errorf("git URL too long: %d characters (max 2048)", len(url))
	}

	// Check for null bytes
	if strings.Contains(url, "\x00") {
		return fmt.Errorf("git URL contains null byte")
	}

	// Check for dangerous characters
	if err := ValidateBuildctlArg(url); err != nil {
		return fmt.Errorf("invalid git URL: %v", err)
	}

	// Must start with valid protocol or git@ format
	validPrefixes := []string{
		"https://",
		"http://",
		"git://",
		"ssh://",
		"git@",
	}

	hasValidPrefix := false
	for _, prefix := range validPrefixes {
		if strings.HasPrefix(url, prefix) {
			hasValidPrefix = true
			break
		}
	}

	if !hasValidPrefix {
		return fmt.Errorf("git URL must start with https://, http://, git://, ssh://, or git@")
	}

	return nil
}

// ValidateLabelKeyValue validates a label in key=value format
// Similar to build args but with different key requirements
func ValidateLabelKeyValue(label string) error {
	// Must be in key=value format
	if !strings.Contains(label, "=") {
		return fmt.Errorf("label must be in key=value format")
	}

	parts := strings.SplitN(label, "=", 2)
	key := parts[0]
	value := parts[1]

	// Check for null bytes
	if strings.Contains(key, "\x00") || strings.Contains(value, "\x00") {
		return fmt.Errorf("label contains null byte")
	}

	// Label keys can contain dots, slashes (for namespacing)
	// Format: [prefix/]name where prefix is often a reverse domain
	labelPattern := regexp.MustCompile(`^[a-z0-9]([a-z0-9._/-]*[a-z0-9])?$`)
	if !labelPattern.MatchString(key) {
		return fmt.Errorf("invalid label key format: %s", key)
	}

	// Validate value for dangerous characters
	if err := ValidateBuildctlArg(value); err != nil {
		return fmt.Errorf("invalid label value for key %s: %v", key, err)
	}

	return nil
}

// ValidateImageReference validates a complete image reference
// Format: [registry[:port]/][namespace/]repository[:tag][@digest]
func ValidateImageReference(ref string) error {
	if ref == "" {
		return fmt.Errorf("image reference cannot be empty")
	}

	if len(ref) > 512 {
		return fmt.Errorf("image reference too long: %d characters (max 512)", len(ref))
	}

	// Check for null bytes
	if strings.Contains(ref, "\x00") {
		return fmt.Errorf("image reference contains null byte")
	}

	// Check for dangerous characters
	if err := ValidateBuildctlArg(ref); err != nil {
		return fmt.Errorf("invalid image reference: %v", err)
	}

	// Find digest separator
	digestIdx := strings.Index(ref, "@")

	// Find the TAG colon: it must come after the last '/' to distinguish
	// it from a registry:port colon.
	// e.g. "10.228.98.157:5000/repo/image:latest"
	//       ↑ port colon (ignored)           ↑ tag colon (used)
	tagColonIdx := -1
	slashIdx := strings.LastIndex(ref, "/")
	searchFrom := slashIdx + 1 // search only within the final path component
	if relIdx := strings.Index(ref[searchFrom:], ":"); relIdx != -1 {
		tagColonIdx = searchFrom + relIdx
	}

	// Determine where the name portion ends
	nameEndIdx := len(ref)
	if digestIdx != -1 {
		nameEndIdx = digestIdx
	} else if tagColonIdx != -1 {
		nameEndIdx = tagColonIdx
	}

	name := ref[:nameEndIdx]

	// Validate the name portion (registry:port + path, no tag/digest)
	if err := ValidateImageName(name); err != nil {
		return err
	}

	// Validate tag if present (and not superseded by a digest)
	if tagColonIdx != -1 && digestIdx == -1 {
		tag := ref[tagColonIdx+1:]
		if err := ValidateImageTag(tag); err != nil {
			return err
		}
	} else if tagColonIdx != -1 && digestIdx != -1 && tagColonIdx < digestIdx {
		// tag present alongside digest: repo:tag@sha256:...
		tag := ref[tagColonIdx+1 : digestIdx]
		if err := ValidateImageTag(tag); err != nil {
			return err
		}
	}

	// Validate digest if present
	if digestIdx != -1 {
		digest := ref[digestIdx+1:]
		digestPattern := regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
		if !digestPattern.MatchString(digest) {
			return fmt.Errorf("invalid digest format: %s", digest)
		}
	}

	return nil
}
