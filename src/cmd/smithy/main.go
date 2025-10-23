package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rapidfort/smithy/internal/auth"
	"github.com/rapidfort/smithy/internal/build"
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

	// Parse configuration
	config := parseArgs(os.Args[1:])

	// Set up logging
	logger.Setup(config.Verbosity, config.LogTimestamp)

	// Log smithy version
	logger.Info("Smithy Container Build System v%s (OSS)", Version)
	logger.Debug("Build Date: %s, Commit: %s, Branch: %s", BuildDate, CommitSHA, Branch)

	// OSS only supports build mode
	if config.Context == "" {
		fmt.Fprintf(os.Stderr, "Error: Smithy OSS only supports BUILD mode\n\n")
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
		fmt.Fprintf(os.Stderr, "  smithy --context=. --destination=myregistry.io/myimage:latest\n")
		os.Exit(1)
	}

	logger.Info("Operating in BUILD-ONLY mode")

	// Setup authentication for pushing built images
	authSetupConfig := auth.SetupConfig{
		Destinations:     config.Destination,
		InsecureRegistry: config.InsecureRegistry,
	}

	authFile, err := auth.Setup(authSetupConfig)
	if err != nil {
		logger.Warning("Failed to setup authentication: %v", err)
		authFile, err = auth.CreateMinimal(authSetupConfig)
		if err != nil {
			logger.Error("Failed to create minimal auth config: %v", err)
		}
	} else if authFile != "" {
		logger.Info("Authentication configured: %s", authFile)
		if err := auth.EnsurePermissions(authFile); err != nil {
			logger.Warning("Failed to set auth file permissions: %v", err)
		}
		os.Setenv("REGISTRY_AUTH_FILE", authFile)
		os.Setenv("DOCKER_CONFIG", filepath.Dir(authFile))
	}

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

	if config.SubContext != "" {
		logger.Debug("Applying sub-context path: %s", config.SubContext)

		// Join the sub-path to the prepared context
		newContextPath := filepath.Join(ctx.Path, config.SubContext)

		// Verify the sub-context exists
		if _, err := os.Stat(newContextPath); os.IsNotExist(err) {
			logger.Fatal("Sub-context path does not exist: %s (full path: %s)", config.SubContext, newContextPath)
		}

		// Update the context path
		ctx.Path = newContextPath
		logger.Info("Using sub-context: %s", ctx.Path)
	}

	// Execute build
	buildConfig := build.Config{
		Dockerfile:                 config.Dockerfile,
		Destination:                config.Destination,
		Target:                     config.Target,
		BuildArgs:                  config.BuildArgs,
		Labels:                     config.Labels,
		CustomPlatform:             config.CustomPlatform,
		Cache:                      config.Cache,
		CacheDir:                   config.CacheDir,
		Insecure:                   config.Insecure,
		InsecurePull:               config.InsecurePull,
		InsecureRegistry:           config.InsecureRegistry,
		SkipTLSVerify:              config.SkipTLSVerify,
		RegistryCertificate:        config.RegistryCertificate,
		ImageDownloadRetry:         config.ImageDownloadRetry,
		NoPush:                     config.NoPush,
		TarPath:                    config.TarPath,
		DigestFile:                 config.DigestFile,
		ImageNameWithDigestFile:    config.ImageNameWithDigestFile,
		ImageNameTagWithDigestFile: config.ImageNameTagWithDigestFile,
		Reproducible:               config.Reproducible,
	}

	if err := build.Execute(buildConfig, ctx, authFile); err != nil {
		logger.Fatal("Build failed: %v", err)
	}

	// Push built images
	if !config.NoPush && len(config.Destination) > 0 {
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

	logger.Info("Build operation completed successfully!")
}
