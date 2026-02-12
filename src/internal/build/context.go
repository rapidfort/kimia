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

	// Expand environment variables in context URL (e.g., ${GITHUB_TOKEN})
	gitConfig.Context = expandEnvInURL(gitConfig.Context)

	// Check if context is a git URL
	if isGitURL(gitConfig.Context) {
		logger.Info("Detected git repository context: %s", logger.SanitizeGitURL(gitConfig.Context))

		// Normalize git:// URLs to https:// for known providers (GitHub, GitLab, etc)
		normalizedURL := normalizeGitURL(gitConfig.Context)
		
		// For BuildKit, pass Git URL directly without cloning (for better SBOM generation)
		if builder == "buildkit" {
			logger.Info("Using BuildKit native Git support (no local clone)")
			ctx.IsGitRepo = true
			ctx.GitURL = normalizedURL  // Use normalized URL
			ctx.Path = "" // No local path needed for BuildKit
			
			// BuildKit will handle branch/revision via Git URL syntax
			logger.Debug("Build context prepared (Git URL for BuildKit): %s", ctx.GitURL)
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
		// #nosec G703 -- workspaceDir validated and cleaned above
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

		// Clone the repository (use normalized URL from line 51)
		normalizedURL = normalizeGitURL(gitConfig.Context)
		if err := cloneGitRepo(normalizedURL, tempDir, gitConfig); err != nil {
			// #nosec G703 -- tempDir created by os.MkdirTemp, safe to remove
			os.RemoveAll(tempDir)
			return nil, fmt.Errorf("failed to clone repository: %v", err)
		}

		ctx.Path = tempDir

		// If GitRevision is specified, try to checkout the revision directly
		if gitConfig.Revision != "" {
			logger.Info("Checking out revision: %s", gitConfig.Revision)
			
			// Try to checkout the revision
			if err := checkoutGitRevision(tempDir, gitConfig.Revision); err != nil {
				// Revision doesn't exist, fall back to branch if specified
				if gitConfig.Branch != "" {
					logger.Warning("Revision %s not found, falling back to branch %s", gitConfig.Revision, gitConfig.Branch)
					if err := checkoutGitBranch(tempDir, gitConfig.Branch); err != nil {
						// #nosec G703 -- tempDir created by os.MkdirTemp, safe to remove
						os.RemoveAll(tempDir)
						return nil, fmt.Errorf("failed to checkout branch %s: %v", gitConfig.Branch, err)
					}
				} else {
					// #nosec G703 -- tempDir created by os.MkdirTemp, safe to remove
					os.RemoveAll(tempDir)
					return nil, fmt.Errorf("failed to checkout revision %s: %v", gitConfig.Revision, err)
				}
			} else {
				// Revision checked out successfully
				// If branch was specified, check if revision is on that branch and warn if not
				if gitConfig.Branch != "" {
					if !isRevisionOnBranch(tempDir, gitConfig.Revision, gitConfig.Branch) {
						logger.Warning("⚠️  WARNING: Revision %s is NOT on branch %s", gitConfig.Revision, gitConfig.Branch)
						logger.Warning("⚠️  Building from revision anyway. This may not be what you intended.")
						logger.Warning("⚠️  Verify this commit is correct for your use case.")
					}
				}
			}
		} else if gitConfig.Branch != "" {
			// No revision specified, just checkout the branch
			logger.Info("Checking out branch: %s", gitConfig.Branch)
			if err := checkoutGitBranch(tempDir, gitConfig.Branch); err != nil {
				// #nosec G703 -- tempDir created by os.MkdirTemp, safe to remove
				os.RemoveAll(tempDir)
				return nil, fmt.Errorf("failed to checkout branch %s: %v", gitConfig.Branch, err)
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

// normalizeGitURL converts deprecated git:// URLs and git@ SSH URLs to https:// for known providers
// GitHub, GitLab, and Bitbucket have all disabled the insecure git:// protocol
// For automation/CI, HTTPS is preferred over SSH (no key management, no prompts)
func normalizeGitURL(url string) string {
	// Check if user wants to force SSH (skip normalization for git@)
	preferSSH := os.Getenv("KIMIA_PREFER_SSH") == "true"
	
	// Convert git:// to https://
	if strings.HasPrefix(url, "git://") {
		knownProviders := []string{
			"github.com",
			"gitlab.com",
			"bitbucket.org",
		}
		
		for _, provider := range knownProviders {
			if strings.Contains(url, provider) {
				normalized := strings.Replace(url, "git://", "https://", 1)
				logger.Warning("Converted deprecated git:// URL to https:// (git:// protocol is disabled on %s)", provider)
				logger.Debug("Original: %s", url)
				logger.Debug("Normalized: %s", normalized)
				return normalized
			}
		}
		
		logger.Warning("Using git:// URL: %s", url)
		logger.Warning("Note: Most modern Git servers have disabled git:// protocol. If build fails, try https:// instead")
		return url
	}
	
	// Convert git@ SSH URLs to https:// for automation-friendly non-interactive cloning
	if strings.HasPrefix(url, "git@") && !preferSSH {
		// Pattern: git@github.com:user/repo.git -> https://github.com/user/repo.git
		if strings.Contains(url, "github.com") || 
		   strings.Contains(url, "gitlab.com") || 
		   strings.Contains(url, "bitbucket.org") {
			
			// Extract host and path
			// git@github.com:user/repo.git
			parts := strings.SplitN(url, "@", 2)
			if len(parts) == 2 {
				hostAndPath := parts[1]
				// github.com:user/repo.git
				hostAndPath = strings.Replace(hostAndPath, ":", "/", 1)
				normalized := "https://" + hostAndPath
				
				logger.Warning("Converted SSH URL (git@) to HTTPS for non-interactive cloning")
				logger.Info("For automation, HTTPS is preferred over SSH (no keys/prompts required)")
				logger.Debug("Original: git@...")
				logger.Debug("Normalized: %s", normalized)
				logger.Info("Note: To force SSH, set environment variable KIMIA_PREFER_SSH=true")
				return normalized
			}
		}
	}
	
	if strings.HasPrefix(url, "git@") && preferSSH {
		logger.Info("Using SSH URL as requested (KIMIA_PREFER_SSH=true)")
		logger.Info("Ensure SSH agent is running with keys loaded for non-interactive operation")
	}
	
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

	// If revision is specified, we need to clone without --single-branch
	// to ensure the revision is available even if it's on a different branch
	if gitConfig.Revision != "" {
		// Clone without depth/single-branch restrictions to get all refs
		// This ensures the revision can be found regardless of which branch it's on
		logger.Debug("Cloning full repository to access revision %s", gitConfig.Revision)
	} else if gitConfig.Branch != "" {
		// Only restrict to single branch if no revision is specified
		args = append(args, "--branch", gitConfig.Branch, "--single-branch")
	} else {
		// Add depth 1 for faster cloning if no specific revision or branch is needed
		args = append(args, "--depth", "1")
	}

	args = append(args, url, targetDir)

	// #nosec G702 -- git command with controlled arguments
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
		// Check if URL already has credentials (user:pass@host or just @host)
		parts := strings.SplitN(url, "https://", 2)
		if len(parts) == 2 {
			remainder := parts[1]
			
			// Check for existing credentials
			if strings.Contains(remainder, "@") {
				// URL already has credentials, don't add more
				logger.Debug("URL already contains credentials, not adding token")
				return url
			}
			
			// Insert credentials after https://
			return fmt.Sprintf("https://%s:%s@%s", user, token, remainder)
		}
	}

	return url
}

// expandEnvInURL expands environment variables in a URL
// Supports both $VAR and ${VAR} syntax
func expandEnvInURL(url string) string {
	// Use os.ExpandEnv which handles both $VAR and ${VAR}
	expanded := os.ExpandEnv(url)
	
	if expanded != url {
		logger.Debug("Expanded environment variables in URL")
		// Don't log the actual values for security
	}
	
	return expanded
}

// checkoutGitBranch checks out a specific Git branch
func checkoutGitBranch(repoDir, branch string) error {
	logger.Info("Checking out branch: %s", branch)

	// First, try to fetch the branch to ensure we have it
	fetchCmd := exec.Command("git", "fetch", "origin", branch)
	fetchCmd.Dir = repoDir
	fetchCmd.Stdout = os.Stdout
	fetchCmd.Stderr = os.Stderr
	fetchCmd.Run() // Ignore error, we'll check checkout result

	// Now checkout the branch (might be remote tracking branch)
	cmd := exec.Command("git", "checkout", branch)
	cmd.Dir = repoDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		logger.Debug("Direct checkout failed, trying remote tracking branch...")
		// Try with explicit remote tracking branch
		cmd2 := exec.Command("git", "checkout", "-b", branch, "origin/"+branch)
		cmd2.Dir = repoDir
		cmd2.Stdout = os.Stdout
		cmd2.Stderr = os.Stderr

		if err2 := cmd2.Run(); err2 != nil {
			return fmt.Errorf("git checkout failed: %v (also tried origin/%s: %v)", err, branch, err2)
		}
		logger.Info("Successfully checked out from remote tracking branch")
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
			suffix = ":" + subContext
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

// isRevisionOnBranch checks if a git revision is an ancestor of the specified branch
func isRevisionOnBranch(repoPath, revision, branch string) bool {
	cmd := exec.Command("git", "merge-base", "--is-ancestor", revision, branch)
	cmd.Dir = repoPath
	return cmd.Run() == nil
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