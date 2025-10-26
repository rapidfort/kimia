package build

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

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
	ImageDownloadRetry  int

	// Output options
	NoPush                     bool
	TarPath                    string
	DigestFile                 string
	ImageNameWithDigestFile    string
	ImageNameTagWithDigestFile string

	// Reproducible builds
	Reproducible bool
}

// DetectBuilder determines which builder is available
func DetectBuilder() string {
	// Check for BuildKit first (preferred/default)
	if _, err := exec.LookPath("buildkitd"); err == nil {
		if _, err := exec.LookPath("buildctl"); err == nil {
			return "buildkit"
		}
	}

	// Check for Buildah (legacy)
	if _, err := exec.LookPath("buildah"); err == nil {
		return "buildah"
	}

	return "unknown"
}

// Execute executes a build using the detected builder (buildah or buildkit)
func Execute(config Config, ctx *Context, authFile string) error {
	builder := DetectBuilder()

	if builder == "unknown" {
		return fmt.Errorf("no builder found (expected buildkitd or buildah)")
	}

	logger.Info("Using builder: %s", strings.ToUpper(builder))

	if builder == "buildkit" {
		return executeBuildKit(config, ctx, authFile)
	}

	return executeBuildah(config, ctx, authFile)
}

// executeBuildah executes a buildah build with authentication
func executeBuildah(config Config, ctx *Context, authFile string) error {
	// Detect if running as root
	isRoot := os.Getuid() == 0

	if isRoot {
		logger.Warning("Running as root (UID 0) - using chroot isolation")
		logger.Warning("For production, use rootless configuration (UID 1000) with SETUID/SETGID capabilities")
	} else {
		logger.Debug("Running as non-root (UID %d) - using chroot isolation with user namespaces", os.Getuid())
	}

	logger.Info("Starting buildah build...")

	// Log storage driver if specified
	if config.StorageDriver != "" {
		storageDriver := strings.ToLower(config.StorageDriver)
		logger.Info("Using storage driver: %s", storageDriver)
		switch storageDriver {
		case "overlay":
			logger.Info("Note: Overlay storage driver selected")
		case "vfs":
			logger.Info("Note: VFS storage driver selected")
		}
	}

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

	// ========================================
	// REPRODUCIBLE BUILDS: Sort build arguments
	// ========================================
	// CRITICAL: Go maps have random iteration order!
	// We must sort keys to ensure deterministic command line
	buildArgKeys := make([]string, 0, len(config.BuildArgs))
	for key := range config.BuildArgs {
		buildArgKeys = append(buildArgKeys, key)
	}
	sort.Strings(buildArgKeys)

	for _, key := range buildArgKeys {
		value := config.BuildArgs[key]
		if value != "" {
			args = append(args, "--build-arg", fmt.Sprintf("%s=%s", key, value))
		} else {
			// Use environment variable
			args = append(args, "--build-arg", key)
		}
	}

	// ========================================
	// REPRODUCIBLE BUILDS: Sort labels
	// ========================================
	labelKeys := make([]string, 0, len(config.Labels))
	for key := range config.Labels {
		labelKeys = append(labelKeys, key)
	}
	sort.Strings(labelKeys)

	for _, key := range labelKeys {
		value := config.Labels[key]
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
	// Note: For reproducible builds, we must run with --no-cache
	if config.Cache && !config.Reproducible {
		if config.CacheDir != "" {
			// Buildah doesn't have direct cache-dir equivalent, but we can use layers
			args = append(args, "--layers")
		} else {
			args = append(args, "--layers")
		}
	} else {
		args = append(args, "--no-cache")
	}

	// Add retry option for image downloads
	if config.ImageDownloadRetry > 0 {
		args = append(args, "--retry", fmt.Sprintf("%d", config.ImageDownloadRetry))
		logger.Info("Image download retry set to %d attempts", config.ImageDownloadRetry)
	}

	// ========================================
	// REPRODUCIBLE BUILDS: Handle timestamp
	// ========================================
	// This sets the image creation timestamp to a deterministic value
	// Note: Buildah will use SOURCE_DATE_EPOCH from environment directly
	// We only add --timestamp flag when SOURCE_DATE_EPOCH is NOT set
	var sourceEpoch string
	if epoch := os.Getenv("SOURCE_DATE_EPOCH"); epoch != "" {
		sourceEpoch = epoch
		// Don't add --timestamp flag - buildah will read SOURCE_DATE_EPOCH from environment
		// Adding both causes "timestamp and source-date-epoch would be ambiguous" error
		logger.Debug("Using SOURCE_DATE_EPOCH=%s from environment (buildah will read directly)", epoch)
	} else if config.Reproducible {
		// Only add --timestamp when SOURCE_DATE_EPOCH is not set
		sourceEpoch = "0"
		args = append(args, "--timestamp", "0")
		logger.Debug("Setting timestamp=0 for reproducible build")
	}

	// Add insecure registry options for build
	if config.Insecure || config.InsecurePull {
		args = append(args, "--tls-verify=false")
	}

	// ========================================
	// REPRODUCIBLE BUILDS: Sort destinations
	// ========================================
	sortedDests := make([]string, len(config.Destination))
	copy(sortedDests, config.Destination)
	sort.Strings(sortedDests)

	for _, dest := range sortedDests {
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

	// Always use chroot isolation for both root and rootless
	if os.Getenv("BUILDAH_ISOLATION") == "" {
		cmd.Env = append(cmd.Env, "BUILDAH_ISOLATION=chroot")
		logger.Debug("Set BUILDAH_ISOLATION=chroot (default for all modes)")
	} else {
		logger.Debug("Using existing BUILDAH_ISOLATION=%s", os.Getenv("BUILDAH_ISOLATION"))
	}

	// Enhanced environment setup for auth
	if authFile != "" {
		cmd.Env = append(cmd.Env,
			fmt.Sprintf("REGISTRY_AUTH_FILE=%s", authFile),
			fmt.Sprintf("DOCKER_CONFIG=%s", filepath.Dir(authFile)),
			fmt.Sprintf("BUILDAH_AUTH_FILE=%s", authFile),
		)
	}

	// Storage driver configuration
	storageDriver := config.StorageDriver
	if storageDriver != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("STORAGE_DRIVER=%s", storageDriver))
		logger.Debug("Set STORAGE_DRIVER=%s", storageDriver)
	}

	// Log environment for debugging
	logger.Debug("Buildah bud environment:")
	for _, env := range cmd.Env {
		if strings.HasPrefix(env, "STORAGE_DRIVER=") ||
			strings.HasPrefix(env, "BUILDAH_") ||
			strings.HasPrefix(env, "REGISTRY_AUTH_FILE=") {
			logger.Debug("  %s", env)
		}
	}

	// ========================================
	// REPRODUCIBLE BUILDS: Set SOURCE_DATE_EPOCH environment
	// ========================================
	// This affects file timestamps in layers
	if sourceEpoch != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("SOURCE_DATE_EPOCH=%s", sourceEpoch))
		logger.Debug("Set SOURCE_DATE_EPOCH=%s in build environment", sourceEpoch)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("buildah build failed: %v", err)
	}

	logger.Info("Build completed successfully")

	// Handle TAR export if requested
	if config.TarPath != "" {
		if err := exportToTar(config); err != nil {
			return err
		}
	}

	if config.NoPush {
		logger.Info("No push requested, skipping image push to registries")
	}

	return nil
}

