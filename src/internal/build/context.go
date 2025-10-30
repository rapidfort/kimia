package build

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rapidfort/kimia/pkg/logger"
)

// Context manages the build context
type Context struct {
	Path       string
	IsGitRepo  bool
	TempDir    string
	GitURL     string    // Original Git URL (for BuildKit)
	SubContext string    // Subdirectory within context
	GitConfig  GitConfig // Git configuration for URL formatting
}

// Cleanup removes temporary directories created for Git repositories
func (ctx *Context) Cleanup() {
	if ctx.TempDir != "" {
		logger.Debug("Cleaning up temporary directory: %s", ctx.TempDir)
		os.RemoveAll(ctx.TempDir)
	}
}

// GitConfig holds Git-specific configuration
type GitConfig struct {
	Context   string
	Branch    string
	Revision  string
	TokenFile string
	TokenUser string
}

// Prepare prepares the build context from either a Git repository or local directory
func Prepare(gitConfig GitConfig, builder string) (*Context, error) {
	ctx := &Context{
		GitConfig: gitConfig, // Store for later use in BuildKit URL formatting
	}

	// Check if context is a git URL
	if isGitURL(gitConfig.Context) {
		logger.Info("Detected git repository context: %s", gitConfig.Context)
		
		// Normalize git:// URLs to https:// for known providers (GitHub, GitLab, etc)
		normalizedURL := normalizeGitURL(gitConfig.Context)

		// For BuildKit, pass Git URL directly without cloning (for better SBOM generation)
		if builder == "buildkit" {
			logger.Info("Using BuildKit native Git support (no local clone)")
			ctx.IsGitRepo = true
			ctx.GitURL = normalizedURL  // Use normalized URL
			ctx.Path = "" // No local path needed for BuildKit
			
			// BuildKit will handle branch/revision via Git URL syntax
			logger.Info("Build context prepared (Git URL for BuildKit): %s", ctx.GitURL)
			return ctx, nil
		}
		
		// For Buildah, clone the repository locally (existing behavior)
		logger.Info("Cloning repository for Buildah...")

		// Create directory in $HOME/workspace for git clone
		homeDir := os.Getenv("HOME")
		if homeDir == "" {
			homeDir = "/home/kimia"
		}

		workspaceDir := filepath.Join(homeDir, "workspace")

		// Ensure workspace directory exists
		if err := os.MkdirAll(workspaceDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create workspace directory: %v", err)
		}

		// Create temporary directory for git clone inside workspace
		tempDir, err := os.MkdirTemp(workspaceDir, "kimia-build-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp directory: %v", err)
		}

		ctx.TempDir = tempDir
		ctx.IsGitRepo = true

		// Clone the repository (use normalized URL)
		normalizedURL = normalizeGitURL(gitConfig.Context)
		if err := cloneGitRepo(normalizedURL, tempDir, gitConfig); err != nil {
			os.RemoveAll(tempDir)
			return nil, fmt.Errorf("failed to clone repository: %v", err)
		}

		ctx.Path = tempDir

		// If GitBranch is specified, checkout the branch
		if gitConfig.Branch != "" {
			if err := checkoutGitBranch(tempDir, gitConfig.Branch); err != nil {
				os.RemoveAll(tempDir)
				return nil, fmt.Errorf("failed to checkout branch %s: %v", gitConfig.Branch, err)
			}
		}

		// If GitRevision is specified, checkout the revision
		if gitConfig.Revision != "" {
			if err := checkoutGitRevision(tempDir, gitConfig.Revision); err != nil {
				os.RemoveAll(tempDir)
				return nil, fmt.Errorf("failed to checkout revision %s: %v", gitConfig.Revision, err)
			}
		}
	} else {
		// Local context
		ctx.Path = gitConfig.Context
		if ctx.Path == "" {
			return nil, fmt.Errorf("build context is required")
		}

		// Verify context exists
		if _, err := os.Stat(ctx.Path); err != nil {
			return nil, fmt.Errorf("context path does not exist: %v", err)
		}
	}

	logger.Info("Build context prepared at: %s", ctx.Path)
	return ctx, nil
}

// isGitURL checks if a URL appears to be a Git repository
func isGitURL(url string) bool {
	return strings.HasPrefix(url, "git://") ||
		strings.HasPrefix(url, "git@") ||
		strings.HasPrefix(url, "https://github.com/") ||
		strings.HasPrefix(url, "https://gitlab.com/") ||
		strings.HasPrefix(url, "https://bitbucket.org/") ||
		strings.HasSuffix(url, ".git") ||
		strings.Contains(url, "git.") ||
		(strings.HasPrefix(url, "https://") && strings.Contains(url, "/"))
}

// normalizeGitURL converts deprecated git:// URLs to https:// for known providers
// GitHub, GitLab, and Bitbucket have all disabled the insecure git:// protocol
func normalizeGitURL(url string) string {
	// Only normalize git:// URLs
	if !strings.HasPrefix(url, "git://") {
		return url
	}

	// List of known providers that have disabled git:// protocol
	knownProviders := []string{
		"github.com",
		"gitlab.com",
		"bitbucket.org",
	}

	// Check if URL is from a known provider
	for _, provider := range knownProviders {
		if strings.Contains(url, provider) {
			// Convert git:// to https://
			normalized := strings.Replace(url, "git://", "https://", 1)
			logger.Warning("Converted deprecated git:// URL to https:// (git:// protocol is disabled on %s)", provider)
			logger.Debug("Original: %s", url)
			logger.Debug("Normalized: %s", normalized)
			return normalized
		}
	}

	// For unknown providers, warn but keep original (might be private server)
	logger.Warning("Using git:// URL: %s", url)
	logger.Warning("Note: Most modern Git servers have disabled git:// protocol. If build fails, try https:// instead")
	return url
}

