package main

// Config holds all smithy configuration options
type Config struct {
	// Core build arguments
	Dockerfile  string
	Context     string
	SubContext  string
	Destination []string

	// Cache configuration
	Cache    bool
	CacheDir string

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
	SkipTLSVerify       bool
	RegistryCertificate string
	PushRetry           int
	ImageDownloadRetry  int

	// Logging options
	Verbosity    string
	LogTimestamp bool

	// Build behavior
	CustomPlatform string
	Target         string
	StorageDriver  string // Storage driver selection (vfs, overlay)
	Reproducible   bool

	// Labels and metadata
	Labels      map[string]string
	GitBranch   string
	GitRevision string

	// Git integration
	GitTokenFile string
	GitTokenUser string

	// Enterprise features (will error in OSS)
	Scan   bool
	Harden bool
}
