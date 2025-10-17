package build

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rapidfort/smithy/pkg/logger"
)

// Config holds build configuration
type Config struct {
	// Core build arguments
	Dockerfile  string
	Destination []string
	Target      string

	// Build arguments and labels
	BuildArgs map[string]string
	Labels    map[string]string

	// Platform
	CustomPlatform string

	// Cache options
	Cache    bool
	CacheDir string

	// Storage driver
	StorageDriver string

	// Security options
	Insecure            bool
	InsecurePull        bool
	InsecureRegistry    []string
	SkipTLSVerify       bool
	RegistryCertificate string

	// Output options
	NoPush                     bool
	TarPath                    string
	DigestFile                 string
	ImageNameWithDigestFile    string
	ImageNameTagWithDigestFile string
}

// Execute executes a buildah build with authentication
func Execute(config Config, ctx *Context, authFile string) error {
	logger.Info("Starting buildah build...")

	// Construct buildah command
	args := []string{"bud"}

	// Add auth file if available
	if authFile != "" {
		// Validate auth file exists and is readable
		if _, err := os.Stat(authFile); err != nil {
			logger.Warning("Auth file not found or not readable: %v", err)
		} else {
			args = append(args, "--authfile", authFile)
		}
	}

	// Add storage driver option
	if config.StorageDriver != "" {
		args = append(args, "--storage-driver", config.StorageDriver)
		logger.Debug("Using storage driver: %s", config.StorageDriver)
	}

	// Add Dockerfile
	dockerfilePath := config.Dockerfile
	if dockerfilePath == "" {
		dockerfilePath = "Dockerfile"
	}

	// If Dockerfile is relative and we have a context, make it absolute
	if !filepath.IsAbs(dockerfilePath) {
		dockerfilePath = filepath.Join(ctx.Path, dockerfilePath)
	}

	args = append(args, "-f", dockerfilePath)

	// Add build arguments
	for key, value := range config.BuildArgs {
		if value != "" {
			args = append(args, "--build-arg", fmt.Sprintf("%s=%s", key, value))
		} else {
			// Use environment variable
			args = append(args, "--build-arg", key)
		}
	}

	// Add labels
	for key, value := range config.Labels {
		args = append(args, "--label", fmt.Sprintf("%s=%s", key, value))
	}

	// Add target if specified
	if config.Target != "" {
		args = append(args, "--target", config.Target)
	}

	// Add platform if specified
	if config.CustomPlatform != "" {
		args = append(args, "--platform", config.CustomPlatform)
	}

	// Add cache options
	if config.Cache {
		if config.CacheDir != "" {
			// Buildah doesn't have direct cache-dir equivalent, but we can use layers
			args = append(args, "--layers")
		} else {
			args = append(args, "--layers")
		}
	} else {
		args = append(args, "--no-cache")
	}

	// Add insecure registry options for build
	if config.Insecure || config.InsecurePull {
		args = append(args, "--tls-verify=false")
	}

	// Add tags (destinations)
	for _, dest := range config.Destination {
		args = append(args, "-t", dest)
	}

	// Add context path
	args = append(args, ctx.Path)

	// Log the command
	logger.Debug("Buildah command: buildah %s", strings.Join(args, " "))

	// Execute buildah
	cmd := exec.Command("buildah", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	// Set BUILDAH_ISOLATION if not already set
	if os.Getenv("BUILDAH_ISOLATION") == "" {
		cmd.Env = append(cmd.Env, "BUILDAH_ISOLATION=chroot")
	}

	// Enhanced environment setup for auth
	if authFile != "" {
		// Set multiple env vars that different tools might look for
		cmd.Env = append(cmd.Env,
			fmt.Sprintf("REGISTRY_AUTH_FILE=%s", authFile),
			fmt.Sprintf("DOCKER_CONFIG=%s", filepath.Dir(authFile)),
			fmt.Sprintf("BUILDAH_AUTH_FILE=%s", authFile),
		)
	}

	// Set storage driver via environment variable (fallback)
	if config.StorageDriver != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("STORAGE_DRIVER=%s", config.StorageDriver))
	}

	if err := cmd.Run(); err != nil {
		// Enhanced error reporting
		if strings.Contains(err.Error(), "authentication") ||
			strings.Contains(err.Error(), "unauthorized") {
			return fmt.Errorf("buildah build failed (authentication issue): %v", err)
		}
		if strings.Contains(err.Error(), "overlay") ||
			strings.Contains(err.Error(), "fuse-overlayfs") {
			return fmt.Errorf("buildah build failed (overlay driver issue - ensure fuse-overlayfs is installed): %v", err)
		}
		return fmt.Errorf("buildah build failed: %v", err)
	}

	logger.Info("Build completed successfully")

	// Handle special output options
	if config.TarPath != "" {
		if err := exportToTar(config); err != nil {
			return fmt.Errorf("failed to export to tar: %v", err)
		}
	}

	if config.DigestFile != "" || config.ImageNameWithDigestFile != "" {
		if err := saveDigestInfo(config); err != nil {
			return fmt.Errorf("failed to save digest info: %v", err)
		}
	}

	return nil
}

// exportToTar exports the built image to a tar archive
func exportToTar(config Config) error {
	if len(config.Destination) == 0 {
		return fmt.Errorf("no destination image to export")
	}

	logger.Info("Exporting image to tar: %s", config.TarPath)

	// Use the first destination as the image to export
	image := config.Destination[0]

	cmd := exec.Command("buildah", "push", image, fmt.Sprintf("docker-archive:%s", config.TarPath))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to export to tar: %v", err)
	}

	logger.Info("Image exported to: %s", config.TarPath)
	return nil
}

// saveDigestInfo saves image digest information to files
func saveDigestInfo(config Config) error {
	if len(config.Destination) == 0 {
		return nil
	}

	// Get image digest
	image := config.Destination[0]
	cmd := exec.Command("buildah", "inspect", "--format", "{{.Digest}}", image)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get image digest: %v", err)
	}

	digest := strings.TrimSpace(string(output))

	// Save digest file
	if config.DigestFile != "" {
		if err := os.WriteFile(config.DigestFile, []byte(digest), 0644); err != nil {
			return fmt.Errorf("failed to write digest file: %v", err)
		}
		logger.Info("Digest saved to: %s", config.DigestFile)
	}

	// Save image name with digest
	if config.ImageNameWithDigestFile != "" {
		imageName := strings.Split(image, ":")[0]
		imageWithDigest := fmt.Sprintf("%s@%s", imageName, digest)
		if err := os.WriteFile(config.ImageNameWithDigestFile, []byte(imageWithDigest), 0644); err != nil {
			return fmt.Errorf("failed to write image name with digest file: %v", err)
		}
		logger.Info("Image name with digest saved to: %s", config.ImageNameWithDigestFile)
	}

	// Save image name tag with digest
	if config.ImageNameTagWithDigestFile != "" {
		content := map[string]string{
			"image":  image,
			"digest": digest,
		}
		data, _ := json.MarshalIndent(content, "", "  ")
		if err := os.WriteFile(config.ImageNameTagWithDigestFile, data, 0644); err != nil {
			return fmt.Errorf("failed to write image name tag with digest file: %v", err)
		}
		logger.Info("Image name tag with digest saved to: %s", config.ImageNameTagWithDigestFile)
	}

	return nil
}
