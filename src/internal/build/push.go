package build

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/rapidfort/smithy/internal/auth"
	"github.com/rapidfort/smithy/pkg/logger"
)

// PushConfig holds push configuration
type PushConfig struct {
	Destinations        []string
	Insecure            bool
	InsecureRegistry    []string
	SkipTLSVerify       bool
	RegistryCertificate string
	PushRetry           int
}

// Push pushes built images to registries with authentication
func Push(config PushConfig, authFile string) error {
	for _, dest := range config.Destinations {
		logger.Info("Pushing image: %s", dest)

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

			// Add container runtime environment variables
			cmd.Env = append(cmd.Env, "STORAGE_DRIVER=vfs")

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

			logger.Info("Successfully pushed: %s", dest)
			lastErr = nil
			break
		}

		if lastErr != nil {
			return fmt.Errorf("failed to push %s after %d attempts: %v", dest, retries, lastErr)
		}
	}

	return nil
}

// PushSingle pushes a single image with retries (used by hardening)
func PushSingle(image string, config PushConfig, authFile string) error {
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
		cmd.Env = append(cmd.Env, "STORAGE_DRIVER=vfs")

		err := cmd.Run()

		if stdout.Len() > 0 {
			logger.Debug("Push stdout: %s", stdout.String())
		}
		if stderr.Len() > 0 && err != nil {
			logger.Debug("Push stderr: %s", stderr.String())
		}

		if err == nil {
			return nil
		}

		lastErr = err
	}

	return lastErr
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
