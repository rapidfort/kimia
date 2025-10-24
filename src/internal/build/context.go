package build

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rapidfort/smithy/pkg/logger"
)

// Context manages the build context
type Context struct {
	Path      string
	IsGitRepo bool
	TempDir   string
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
func Prepare(gitConfig GitConfig) (*Context, error) {
	ctx := &Context{}

	// Check if context is a git URL
	if isGitURL(gitConfig.Context) {
		logger.Info("Detected git repository context: %s", gitConfig.Context)

		// Create directory in $HOME/workspace for git clone
		homeDir := os.Getenv("HOME")
		if homeDir == "" {
			homeDir = "/home/smithy"
		}

		workspaceDir := filepath.Join(homeDir, "workspace")

		// Ensure workspace directory exists
		if err := os.MkdirAll(workspaceDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create workspace directory: %v", err)
		}

		// Create temporary directory for git clone inside workspace
		tempDir, err := os.MkdirTemp(workspaceDir, "smithy-build-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp directory: %v", err)
		}

		ctx.TempDir = tempDir
		ctx.IsGitRepo = true

		// Clone the repository
		if err := cloneGitRepo(gitConfig.Context, tempDir, gitConfig); err != nil {
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
