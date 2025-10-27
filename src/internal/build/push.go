package build

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/rapidfort/kimia/internal/auth"
	"github.com/rapidfort/kimia/pkg/logger"
)

// PushConfig holds push configuration
type PushConfig struct {
	Destinations        []string
	Insecure            bool
	InsecureRegistry    []string
	SkipTLSVerify       bool
	RegistryCertificate string
	PushRetry           int
	StorageDriver       string
}

// Push pushes built images to registries with authentication
// Returns a map of destination->digest for each successfully pushed image
func Push(config PushConfig, authFile string) (map[string]string, error) {
	// BuildKit pushes during build (via --output with push=true)
	// Only buildah needs a separate push step
	builder := DetectBuilder()
	if builder == "buildkit" {
		logger.Debug("Skipping separate push step (BuildKit pushes during build)")
		return make(map[string]string), nil
	}

	digestMap := make(map[string]string)

	for _, dest := range config.Destinations {
		logger.Info("Pushing image: %s", dest)

		// List images to verify the image exists before pushing
		listCmd := exec.Command("buildah", "images", "--format", "{{.Name}}:{{.Tag}}")
		listCmd.Env = os.Environ()
		if config.StorageDriver != "" {
			listCmd.Env = append(listCmd.Env, fmt.Sprintf("STORAGE_DRIVER=%s", config.StorageDriver))
		}
		if listOutput, err := listCmd.Output(); err == nil {
			logger.Debug("Available images in storage before push:")
			logger.Debug("%s", string(listOutput))
		} else {
			logger.Debug("Failed to list images: %v", err)
		}

		// Extract and normalize registry
		registry := auth.ExtractRegistry(dest)
		normalizedRegistry := auth.NormalizeRegistryURL(registry)
		logger.Debug("Destination registry: %s (normalized: %s)", registry, normalizedRegistry)

		// Try to refresh cloud credentials if it's a cloud registry
		if auth.IsECRRegistry(normalizedRegistry) || auth.IsGCRRegistry(normalizedRegistry) || auth.IsGARRegistry(normalizedRegistry) {
			// Note: Cloud credential refresh would need auth package
			// This is a simplified version
			logger.Debug("Detected cloud registry: %s", normalizedRegistry)
		}

		args := []string{"push"}

		// Add auth file if available
		if authFile != "" {
			args = append(args, "--authfile", authFile)
		}

		// Add insecure registry option
		if config.Insecure || isInsecureRegistry(dest, config.InsecureRegistry) {
			args = append(args, "--tls-verify=false")
			logger.Debug("Using insecure mode for registry: %s", normalizedRegistry)
		}

		// Add specific registry certificates if configured
		if config.RegistryCertificate != "" {
			args = append(args, "--cert-dir", config.RegistryCertificate)
		}

		// Add retry logic
		retries := config.PushRetry
		if retries == 0 {
			retries = 1
		}

		args = append(args, dest)

		// Try push with retries
		var lastErr error
		for i := 0; i < retries; i++ {
			if i > 0 {
				logger.Info("Retrying push (attempt %d/%d)...", i+1, retries)
				// Wait a bit before retry
				time.Sleep(time.Second * time.Duration(i*2))
			}

			cmd := exec.Command("buildah", args...)

			// Capture both stdout and stderr for better debugging
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			// Set up environment
			cmd.Env = os.Environ()

			// Ensure REGISTRY_AUTH_FILE is set if we have auth
			if authFile != "" {
				cmd.Env = append(cmd.Env, fmt.Sprintf("REGISTRY_AUTH_FILE=%s", authFile))
			}

			// Use storage driver from config for buildah
			if config.StorageDriver != "" {
				cmd.Env = append(cmd.Env, fmt.Sprintf("STORAGE_DRIVER=%s", config.StorageDriver))
				logger.Debug("Set STORAGE_DRIVER=%s for push", config.StorageDriver)
			}

			err := cmd.Run()

			// Log output for debugging
			if stdout.Len() > 0 {
				logger.Debug("Push stdout: %s", stdout.String())
			}
			if stderr.Len() > 0 {
				if err != nil {
					logger.Error("Push stderr: %s", stderr.String())
				} else {
					logger.Debug("Push stderr: %s", stderr.String())
				}
			}

			if err != nil {
				lastErr = err

				// Analyze the error for better feedback
				stderrStr := stderr.String()
				if strings.Contains(stderrStr, "insufficient_scope") ||
					strings.Contains(stderrStr, "authentication required") ||
					strings.Contains(stderrStr, "unauthorized") {
					logger.Warning("Authentication failed for %s", dest)

					// Provide helpful suggestions
					fmt.Fprintf(os.Stderr, "\n")
					fmt.Fprintf(os.Stderr, "AUTHENTICATION ERROR: Cannot push to %s\n", dest)
					fmt.Fprintf(os.Stderr, "\n")
					fmt.Fprintf(os.Stderr, "Possible solutions:\n")
					fmt.Fprintf(os.Stderr, "1. Login to the registry:\n")
					fmt.Fprintf(os.Stderr, "   docker login %s\n", normalizedRegistry)
					fmt.Fprintf(os.Stderr, "\n")
					fmt.Fprintf(os.Stderr, "2. Mount Docker config in Kubernetes:\n")
					fmt.Fprintf(os.Stderr, "   kubectl create secret docker-registry regcred \\\n")
					fmt.Fprintf(os.Stderr, "     --docker-server=%s \\\n", normalizedRegistry)
					fmt.Fprintf(os.Stderr, "     --docker-username=<username> \\\n")
					fmt.Fprintf(os.Stderr, "     --docker-password=<password>\n")
					fmt.Fprintf(os.Stderr, "\n")
					fmt.Fprintf(os.Stderr, "3. Use environment variables:\n")
					fmt.Fprintf(os.Stderr, "   export DOCKER_USERNAME=<username>\n")
					fmt.Fprintf(os.Stderr, "   export DOCKER_PASSWORD=<password>\n")
					fmt.Fprintf(os.Stderr, "   export DOCKER_REGISTRY=%s\n", normalizedRegistry)
					fmt.Fprintf(os.Stderr, "\n")

					// Don't retry on auth errors
					break
				} else if strings.Contains(stderrStr, "no such host") ||
					strings.Contains(stderrStr, "connection refused") {
					logger.Warning("Network error pushing to %s (attempt %d/%d)", dest, i+1, retries)
				} else {
					logger.Warning("Push attempt %d failed: %v", i+1, err)
				}
				continue
			}

			// Success - extract digest from stderr
			stderrStr := stderr.String()
			digest := extractDigestFromPushOutput(stderrStr)
			if digest != "" {
				digestMap[dest] = digest
				logger.Debug("Extracted digest for %s: %s", dest, digest)
			}

			logger.Info("Successfully pushed: %s", dest)
			lastErr = nil
			break
		}

		if lastErr != nil {
			return digestMap, fmt.Errorf("failed to push %s after %d attempts: %v", dest, retries, lastErr)
		}
	}

	return digestMap, nil
}

