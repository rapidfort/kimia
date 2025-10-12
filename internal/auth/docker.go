package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rapidfort/smithy/pkg/logger"
)

// DockerConfig represents Docker's config.json structure
type DockerConfig struct {
	Auths map[string]DockerAuth `json:"auths"`
}

// DockerAuth represents auth entry in Docker config
type DockerAuth struct {
	Auth string `json:"auth,omitempty"`
}

// AuthFormat represents the detected format of auth file
type AuthFormat int

const (
	FormatUnknown AuthFormat = iota
	FormatDockerConfig
	FormatBuildahAuth
)

// AuthFileContent represents a generic auth file structure
type AuthFileContent struct {
	Auths        map[string]DockerAuth `json:"auths,omitempty"`
	CredHelpers  map[string]string     `json:"credHelpers,omitempty"`
	CredsStore   string                `json:"credsStore,omitempty"`
	HttpHeaders  map[string]string     `json:"HttpHeaders,omitempty"`
	Experimental string                `json:"experimental,omitempty"`
}

// CredentialHelperResponse represents the response from a credential helper
type CredentialHelperResponse struct {
	ServerURL string `json:"ServerURL"`
	Username  string `json:"Username"`
	Secret    string `json:"Secret"`
}

// SetupConfig holds configuration for authentication setup
type SetupConfig struct {
	Destinations     []string
	InsecureRegistry []string
}

