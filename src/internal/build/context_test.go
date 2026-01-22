package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ===== TESTS FOR Context.Cleanup() =====

func TestContext_Cleanup(t *testing.T) {
	t.Run("cleanup with temp directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		tempSubDir := filepath.Join(tmpDir, "temp-context")
		os.MkdirAll(tempSubDir, 0755)

		ctx := &Context{
			TempDir: tempSubDir,
		}

		// Cleanup should remove the temp directory
		ctx.Cleanup()

		// Verify directory was removed
		if _, err := os.Stat(tempSubDir); !os.IsNotExist(err) {
			t.Error("TempDir should be removed after Cleanup()")
		}
	})

	t.Run("cleanup without temp directory", func(t *testing.T) {
		ctx := &Context{
			TempDir: "",
		}

		// Should not panic or error
		ctx.Cleanup()
	})

	t.Run("cleanup with non-existent directory", func(t *testing.T) {
		ctx := &Context{
			TempDir: "/nonexistent/temp/dir",
		}

		// Should not panic even if directory doesn't exist
		ctx.Cleanup()
	})
}

// ===== TESTS FOR isGitURL() =====

func TestIsGitURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{
			name: "git:// protocol",
			url:  "git://github.com/user/repo.git",
			want: true,
		},
		{
			name: "git@ SSH format",
			url:  "git@github.com:user/repo.git",
			want: true,
		},
		{
			name: "https github",
			url:  "https://github.com/user/repo",
			want: true,
		},
		{
			name: "https gitlab",
			url:  "https://gitlab.com/user/repo",
			want: true,
		},
		{
			name: "https bitbucket",
			url:  "https://bitbucket.org/user/repo",
			want: true,
		},
		{
			name: ".git suffix",
			url:  "https://example.com/repo.git",
			want: true,
		},
		{
			name: "contains git.",
			url:  "https://git.example.com/repo",
			want: true,
		},
		{
			name: "https with path",
			url:  "https://example.com/path/to/repo",
			want: true,
		},
		{
			name: "local path",
			url:  "/local/path/to/repo",
			want: false,
		},
		{
			name: "relative path",
			url:  "./local/repo",
			want: false,
		},
		{
			name: "simple directory name",
			url:  "myrepo",
			want: false,
		},
		{
			name: "http without git indicators",
			url:  "http://example.com",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isGitURL(tt.url)
			if got != tt.want {
				t.Errorf("isGitURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

// ===== TESTS FOR normalizeGitURL() =====

func TestNormalizeGitURL(t *testing.T) {
	// Save and restore environment
	originalSSH := os.Getenv("KIMIA_PREFER_SSH")
	defer func() {
		if originalSSH == "" {
			os.Unsetenv("KIMIA_PREFER_SSH")
		} else {
			os.Setenv("KIMIA_PREFER_SSH", originalSSH)
		}
	}()

	tests := []struct {
		name        string
		url         string
		preferSSH   bool
		want        string
		description string
	}{
		{
			name:        "git:// to https:// for github",
			url:         "git://github.com/user/repo.git",
			preferSSH:   false,
			want:        "https://github.com/user/repo.git",
			description: "Should convert deprecated git:// to https://",
		},
		{
			name:        "git:// to https:// for gitlab",
			url:         "git://gitlab.com/user/repo.git",
			preferSSH:   false,
			want:        "https://gitlab.com/user/repo.git",
			description: "Should convert deprecated git:// to https://",
		},
		{
			name:        "git:// to https:// for bitbucket",
			url:         "git://bitbucket.org/user/repo.git",
			preferSSH:   false,
			want:        "https://bitbucket.org/user/repo.git",
			description: "Should convert deprecated git:// to https://",
		},
		{
			name:        "git:// unknown provider unchanged",
			url:         "git://unknown.com/repo.git",
			preferSSH:   false,
			want:        "git://unknown.com/repo.git",
			description: "Should keep git:// for unknown providers",
		},
		{
			name:        "git@ SSH to https:// for github",
			url:         "git@github.com:user/repo.git",
			preferSSH:   false,
			want:        "https://github.com/user/repo.git",
			description: "Should convert SSH to HTTPS when not preferring SSH",
		},
		{
			name:        "git@ SSH to https:// for gitlab",
			url:         "git@gitlab.com:user/repo.git",
			preferSSH:   false,
			want:        "https://gitlab.com/user/repo.git",
			description: "Should convert SSH to HTTPS",
		},
		{
			name:        "git@ SSH preserved when KIMIA_PREFER_SSH=true",
			url:         "git@github.com:user/repo.git",
			preferSSH:   true,
			want:        "git@github.com:user/repo.git",
			description: "Should keep SSH format when preferring SSH",
		},
		{
			name:        "https:// unchanged",
			url:         "https://github.com/user/repo.git",
			preferSSH:   false,
			want:        "https://github.com/user/repo.git",
			description: "Should not modify https:// URLs",
		},
		{
			name:        "http:// unchanged",
			url:         "http://example.com/repo.git",
			preferSSH:   false,
			want:        "http://example.com/repo.git",
			description: "Should not modify http:// URLs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment
			if tt.preferSSH {
				os.Setenv("KIMIA_PREFER_SSH", "true")
			} else {
				os.Unsetenv("KIMIA_PREFER_SSH")
			}

			got := normalizeGitURL(tt.url)
			if got != tt.want {
				t.Errorf("normalizeGitURL(%q) = %q, want %q\n%s", tt.url, got, tt.want, tt.description)
			}
		})
	}
}

// ===== TESTS FOR expandEnvInURL() =====

func TestExpandEnvInURL(t *testing.T) {
	// Save and restore environment
	originalToken := os.Getenv("TEST_TOKEN")
	originalUser := os.Getenv("TEST_USER")
	defer func() {
		if originalToken == "" {
			os.Unsetenv("TEST_TOKEN")
		} else {
			os.Setenv("TEST_TOKEN", originalToken)
		}
		if originalUser == "" {
			os.Unsetenv("TEST_USER")
		} else {
			os.Setenv("TEST_USER", originalUser)
		}
	}()

	tests := []struct {
		name    string
		url     string
		envVars map[string]string
		want    string
	}{
		{
			name: "$VAR syntax",
			url:  "https://$TEST_USER:$TEST_TOKEN@github.com/repo.git",
			envVars: map[string]string{
				"TEST_USER":  "myuser",
				"TEST_TOKEN": "mytoken",
			},
			want: "https://myuser:mytoken@github.com/repo.git",
		},
		{
			name: "${VAR} syntax",
			url:  "https://${TEST_USER}:${TEST_TOKEN}@github.com/repo.git",
			envVars: map[string]string{
				"TEST_USER":  "user123",
				"TEST_TOKEN": "token456",
			},
			want: "https://user123:token456@github.com/repo.git",
		},
		{
			name: "mixed syntax",
			url:  "https://$TEST_USER:${TEST_TOKEN}@github.com/repo.git",
			envVars: map[string]string{
				"TEST_USER":  "alice",
				"TEST_TOKEN": "secret",
			},
			want: "https://alice:secret@github.com/repo.git",
		},
		{
			name:    "no variables",
			url:     "https://github.com/repo.git",
			envVars: map[string]string{},
			want:    "https://github.com/repo.git",
		},
		{
			name:    "undefined variable",
			url:     "https://$UNDEFINED_VAR@github.com/repo.git",
			envVars: map[string]string{},
			want:    "https://@github.com/repo.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			got := expandEnvInURL(tt.url)
			if got != tt.want {
				t.Errorf("expandEnvInURL(%q) = %q, want %q", tt.url, got, tt.want)
			}

			// Clean up
			for key := range tt.envVars {
				os.Unsetenv(key)
			}
		})
	}
}

// ===== TESTS FOR addGitToken() =====

func TestAddGitToken(t *testing.T) {
	tests := []struct {
		name  string
		url   string
		token string
		user  string
		want  string
	}{
		{
			name:  "add token to https URL",
			url:   "https://github.com/user/repo.git",
			token: "mytoken123",
			user:  "oauth2",
			want:  "https://oauth2:mytoken123@github.com/user/repo.git",
		},
		{
			name:  "custom user",
			url:   "https://github.com/user/repo.git",
			token: "token456",
			user:  "customuser",
			want:  "https://customuser:token456@github.com/user/repo.git",
		},
		{
			name:  "empty user defaults to oauth2",
			url:   "https://github.com/user/repo.git",
			token: "token789",
			user:  "",
			want:  "https://oauth2:token789@github.com/user/repo.git",
		},
		{
			name:  "URL with path",
			url:   "https://gitlab.com/group/project/repo.git",
			token: "glpat-xxx",
			user:  "oauth2",
			want:  "https://oauth2:glpat-xxx@gitlab.com/group/project/repo.git",
		},
		{
			name:  "URL already has credentials - no change",
			url:   "https://user:pass@github.com/repo.git",
			token: "newtoken",
			user:  "oauth2",
			want:  "https://user:pass@github.com/repo.git",
		},
		{
			name:  "non-https URL unchanged",
			url:   "git@github.com:user/repo.git",
			token: "token",
			user:  "oauth2",
			want:  "git@github.com:user/repo.git",
		},
		{
			name:  "token with whitespace trimmed",
			url:   "https://github.com/repo.git",
			token: "  token123  \n",
			user:  "oauth2",
			want:  "https://oauth2:token123@github.com/repo.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := addGitToken(tt.url, tt.token, tt.user)
			if got != tt.want {
				t.Errorf("addGitToken(%q, %q, %q) = %q, want %q",
					tt.url, tt.token, tt.user, got, tt.want)
			}
		})
	}
}

