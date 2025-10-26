package main

import (
	"os"
	"strconv"
	"strings"

	"github.com/rapidfort/smithy/pkg/logger"
)

func parseArgs(args []string) *Config {
	config := &Config{
		BuildArgs:        make(map[string]string),
		Labels:           make(map[string]string),
		Verbosity:        "info",
		InsecureRegistry: []string{},
		Destination:      []string{},
		StorageDriver:    "", // Empty by default, only used by Buildah
	}

	// If no arguments provided, show help
	if len(args) == 0 {
		printHelp()
		os.Exit(0)
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]

		// Handle both --flag=value and --flag value formats
		var key, value string
		if strings.Contains(arg, "=") {
			parts := strings.SplitN(arg, "=", 2)
			key = parts[0]
			value = parts[1]
		} else {
			key = arg
		}

		switch key {
		case "--help", "-h":
			printHelp()
			os.Exit(0)

		case "--version":
			printVersion()
			os.Exit(0)

		case "-f", "--dockerfile":
			if value != "" {
				config.Dockerfile = value
			} else if i+1 < len(args) {
				i++
				config.Dockerfile = args[i]
			}

		case "-c", "--context":
			if value != "" {
				config.Context = value
			} else if i+1 < len(args) {
				i++
				config.Context = args[i]
			}

		case "--context-sub-path":
			// Handle cases where --context-sub-path=""
			// Only consume the next arg if it doesn't look like a flag
			// Also check for literal ""
			if value != "" && value != `""` {
				config.SubContext = value
			} else if i+1 < len(args) && len(args[i+1]) > 0 && args[i+1][0] != '-' {
				i++
				if args[i] != `""` {
					config.SubContext = args[i]
				}
			} else {
				config.SubContext = ""
			}

		case "-d", "--destination":
			dest := value
			if dest == "" && i+1 < len(args) {
				i++
				dest = args[i]
			}
			if dest != "" {
				config.Destination = append(config.Destination, dest)
			}

		case "--cache":
			if value != "" {
				config.Cache = parseBool(value)
			} else if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				config.Cache = parseBool(args[i])
			} else {
				config.Cache = true
			}

		case "--cache-dir":
			if value != "" {
				config.CacheDir = value
			} else if i+1 < len(args) {
				i++
				config.CacheDir = args[i]
			}

		case "--storage-driver":
			if value != "" {
				config.StorageDriver = value
			} else if i+1 < len(args) {
				i++
				config.StorageDriver = args[i]
			}

		case "--build-arg":
			buildArg := value
			if buildArg == "" && i+1 < len(args) {
				i++
				buildArg = args[i]
			}
			if buildArg != "" {
				parseBuildArg(buildArg, config)
			}

		case "--no-push":
			config.NoPush = true

		case "--tar-path":
			if value != "" {
				config.TarPath = value
			} else if i+1 < len(args) {
				i++
				config.TarPath = args[i]
			}

		case "--digest-file":
			if value != "" {
				config.DigestFile = value
			} else if i+1 < len(args) {
				i++
				config.DigestFile = args[i]
			}

		case "--image-name-with-digest-file":
			if value != "" {
				config.ImageNameWithDigestFile = value
			} else if i+1 < len(args) {
				i++
				config.ImageNameWithDigestFile = args[i]
			}

		case "--insecure":
			config.Insecure = true

		case "--insecure-pull":
			config.InsecurePull = true

		case "--insecure-registry":
			reg := value
			if reg == "" && i+1 < len(args) {
				i++
				reg = args[i]
			}
			if reg != "" {
				config.InsecureRegistry = append(config.InsecureRegistry, reg)
			}

		case "--skip-tls-verify":
			config.SkipTLSVerify = true

		case "--push-retry":
			if value != "" {
				config.PushRetry = parseInt(value)
			} else if i+1 < len(args) {
				i++
				config.PushRetry = parseInt(args[i])
			}

		case "--image-download-retry":
			if value != "" {
				config.ImageDownloadRetry = parseInt(value)
			} else if i+1 < len(args) {
				i++
				config.ImageDownloadRetry = parseInt(args[i])
			}

		case "-v", "--verbosity":
			if value != "" {
				config.Verbosity = value
			} else if i+1 < len(args) {
				i++
				config.Verbosity = args[i]
			}

		case "--log-timestamp":
			config.LogTimestamp = true

		case "--custom-platform":
			if value != "" {
				config.CustomPlatform = value
			} else if i+1 < len(args) {
				i++
				config.CustomPlatform = args[i]
			}

		case "-t", "--target":
			if value != "" {
				config.Target = value
			} else if i+1 < len(args) {
				i++
				config.Target = args[i]
			}

		case "--label":
			label := value
			if label == "" && i+1 < len(args) {
				i++
				label = args[i]
			}
			if label != "" {
				parseLabel(label, config)
			}

		case "--git-branch":
			if value != "" {
				config.GitBranch = value
			} else if i+1 < len(args) {
				i++
				config.GitBranch = args[i]
			}

		case "--git-revision":
			if value != "" {
				config.GitRevision = value
			} else if i+1 < len(args) {
				i++
				config.GitRevision = args[i]
			}

		case "--git-token-file":
			if value != "" {
				config.GitTokenFile = value
			} else if i+1 < len(args) {
				i++
				config.GitTokenFile = args[i]
			}

		case "--git-token-user":
			if value != "" {
				config.GitTokenUser = value
			} else if i+1 < len(args) {
				i++
				config.GitTokenUser = args[i]
			}

		case "--registry-certificate":
			if value != "" {
				config.RegistryCertificate = value
			} else if i+1 < len(args) {
				i++
				config.RegistryCertificate = args[i]
			}

		case "--reproducible":
			config.Reproducible = true

		case "--timestamp":
			if value != "" {
				config.Timestamp = value
			} else if i+1 < len(args) {
				i++
				config.Timestamp = args[i]
			}
			// Auto-enable reproducible mode when timestamp is set
			config.Reproducible = true

		// Enterprise flags (will error out)
		case "--scan":
			config.Scan = true

		case "--harden":
			config.Harden = true

		default:
			if !strings.HasPrefix(arg, "-") {
				logger.Warning("Unexpected argument: %s", arg)
			} else {
				logger.Warning("Unknown option: %s", arg)
			}
		}
	}

	// ========================================
	// REPRODUCIBLE BUILDS: Timestamp precedence logic
	// ========================================
	// Priority (highest to lowest):
	// 1. --timestamp flag (explicit)
	// 2. SOURCE_DATE_EPOCH env var (if --reproducible is set)
	// 3. Default to "0" (if --reproducible is set)
	if config.Reproducible {
		if config.Timestamp == "" {
			// No explicit timestamp, check environment variable
			if epoch := os.Getenv("SOURCE_DATE_EPOCH"); epoch != "" {
				config.Timestamp = epoch
				logger.Debug("Using timestamp from SOURCE_DATE_EPOCH environment variable: %s", epoch)
			} else {
				// Default to epoch 0 for reproducible builds
				config.Timestamp = "0"
				logger.Debug("Using default timestamp 0 for reproducible build")
			}
		} else {
			logger.Debug("Using explicit timestamp from --timestamp flag: %s", config.Timestamp)
		}
	}

	return config
}

func parseBool(value string) bool {
	switch strings.ToLower(value) {
	case "true", "yes", "1", "on":
		return true
	case "false", "no", "0", "off":
		return false
	default:
		logger.Fatal("Invalid boolean value: %s", value)
		return false
	}
}

func parseInt(value string) int {
	val, err := strconv.Atoi(value)
	if err != nil {
		logger.Fatal("Invalid integer value: %s", value)
	}
	return val
}

func parseBuildArg(arg string, config *Config) {
	parts := strings.SplitN(arg, "=", 2)
	if len(parts) == 2 {
		config.BuildArgs[parts[0]] = parts[1]
	} else {
		// Allow just key without value (will use environment variable)
		config.BuildArgs[parts[0]] = ""
	}
}

func parseLabel(label string, config *Config) {
	parts := strings.SplitN(label, "=", 2)
	if len(parts) == 2 {
		config.Labels[parts[0]] = parts[1]
	} else {
		logger.Fatal("Invalid label format: %s", label)
	}
}