// Setup converts Docker config to Buildah auth format (ENHANCED VERSION)
func Setup(config SetupConfig) (string, error) {
	logger.Debug("Setting up authentication...")

	// DOCKER_CONFIG is the primary source (Kaniko standard)
	dockerConfigDir := os.Getenv("DOCKER_CONFIG")
	if dockerConfigDir == "" {
		dockerConfigDir = "/home/smithy/.docker"
	}

	// Build list of paths to check - prioritize DOCKER_CONFIG
	authConfigPaths := []string{
		filepath.Join(dockerConfigDir, "config.json"), // Primary: DOCKER_CONFIG/config.json
		filepath.Join(dockerConfigDir, "auth.json"),   // Secondary: DOCKER_CONFIG/auth.json
		"/kaniko/.docker/config.json",                 // Kaniko default location
		"/home/smithy/.docker/config.json",            // Smithy default location
		"/home/smithy/.docker/auth.json",
		os.Getenv("HOME") + "/.docker/config.json",
		os.Getenv("HOME") + "/.docker/auth.json",
		"/.docker/config.json",
		"/.docker/auth.json",
		"/run/secrets/config.json",
		"/run/secrets/auth.json",
		"/workspace/.docker/config.json", // Common CI/CD location
	}

	// Also check REGISTRY_AUTH_FILE environment variable (Buildah standard)
	if authFile := os.Getenv("REGISTRY_AUTH_FILE"); authFile != "" {
		authConfigPaths = append([]string{authFile}, authConfigPaths...)
	}

	var configPath string
	var configData []byte
	var configFormat AuthFormat
	var hasCredHelpers bool

	for _, path := range authConfigPaths {
		if path == "/config.json" || path == "/auth.json" || path == "" {
			continue // Skip invalid paths
		}
		if data, err := os.ReadFile(path); err == nil {
			format := detectAuthFormat(data)
			if format != FormatUnknown {
				configPath = path
				configData = data
				configFormat = format

				// Check if config has credential helpers
				var checkConfig AuthFileContent
				if json.Unmarshal(data, &checkConfig) == nil {
					if checkConfig.CredHelpers != nil || checkConfig.CredsStore != "" {
						hasCredHelpers = true
						logger.Debug("Found credential helpers in config")
					}
				}

				logger.Debug("Found auth config at: %s (format: %v, has helpers: %v)", path, format, hasCredHelpers)
				break
			}
		}
	}

	// Try to get auth from various sources
	var auths map[string]DockerAuth

	// 1. Handle credential helpers if present
	if hasCredHelpers && configFormat == FormatDockerConfig {
		logger.Info("Executing Docker credential helpers...")
		helperAuths, err := detectAndHandleCredentialHelpers(configData, config.Destinations)
		if err != nil {
			logger.Warning("Failed to handle credential helpers: %v", err)
		} else {
			auths = helperAuths
		}
	}

	// 2. Try cloud-specific auth for destination registries
	if len(auths) == 0 || HasCloudRegistries(config.Destinations) {
		for _, dest := range config.Destinations {
			registry := ExtractRegistry(dest)
			normalizedRegistry := NormalizeRegistryURL(registry)

			// Skip if we already have auth for this registry
			if auths != nil {
				if existingAuth, exists := auths[normalizedRegistry]; exists && existingAuth.Auth != "" {
					continue
				}
			}

			if creds, err := RefreshCloudCredentials(registry); err == nil {
				if auths == nil {
					auths = make(map[string]DockerAuth)
				}
				auths[normalizedRegistry] = DockerAuth{Auth: creds}
				logger.Info("Successfully obtained cloud credentials for %s", normalizedRegistry)
			}
		}
	}

	// 3. If we have config data but no helpers, parse existing auth
	if configPath != "" && len(auths) == 0 {
		authData, err := convertToAuthJSON(configData, configFormat)
		if err == nil {
			var authCheck map[string]map[string]DockerAuth
			if err := json.Unmarshal(authData, &authCheck); err == nil {
				if existingAuths, ok := authCheck["auths"]; ok && len(existingAuths) > 0 {
					auths = existingAuths
				}
			}
		}
	}

	// 4. If still no auth, check environment variables
	if len(auths) == 0 {
		dockerUsername := os.Getenv("DOCKER_USERNAME")
		dockerPassword := os.Getenv("DOCKER_PASSWORD")
		dockerRegistry := os.Getenv("DOCKER_REGISTRY")

		if dockerUsername != "" && dockerPassword != "" {
			authString := base64.StdEncoding.EncodeToString([]byte(dockerUsername + ":" + dockerPassword))
			auths = make(map[string]DockerAuth)

			if dockerRegistry != "" {
				normalizedRegistry := NormalizeRegistryURL(dockerRegistry)
				auths[normalizedRegistry] = DockerAuth{Auth: authString}
				logger.Debug("Added auth from environment for registry: %s", normalizedRegistry)
			} else {
				// Add auth for common registries
				registries := []string{"docker.io", "quay.io", "ghcr.io"}
				for _, reg := range registries {
					auths[reg] = DockerAuth{Auth: authString}
				}
			}
		}
	}

	// 5. Add empty entries for destination registries (for insecure registries)
	auths = addDestinationCredentials(auths, config.Destinations)

	// 6. Add empty entries for explicitly insecure registries
	for _, reg := range config.InsecureRegistry {
		normalizedReg := NormalizeRegistryURL(reg)
		if _, exists := auths[normalizedReg]; !exists {
			if auths == nil {
				auths = make(map[string]DockerAuth)
			}
			auths[normalizedReg] = DockerAuth{}
			logger.Debug("Added empty auth for insecure registry: %s", normalizedReg)
		}
	}

	// Create temp directory for auth files
	authDir := "/tmp/smithy-auth"
	if err := os.MkdirAll(authDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create auth directory: %v", err)
	}

	// Create Buildah auth format
	buildahAuth := map[string]map[string]DockerAuth{
		"auths": auths,
	}

	authData, err := json.MarshalIndent(buildahAuth, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal auth data: %v", err)
	}

	// Write auth.json
	authFile := filepath.Join(authDir, "auth.json")
	if err := os.WriteFile(authFile, authData, 0600); err != nil {
		return "", fmt.Errorf("failed to write auth file: %v", err)
	}

	logger.Debug("Created Buildah auth file at: %s", authFile)

	// Also create registries.conf for insecure registries
	if err := CreateRegistriesConf(authDir, config.InsecureRegistry, config.Destinations); err != nil {
		logger.Warning("Failed to create registries.conf: %v", err)
	}

	// Copy to multiple locations where Buildah might look
	additionalPaths := []string{
		"/run/containers/0/auth.json",
		"/run/containers/1000/auth.json",
		"/run/user/1000/containers/auth.json",
		os.Getenv("HOME") + "/.config/containers/auth.json",
	}

	for _, path := range additionalPaths {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err == nil {
			if err := os.WriteFile(path, authData, 0600); err == nil {
				logger.Debug("Also copied auth to: %s", path)
			}
		}
	}

	// Set environment variables for Buildah
	os.Setenv("REGISTRY_AUTH_FILE", authFile)

	// Also set DOCKER_CONFIG to the directory containing our auth.json
	os.Setenv("DOCKER_CONFIG", authDir)

	// Log authentication summary
	if len(auths) > 0 {
		logger.Info("Authentication configured for %d registries", len(auths))
		for registry, auth := range auths {
			if auth.Auth != "" {
				logger.Debug("  - %s (authenticated)", registry)
			} else {
				logger.Debug("  - %s (no auth/insecure)", registry)
			}
		}
	}

	return authFile, nil
}