func executeBuildKit(config Config, ctx *Context, authFile string) error {
	logger.Info("Starting BuildKit build...")

	// Setup environment
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		homeDir = "/home/smithy"
	}

	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntimeDir == "" {
		xdgRuntimeDir = "/tmp/run"
	}

	buildkitSocket := filepath.Join(xdgRuntimeDir, "buildkitd.sock")
	buildkitConfig := filepath.Join(homeDir, ".config/buildkit/buildkitd.toml")

	// Handle bind-mounted contexts - copy to real filesystem
	// Bind mounts from Docker may not be accessible to buildctl/buildkitd
	// BUT: Don't copy git-cloned directories (they're already in real filesystem)
	buildContext := ctx.Path
	var tempContext string

	workspaceMount := filepath.Join(homeDir, "workspace")

	// Only copy if it's a bind mount, not a git clone
	// Git clones are in subdirectories like /home/smithy/workspace/smithy-build-*
	// Bind mounts can be at /workspace or /home/smithy/workspace
	isBindMount := (ctx.Path == workspaceMount || ctx.Path == "/workspace") && !ctx.IsGitRepo
	if isBindMount {
		logger.Debug("Detected bind-mounted context at %s, copying to buildkit cache...", ctx.Path)

		// Create cache directory
		cacheDir := filepath.Join(homeDir, ".cache/buildkit")
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			return fmt.Errorf("failed to create cache directory: %v", err)
		}

		// Create temp directory for context copy
		tempDir, err := os.MkdirTemp(cacheDir, "context-*")
		if err != nil {
			return fmt.Errorf("failed to create temp context directory: %v", err)
		}
		tempContext = tempDir

		defer func() {
			logger.Debug("Cleaning up temp context directory: %s", tempContext)
			os.RemoveAll(tempContext)
		}()

		// Copy context to temp directory
		logger.Debug("Copying context from %s to %s", ctx.Path, tempContext)
		if err := copyDir(ctx.Path, tempContext); err != nil {
			return fmt.Errorf("failed to copy context: %v", err)
		}

		buildContext = tempContext
		logger.Debug("Using copied context at: %s", buildContext)
	}

	logger.Debug("BuildKit configuration:")
	logger.Debug("  HOME: %s", homeDir)
	logger.Debug("  XDG_RUNTIME_DIR: %s", xdgRuntimeDir)
	logger.Debug("  BUILDKIT_HOST: unix://%s", buildkitSocket)
	logger.Debug("  Config file: %s", buildkitConfig)
	logger.Debug("  Build context: %s", buildContext)

	// Add insecure registry configurations to buildkit config when --insecure flag is passed
	if config.Insecure {
		// Create buildkit config directory if needed
		configDir := filepath.Dir(buildkitConfig)
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return fmt.Errorf("failed to create buildkit config directory: %v", err)
		}

		// Read existing config if it exists
		var existingConfig string
		if data, err := os.ReadFile(buildkitConfig); err == nil {
			existingConfig = string(data)
			logger.Debug("Read existing buildkit config")
		} else {
			// Create default config if file doesn't exist
			existingConfig = `[worker.oci]
  enabled = true
[worker.containerd]
  enabled = false

[registry."docker.io"]
  mirrors = ["mirror.gcr.io"]
`
			logger.Debug("Creating new buildkit config")
		}

		// Extract registries from destinations
		registries := make(map[string]bool)
		for _, dest := range config.Destination {
			if idx := strings.Index(dest, "/"); idx > 0 {
				registry := dest[:idx]
				registries[registry] = true
			}
		}

		// Append insecure config for each registry
		configContent := existingConfig
		for registry := range registries {
			// Check if this registry is already configured
			if !strings.Contains(existingConfig, fmt.Sprintf(`[registry."%s"]`, registry)) {
				configContent += fmt.Sprintf(`
[registry."%s"]
  http = true
  insecure = true
`, registry)
				logger.Debug("Adding insecure registry to buildkit config: %s", registry)
			} else {
				logger.Debug("Registry already configured in buildkit config: %s", registry)
			}
		}

		if err := os.WriteFile(buildkitConfig, []byte(configContent), 0644); err != nil {
			return fmt.Errorf("failed to write buildkit config: %v", err)
		}
	}

	// Start buildkitd with rootlesskit
	logger.Debug("Starting buildkitd with rootlesskit...")
	daemonCmd := exec.Command(
		"rootlesskit",
		"--state-dir="+filepath.Join(xdgRuntimeDir, "rk-buildkit"),
		"--net=host",
		"--disable-host-loopback",
		"buildkitd",
		"--config="+buildkitConfig,
		"--addr=unix://"+buildkitSocket,
	)

	// Capture buildkitd output
	daemonCmd.Stdout = os.Stdout
	daemonCmd.Stderr = os.Stderr

	if err := daemonCmd.Start(); err != nil {
		return fmt.Errorf("failed to start buildkitd: %v", err)
	}

	logger.Debug("buildkitd process started (PID: %d)", daemonCmd.Process.Pid)

	// Ensure daemon cleanup
	defer func() {
		logger.Debug("Stopping buildkitd...")
		if daemonCmd.Process != nil {
			daemonCmd.Process.Kill()
		}
	}()

	// Wait for buildkitd to be ready
	logger.Debug("Waiting for buildkitd to be ready...")
	ready := false
	for i := 0; i < 30; i++ {
		checkCmd := exec.Command("buildctl", "--addr=unix://"+buildkitSocket, "debug", "info")
		output, err := checkCmd.CombinedOutput()

		if err == nil {
			ready = true
			break
		}

		// Show progress with actual error
		logger.Debug("Waiting for buildkitd... (%d/30) - error: %v", i+1, err)
		if len(output) > 0 {
			logger.Debug("  Output: %s", string(output))
		}

		// Check if daemon is still running
		if daemonCmd.Process != nil {
			// Process is still running
			time.Sleep(1 * time.Second)
		} else {
			return fmt.Errorf("buildkitd process died")
		}
	}

	if !ready {
		return fmt.Errorf("buildkitd failed to become ready after 30 seconds")
	}

	logger.Debug("buildkitd is ready")

	// Construct buildctl command
	args := []string{"build", "--frontend", "dockerfile.v0"}

	// Add Dockerfile
	dockerfilePath := config.Dockerfile
	if dockerfilePath == "" {
		dockerfilePath = "Dockerfile"
	}

	// CRITICAL FIX: If we copied the context, update dockerfile path
	// The dockerfile should be relative to the NEW buildContext location
	if buildContext != ctx.Path {
		// Context was copied to temp directory
		// Make dockerfile path relative to the ORIGINAL context first
		if filepath.IsAbs(dockerfilePath) {
			// Try to make it relative to original context
			if relPath, err := filepath.Rel(ctx.Path, dockerfilePath); err == nil {
				dockerfilePath = relPath
			}
		}
		// Now dockerfilePath is relative to context (e.g. "Dockerfile")
		// and will work with the copied buildContext
	} else {
		// Context not copied, use normal relative path logic
		if filepath.IsAbs(dockerfilePath) {
			relPath, err := filepath.Rel(buildContext, dockerfilePath)
			if err == nil {
				dockerfilePath = relPath
			}
		}
	}

	args = append(args, "--opt", fmt.Sprintf("filename=%s", dockerfilePath))

	// Add context
	// Add context
	args = append(args, "--local", fmt.Sprintf("context=%s", buildContext))
	args = append(args, "--local", fmt.Sprintf("dockerfile=%s", buildContext))

	// ========================================
	// REPRODUCIBLE BUILDS: Sort build arguments
	// ========================================
	buildArgKeys := make([]string, 0, len(config.BuildArgs))
	for key := range config.BuildArgs {
		buildArgKeys = append(buildArgKeys, key)
	}
	sort.Strings(buildArgKeys)

	for _, key := range buildArgKeys {
		value := config.BuildArgs[key]
		if value != "" {
			args = append(args, "--opt", fmt.Sprintf("build-arg:%s=%s", key, value))
		} else {
			// Use environment variable
			args = append(args, "--opt", fmt.Sprintf("build-arg:%s", key))
		}
	}

	// ========================================
	// REPRODUCIBLE BUILDS: Sort labels
	// ========================================
	labelKeys := make([]string, 0, len(config.Labels))
	for key := range config.Labels {
		labelKeys = append(labelKeys, key)
	}
	sort.Strings(labelKeys)

	for _, key := range labelKeys {
		value := config.Labels[key]
		args = append(args, "--opt", fmt.Sprintf("label:%s=%s", key, value))
	}

	// Add target if specified
	if config.Target != "" {
		args = append(args, "--opt", fmt.Sprintf("target=%s", config.Target))
	}

	// Add platform if specified
	if config.CustomPlatform != "" {
		args = append(args, "--opt", fmt.Sprintf("platform=%s", config.CustomPlatform))
	}

	// ========================================
	// REPRODUCIBLE BUILDS: Add source-date-epoch
	// ========================================
	// BuildKit requires TWO settings for reproducible builds:
	// 1. source-date-epoch: Sets the image creation timestamp
	// 2. rewrite-timestamp=true: Rewrites all file timestamps in layers (added to output below)
	var sourceEpoch string
	if epoch := os.Getenv("SOURCE_DATE_EPOCH"); epoch != "" {
		sourceEpoch = epoch
		args = append(args, "--opt", fmt.Sprintf("source-date-epoch=%s", epoch))
		args = append(args, "--opt", fmt.Sprintf("build-arg:SOURCE_DATE_EPOCH=%s", sourceEpoch))
		logger.Debug("Using SOURCE_DATE_EPOCH=%s from environment", epoch)
	} else if config.Reproducible {
		sourceEpoch = "0"
		args = append(args, "--opt", "source-date-epoch=0")
		args = append(args, "--opt", fmt.Sprintf("build-arg:SOURCE_DATE_EPOCH=%s", sourceEpoch))
		logger.Debug("Setting source-date-epoch=0 for reproducible build")
	}

	// ========================================
	// REPRODUCIBLE BUILDS: Cache control
	// ========================================
	if !config.Cache || config.Reproducible {
		args = append(args, "--no-cache")
		if config.Reproducible {
			logger.Debug("Cache disabled for reproducible build")
		}
	}

	// ========================================
	// REPRODUCIBLE BUILDS: Sort destinations
	// ========================================
	sortedDests := make([]string, len(config.Destination))
	copy(sortedDests, config.Destination)
	sort.Strings(sortedDests)

	// Handle outputs (destinations)
	if config.TarPath != "" {
		// Export to tar
		outputOpts := fmt.Sprintf("type=docker,dest=%s", config.TarPath)
		if config.Reproducible && sourceEpoch != "" {
			outputOpts += ",rewrite-timestamp=true"
			logger.Debug("Added rewrite-timestamp=true for reproducible tar export")
		}
		args = append(args, "--output", outputOpts)
	} else if !config.NoPush {
		// Push to registries
		for _, dest := range sortedDests {
			outputOpts := fmt.Sprintf("type=image,name=%s,push=true", dest)
			if config.Reproducible && sourceEpoch != "" {
				outputOpts += ",rewrite-timestamp=true"
				logger.Debug("Added rewrite-timestamp=true for reproducible push: %s", dest)
			}
			args = append(args, "--output", outputOpts)
		}
	} else {
		// Build only, no push (store in local image store)
		for _, dest := range sortedDests {
			outputOpts := fmt.Sprintf("type=image,name=%s,push=false", dest)
			if config.Reproducible && sourceEpoch != "" {
				outputOpts += ",rewrite-timestamp=true"
				logger.Debug("Added rewrite-timestamp=true for reproducible build: %s", dest)
			}
			args = append(args, "--output", outputOpts)
		}
	}

	// Log the command
	logger.Debug("BuildKit command: buildctl %s", strings.Join(args, " "))

	// Execute buildctl with output capture to extract digest
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd := exec.Command("buildctl", args...)
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	cmd.Env = os.Environ()

	// Set BUILDKIT_HOST
	cmd.Env = append(cmd.Env, fmt.Sprintf("BUILDKIT_HOST=unix://%s", buildkitSocket))

	// Enhanced environment setup for auth
	if authFile != "" {
		cmd.Env = append(cmd.Env,
			fmt.Sprintf("DOCKER_CONFIG=%s", filepath.Dir(authFile)),
		)
	}

	// ========================================
	// REPRODUCIBLE BUILDS: Set SOURCE_DATE_EPOCH environment
	// ========================================
	if sourceEpoch != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("SOURCE_DATE_EPOCH=%s", sourceEpoch))
		logger.Debug("Set SOURCE_DATE_EPOCH=%s in build environment", sourceEpoch)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("buildkit build failed: %v", err)
	}

	logger.Info("Build completed successfully")

	// ========================================
	// REPRODUCIBLE BUILDS: Extract and log digest from BuildKit output
	// ========================================
	if !config.NoPush && len(config.Destination) > 0 {
		// Try to extract digest from stderr output
		// BuildKit outputs manifest digest in stderr like: "exporting manifest sha256:xxxxx"
		stderrOutput := stderrBuf.String()
		stdoutOutput := stdoutBuf.String()
		
		for _, dest := range config.Destination {
			var digest string
			
			// Pattern 1: Look for "exporting manifest sha256:xxx" in stderr
			if lines := strings.Split(stderrOutput, "\n"); len(lines) > 0 {
				for _, line := range lines {
					if strings.Contains(line, "exporting manifest sha256:") {
						parts := strings.Fields(line)
						for _, part := range parts {
							if strings.HasPrefix(part, "sha256:") {
								digest = part
								break
							}
						}
					}
					if digest != "" {
						break
					}
				}
			}
			
			// Pattern 2: Look for digest in stdout (some buildkit versions)
			if digest == "" {
				if lines := strings.Split(stdoutOutput, "\n"); len(lines) > 0 {
					for _, line := range lines {
						if strings.Contains(line, "sha256:") {
							parts := strings.Fields(line)
							for _, part := range parts {
								if strings.HasPrefix(part, "sha256:") && len(part) == 71 { // sha256: + 64 chars
									digest = part
									break
								}
							}
						}
						if digest != "" {
							break
						}
					}
				}
			}
			
			if digest != "" {
				logger.Debug("Extracted digest for %s: %s", dest, digest)
				logger.Debug("Using digest from push output: %s", digest)
			} else {
				logger.Debug("Could not extract digest from BuildKit output for %s", dest)
			}
		}
	}

	// Note: BuildKit handles push as part of the build, so no separate push step needed
	// Digest file export for BuildKit
	if config.DigestFile != "" || config.ImageNameWithDigestFile != "" {
		logger.Warning("Digest file export not yet implemented for BuildKit")
	}

	return nil
}

