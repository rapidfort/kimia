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
	"github.com/rapidfort/kimia/internal/auth"
	"github.com/rapidfort/kimia/pkg/logger"
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
	Timestamp    string

	// Attestation and signing (BuildKit only)
	// Level 1: Simple mode (backward compatible)
	Attestation string // "off", "min" or "max"
	
	// Level 2: Docker-style attestations (advanced)
	AttestationConfigs []AttestationConfig
	
	// Level 3: Direct BuildKit options (escape hatch)
	BuildKitOpts []string
	
	// Signing
	Sign              bool   // Enable signing with cosign
	CosignKeyPath     string // Path to cosign private key
	CosignPasswordEnv string // Environment variable for cosign password
}

// AttestationConfig represents a single --attest flag
type AttestationConfig struct {
	Type   string            // "sbom" or "provenance"
	Params map[string]string // Key-value pairs from the flag
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
func Execute(config Config, ctx *Context) error {
	builder := DetectBuilder()

	if builder == "unknown" {
		return fmt.Errorf("no builder found (expected buildkitd or buildah)")
	}

	logger.Info("Using builder: %s", strings.ToUpper(builder))

	if builder == "buildkit" {
		return executeBuildKit(config, ctx)
	}

	return executeBuildah(config, ctx)
}

// executeBuildah executes a buildah build with authentication
func executeBuildah(config Config, ctx *Context) error {
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
	// Config.Timestamp is already set by args.go with proper precedence
	var sourceEpoch string
	if config.Reproducible && config.Timestamp != "" {
		sourceEpoch = config.Timestamp
    
    	// 1. Set timestamp for image metadata
    	args = append(args, "--timestamp", sourceEpoch)
    
    	// 2. Pass as build arg so Dockerfile can use it
    	//args = append(args, "--build-arg", fmt.Sprintf("SOURCE_DATE_EPOCH=%s", sourceEpoch))
    
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

	// Set DOCKER_CONFIG for authentication
	dockerConfigDir := auth.GetDockerConfigDir()
	cmd.Env = append(cmd.Env, fmt.Sprintf("DOCKER_CONFIG=%s", dockerConfigDir))

	// Storage driver configuration
	storageDriver := config.StorageDriver
	if storageDriver != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("STORAGE_DRIVER=%s", storageDriver))
		logger.Debug("Set STORAGE_DRIVER=%s", storageDriver)
	}

	// Print environment AFTER all variables are set
	logger.Info("Buildah build environment:")
	for _, env := range cmd.Env {
		if strings.HasPrefix(env, "STORAGE_DRIVER=") ||
			strings.HasPrefix(env, "BUILDAH_") ||
			strings.HasPrefix(env, "DOCKER_CONFIG=") {
			logger.Info("  %s", env)
		}
	}

	// Log the command being executed
	logger.Info("Executing: buildah %s", strings.Join(sanitizeCommandArgs(args), " "))

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

func executeBuildKit(config Config, ctx *Context) error {
	logger.Info("Starting BuildKit build...")

	// ========================================
	// SETUP: Environment and paths
	// ========================================
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		homeDir = "/home/kimia"
	}

	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntimeDir == "" {
		xdgRuntimeDir = "/tmp/run"
	}

	buildkitSocket := filepath.Join(xdgRuntimeDir, "buildkitd.sock")
	buildkitConfig := filepath.Join(homeDir, ".config/buildkit/buildkitd.toml")

	logger.Debug("BuildKit configuration:")
	logger.Debug("  HOME: %s", homeDir)
	logger.Debug("  XDG_RUNTIME_DIR: %s", xdgRuntimeDir)
	logger.Debug("  BUILDKIT_HOST: unix://%s", buildkitSocket)
	logger.Debug("  Config file: %s", buildkitConfig)

	// ========================================
	// CONTEXT HANDLING: Use Git URL for BuildKit or copy bind mounts to real filesystem
	// ========================================
	var buildContext string
	var isGitContext bool
	var tempContext string
	workspaceMount := filepath.Join(homeDir, "workspace")

	// Check if this is a Git context (BuildKit native Git support)
	if ctx.IsGitRepo && ctx.GitURL != "" {
		logger.Info("Using BuildKit native Git context (no local clone)")
		isGitContext = true
		
		// Format Git URL with authentication, branch/revision, and subcontext
		formattedURL, err := FormatGitURLForBuildKit(ctx.GitURL, ctx.GitConfig, ctx.SubContext)
		if err != nil {
			return fmt.Errorf("failed to format Git URL for BuildKit: %v", err)
		}
		buildContext = formattedURL
	} else {
		// Local context handling
		buildContext = ctx.Path
		
		// Only copy if it's a bind mount, not a git clone
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
		} else {
			logger.Debug("Using original context at: %s", buildContext)
		}
	}

	// ========================================
	// INSECURE REGISTRY CONFIGURATION
	// ========================================
	if config.Insecure || len(config.InsecureRegistry) > 0 {
		// Read existing config (should always exist from Dockerfile)
		var existingConfig string
		if data, err := os.ReadFile(buildkitConfig); err == nil {
			existingConfig = string(data)
			logger.Debug("Read existing buildkit config from: %s", buildkitConfig)
		} else {
			// Fallback: match what's in Dockerfile (should rarely happen)
			existingConfig = `[worker.oci]
  enabled = true
  rootless = true
  binary = "crun"
  noProcessSandbox = true
`
			logger.Debug("Config file not found, using default (matches Dockerfile)")
			
			// Create config directory if it doesn't exist
			configDir := filepath.Dir(buildkitConfig)
			if err := os.MkdirAll(configDir, 0755); err != nil {
				return fmt.Errorf("failed to create buildkit config directory: %v", err)
			}
		}

		// Collect all registries that need insecure config
		registries := make(map[string]bool)
		
		// If --insecure is set, add all destination registries
		if config.Insecure {
			for _, dest := range config.Destination {
				if idx := strings.Index(dest, "/"); idx > 0 {
					registry := dest[:idx]
					registries[registry] = true
				}
			}
		}
		
		// Add specific insecure registries from --insecure-registry
		for _, registry := range config.InsecureRegistry {
			registries[registry] = true
		}

		// Append insecure config for each registry
		configContent := existingConfig
		configModified := false

		for registry := range registries {
			if !strings.Contains(existingConfig, fmt.Sprintf(`[registry."%s"]`, registry)) {
				configContent += fmt.Sprintf(`
[registry."%s"]
  http = true
  insecure = true
`, registry)
				logger.Info("Adding insecure registry: %s", registry)
				configModified = true
			} else {
				logger.Debug("Registry already configured: %s", registry)
			}
		}

		// Only write if we modified it
		if configModified {
			if err := os.WriteFile(buildkitConfig, []byte(configContent), 0644); err != nil {
				return fmt.Errorf("failed to write buildkit config: %v", err)
			}
			logger.Debug("Updated buildkit config written to: %s", buildkitConfig)
		} else {
			logger.Debug("No changes needed to buildkit config")
		}
	}

	// ========================================
	// START BUILDKITD DAEMON
	// ========================================
	logger.Debug("Starting buildkitd with rootlesskit...")
	daemonCmd := exec.Command(
		"rootlesskit",
		"--state-dir="+filepath.Join(xdgRuntimeDir, "rk-buildkit"),
		"--net=host",
		"--copy-up=/home",  // <-- rootlesskit creates new mount namespaces.
		"--disable-host-loopback",
		"buildkitd",
		"--config="+buildkitConfig,
		"--addr=unix://"+buildkitSocket,
	)

	daemonCmd.Env = append(os.Environ(),
		"HOME=/home/kimia",
		"DOCKER_CONFIG=/home/kimia/.docker",
		"XDG_RUNTIME_DIR=/tmp/run",
	)

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

	// ========================================
	// WAIT FOR BUILDKITD TO BE READY
	// ========================================
	logger.Debug("Waiting for buildkitd to be ready...")
	ready := false
	for i := 0; i < 30; i++ {
		checkCmd := exec.Command("buildctl", "--addr=unix://"+buildkitSocket, "debug", "info")
		output, err := checkCmd.CombinedOutput()

		if err == nil {
			ready = true
			break
		}

		logger.Debug("Waiting for buildkitd... (%d/30) - error: %v", i+1, err)
		if len(output) > 0 {
			logger.Debug("  Output: %s", string(output))
		}

		// Check if daemon is still running
		if daemonCmd.Process == nil {
			return fmt.Errorf("buildkitd process died")
		}

		time.Sleep(1 * time.Second)
	}

	if !ready {
		return fmt.Errorf("buildkitd failed to become ready after 30 seconds")
	}

	logger.Debug("buildkitd is ready")

	// ========================================
	// BUILD BUILDCTL COMMAND
	// ========================================
	args := []string{"build", "--frontend", "dockerfile.v0"}

	// Add Dockerfile
	dockerfilePath := config.Dockerfile
	if dockerfilePath == "" {
		dockerfilePath = "Dockerfile"
	}

	// Handle dockerfile path for copied contexts
	if !isGitContext && buildContext != ctx.Path {
		// Context was copied to temp directory
		if filepath.IsAbs(dockerfilePath) {
			if relPath, err := filepath.Rel(ctx.Path, dockerfilePath); err == nil {
				dockerfilePath = relPath
			}
		}
	} else if !isGitContext {
		// Context not copied, use normal relative path logic
		if filepath.IsAbs(dockerfilePath) {
			relPath, err := filepath.Rel(buildContext, dockerfilePath)
			if err == nil {
				dockerfilePath = relPath
			}
		}
	}

	args = append(args, "--opt", fmt.Sprintf("filename=%s", dockerfilePath))

	// Add context: Git URL or local path
	if isGitContext {
		// Use Git URL for BuildKit native Git support
		// BuildKit requires Git URLs to be passed as --opt context=
		logger.Debug("Using Git context: %s", logger.SanitizeGitURL(buildContext))
		args = append(args, "--opt", fmt.Sprintf("context=%s", buildContext))
		args = append(args, "--opt", fmt.Sprintf("dockerfile=%s", buildContext))
	} else {
		// Use local context
		logger.Debug("Using local context: %s", buildContext)
		args = append(args, "--local", fmt.Sprintf("context=%s", buildContext))
		args = append(args, "--local", fmt.Sprintf("dockerfile=%s", buildContext))
	}

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
	// 2. rewrite-timestamp=true: Rewrites all file timestamps in layers
	var sourceEpoch string
	if config.Reproducible && config.Timestamp != "" {
		sourceEpoch = config.Timestamp
		args = append(args, "--opt", fmt.Sprintf("source-date-epoch=%s", sourceEpoch))
		args = append(args, "--opt", fmt.Sprintf("build-arg:SOURCE_DATE_EPOCH=%s", sourceEpoch))
		logger.Debug("Using timestamp=%s for reproducible build", sourceEpoch)
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

	// ========================================
	// OUTPUT CONFIGURATION
	// ========================================
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
		// Build only, no push
		for _, dest := range sortedDests {
			outputOpts := fmt.Sprintf("type=image,name=%s,push=false", dest)
			if config.Reproducible && sourceEpoch != "" {
				outputOpts += ",rewrite-timestamp=true"
				logger.Debug("Added rewrite-timestamp=true for reproducible build: %s", dest)
			}
			args = append(args, "--output", outputOpts)
		}
	}

	// ========================================
	// ATTESTATION: Configure attestations for BuildKit
	// ========================================
	
	// Determine which attestation mode to use
	var attestOpts []string
	
	if len(config.AttestationConfigs) > 0 {
		// Level 2: Docker-style attestations
		attestOpts = buildAttestationOptsFromConfigs(config.AttestationConfigs, &args)
		logger.Info("Attestation mode: advanced (--attest)")
	} else if config.Attestation != "off" && config.Attestation != "" {
		// Level 1: Simple mode
		attestOpts = buildAttestationOptsFromSimpleMode(config.Attestation)
		logger.Info("Attestation mode: %s", config.Attestation)
	} else {
		// No attestations
		logger.Debug("Attestations disabled")
	}
	
	// Add attestation options to args
	for _, opt := range attestOpts {
		args = append(args, "--opt", opt)
	}
	
	// Level 3: Direct BuildKit options (pass-through)
	for _, opt := range config.BuildKitOpts {
		args = append(args, "--opt", opt)
		logger.Debug("Added direct BuildKit opt: %s", opt)
	}

	// ========================================
	// EXECUTE BUILDCTL
	// ========================================
	// Create command with output capture for digest extraction
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd := exec.Command("buildctl", args...)
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	cmd.Env = os.Environ()

	// Set BUILDKIT_HOST
	cmd.Env = append(cmd.Env, fmt.Sprintf("BUILDKIT_HOST=unix://%s", buildkitSocket))

	// Set DOCKER_CONFIG for authentication
	dockerConfigDir := auth.GetDockerConfigDir()
	cmd.Env = append(cmd.Env, fmt.Sprintf("DOCKER_CONFIG=%s", dockerConfigDir))

	// Set SOURCE_DATE_EPOCH for reproducible builds
	if sourceEpoch != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("SOURCE_DATE_EPOCH=%s", sourceEpoch))
	}

	// Log environment variables
	logger.Info("BuildKit build environment:")
	for _, env := range cmd.Env {
		if strings.HasPrefix(env, "BUILDKIT_HOST=") ||
			strings.HasPrefix(env, "DOCKER_CONFIG=") ||
			strings.HasPrefix(env, "SOURCE_DATE_EPOCH=") {
			logger.Info("  %s", env)
		}
	}

	// Log the command being executed
	logger.Info("Executing: buildctl %s", strings.Join(sanitizeCommandArgs(args), " "))

	// BuildKit may log Git credentials in logs -- warn users accordingly
	if isGitContext && strings.Contains(buildContext, "@") {
		logger.Warning("BuildKit may expose Git credentials in build logs. Consider using SSH authentication instead of HTTPS tokens for better security.")
	}

	// Execute build
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("buildkit build failed: %v", err)
	}

	logger.Info("Build completed successfully")

	// ========================================
	// REPRODUCIBLE BUILDS: Extract digest from output
	// ========================================
	digestMap := make(map[string]string) // Map tag -> digest
	
	if !config.NoPush && len(config.Destination) > 0 {
		stderrOutput := stderrBuf.String()
		stdoutOutput := stdoutBuf.String()

		for _, dest := range config.Destination {
			var digest string

			// Pattern 1: Look for "exporting manifest list sha256:xxx" in stderr (PRIORITY)
			// This is the correct digest to sign when attestations are present
			for _, line := range strings.Split(stderrOutput, "\n") {
				if strings.Contains(line, "exporting manifest list sha256:") {
					parts := strings.Fields(line)
					for _, part := range parts {
						if strings.HasPrefix(part, "sha256:") {
							digest = part
							logger.Debug("Found manifest list digest: %s", digest)
							break
						}
					}
				}
				if digest != "" {
					break
				}
			}

			// Pattern 2: Look for "exporting manifest sha256:xxx" in stderr (fallback)
			// This is for images without attestations (single manifest)
			if digest == "" {
				for _, line := range strings.Split(stderrOutput, "\n") {
					if strings.Contains(line, "exporting manifest sha256:") {
						parts := strings.Fields(line)
						for _, part := range parts {
							if strings.HasPrefix(part, "sha256:") {
								digest = part
								logger.Debug("Found platform manifest digest: %s", digest)
								break
							}
						}
					}
					if digest != "" {
						break
					}
				}
			}

			// Pattern 3: Look for digest in stdout (last resort fallback)
			if digest == "" {
				for _, line := range strings.Split(stdoutOutput, "\n") {
					if strings.Contains(line, "sha256:") {
						parts := strings.Fields(line)
						for _, part := range parts {
							if strings.HasPrefix(part, "sha256:") && len(part) == 71 {
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

			if digest != "" {
				logger.Debug("Extracted digest for %s: %s", dest, digest)
				digestMap[dest] = digest
			} else {
				logger.Debug("Could not extract digest from BuildKit output for %s", dest)
			}
		}
	}

	// ========================================
	// SIGNING: Sign images with cosign if requested
	// ========================================
	if config.Sign && !config.NoPush {
		if config.CosignKeyPath == "" {
			logger.Warning("Signing requested but no cosign key provided (--cosign-key), skipping signature")
		} else {
			logger.Info("Signing images with cosign...")
			
			for _, dest := range config.Destination {
				// Use digest-based reference if available
				imageToSign := dest
				if digest, ok := digestMap[dest]; ok {
					// Remove tag but keep registry:port/repo
					// Find the last ':' which separates the tag
					if idx := strings.LastIndex(dest, ":"); idx > 0 {
						// Check if this is actually a tag (not a port number)
						// Port numbers come before '/', tags come after
						afterColon := dest[idx+1:]
						if !strings.Contains(afterColon, "/") {
							// This is a tag, remove it
							imageToSign = dest[:idx] + "@" + digest
						} else {
							// This is a port, append digest to the full reference
							imageToSign = dest + "@" + digest
						}
					} else {
						imageToSign = dest + "@" + digest
					}
					logger.Info("Signing with digest reference: %s", imageToSign)
				} else {
					logger.Warning("No digest found for %s, signing with tag (not recommended)", dest)
				}
				
				if err := signImageWithCosign(imageToSign, config); err != nil {
					return fmt.Errorf("failed to sign image %s: %v", imageToSign, err)
				}
				logger.Info("Successfully signed: %s", imageToSign)
			}
		}
	}

	// ========================================
	// DIGEST FILE EXPORT (TODO)
	// ========================================
	if config.DigestFile != "" || config.ImageNameWithDigestFile != "" {
		logger.Warning("Digest file export not yet implemented for BuildKit")
	}

	return nil
}

// exportToTar exports the built image to a tar file (Buildah only)
func exportToTar(config Config) error {
	logger.Info("Exporting image to TAR: %s", config.TarPath)

	// Ensure Docker config exists - buildah requires a credentials file
	// even for local tar export operations
	dockerConfigDir := auth.GetDockerConfigDir()
	configPath := filepath.Join(dockerConfigDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := os.MkdirAll(dockerConfigDir, 0755); err != nil {
			return fmt.Errorf("failed to create Docker config directory: %v", err)
		}
		emptyConfig := []byte(`{"auths":{}}`)
		if err := os.WriteFile(configPath, emptyConfig, 0600); err != nil {
			return fmt.Errorf("failed to create empty Docker config: %v", err)
		}
		logger.Debug("Created empty Docker config for tar export")
	}

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
		// Strip the tag but preseve the host:port, so we are stripping at the last colon
		imageName := image
		if lastSlash := strings.LastIndex(image, "/"); lastSlash != -1 {
			if lastColon := strings.LastIndex(image, ":"); lastColon > lastSlash {
				imageName = image[:lastColon]
			}
		} else if lastColon := strings.LastIndex(image, ":"); lastColon != -1 {
			imageName = image[:lastColon]
		}
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

// buildAttestationOptsFromSimpleMode converts simple mode to BuildKit opts
func buildAttestationOptsFromSimpleMode(mode string) []string {
	var opts []string
	
	switch mode {
	case "min":
		// Provenance only, minimal info
		// CRITICAL: Explicitly disable SBOM to fix bug where BuildKit enables it by default
		opts = append(opts, "attest:sbom=false")
		opts = append(opts, "attest:provenance=mode=min")
		logger.Debug("Simple mode 'min': provenance only (SBOM explicitly disabled)")
		
	case "max":
		// SBOM + Provenance, maximum info
		opts = append(opts, "attest:sbom=true")
		opts = append(opts, "attest:provenance=mode=max")
		logger.Debug("Simple mode 'max': SBOM + provenance")
		
	default:
		logger.Fatal("Invalid attestation mode: %s", mode)
	}
	
	return opts
}

// buildAttestationOptsFromConfigs converts --attest configs to BuildKit opts
func buildAttestationOptsFromConfigs(configs []AttestationConfig, args *[]string) []string {
	var opts []string
	
	for _, config := range configs {
		switch config.Type {
		case "sbom":
			opt := buildSBOMOpt(config)
			opts = append(opts, opt)
			
			// Handle scan options as build args
			if config.Params["scan-context"] == "true" {
				*args = append(*args, "--opt", "build-arg:BUILDKIT_SBOM_SCAN_CONTEXT=1")
				logger.Debug("Added SBOM scan build arg: BUILDKIT_SBOM_SCAN_CONTEXT=1")
			}
			if config.Params["scan-stage"] == "true" {
				*args = append(*args, "--opt", "build-arg:BUILDKIT_SBOM_SCAN_STAGE=1")
				logger.Debug("Added SBOM scan build arg: BUILDKIT_SBOM_SCAN_STAGE=1")
			}
			
		case "provenance":
			opts = append(opts, buildProvenanceOpt(config))
		default:
			logger.Fatal("Unknown attestation type: %s", config.Type)
		}
	}
	
	return opts
}

// buildSBOMOpt builds a single SBOM attestation opt
func buildSBOMOpt(config AttestationConfig) string {
	// If no params, just enable with defaults
	if len(config.Params) == 0 {
		return "attest:sbom=true"
	}
	
	// Build comma-separated params
	var parts []string
	
	// Special handling for generator param
	if generator, ok := config.Params["generator"]; ok {
		parts = append(parts, fmt.Sprintf("generator=%s", generator))
	} else {
		parts = append(parts, "true") // Enable with default generator
	}
	
	// Add any other params as-is (except scan-context and scan-stage which are handled separately)
	for key, value := range config.Params {
		if key != "generator" && key != "scan-context" && key != "scan-stage" {
			parts = append(parts, fmt.Sprintf("%s=%s", key, value))
		}
	}
	
	return fmt.Sprintf("attest:sbom=%s", strings.Join(parts, ","))
}

// buildProvenanceOpt builds a single provenance attestation opt
func buildProvenanceOpt(config AttestationConfig) string {
	var parts []string
	
	// Mode (default to max if not specified)
	mode := config.Params["mode"]
	if mode == "" {
		mode = "max"
	}
	parts = append(parts, fmt.Sprintf("mode=%s", mode))
	
	// Add all other parameters in a consistent order
	paramOrder := []string{"builder-id", "reproducible", "inline-only", "version", "filename"}
	for _, key := range paramOrder {
		if value, ok := config.Params[key]; ok {
			parts = append(parts, fmt.Sprintf("%s=%s", key, value))
		}
	}
	
	// Add any remaining params not in the order list
	for key, value := range config.Params {
		if key != "mode" && !contains(paramOrder, key) {
			parts = append(parts, fmt.Sprintf("%s=%s", key, value))
		}
	}
	
	return fmt.Sprintf("attest:provenance=%s", strings.Join(parts, ","))
}

// contains checks if a string slice contains a specific item
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// signImageWithCosign signs a container image using cosign
func signImageWithCosign(image string, config Config) error {
	logger.Debug("Signing image with cosign: %s", image)

	// Prepare cosign command
	args := []string{"sign", "--key", config.CosignKeyPath}

	// Add insecure registry flag if needed
	if config.Insecure || len(config.InsecureRegistry) > 0 {
		args = append(args, "--allow-insecure-registry")
		logger.Debug("Added --allow-insecure-registry flag for insecure registry")
	}

	// Add the image reference
	args = append(args, image)

	// Create the command
	cmd := exec.Command("cosign", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	
	cmd.Env = append(cmd.Env, "COSIGN_EXPERIMENTAL=1")

	// Set cosign password from environment variable if specified
	if config.CosignPasswordEnv != "" {
		password := os.Getenv(config.CosignPasswordEnv)
		if password == "" {
			logger.Warning("Cosign password environment variable %s is not set or empty", config.CosignPasswordEnv)
		} else {
			cmd.Env = append(cmd.Env, fmt.Sprintf("COSIGN_PASSWORD=%s", password))
			logger.Debug("Set COSIGN_PASSWORD from %s", config.CosignPasswordEnv)
		}
	}

	// Log the command being executed
	logger.Debug("Executing: cosign %s", strings.Join(sanitizeCommandArgs(args), " "))

	// Execute cosign
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cosign signing failed: %v", err)
	}

	return nil
}

// sanitizeCommandArgs removes credentials from Git URLs and sensitive build-args
func sanitizeCommandArgs(args []string) []string {
	// List of build-arg names that contain sensitive data
	sensitiveArgs := []string{
		"GIT_PASSWORD",
		"GIT_TOKEN",
		"PASSWORD",
		"TOKEN",
		"API_KEY",
		"SECRET",
		"CREDENTIALS",
	}
	
	sanitized := make([]string, len(args))
	for i, arg := range args {
		if strings.HasPrefix(arg, "context=") || strings.HasPrefix(arg, "dockerfile=") {
			// Handle --opt context=URL or --opt dockerfile=URL format
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) == 2 {
				sanitized[i] = parts[0] + "=" + logger.SanitizeGitURL(parts[1])
			} else {
				sanitized[i] = arg
			}
		} else if strings.HasPrefix(arg, "build-arg:") {
			// Handle --opt build-arg:KEY=VALUE format
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) == 2 {
				argName := strings.TrimPrefix(parts[0], "build-arg:")
				// Check if this is a sensitive build arg
				isSensitive := false
				for _, sensitive := range sensitiveArgs {
					if strings.Contains(strings.ToUpper(argName), sensitive) {
						isSensitive = true
						break
					}
				}
				if isSensitive {
					sanitized[i] = parts[0] + "=***REDACTED***"
				} else {
					sanitized[i] = arg
				}
			} else {
				sanitized[i] = arg
			}
		} else {
			// For any other arg that might be a URL
			sanitized[i] = logger.SanitizeGitURL(arg)
		}
	}
	return sanitized
}