// CreateMinimal creates a minimal auth config for insecure registries
func CreateMinimal(config SetupConfig) (string, error) {
	logger.Debug("Creating minimal auth configuration")

	// Create temp directory for auth files
	authDir := "/tmp/smithy-auth"
	if err := os.MkdirAll(authDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create auth directory: %v", err)
	}

	// Create auth entries map
	auths := make(map[string]DockerAuth)

	// Check environment variables for Docker credentials
	dockerUsername := os.Getenv("DOCKER_USERNAME")
	dockerPassword := os.Getenv("DOCKER_PASSWORD")
	dockerRegistry := os.Getenv("DOCKER_REGISTRY")

	if dockerUsername != "" && dockerPassword != "" {
		// Create base64 encoded auth
		authString := base64.StdEncoding.EncodeToString([]byte(dockerUsername + ":" + dockerPassword))

		if dockerRegistry != "" {
			// Add auth for specific registry
			normalizedRegistry := NormalizeRegistryURL(dockerRegistry)
			auths[normalizedRegistry] = DockerAuth{Auth: authString}
			logger.Debug("Added auth from environment for registry: %s", normalizedRegistry)
		} else {
			// Add auth for common registries
			registries := []string{"docker.io", "quay.io", "ghcr.io"}
			for _, reg := range registries {
				auths[reg] = DockerAuth{Auth: authString}
				logger.Debug("Added auth from environment for registry: %s", reg)
			}
		}
	}

	// Add empty auth entries for destination registries
	auths = addDestinationCredentials(auths, config.Destinations)

	// Add empty auth entries for insecure registries
	for _, reg := range config.InsecureRegistry {
		normalizedReg := NormalizeRegistryURL(reg)
		if _, exists := auths[normalizedReg]; !exists {
			auths[normalizedReg] = DockerAuth{}
			logger.Debug("Added empty auth for insecure registry: %s", normalizedReg)
		}
	}

	// Add standard registries if not present
	standardRegistries := []string{"docker.io", "quay.io", "ghcr.io", "gcr.io"}
	for _, reg := range standardRegistries {
		if _, exists := auths[reg]; !exists {
			auths[reg] = DockerAuth{}
		}
	}

	// Create Buildah auth format
	buildahAuth := map[string]map[string]DockerAuth{
		"auths": auths,
	}

	authData, err := json.MarshalIndent(buildahAuth, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal minimal auth: %v", err)
	}

	// Write auth.json
	authFile := filepath.Join(authDir, "auth.json")
	if err := os.WriteFile(authFile, authData, 0600); err != nil {
		return "", fmt.Errorf("failed to write minimal auth file: %v", err)
	}

	logger.Debug("Created minimal auth file at: %s", authFile)

	// Create registries.conf
	if err := CreateRegistriesConf(authDir, config.InsecureRegistry, config.Destinations); err != nil {
		logger.Warning("Failed to create registries.conf: %v", err)
	}

	// Copy to multiple locations where Buildah might look
	additionalPaths := []string{
		"/run/containers/0/auth.json",
		"/run/containers/1000/auth.json",
		"/run/user/1000/containers/auth.json",
		os.Getenv("HOME") + "/.config/containers/auth.json",
	}

	for _, path := range additionalPaths {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err == nil {
			if err := os.WriteFile(path, authData, 0600); err == nil {
				logger.Debug("Also copied auth to: %s", path)
			}
		}
	}

	// Set environment variable
	os.Setenv("REGISTRY_AUTH_FILE", authFile)
	os.Setenv("DOCKER_CONFIG", authDir)

	return authFile, nil
}

// CreateRegistriesConf creates registries.conf for buildah
func CreateRegistriesConf(authDir string, insecureRegistries []string, destinations []string) error {
	registriesConf := filepath.Join(authDir, "registries.conf")
	registriesContent := `unqualified-search-registries = ['docker.io', 'quay.io']

[[registry]]
location = "docker.io"

[[registry]]
location = "quay.io"
`

	// Add insecure registries from config
	insecureRegs := make(map[string]bool)

	// Add from InsecureRegistry list
	for _, registry := range insecureRegistries {
		insecureRegs[registry] = true
	}

	// Add from destinations if they look like local registries
	for _, dest := range destinations {
		if idx := strings.Index(dest, "/"); idx > 0 {
			registry := dest[:idx]
			// Check if it looks like a local/insecure registry
			if strings.Contains(registry, ":") && !strings.Contains(registry, ".") {
				// Likely format like 192.168.1.1:5000 or localhost:5000
				insecureRegs[registry] = true
			}
			// Also add any registry with port number
			if strings.Contains(registry, ":") {
				insecureRegs[registry] = true
			}
		}
	}

	for registry := range insecureRegs {
		registriesContent += fmt.Sprintf(`
[[registry]]
location = "%s"
insecure = true
`, registry)
	}

	return os.WriteFile(registriesConf, []byte(registriesContent), 0644)
}