// PushSingle pushes a single image with retries (used by hardening)
// Returns the manifest digest of the pushed image
func PushSingle(image string, config PushConfig, authFile string) (string, error) {
	// BuildKit pushes during build (via --output with push=true)
	// Only buildah needs a separate push step
	builder := DetectBuilder()
	if builder == "buildkit" {
		logger.Debug("Skipping separate push step for %s (BuildKit pushes during build)", image)
		return "", nil
	}

	// Build push command
	args := []string{"push"}

	// Add auth file if available
	if authFile != "" {
		args = append(args, "--authfile", authFile)
	}

	// Add insecure registry option
	if config.Insecure || isInsecureRegistry(image, config.InsecureRegistry) {
		args = append(args, "--tls-verify=false")
	}

	// Add specific registry certificates if configured
	if config.RegistryCertificate != "" {
		args = append(args, "--cert-dir", config.RegistryCertificate)
	}

	// Add the image
	args = append(args, image)

	// Try push with retries
	retries := config.PushRetry
	if retries == 0 {
		retries = 1
	}

	var lastErr error
	for i := 0; i < retries; i++ {
		if i > 0 {
			logger.Debug("Retrying push of %s (attempt %d/%d)...", image, i+1, retries)
			time.Sleep(time.Second * time.Duration(i*2))
		}

		cmd := exec.Command("buildah", args...)

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		cmd.Env = os.Environ()

		if authFile != "" {
			cmd.Env = append(cmd.Env, fmt.Sprintf("REGISTRY_AUTH_FILE=%s", authFile))
		}

		// Use storage driver from config for buildah
		if config.StorageDriver != "" {
			cmd.Env = append(cmd.Env, fmt.Sprintf("STORAGE_DRIVER=%s", config.StorageDriver))
			logger.Debug("Set STORAGE_DRIVER=%s for push", config.StorageDriver)
		}

		// Log full command for debugging
		logger.Debug("Buildah push command: buildah %s", strings.Join(args, " "))
		logger.Debug("Push command environment:")
		for _, env := range cmd.Env {
			if strings.HasPrefix(env, "STORAGE_DRIVER=") ||
				strings.HasPrefix(env, "REGISTRY_AUTH_FILE=") {
				logger.Debug("  %s", env)
			}
		}

		err := cmd.Run()

		if stdout.Len() > 0 {
			logger.Debug("Push stdout: %s", stdout.String())
		}
		if stderr.Len() > 0 && err != nil {
			logger.Debug("Push stderr: %s", stderr.String())
		}

		if err == nil {
			// Extract digest from stderr
			digest := extractDigestFromPushOutput(stderr.String())
			if digest != "" {
				logger.Debug("Extracted digest for %s: %s", image, digest)
			}
			return digest, nil
		}

		lastErr = err
	}

	return "", lastErr
}

// isInsecureRegistry checks if a destination matches an insecure registry pattern
func isInsecureRegistry(dest string, insecureRegistries []string) bool {
	for _, reg := range insecureRegistries {
		if strings.HasPrefix(dest, reg) {
			return true
		}
	}
	return false
}

// extractDigestFromPushOutput extracts the manifest digest from buildah push stderr
// Example stderr line: "Copying config sha256:0b0a90c89d1e19e603b72d1d02efdd324a622d7ee93071c8e268165f2f0e6821"
func extractDigestFromPushOutput(stderr string) string {
	// Look for "Copying config sha256:..." in the output
	lines := strings.Split(stderr, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Copying config sha256:") {
			// Extract the sha256 digest
			parts := strings.Split(line, "sha256:")
			if len(parts) >= 2 {
				digest := strings.TrimSpace(parts[1])
				// Return with sha256: prefix
				return "sha256:" + digest
			}
		}
	}
	return ""
}