// ===== TESTS FOR FormatGitURLForBuildKit() =====

func TestFormatGitURLForBuildKit(t *testing.T) {
	tests := []struct {
		name       string
		gitURL     string
		gitConfig  GitConfig
		subContext string
		want       string
		wantErr    bool
	}{
		{
			name:   "URL with branch",
			gitURL: "https://github.com/user/repo.git",
			gitConfig: GitConfig{
				Branch: "main",
			},
			subContext: "",
			want:       "https://github.com/user/repo.git#main",
			wantErr:    false,
		},
		{
			name:   "URL with revision",
			gitURL: "https://github.com/user/repo.git",
			gitConfig: GitConfig{
				Revision: "abc123def456",
			},
			subContext: "",
			want:       "https://github.com/user/repo.git#abc123def456",
			wantErr:    false,
		},
		{
			name:   "URL with branch and subcontext",
			gitURL: "https://github.com/user/repo.git",
			gitConfig: GitConfig{
				Branch: "develop",
			},
			subContext: "services/api",
			want:       "https://github.com/user/repo.git#develop:services/api",
			wantErr:    false,
		},
		{
			name:   "URL with revision and subcontext",
			gitURL: "https://github.com/user/repo.git",
			gitConfig: GitConfig{
				Revision: "v1.2.3",
			},
			subContext: "docker",
			want:       "https://github.com/user/repo.git#v1.2.3:docker",
			wantErr:    false,
		},
		{
			name:       "URL with only subcontext",
			gitURL:     "https://github.com/user/repo.git",
			gitConfig:  GitConfig{},
			subContext: "build",
			want:       "https://github.com/user/repo.git#:build",
			wantErr:    false,
		},
		{
			name:       "URL with no modifications",
			gitURL:     "https://github.com/user/repo.git",
			gitConfig:  GitConfig{},
			subContext: "",
			want:       "https://github.com/user/repo.git",
			wantErr:    false,
		},
		{
			name:   "revision takes precedence over branch",
			gitURL: "https://github.com/user/repo.git",
			gitConfig: GitConfig{
				Branch:   "main",
				Revision: "abc123",
			},
			subContext: "",
			want:       "https://github.com/user/repo.git#abc123",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatGitURLForBuildKit(tt.gitURL, tt.gitConfig, tt.subContext)

			if (err != nil) != tt.wantErr {
				t.Errorf("FormatGitURLForBuildKit() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if got != tt.want {
				t.Errorf("FormatGitURLForBuildKit() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatGitURLForBuildKit_WithToken(t *testing.T) {
	tmpDir := t.TempDir()
	tokenFile := filepath.Join(tmpDir, "token.txt")
	tokenContent := "ghp_test_token_123"

	err := os.WriteFile(tokenFile, []byte(tokenContent), 0600)
	if err != nil {
		t.Fatalf("Failed to create token file: %v", err)
	}

	gitConfig := GitConfig{
		TokenFile: tokenFile,
		TokenUser: "oauth2",
		Branch:    "main",
	}

	got, err := FormatGitURLForBuildKit("https://github.com/user/repo.git", gitConfig, "")
	if err != nil {
		t.Fatalf("FormatGitURLForBuildKit() error = %v", err)
	}

	// Should contain the token
	if !strings.Contains(got, "oauth2:ghp_test_token_123@") {
		t.Errorf("FormatGitURLForBuildKit() = %q, should contain token", got)
	}

	// Should contain the branch
	if !strings.HasSuffix(got, "#main") {
		t.Errorf("FormatGitURLForBuildKit() = %q, should end with #main", got)
	}
}

func TestFormatGitURLForBuildKit_TokenFileError(t *testing.T) {
	gitConfig := GitConfig{
		TokenFile: "/nonexistent/token.txt",
	}

	_, err := FormatGitURLForBuildKit("https://github.com/user/repo.git", gitConfig, "")
	if err == nil {
		t.Error("FormatGitURLForBuildKit() should error with nonexistent token file")
	}

	if !strings.Contains(err.Error(), "failed to read git token file") {
		t.Errorf("Error should mention token file, got: %v", err)
	}
}

// ===== TESTS FOR maskToken() =====

func TestMaskToken(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "URL with user and token",
			url:  "https://user:ghp_token123@github.com/repo.git",
			want: "https://user:***@github.com/repo.git",
		},
		{
			name: "URL with oauth2 and token",
			url:  "https://oauth2:glpat-abc123@gitlab.com/repo.git",
			want: "https://oauth2:***@gitlab.com/repo.git",
		},
		{
			name: "URL without credentials",
			url:  "https://github.com/user/repo.git",
			want: "https://github.com/user/repo.git",
		},
		{
			name: "URL with @ but no credentials",
			url:  "https://github.com/@user/repo.git",
			want: "https://github.com/@user/repo.git",
		},
		{
			name: "SSH URL",
			url:  "git@github.com:user/repo.git",
			want: "git@github.com:user/repo.git",
		},
		{
			name: "URL with port",
			url:  "https://user:token@localhost:5000/repo.git",
			want: "https://user:***@localhost:5000/repo.git",
		},
		{
			name: "URL with path and query",
			url:  "https://user:token@github.com/repo.git?ref=main#fragment",
			want: "https://user:***@github.com/repo.git?ref=main#fragment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskToken(tt.url)
			if got != tt.want {
				t.Errorf("maskToken(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

// ===== TESTS FOR Prepare() - Local Context =====

func TestPrepare_LocalContext(t *testing.T) {
	t.Run("valid local directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		gitConfig := GitConfig{
			Context: tmpDir,
		}

		ctx, err := Prepare(gitConfig, "buildah")
		if err != nil {
			t.Fatalf("Prepare() error = %v", err)
		}

		if ctx.Path != tmpDir {
			t.Errorf("Context.Path = %q, want %q", ctx.Path, tmpDir)
		}

		if ctx.IsGitRepo {
			t.Error("Context.IsGitRepo should be false for local directory")
		}

		if ctx.TempDir != "" {
			t.Error("Context.TempDir should be empty for local directory")
		}
	})

	t.Run("nonexistent local directory", func(t *testing.T) {
		gitConfig := GitConfig{
			Context: "/nonexistent/directory",
		}

		_, err := Prepare(gitConfig, "buildah")
		if err == nil {
			t.Error("Prepare() should error for nonexistent directory")
		}

		if !strings.Contains(err.Error(), "does not exist") {
			t.Errorf("Error should mention 'does not exist', got: %v", err)
		}
	})

	t.Run("empty context", func(t *testing.T) {
		gitConfig := GitConfig{
			Context: "",
		}

		_, err := Prepare(gitConfig, "buildah")
		if err == nil {
			t.Error("Prepare() should error for empty context")
		}

		if !strings.Contains(err.Error(), "required") {
			t.Errorf("Error should mention 'required', got: %v", err)
		}
	})
}

// ===== TESTS FOR Prepare() - Git Context with BuildKit =====

func TestPrepare_GitContextBuildKit(t *testing.T) {
	t.Run("git URL with buildkit", func(t *testing.T) {
		gitConfig := GitConfig{
			Context: "https://github.com/user/repo.git",
			Branch:  "main",
		}

		ctx, err := Prepare(gitConfig, "buildkit")
		if err != nil {
			t.Fatalf("Prepare() error = %v", err)
		}

		if !ctx.IsGitRepo {
			t.Error("Context.IsGitRepo should be true for git URL")
		}

		if ctx.GitURL == "" {
			t.Error("Context.GitURL should be set")
		}

		if ctx.Path != "" {
			t.Error("Context.Path should be empty for BuildKit with Git URL")
		}

		if ctx.TempDir != "" {
			t.Error("Context.TempDir should be empty for BuildKit")
		}
	})

	t.Run("git:// URL normalized with buildkit", func(t *testing.T) {
		gitConfig := GitConfig{
			Context: "git://github.com/user/repo.git",
		}

		ctx, err := Prepare(gitConfig, "buildkit")
		if err != nil {
			t.Fatalf("Prepare() error = %v", err)
		}

		// Should be normalized to https://
		if !strings.HasPrefix(ctx.GitURL, "https://") {
			t.Errorf("GitURL should be normalized to https://, got: %s", ctx.GitURL)
		}
	})

	t.Run("git@ SSH URL normalized with buildkit", func(t *testing.T) {
		gitConfig := GitConfig{
			Context: "git@github.com:user/repo.git",
		}

		ctx, err := Prepare(gitConfig, "buildkit")
		if err != nil {
			t.Fatalf("Prepare() error = %v", err)
		}

		// Should be normalized to https:// by default (unless KIMIA_PREFER_SSH is set)
		// Note: The GitURL field will contain the normalized URL
		if ctx.GitURL == "" {
			t.Error("GitURL should be set for git context")
		}

		// By default (without KIMIA_PREFER_SSH), SSH URLs are converted to HTTPS
		if !strings.HasPrefix(ctx.GitURL, "https://") && !strings.HasPrefix(ctx.GitURL, "git@") {
			t.Errorf("GitURL should be either https:// or git@, got: %s", ctx.GitURL)
		}
	})
}

// ===== TESTS FOR Prepare() - Git Context with Buildah =====

func TestPrepare_GitContextBuildah(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping git clone tests in short mode")
	}

	// Check if git is available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git not available, skipping git clone tests")
	}

	t.Run("git URL with buildah requires clone", func(t *testing.T) {
		// This test would require actual git clone, which needs network
		// We'll test the logic without actual cloning
		gitConfig := GitConfig{
			Context: "https://github.com/user/nonexistentrepo12345.git",
			Branch:  "main",
		}

		// This will fail because the repo doesn't exist, but we can verify
		// it attempts to clone for buildah
		_, err := Prepare(gitConfig, "buildah")

		// Should error trying to clone (network/repo not found)
		if err == nil {
			t.Error("Prepare() should error when trying to clone non-existent repo")
		}
	})
}

// ===== TESTS FOR Prepare() - Environment Variable Expansion =====

func TestPrepare_EnvExpansion(t *testing.T) {
	tmpDir := t.TempDir()

	// Set test environment variable
	os.Setenv("TEST_REPO_PATH", tmpDir)
	defer os.Unsetenv("TEST_REPO_PATH")

	gitConfig := GitConfig{
		Context: "${TEST_REPO_PATH}",
	}

	ctx, err := Prepare(gitConfig, "buildah")
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	if ctx.Path != tmpDir {
		t.Errorf("Context.Path = %q, want %q (env var should be expanded)", ctx.Path, tmpDir)
	}
}

// ===== EDGE CASE TESTS =====

func TestContext_EdgeCases(t *testing.T) {
	t.Run("context with special characters in path", func(t *testing.T) {
		// Create a directory with spaces
		tmpDir := t.TempDir()
		specialDir := filepath.Join(tmpDir, "path with spaces")
		os.MkdirAll(specialDir, 0755)

		gitConfig := GitConfig{
			Context: specialDir,
		}

		ctx, err := Prepare(gitConfig, "buildah")
		if err != nil {
			t.Fatalf("Prepare() should handle special characters: %v", err)
		}

		if ctx.Path != specialDir {
			t.Errorf("Path with special chars not preserved correctly")
		}
	})

	t.Run("cleanup called multiple times", func(t *testing.T) {
		tmpDir := t.TempDir()
		tempSubDir := filepath.Join(tmpDir, "temp")
		os.MkdirAll(tempSubDir, 0755)

		ctx := &Context{
			TempDir: tempSubDir,
		}

		// Should not panic when called multiple times
		ctx.Cleanup()
		ctx.Cleanup()
		ctx.Cleanup()
	})
}

// ===== BENCHMARK TESTS =====

func BenchmarkIsGitURL(b *testing.B) {
	urls := []string{
		"https://github.com/user/repo.git",
		"git@github.com:user/repo.git",
		"/local/path/to/repo",
		"https://gitlab.com/project/repo",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		isGitURL(urls[i%len(urls)])
	}
}

func BenchmarkNormalizeGitURL(b *testing.B) {
	url := "git@github.com:user/repo.git"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		normalizeGitURL(url)
	}
}

func BenchmarkAddGitToken(b *testing.B) {
	url := "https://github.com/user/repo.git"
	token := "ghp_test_token_123"
	user := "oauth2"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		addGitToken(url, token, user)
	}
}

func BenchmarkMaskToken(b *testing.B) {
	url := "https://user:ghp_secret_token_123@github.com/repo.git"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		maskToken(url)
	}
}