// cloneGitRepo clones a Git repository to the target directory
func cloneGitRepo(url, targetDir string, gitConfig GitConfig) error {
	logger.Info("Cloning git repository...")

	// Prepare git clone command
	args := []string{"clone"}

	// Add authentication if token is provided
	if gitConfig.TokenFile != "" {
		token, err := os.ReadFile(gitConfig.TokenFile)
		if err != nil {
			return fmt.Errorf("failed to read git token file: %v", err)
		}

		// Modify URL to include token
		url = addGitToken(url, string(token), gitConfig.TokenUser)
	}

	// Add depth 1 for faster cloning if no specific revision is needed
	if gitConfig.Revision == "" {
		args = append(args, "--depth", "1")
	}

	args = append(args, url, targetDir)

	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone failed: %v", err)
	}

	logger.Info("Repository cloned successfully")
	return nil
}

// addGitToken adds authentication token to a Git URL
func addGitToken(url, token, user string) string {
	token = strings.TrimSpace(token)
	if user == "" {
		user = "oauth2"
	}

	// Handle different URL formats
	if strings.HasPrefix(url, "https://") {
		// Insert credentials after https://
		parts := strings.SplitN(url, "https://", 2)
		if len(parts) == 2 {
			return fmt.Sprintf("https://%s:%s@%s", user, token, parts[1])
		}
	}

	return url
}

// checkoutGitBranch checks out a specific Git branch
func checkoutGitBranch(repoDir, branch string) error {
	logger.Info("Checking out branch: %s", branch)

	cmd := exec.Command("git", "checkout", branch)
	cmd.Dir = repoDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// Try fetching the branch first
		fetchCmd := exec.Command("git", "fetch", "origin", branch)
		fetchCmd.Dir = repoDir
		fetchCmd.Run() // Ignore error, might already have it

		// Try checkout again
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git checkout failed: %v", err)
		}
	}

	return nil
}

// checkoutGitRevision checks out a specific Git commit
func checkoutGitRevision(repoDir, revision string) error {
	logger.Info("Checking out revision: %s", revision)

	cmd := exec.Command("git", "checkout", revision)
	cmd.Dir = repoDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git checkout revision failed: %v", err)
	}

	return nil
}

// FormatGitURLForBuildKit formats a Git URL for BuildKit with authentication, branch, revision, and subpath
// BuildKit Git URL format: git://host/repo.git#ref:subdir
// Returns the formatted URL and whether authentication was applied
func FormatGitURLForBuildKit(gitURL string, gitConfig GitConfig, subContext string) (string, error) {
	url := gitURL
	
	// Add authentication token if provided
	if gitConfig.TokenFile != "" {
		token, err := os.ReadFile(gitConfig.TokenFile)
		if err != nil {
			return "", fmt.Errorf("failed to read git token file: %v", err)
		}
		url = addGitToken(url, string(token), gitConfig.TokenUser)
		logger.Debug("Added authentication token to Git URL")
	}
	
	// BuildKit Git URL format: URL#<ref>:<subdir>
	// ref can be: branch name, tag, or commit hash
	// Examples:
	//   git://host/repo.git#main:path/to/subdir
	//   git://host/repo.git#v1.0.0:path/to/subdir
	//   git://host/repo.git#abc123:path/to/subdir
	
	var suffix string
	
	// Add branch or revision
	if gitConfig.Revision != "" {
		suffix = gitConfig.Revision
		logger.Debug("Using Git revision: %s", gitConfig.Revision)
	} else if gitConfig.Branch != "" {
		suffix = gitConfig.Branch
		logger.Debug("Using Git branch: %s", gitConfig.Branch)
	}
	
	// Add subcontext path
	if subContext != "" {
		if suffix != "" {
			suffix = suffix + ":" + subContext
		} else {
			suffix = "HEAD:" + subContext
		}
		logger.Debug("Using sub-context path: %s", subContext)
	}
	
	// Append suffix if any
	if suffix != "" {
		url = url + "#" + suffix
	}
	
	logger.Info("Formatted Git URL for BuildKit: %s", maskToken(url))
	return url, nil
}

// maskToken masks the authentication token in a URL for logging
func maskToken(url string) string {
	// Mask pattern like "https://user:TOKEN@host" to "https://user:***@host"
	if strings.Contains(url, "@") && strings.Contains(url, "://") {
		parts := strings.SplitN(url, "://", 2)
		if len(parts) == 2 {
			authAndHost := parts[1]
			if atIdx := strings.Index(authAndHost, "@"); atIdx > 0 {
				auth := authAndHost[:atIdx]
				if colonIdx := strings.Index(auth, ":"); colonIdx > 0 {
					user := auth[:colonIdx]
					host := authAndHost[atIdx:]
					return parts[0] + "://" + user + ":***" + host
				}
			}
		}
	}
	return url
}