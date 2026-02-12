package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rapidfort/kimia/pkg/logger"
)

// DockerConfig represents Docker's config.json structure
type DockerConfig struct {
	Auths       map[string]DockerAuth `json:"auths"`
	CredHelpers map[string]string     `json:"credHelpers,omitempty"`
	CredsStore  string                `json:"credsStore,omitempty"`
}

// DockerAuth represents auth entry in Docker config
type DockerAuth struct {
	Auth     string `json:"auth,omitempty"`
	Username string `json:"username,omitempty"`
	// #nosec G117 -- Password field required for Docker config.json schema compatibility
	Password string `json:"password,omitempty"`
}

// SetupConfig holds configuration for authentication setup
type SetupConfig struct {
	Destinations     []string
	InsecureRegistry []string
}

// GetDockerConfigDir returns the Docker config directory
func GetDockerConfigDir() string {
	if dir := os.Getenv("DOCKER_CONFIG"); dir != "" {
		return dir
	}
	return "/home/kimia/.docker"
}

// Setup validates Docker config.json authentication or creates it from environment variables
func Setup(config SetupConfig) error {
	logger.Debug("Setting up authentication...")

	dockerConfigDir := GetDockerConfigDir()
	configPath := filepath.Join(dockerConfigDir, "config.json")

	logger.Debug("Looking for Docker config at: %s", configPath)

	// Check if config.json exists
	if _, err := os.Stat(configPath); err != nil {
		if os.IsNotExist(err) {
			logger.Debug("No Docker config found at %s", configPath)
			
			// Fallback: Check environment variables
			dockerUsername := os.Getenv("DOCKER_USERNAME")
			dockerPassword := os.Getenv("DOCKER_PASSWORD")
			dockerRegistry := os.Getenv("DOCKER_REGISTRY")

			if dockerUsername != "" && dockerPassword != "" {
				logger.Info("Creating Docker config from environment variables")
				
				// Create config from environment variables
				auths := make(map[string]DockerAuth)
				authString := EncodeAuth(dockerUsername, dockerPassword)

				if dockerRegistry != "" {
					// Specific registry provided
					normalizedRegistry := NormalizeRegistryURL(dockerRegistry)
					auths[normalizedRegistry] = DockerAuth{Auth: authString}
					
					// For Docker Hub, also add legacy format
					if normalizedRegistry == "docker.io" {
						auths["https://index.docker.io/v1/"] = DockerAuth{Auth: authString}
						logger.Debug("Added auth for Docker Hub (both modern and legacy formats)")
					} else {
						logger.Debug("Added auth for registry: %s", normalizedRegistry)
					}
				} else {
					// No DOCKER_REGISTRY specified - extract from destinations
					if len(config.Destinations) > 0 {
						registryMap := make(map[string]bool)
						for _, dest := range config.Destinations {
							registry := ExtractRegistry(dest)
							normalizedRegistry := NormalizeRegistryURL(registry)
							if !registryMap[normalizedRegistry] {
								auths[normalizedRegistry] = DockerAuth{Auth: authString}
								
								// For Docker Hub, also add legacy format
								if normalizedRegistry == "docker.io" {
									auths["https://index.docker.io/v1/"] = DockerAuth{Auth: authString}
								}
								
								registryMap[normalizedRegistry] = true
								logger.Debug("Added auth for destination registry: %s", normalizedRegistry)
							}
						}
					} else {
						// No destinations - add Docker Hub with BOTH formats
						auths["docker.io"] = DockerAuth{Auth: authString}
						auths["https://index.docker.io/v1/"] = DockerAuth{Auth: authString}
						auths["quay.io"] = DockerAuth{Auth: authString}
						auths["ghcr.io"] = DockerAuth{Auth: authString}
						logger.Debug("Added auth for common registries")
					}
				}

				// Create the config directory if it doesn't exist
				if err := os.MkdirAll(dockerConfigDir, 0755); err != nil {
					return fmt.Errorf("failed to create Docker config directory: %v", err)
				}

				// Create the config.json file
				if err := CreateDockerConfig(configPath, auths); err != nil {
					return fmt.Errorf("failed to create Docker config from environment: %v", err)
				}

				logger.Info("Created Docker config at: %s", configPath)
				return nil
			}

			// No config and no env vars - that's OK for public registries
			logger.Debug("No authentication configured (OK for public registries)")
			return nil
		}
		return fmt.Errorf("error accessing Docker config: %v", err)
	}

	// Read and validate the config
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read Docker config: %v", err)
	}

	var dockerConfig DockerConfig
	if err := json.Unmarshal(data, &dockerConfig); err != nil {
		return fmt.Errorf("invalid Docker config JSON: %v", err)
	}

	// Log authentication info
	authCount := 0
	helperCount := 0

	if dockerConfig.Auths != nil {
		for registry, auth := range dockerConfig.Auths {
			if auth.Auth != "" || auth.Username != "" {
				authCount++
				logger.Debug("Found credentials for: %s", registry)
			}
		}
	}

	if dockerConfig.CredHelpers != nil {
		helperCount = len(dockerConfig.CredHelpers)
		for registry, helper := range dockerConfig.CredHelpers {
			logger.Debug("Found credential helper '%s' for: %s", helper, registry)
		}
	}

	if dockerConfig.CredsStore != "" {
		logger.Debug("Found global credential store: %s", dockerConfig.CredsStore)
	}

	// Summary
	if authCount > 0 || helperCount > 0 || dockerConfig.CredsStore != "" {
		logger.Info("Authentication configured: %d direct auths, %d helpers, global store: %v",
			authCount, helperCount, dockerConfig.CredsStore != "")
	} else {
		logger.Debug("Docker config exists but contains no credentials (using anonymous access)")
	}

	// Check for cloud registries in destinations
	for _, dest := range config.Destinations {
		registry := ExtractRegistry(dest)
		if IsECRRegistry(registry) {
			logger.Debug("Detected AWS ECR registry: %s", registry)
			logger.Debug("Ensure 'docker-credential-ecr-login' is installed for authentication")
		} else if IsGCRRegistry(registry) || IsGARRegistry(registry) {
			logger.Debug("Detected Google Cloud registry: %s", registry)
			logger.Debug("Ensure 'docker-credential-gcr' is installed for authentication")
		}
	}

	return nil
}