// EnsurePermissions ensures auth file has correct permissions
func EnsurePermissions(authFile string) error {
	if authFile == "" {
		return nil
	}

	// Ensure the auth file has correct permissions (readable by user)
	if err := os.Chmod(authFile, 0644); err != nil {
		// Try 0600 if 0644 fails
		if err := os.Chmod(authFile, 0600); err != nil {
			return fmt.Errorf("failed to set auth file permissions: %v", err)
		}
	}

	// Validate and fix the auth file format if needed
	if err := validateAndFixAuthFile(authFile); err != nil {
		logger.Warning("Failed to validate auth file: %v", err)
		// Don't fail, just warn
	}

	return nil
}

// detectAuthFormat detects whether a file is Docker config.json or Buildah auth.json
func detectAuthFormat(data []byte) AuthFormat {
	// Try to unmarshal as a generic map first
	var genericMap map[string]interface{}
	if err := json.Unmarshal(data, &genericMap); err != nil {
		return FormatUnknown
	}

	// Check if it has the "auths" key at root level (Docker format)
	if _, hasAuths := genericMap["auths"]; hasAuths {
		// Check for other Docker-specific keys
		if _, hasCredHelpers := genericMap["credHelpers"]; hasCredHelpers {
			return FormatDockerConfig
		}
		if _, hasCredsStore := genericMap["credsStore"]; hasCredsStore {
			return FormatDockerConfig
		}
		// If it ONLY has "auths" key, it could still be Docker format
		if len(genericMap) == 1 {
			// Could be minimal Docker config or Buildah wrapped in auths
			// Let's check the structure inside
			if authsMap, ok := genericMap["auths"].(map[string]interface{}); ok {
				// Check if entries look like registry entries
				for key := range authsMap {
					if IsValidRegistryURL(key) {
						return FormatDockerConfig
					}
				}
			}
		}
		// Multiple keys suggests Docker format
		return FormatDockerConfig
	}

	// If no "auths" key at root, check if it's a direct registry map (Buildah format)
	for key := range genericMap {
		if IsValidRegistryURL(key) {
			return FormatBuildahAuth
		}
	}

	return FormatUnknown
}

// normalizeAuthData normalizes all registry URLs in auth data
func normalizeAuthData(auths map[string]DockerAuth) map[string]DockerAuth {
	normalized := make(map[string]DockerAuth)

	for registry, auth := range auths {
		normalizedRegistry := NormalizeRegistryURL(registry)

		// Keep empty auth entries for insecure registries
		if auth.Auth == "" {
			logger.Debug("Keeping empty auth for registry: %s", normalizedRegistry)
		}

		// If we already have an entry for this normalized registry, prefer the one with auth
		if existing, exists := normalized[normalizedRegistry]; exists {
			if existing.Auth == "" && auth.Auth != "" {
				normalized[normalizedRegistry] = auth
			}
		} else {
			normalized[normalizedRegistry] = auth
		}
	}

	return normalized
}

