package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rapidfort/kimia/internal/auth"
	"github.com/rapidfort/kimia/internal/build"
	"github.com/rapidfort/kimia/internal/preflight"
	"github.com/rapidfort/kimia/pkg/logger"
)

func main() {
	// Handle version flag
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-version" || os.Args[1] == "version") {
		printVersion()
		os.Exit(0)
	}

	// Handle help flag
	if len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-help" || os.Args[1] == "help" || os.Args[1] == "-h") {
		printHelp()
		os.Exit(0)
	}

	// Handle check-environment command
	if len(os.Args) > 1 && os.Args[1] == "check-environment" {
		exitCode := preflight.CheckEnvironment()
		os.Exit(exitCode)
	}

	// Detect which builder is available (moved to build.Execute)
	// No need to detect here anymore - build.Execute handles it

	// Parse configuration
	config := parseArgs(os.Args[1:])

	// Log kimia version (builder will be logged by build.Execute)
	logger.Info("Kimia - Kubernetes-Native OCI Image Builder v%s", Version)
	logger.Debug("Build Date: %s, Commit: %s, Branch: %s", BuildDate, CommitSHA, Branch)

	// TODO
	// Validate storage driver only if specified
	// BuildKit supports: native, overlay, fuse-overlayfs
	// Buildah supports: vfs, overlay
	if config.StorageDriver != "" {
		validDrivers := []string{"vfs", "overlay", "fuse-overlayfs", "native"}
		storageDriver := strings.ToLower(config.StorageDriver)
		isValid := false
		for _, driver := range validDrivers {
			if storageDriver == driver {
				isValid = true
				break
			}
		}
		if !isValid {
			fmt.Fprintf(os.Stderr, "Error: Invalid storage driver '%s'\n", config.StorageDriver)
			fmt.Fprintf(os.Stderr, "Valid options:\n")
			fmt.Fprintf(os.Stderr, "  BuildKit: native, overlay, fuse-overlayfs\n")
			fmt.Fprintf(os.Stderr, "  Buildah:  vfs, overlay\n\n")
			os.Exit(1)
		}

		// Log storage driver selection
		logger.Info("Using storage driver: %s", storageDriver)
		
		switch storageDriver {
		case "overlay":
			logger.Info("Note: Overlay driver requires kernel 5.11+ and overlay filesystem support")
		case "fuse-overlayfs":
			logger.Info("Note: FUSE-overlayfs driver (recommended for rootless/Kubernetes environments)")
			// Check if fuse-overlayfs is available
			if _, err := exec.LookPath("fuse-overlayfs"); err != nil {
				logger.Warning("fuse-overlayfs binary not found. Install with: apk add fuse-overlayfs")
			}
		case "vfs":
			logger.Info("Note: VFS storage (Buildah only, slower but most compatible)")
		case "native":
			logger.Info("Note: Native snapshotter (BuildKit only, compatible but slower than overlay)")
		}
	}

	if config.Context == "" {
		fmt.Fprintf(os.Stderr, "Error: Kimia only supports BUILD mode\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  kimia --context=. --destination=registry/image:tag\n\n")
		fmt.Fprintf(os.Stderr, "Run 'kimia --help' for more information.\n")
		os.Exit(1)
	}

	// Check for enterprise-only flags
	if config.Scan {
		fmt.Fprintf(os.Stderr, "Error: --scan is an enterprise-only feature\n")
		fmt.Fprintf(os.Stderr, "This is the OSS version which supports build-only operations.\n")
		os.Exit(1)
	}

	if config.Harden {
		fmt.Fprintf(os.Stderr, "Error: --harden is an enterprise-only feature\n")
		fmt.Fprintf(os.Stderr, "This is the OSS version which supports build-only operations.\n")
		os.Exit(1)
	}

	// Validate build requirements
	if len(config.Destination) == 0 {
		fmt.Fprintf(os.Stderr, "Error: Build mode requires:\n")
		fmt.Fprintf(os.Stderr, "  --context: Build context (directory or Git URL)\n")
		fmt.Fprintf(os.Stderr, "  --destination: Target image name\n\n")
		fmt.Fprintf(os.Stderr, "Example:\n")
		fmt.Fprintf(os.Stderr, "  kimia --context=. --destination=registry/image:tag\n\n")
		os.Exit(1)
	}

	// Setup logging
	logger.Setup(config.Verbosity, config.LogTimestamp)

	// Detect which builder is available early (needed for context preparation)
	builder := build.DetectBuilder()
	if builder == "unknown" {
		logger.Fatal("No builder found (expected buildkitd or buildah)")
	}
	logger.Info("Detected builder: %s", strings.ToUpper(builder))

	// Prepare build context
	gitConfig := build.GitConfig{
		Context:   config.Context,
		Branch:    config.GitBranch,
		Revision:  config.GitRevision,
		TokenFile: config.GitTokenFile,
		TokenUser: config.GitTokenUser,
	}

	ctx, err := build.Prepare(gitConfig, builder)
	if err != nil {
		logger.Fatal("Failed to prepare build context: %v", err)
	}
	defer ctx.Cleanup()
	
	// Store SubContext in context for BuildKit Git URL formatting
	ctx.SubContext = config.SubContext

	// Apply context-sub-path for local contexts (not Git URLs)
	// For Git URLs with BuildKit, SubContext is handled in FormatGitURLForBuildKit
	if config.SubContext != "" && ctx.Path != "" {
		subPath := filepath.Join(ctx.Path, config.SubContext)
		
		// Verify the subdirectory exists
		if _, err := os.Stat(subPath); err != nil {
			logger.Fatal("Context sub-path does not exist: %s (full path: %s)", config.SubContext, subPath)
		}
		
		logger.Info("Using context sub-path: %s", config.SubContext)
		ctx.Path = subPath
	}

	// Setup authentication
	authSetup := auth.SetupConfig{
		Destinations:     config.Destination,
		InsecureRegistry: config.InsecureRegistry,
	}

	err = auth.Setup(authSetup)
	if err != nil {
		logger.Fatal("Failed to setup authentication: %v", err)
	}

	// Execute build based on detected builder
	buildConfig := build.Config{
		Dockerfile:                 config.Dockerfile,
		Destination:                config.Destination,
		Target:                     config.Target,
		BuildArgs:                  config.BuildArgs,
		Labels:                     config.Labels,
		CustomPlatform:             config.CustomPlatform,
		Cache:                      config.Cache,
		CacheDir:                   config.CacheDir,
		StorageDriver:              config.StorageDriver,
		Insecure:                   config.Insecure,
		InsecurePull:               config.InsecurePull,
		InsecureRegistry:           config.InsecureRegistry,
		RegistryCertificate:        config.RegistryCertificate,
		ImageDownloadRetry:         config.ImageDownloadRetry,
		NoPush:                     config.NoPush,
		TarPath:                    config.TarPath,
		DigestFile:                 config.DigestFile,
		ImageNameWithDigestFile:    config.ImageNameWithDigestFile,
		ImageNameTagWithDigestFile: config.ImageNameTagWithDigestFile,
		Reproducible:               config.Reproducible,
		Timestamp:                  config.Timestamp,
		Attestation:                config.Attestation,
		AttestationConfigs:         convertAttestationConfigs(config.AttestationConfigs),
		BuildKitOpts:               config.BuildKitOpts,
		Sign:                       config.Sign,
		CosignKeyPath:              config.CosignKeyPath,
		CosignPasswordEnv:          config.CosignPasswordEnv,
	}

	// Execute build
	if err := build.Execute(buildConfig, ctx); err != nil {
		logger.Fatal("Build failed: %v", err)
	}

	// Push images if not disabled
	if !config.NoPush && config.TarPath == "" {
		pushConfig := build.PushConfig{
			Destinations:        config.Destination,
			Insecure:            config.Insecure,
			InsecureRegistry:    config.InsecureRegistry,
			RegistryCertificate: config.RegistryCertificate,
			PushRetry:           config.PushRetry,
			StorageDriver:       config.StorageDriver,
		}

		digestMap, err := build.Push(pushConfig)
		if err != nil {
			logger.Fatal("Push failed: %v", err)
		}

		// Save digest information after successful push
		if err := build.SaveDigestInfo(buildConfig, digestMap); err != nil {
			logger.Warning("Failed to save digest information: %v", err)
		}
	}

	logger.Info("Build completed successfully!")
}

// convertAttestationConfigs converts main package AttestationConfig to build package AttestationConfig
func convertAttestationConfigs(mainConfigs []AttestationConfig) []build.AttestationConfig {
	buildConfigs := make([]build.AttestationConfig, len(mainConfigs))
	for i, mainConfig := range mainConfigs {
		buildConfigs[i] = build.AttestationConfig{
			Type:   mainConfig.Type,
			Params: mainConfig.Params,
		}
	}
	return buildConfigs
}