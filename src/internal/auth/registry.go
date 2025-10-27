package auth

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/rapidfort/kimia/pkg/logger"
)

// ExtractRegistry extracts the registry from an image reference
func ExtractRegistry(imageRef string) string {
	// Remove tag if present
	if idx := strings.LastIndex(imageRef, ":"); idx > 0 && !strings.Contains(imageRef[idx:], "/") {
		imageRef = imageRef[:idx]
	}

	// Remove digest if present
	if idx := strings.Index(imageRef, "@"); idx > 0 {
		imageRef = imageRef[:idx]
	}

	// Extract registry (everything before the first /)
	if idx := strings.Index(imageRef, "/"); idx > 0 {
		registry := imageRef[:idx]
		// Check if it looks like a registry (has dots or port)
		if strings.Contains(registry, ".") || strings.Contains(registry, ":") {
			return registry
		}
	}

	// Default to docker.io for Docker Hub images
	return "docker.io"
}

// NormalizeRegistryURL normalizes registry URLs to a consistent format
func NormalizeRegistryURL(registry string) string {
	// Remove protocol prefix
	registry = strings.TrimPrefix(registry, "https://")
	registry = strings.TrimPrefix(registry, "http://")

	// Handle Docker Hub special cases
	dockerHubAliases := []string{
		"index.docker.io",
		"registry-1.docker.io",
		"registry.docker.io",
	}

	// Normalize Docker Hub URLs
	if strings.HasPrefix(registry, "index.docker.io") {
		return "docker.io"
	}

	for _, alias := range dockerHubAliases {
		if strings.Contains(registry, alias) {
			return "docker.io"
		}
	}

	// Remove /v1/ or /v2/ suffixes
	registry = strings.TrimSuffix(registry, "/v1/")
	registry = strings.TrimSuffix(registry, "/v2/")
	registry = strings.TrimSuffix(registry, "/v1")
	registry = strings.TrimSuffix(registry, "/v2")

	// Remove trailing slashes
	registry = strings.TrimSuffix(registry, "/")

	return registry
}

// IsValidRegistryURL checks if a string looks like a registry URL
func IsValidRegistryURL(s string) bool {
	// Common registry patterns
	registryPatterns := []string{
		"docker.io",
		"index.docker.io",
		"quay.io",
		"ghcr.io",
		"gcr.io",
		"localhost",
		"127.0.0.1",
	}

	for _, pattern := range registryPatterns {
		if strings.Contains(s, pattern) {
			return true
		}
	}

	// Check for URLs with ports (e.g., "myregistry:5000")
	if strings.Contains(s, ":") && !strings.HasPrefix(s, "http") {
		parts := strings.Split(s, ":")
		if len(parts) == 2 {
			// Simple port validation
			return true
		}
	}

	// Check for ECR registries
	if strings.Contains(s, ".dkr.ecr.") && strings.Contains(s, ".amazonaws.com") {
		return true
	}

	// Check for GCR/GAR registries
	if strings.Contains(s, ".gcr.io") || strings.Contains(s, "-docker.pkg.dev") {
		return true
	}

	// Check for domains with dots
	if strings.Contains(s, ".") && !strings.Contains(s, " ") {
		return true
	}

	return strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")
}

// IsECRRegistry checks if a registry is AWS ECR
func IsECRRegistry(registry string) bool {
	return strings.Contains(registry, ".dkr.ecr.") && strings.Contains(registry, ".amazonaws.com")
}

// IsGCRRegistry checks if a registry is Google GCR
func IsGCRRegistry(registry string) bool {
	return strings.HasPrefix(registry, "gcr.io/") ||
		strings.Contains(registry, ".gcr.io/") ||
		strings.HasPrefix(registry, "us.gcr.io/") ||
		strings.HasPrefix(registry, "eu.gcr.io/") ||
		strings.HasPrefix(registry, "asia.gcr.io/")
}

// IsGARRegistry checks if a registry is Google Artifact Registry
func IsGARRegistry(registry string) bool {
	return strings.Contains(registry, "-docker.pkg.dev")
}

// HasCloudRegistries checks if any destination uses cloud registries
func HasCloudRegistries(destinations []string) bool {
	for _, dest := range destinations {
		registry := ExtractRegistry(dest)
		if IsECRRegistry(registry) || IsGCRRegistry(registry) || IsGARRegistry(registry) {
			return true
		}
	}
	return false
}

// RefreshCloudCredentials attempts to refresh credentials for cloud registries
func RefreshCloudCredentials(registry string) (string, error) {
	normalizedRegistry := NormalizeRegistryURL(registry)

	// AWS ECR
	if IsECRRegistry(normalizedRegistry) {
		logger.Info("Detected AWS ECR registry, attempting to get fresh token...")
		return refreshECRCredentials(normalizedRegistry)
	}

	// Google GCR
	if IsGCRRegistry(normalizedRegistry) {
		logger.Info("Detected Google GCR registry, attempting to get fresh token...")
		return refreshGCRCredentials(normalizedRegistry)
	}

	// Google Artifact Registry
	if IsGARRegistry(normalizedRegistry) {
		logger.Info("Detected Google Artifact Registry, attempting to get fresh token...")
		return refreshGARCredentials(normalizedRegistry)
	}

	return "", fmt.Errorf("not a cloud registry")
}

// refreshECRCredentials gets fresh ECR credentials
func refreshECRCredentials(registry string) (string, error) {
	if creds, err := executeCredentialHelper("ecr-login", registry); err == nil {
		return creds, nil
	}

	return "", fmt.Errorf("unable to get ECR credentials - ensure IAM role is configured")
}

// refreshGCRCredentials gets fresh GCR credentials
func refreshGCRCredentials(registry string) (string, error) {
	if creds, err := executeCredentialHelper("gcr", registry); err == nil {
		return creds, nil
	}

	return "", fmt.Errorf("unable to get GCR credentials - ensure Workload Identity is configured")
}

// refreshGARCredentials gets fresh Google Artifact Registry credentials
func refreshGARCredentials(registry string) (string, error) {
	// GAR uses the same authentication as GCR
	return refreshGCRCredentials(registry)
}

// executeCredentialHelper executes a Docker credential helper
func executeCredentialHelper(helper string, registry string) (string, error) {
	// Only include helpers that make sense in container environments
	helperMap := map[string]string{
		"ecr-login": "docker-credential-ecr-login",
		"gcr":       "docker-credential-gcr",
	}

	// Get the actual helper executable name
	helperExe := helperMap[helper]
	if helperExe == "" {
		// For any unknown helper, try the standard naming convention
		// This allows flexibility without hardcoding everything
		helperExe = "docker-credential-" + helper
	}

	// Check if helper exists
	if _, err := exec.LookPath(helperExe); err != nil {
		return "", fmt.Errorf("credential helper not found: %s", helper)
	}

	// Execute the helper with "get" command
	cmd := exec.Command(helperExe, "get")
	cmd.Stdin = strings.NewReader(registry)

	output, err := cmd.Output()
	if err != nil {
		// Some helpers return error when no creds found, which is OK
		return "", err
	}

	// Parse the response - simplified version
	// In production, you'd properly unmarshal the JSON response
	return string(output), nil
}