// detectAndHandleCredentialHelpers checks for credential helpers in Docker config
func detectAndHandleCredentialHelpers(configData []byte, destinations []string) (map[string]DockerAuth, error) {
	var dockerConfig AuthFileContent
	if err := json.Unmarshal(configData, &dockerConfig); err != nil {
		return nil, fmt.Errorf("failed to parse Docker config: %v", err)
	}

	// Start with existing auths if any
	auths := dockerConfig.Auths
	if auths == nil {
		auths = make(map[string]DockerAuth)
	}

	// Handle credHelpers (per-registry helpers)
	if dockerConfig.CredHelpers != nil {
		for registry, helper := range dockerConfig.CredHelpers {
			logger.Debug("Found credential helper '%s' for registry: %s", helper, registry)
			if creds, err := executeCredentialHelper(helper, registry); err == nil {
				auths[registry] = DockerAuth{Auth: creds}
				logger.Info("Successfully obtained credentials from helper '%s' for %s", helper, registry)
			} else {
				logger.Warning("Failed to execute credential helper %s: %v", helper, err)
			}
		}
	}

	// Handle credsStore (default helper for all registries)
	if dockerConfig.CredsStore != "" {
		logger.Debug("Found default credential store: %s", dockerConfig.CredsStore)

		// Get credentials for destination registries
		for _, dest := range destinations {
			registry := ExtractRegistry(dest)
			normalizedRegistry := NormalizeRegistryURL(registry)

			// Skip if we already have auth from credHelpers
			if _, exists := auths[normalizedRegistry]; !exists {
				if creds, err := executeCredentialHelper(dockerConfig.CredsStore, normalizedRegistry); err == nil {
					auths[normalizedRegistry] = DockerAuth{Auth: creds}
					logger.Info("Successfully obtained credentials from store '%s' for %s", dockerConfig.CredsStore, normalizedRegistry)
				} else {
					logger.Debug("No credentials from helper %s for %s: %v", dockerConfig.CredsStore, normalizedRegistry, err)
				}
			}
		}
	}

	return auths, nil
}

// convertToAuthJSON converts any format to Buildah auth.json format
func convertToAuthJSON(data []byte, format AuthFormat) ([]byte, error) {
	switch format {
	case FormatDockerConfig:
		// Parse Docker config
		var dockerConfig AuthFileContent
		if err := json.Unmarshal(data, &dockerConfig); err != nil {
			return nil, fmt.Errorf("failed to parse Docker config: %v", err)
		}

		// Normalize registry URLs
		normalizedAuths := normalizeAuthData(dockerConfig.Auths)

		// Create Buildah auth format (just the auths content)
		buildahAuth := map[string]map[string]DockerAuth{
			"auths": normalizedAuths,
		}

		return json.MarshalIndent(buildahAuth, "", "  ")

	case FormatBuildahAuth:
		// Already in Buildah format, but normalize registry URLs
		var buildahAuths map[string]DockerAuth
		if err := json.Unmarshal(data, &buildahAuths); err != nil {
			// Try parsing as wrapped format
			var wrapped map[string]map[string]DockerAuth
			if err := json.Unmarshal(data, &wrapped); err != nil {
				return nil, fmt.Errorf("failed to parse Buildah auth: %v", err)
			}
			if auths, ok := wrapped["auths"]; ok {
				buildahAuths = auths
			} else {
				return nil, fmt.Errorf("invalid Buildah auth format")
			}
		}

		normalizedAuths := normalizeAuthData(buildahAuths)
		buildahAuth := map[string]map[string]DockerAuth{
			"auths": normalizedAuths,
		}

		return json.MarshalIndent(buildahAuth, "", "  ")

	default:
		return nil, fmt.Errorf("unknown auth format")
	}
}

// addDestinationCredentials ensures credentials exist for destination registries
func addDestinationCredentials(auths map[string]DockerAuth, destinations []string) map[string]DockerAuth {
	if auths == nil {
		auths = make(map[string]DockerAuth)
	}

	for _, dest := range destinations {
		// Extract registry from destination
		registry := ExtractRegistry(dest)
		normalizedRegistry := NormalizeRegistryURL(registry)

		// Check if we already have auth for this registry
		if _, exists := auths[normalizedRegistry]; !exists {
			// Add empty auth entry (for insecure registries)
			logger.Debug("Adding empty auth entry for destination registry: %s", normalizedRegistry)
			auths[normalizedRegistry] = DockerAuth{}
		}
	}

	return auths
}

// validateAndFixAuthFile validates auth file and attempts to fix common issues
func validateAndFixAuthFile(authFile string) error {
	data, err := os.ReadFile(authFile)
	if err != nil {
		return fmt.Errorf("failed to read auth file: %v", err)
	}

	// Detect format
	format := detectAuthFormat(data)
	if format == FormatUnknown {
		return fmt.Errorf("unknown auth file format")
	}

	// Convert to proper Buildah format
	fixedData, err := convertToAuthJSON(data, format)
	if err != nil {
		return fmt.Errorf("failed to fix auth file: %v", err)
	}

	// Write back the fixed data
	if err := os.WriteFile(authFile, fixedData, 0600); err != nil {
		return fmt.Errorf("failed to write fixed auth file: %v", err)
	}

	logger.Debug("Auth file validated and fixed if needed")
	return nil
}
