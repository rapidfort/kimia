package main

// Config holds all kimia configuration options
type Config struct {
	// Core build arguments
	Dockerfile  string
	Context     string
	SubContext  string
	Destination []string

	// Cache configuration
	Cache        bool
	CacheDir     string
	ExportCache  []string // BuildKit --export-cache options (e.g. "type=registry,ref=...,mode=max")
	ImportCache  []string // BuildKit --import-cache options (e.g. "type=registry,ref=...")

	// Build arguments
	BuildArgs map[string]string

	// Output options
	NoPush                     bool
	TarPath                    string
	DigestFile                 string
	ImageNameWithDigestFile    string
	ImageNameTagWithDigestFile string

	// Security and registry options
	Insecure            bool
	InsecurePull        bool
	InsecureRegistry    []string
	RegistryCertificate string
	PushRetry           int
	ImageDownloadRetry  int

	// Logging options
	Verbosity    string
	LogTimestamp bool

	// Build behavior
	CustomPlatform string
	Target         string
	StorageDriver  string // Storage driver selection (vfs, overlay, native)
	Reproducible   bool   // Enable reproducible builds
	Timestamp      string // Custom timestamp for reproducible builds (Unix epoch)

	// Labels and metadata
	Labels      map[string]string
	GitBranch   string
	GitRevision string

	// Git integration
	GitTokenFile string
	GitTokenUser string

	// Enterprise features
	Scan   bool
	Harden bool

	// Attestation and signing
	// Level 1: Simple mode (backward compatible)
	Attestation string // Attestation mode: "", "off", "min", or "max"
	
	// Level 2: Docker-style attestations (advanced)
	// Parsed from --attest flags
	AttestationConfigs []AttestationConfig
	
	// Level 3: Direct BuildKit options (escape hatch)
	BuildKitOpts []string // Raw --opt values to pass to buildctl
	
	// Signing
	Sign              bool   // Enable cosign signing
	CosignKeyPath     string // Path to cosign private key
	CosignPasswordEnv string // Environment variable for cosign password
}

// AttestationConfig represents a single --attest flag
type AttestationConfig struct {
	Type   string            // "sbom" or "provenance"
	Params map[string]string // Key-value pairs from the flag
}