// exportToTar exports the built image to a tar file (Buildah only)
func exportToTar(config Config) error {
	logger.Info("Exporting image to TAR: %s", config.TarPath)

	if len(config.Destination) == 0 {
		return fmt.Errorf("no destination specified for TAR export")
	}

	image := config.Destination[0]

	// Method 1: Try direct buildah push (works for VFS and newer buildah versions)
	logger.Debug("Attempting TAR export with buildah push...")
	cmd := exec.Command("buildah", "push", image, fmt.Sprintf("docker-archive:%s", config.TarPath))

	var stderr strings.Builder
	cmd.Stdout = os.Stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		logger.Debug("Direct buildah push failed: %v", err)
		logger.Debug("Stderr: %s", stderr.String())

		// Method 2: Try with image ID instead of name (most reliable for overlay)
		logger.Debug("Attempting with image ID...")
		getIDCmd := exec.Command("buildah", "images", "--format", "{{.ID}}", "--filter", fmt.Sprintf("reference=%s", image))
		idOutput, idErr := getIDCmd.Output()

		if idErr == nil && len(strings.TrimSpace(string(idOutput))) > 0 {
			imageID := strings.TrimSpace(string(idOutput))
			logger.Debug("Found image ID: %s", imageID)

			cmd2 := exec.Command("buildah", "push", imageID, fmt.Sprintf("docker-archive:%s", config.TarPath))
			cmd2.Stdout = os.Stdout
			cmd2.Stderr = os.Stderr

			if err2 := cmd2.Run(); err2 != nil {
				return fmt.Errorf("TAR export failed with both name and ID:\n  by name: %v\n  by ID: %v", err, err2)
			}
			logger.Info("Successfully exported using image ID")
		} else {
			// Method 3: List all images and find a match
			logger.Debug("Image ID lookup failed, searching all images...")
			listCmd := exec.Command("buildah", "images", "--format", "{{.ID}}:{{.Names}}")
			listOutput, listErr := listCmd.Output()

			if listErr == nil {
				lines := strings.Split(string(listOutput), "\n")
				for _, line := range lines {
					if strings.Contains(line, image) {
						parts := strings.Split(line, ":")
						if len(parts) >= 2 {
							foundID := strings.TrimSpace(parts[0])
							logger.Debug("Found matching image ID from list: %s", foundID)

							cmd3 := exec.Command("buildah", "push", foundID, fmt.Sprintf("docker-archive:%s", config.TarPath))
							cmd3.Stdout = os.Stdout
							cmd3.Stderr = os.Stderr

							if err3 := cmd3.Run(); err3 != nil {
								return fmt.Errorf("TAR export failed with all methods:\n  by name: %v\n  by ID lookup: %v\n  by search: %v", err, idErr, err3)
							}
							logger.Info("Successfully exported using searched image ID")
							goto success
						}
					}
				}
			}

			return fmt.Errorf("failed to export to tar: could not find image %s\n  direct push error: %v\n  ID lookup error: %v", image, err, idErr)
		}
	} else {
		logger.Info("Successfully exported using direct buildah push")
	}