// ValidateDockerConfig validates a Docker config.json file
func ValidateDockerConfig(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config: %v", err)
	}

	var config DockerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("invalid JSON: %v", err)
	}

	// Basic validation - just ensure it's valid JSON with expected structure
	if config.Auths == nil && config.CredHelpers == nil && config.CredsStore == "" {
		return fmt.Errorf("config contains no auths, credHelpers, or credsStore")
	}

	return nil
}

// CreateRegistriesConf creates a registries.conf file for insecure registries
func CreateRegistriesConf(configDir string, insecureRegistries []string, destinations []string) error {
	if len(insecureRegistries) == 0 {
		return nil
	}

	// Build registries.conf content
	var sb strings.Builder
	sb.WriteString("# Generated by Kimia\n")
	sb.WriteString("unqualified-search-registries = ['docker.io']\n\n")

	// Add insecure registries
	for _, registry := range insecureRegistries {
		normalizedReg := NormalizeRegistryURL(registry)
		sb.WriteString(fmt.Sprintf("[[registry]]\n"))
		sb.WriteString(fmt.Sprintf("location = \"%s\"\n", normalizedReg))
		sb.WriteString("insecure = true\n\n")
	}

	// Write registries.conf
	confPath := filepath.Join(configDir, "registries.conf")
	if err := os.WriteFile(confPath, []byte(sb.String()), 0600); err != nil {
		return fmt.Errorf("failed to write registries.conf: %v", err)
	}

	logger.Debug("Created registries.conf at: %s", confPath)
	return nil
}

// EncodeAuth encodes username and password to base64 auth string
func EncodeAuth(username, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
}

// DecodeAuth decodes a base64 auth string to username and password
func DecodeAuth(auth string) (username, password string, err error) {
	decoded, err := base64.StdEncoding.DecodeString(auth)
	if err != nil {
		return "", "", err
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid auth format")
	}

	return parts[0], parts[1], nil
}

// GetRegistryAuth retrieves auth for a specific registry from Docker config
func GetRegistryAuth(registry string) (string, error) {
	dockerConfigDir := GetDockerConfigDir()
	configPath := filepath.Join(dockerConfigDir, "config.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read Docker config: %v", err)
	}

	var config DockerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return "", fmt.Errorf("invalid Docker config: %v", err)
	}

	// Try to find auth for the registry
	normalizedReg := NormalizeRegistryURL(registry)

	// Direct match
	if auth, exists := config.Auths[normalizedReg]; exists {
		if auth.Auth != "" {
			return auth.Auth, nil
		}
		if auth.Username != "" && auth.Password != "" {
			return EncodeAuth(auth.Username, auth.Password), nil
		}
	}

	// Try with https:// prefix
	httpsReg := "https://" + normalizedReg
	if auth, exists := config.Auths[httpsReg]; exists {
		if auth.Auth != "" {
			return auth.Auth, nil
		}
		if auth.Username != "" && auth.Password != "" {
			return EncodeAuth(auth.Username, auth.Password), nil
		}
	}

	return "", fmt.Errorf("no auth found for registry: %s", registry)
}

// CreateDockerConfig creates a Docker config.json from credentials
func CreateDockerConfig(outputPath string, auths map[string]DockerAuth) error {
	config := DockerConfig{
		Auths: auths,
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	// Write with restrictive permissions
	if err := os.WriteFile(outputPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %v", err)
	}

	logger.Debug("Created Docker config at: %s", outputPath)
	return nil
}

// AddCredentialHelper adds a credential helper to Docker config
func AddCredentialHelper(registry, helper string) error {
	dockerConfigDir := GetDockerConfigDir()
	configPath := filepath.Join(dockerConfigDir, "config.json")

	var config DockerConfig

	// Read existing config if it exists
	if data, err := os.ReadFile(configPath); err == nil {
		if err := json.Unmarshal(data, &config); err != nil {
			logger.Warning("Failed to parse existing config, creating new one")
			config = DockerConfig{}
		}
	} else {
		config = DockerConfig{}
	}

	// Initialize maps if needed
	if config.CredHelpers == nil {
		config.CredHelpers = make(map[string]string)
	}
	if config.Auths == nil {
		config.Auths = make(map[string]DockerAuth)
	}

	// Add credential helper
	config.CredHelpers[registry] = helper

	// Save config
	return CreateDockerConfig(configPath, config.Auths)
}