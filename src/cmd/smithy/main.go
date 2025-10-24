package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/rapidfort/smithy/internal/auth"
	"github.com/rapidfort/smithy/internal/build"
	"github.com/rapidfort/smithy/internal/preflight"
	"github.com/rapidfort/smithy/pkg/logger"
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

	// Log smithy version (builder will be logged by build.Execute)
	logger.Info("Smithy - Kubernetes-Native OCI Image Builder v%s", Version)
	logger.Debug("Build Date: %s, Commit: %s, Branch: %s", BuildDate, CommitSHA, Branch)

	// Validate storage driver only if specified
	// BuildKit supports: native, overlay
	// Buildah supports: vfs, overlay
	if config.StorageDriver != "" {
		validDrivers := []string{"vfs", "overlay", "native"}
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
			fmt.Fprintf(os.Stderr, "Valid options: native, overlay (BuildKit), vfs, overlay (Buildah)\n\n")
			os.Exit(1)
		}

		// Log storage driver selection
		logger.Info("Using storage driver: %s", storageDriver)
		if storageDriver == "overlay" {
			logger.Info("Note: Overlay driver requires fuse-overlayfs and additional capabilities")
		}
		if storageDriver == "vfs" {
			logger.Info("Note: VFS storage (Buildah only)")
		}
		if storageDriver == "native" {
			logger.Info("Note: Native snapshotter (BuildKit only)")
		}
	}

	if config.Context == "" {
		fmt.Fprintf(os.Stderr, "Error: Smithy only supports BUILD mode\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  smithy --context=. --destination=registry/image:tag\n\n")
		fmt.Fprintf(os.Stderr, "Run 'smithy --help' for more information.\n")
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
		fmt.Fprintf(os.Stderr, "  smithy --context=. --destination=registry/image:tag\n\n")
		os.Exit(1)
	}

	// Setup logging
	logger.Setup(config.Verbosity, config.LogTimestamp)

	// Prepare build context
	gitConfig := build.GitConfig{
		Context:   config.Context,
		Branch:    config.GitBranch,
		Revision:  config.GitRevision,
		TokenFile: config.GitTokenFile,
		TokenUser: config.GitTokenUser,
	}
	
	ctx, err := build.Prepare(gitConfig)
	if err != nil {
		logger.Fatal("Failed to prepare build context: %v", err)
	}
	defer ctx.Cleanup()

	// Setup authentication
	authSetup := auth.SetupConfig{
		Destinations:     config.Destination,
		InsecureRegistry: config.InsecureRegistry,
	}

	authFile, err := auth.Setup(authSetup)
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
		SkipTLSVerify:              config.SkipTLSVerify,
		RegistryCertificate:        config.RegistryCertificate,
		NoPush:                     config.NoPush,
		TarPath:                    config.TarPath,
		DigestFile:                 config.DigestFile,
		ImageNameWithDigestFile:    config.ImageNameWithDigestFile,
		ImageNameTagWithDigestFile: config.ImageNameTagWithDigestFile,
	}

	// Execute build
	if err := build.Execute(buildConfig, ctx, authFile); err != nil {
		logger.Fatal("Build failed: %v", err)
	}

	// Push images if not disabled
	if !config.NoPush && config.TarPath == "" {
		pushConfig := build.PushConfig{
			Destinations:        config.Destination,
			Insecure:            config.Insecure,
			InsecureRegistry:    config.InsecureRegistry,
			SkipTLSVerify:       config.SkipTLSVerify,
			RegistryCertificate: config.RegistryCertificate,
			PushRetry:           config.PushRetry,
		}

		if err := build.Push(pushConfig, authFile); err != nil {
			logger.Fatal("Push failed: %v", err)
		}
	}

	logger.Info("Build completed successfully!")
}