success:
	logger.Info("Image exported to: %s", config.TarPath)

	// Verify the tar file was created and is not empty
	if info, err := os.Stat(config.TarPath); err != nil {
		return fmt.Errorf("TAR file was not created: %v", err)
	} else if info.Size() == 0 {
		return fmt.Errorf("TAR file is empty")
	} else {
		logger.Debug("TAR file size: %d bytes", info.Size())
	}

	return nil
}

// SaveDigestInfo saves image digest information to files (Buildah only)
// The digest should be obtained from the push operation output
func SaveDigestInfo(config Config, digestMap map[string]string) error {
	if len(config.Destination) == 0 || len(digestMap) == 0 {
		return nil
	}

	// Use the first destination's digest
	image := config.Destination[0]
	digest, ok := digestMap[image]
	if !ok {
		logger.Debug("No digest available for %s", image)
		return nil
	}

	logger.Debug("Using digest from push output: %s", digest)

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

// copyDir recursively copies a directory from src to dst
func copyDir(src, dst string) error {
	// Get source directory info
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("failed to stat source: %v", err)
	}

	// Create destination directory
	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return fmt.Errorf("failed to create destination: %v", err)
	}

	// Read directory entries
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("failed to read directory: %v", err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			// Recursively copy subdirectory
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			// Copy file
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyFile copies a single file from src to dst
func copyFile(src, dst string) error {
	// Get source file info for permissions
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("failed to stat source file: %v", err)
	}

	// Read source file
	srcData, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to read source: %v", err)
	}

	// Write to destination with same permissions
	if err := os.WriteFile(dst, srcData, srcInfo.Mode()); err != nil {
		return fmt.Errorf("failed to write destination: %v", err)
	}

	return nil
}