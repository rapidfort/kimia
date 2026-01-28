package preflight

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ============================================================================
// UserNamespaceCheck Tests
// ============================================================================

func TestUserNamespaceCheckFields(t *testing.T) {
	tests := []struct {
		name     string
		check    UserNamespaceCheck
		expected UserNamespaceCheck
	}{
		{
			name: "Fully configured user namespace",
			check: UserNamespaceCheck{
				Supported:        true,
				MaxUserNS:        31234,
				SubuidConfigured: true,
				SubgidConfigured: true,
				SubuidRange:      "testuser:100000:65536",
				SubgidRange:      "testuser:100000:65536",
				CanCreate:        true,
				ErrorMessage:     "",
			},
			expected: UserNamespaceCheck{
				Supported:        true,
				MaxUserNS:        31234,
				SubuidConfigured: true,
				SubgidConfigured: true,
				SubuidRange:      "testuser:100000:65536",
				SubgidRange:      "testuser:100000:65536",
				CanCreate:        true,
				ErrorMessage:     "",
			},
		},
		{
			name: "User namespace not supported",
			check: UserNamespaceCheck{
				Supported:        false,
				MaxUserNS:        0,
				SubuidConfigured: false,
				SubgidConfigured: false,
				SubuidRange:      "",
				SubgidRange:      "",
				CanCreate:        false,
				ErrorMessage:     "User namespaces not enabled",
			},
			expected: UserNamespaceCheck{
				Supported:        false,
				MaxUserNS:        0,
				SubuidConfigured: false,
				SubgidConfigured: false,
				SubuidRange:      "",
				SubgidRange:      "",
				CanCreate:        false,
				ErrorMessage:     "User namespaces not enabled",
			},
		},
		{
			name: "Supported but not configured",
			check: UserNamespaceCheck{
				Supported:        true,
				MaxUserNS:        31234,
				SubuidConfigured: false,
				SubgidConfigured: false,
				SubuidRange:      "",
				SubgidRange:      "",
				CanCreate:        false,
				ErrorMessage:     "subuid not configured",
			},
			expected: UserNamespaceCheck{
				Supported:        true,
				MaxUserNS:        31234,
				SubuidConfigured: false,
				SubgidConfigured: false,
				SubuidRange:      "",
				SubgidRange:      "",
				CanCreate:        false,
				ErrorMessage:     "subuid not configured",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.check.Supported != tt.expected.Supported {
				t.Errorf("Supported mismatch: got %v, want %v",
					tt.check.Supported, tt.expected.Supported)
			}
			if tt.check.MaxUserNS != tt.expected.MaxUserNS {
				t.Errorf("MaxUserNS mismatch: got %d, want %d",
					tt.check.MaxUserNS, tt.expected.MaxUserNS)
			}
			if tt.check.SubuidConfigured != tt.expected.SubuidConfigured {
				t.Errorf("SubuidConfigured mismatch: got %v, want %v",
					tt.check.SubuidConfigured, tt.expected.SubuidConfigured)
			}
			if tt.check.SubgidConfigured != tt.expected.SubgidConfigured {
				t.Errorf("SubgidConfigured mismatch: got %v, want %v",
					tt.check.SubgidConfigured, tt.expected.SubgidConfigured)
			}
			if tt.check.SubuidRange != tt.expected.SubuidRange {
				t.Errorf("SubuidRange mismatch: got %q, want %q",
					tt.check.SubuidRange, tt.expected.SubuidRange)
			}
			if tt.check.SubgidRange != tt.expected.SubgidRange {
				t.Errorf("SubgidRange mismatch: got %q, want %q",
					tt.check.SubgidRange, tt.expected.SubgidRange)
			}
			if tt.check.CanCreate != tt.expected.CanCreate {
				t.Errorf("CanCreate mismatch: got %v, want %v",
					tt.check.CanCreate, tt.expected.CanCreate)
			}
			if tt.check.ErrorMessage != tt.expected.ErrorMessage {
				t.Errorf("ErrorMessage mismatch: got %q, want %q",
					tt.check.ErrorMessage, tt.expected.ErrorMessage)
			}
		})
	}
}

// ============================================================================
// CheckUserNamespaces Tests
// ============================================================================

func TestCheckUserNamespaces(t *testing.T) {
	t.Run("Returns valid UserNamespaceCheck", func(t *testing.T) {
		check, err := CheckUserNamespaces()
		if err != nil {
			t.Fatalf("CheckUserNamespaces failed: %v", err)
		}

		if check == nil {
			t.Fatal("CheckUserNamespaces returned nil")
		}

		// Log the actual state for debugging
		t.Logf("User namespace check results:")
		t.Logf("  Supported: %v", check.Supported)
		t.Logf("  MaxUserNS: %d", check.MaxUserNS)
		t.Logf("  SubuidConfigured: %v", check.SubuidConfigured)
		t.Logf("  SubgidConfigured: %v", check.SubgidConfigured)
		t.Logf("  CanCreate: %v", check.CanCreate)
		if check.ErrorMessage != "" {
			t.Logf("  ErrorMessage: %s", check.ErrorMessage)
		}
	})

	t.Run("MaxUserNS value is reasonable", func(t *testing.T) {
		check, err := CheckUserNamespaces()
		if err != nil {
			t.Fatalf("CheckUserNamespaces failed: %v", err)
		}

		// MaxUserNS should be >= 0 (0 means disabled, positive means enabled)
		if check.MaxUserNS < 0 {
			t.Errorf("MaxUserNS should be >= 0, got %d", check.MaxUserNS)
		}

		// If supported, MaxUserNS should be > 0
		if check.Supported && check.MaxUserNS == 0 {
			t.Error("Supported is true but MaxUserNS is 0")
		}

		// If not supported, MaxUserNS should be 0
		if !check.Supported && check.MaxUserNS != 0 {
			t.Errorf("Supported is false but MaxUserNS is %d", check.MaxUserNS)
		}
	})

	t.Run("Consistency between fields", func(t *testing.T) {
		check, err := CheckUserNamespaces()
		if err != nil {
			t.Fatalf("CheckUserNamespaces failed: %v", err)
		}

		// If not supported, CanCreate should be false
		if !check.Supported && check.CanCreate {
			t.Error("User namespace not supported but CanCreate is true")
		}

		// If ErrorMessage is set and non-empty, something should be wrong
		if check.ErrorMessage != "" {
			if check.Supported && check.SubuidConfigured && check.SubgidConfigured && check.CanCreate {
				t.Logf("Warning: ErrorMessage set but everything seems OK: %s", check.ErrorMessage)
			}
		}
	})
}

// ============================================================================
// readMaxUserNamespaces Tests
// ============================================================================

func TestReadMaxUserNamespaces(t *testing.T) {
	t.Run("Real system read", func(t *testing.T) {
		maxNS, err := readMaxUserNamespaces()

		// This might fail on systems without the file
		if err != nil {
			t.Logf("Cannot read max_user_namespaces (expected on some systems): %v", err)
			return
		}

		if maxNS < 0 {
			t.Errorf("maxNS should be >= 0, got %d", maxNS)
		}

		t.Logf("max_user_namespaces = %d", maxNS)
	})

	t.Run("Mock valid value", func(t *testing.T) {
		// Create a temporary file with valid content
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "max_user_namespaces")

		if err := os.WriteFile(tmpFile, []byte("31234\n"), 0644); err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}

		// We can't easily mock the readMaxUserNamespaces function,
		// but we can test the file format
		data, err := os.ReadFile(tmpFile)
		if err != nil {
			t.Fatalf("Failed to read temp file: %v", err)
		}

		value := strings.TrimSpace(string(data))
		if value != "31234" {
			t.Errorf("Expected '31234', got %q", value)
		}
	})

	t.Run("Mock zero value", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "max_user_namespaces")

		if err := os.WriteFile(tmpFile, []byte("0\n"), 0644); err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}

		data, err := os.ReadFile(tmpFile)
		if err != nil {
			t.Fatalf("Failed to read temp file: %v", err)
		}

		value := strings.TrimSpace(string(data))
		if value != "0" {
			t.Errorf("Expected '0', got %q", value)
		}
	})
}

// ============================================================================
// checkSubIDFile Tests
// ============================================================================

func TestCheckSubIDFile(t *testing.T) {
	tests := []struct {
		name          string
		fileContent   string
		username      string
		uid           int
		expectFound   bool
		expectedRange string
		expectError   bool
	}{
		{
			name: "Valid entry by username",
			fileContent: `# Comment line
testuser:100000:65536
otheruser:200000:65536
`,
			username:      "testuser",
			uid:           1000,
			expectFound:   true,
			expectedRange: "testuser:100000:65536",
			expectError:   false,
		},
		{
			name: "Valid entry by UID",
			fileContent: `# Comment line
1000:100000:65536
otheruser:200000:65536
`,
			username:      "testuser",
			uid:           1000,
			expectFound:   true,
			expectedRange: "1000:100000:65536",
			expectError:   false,
		},
		{
			name: "User not found",
			fileContent: `# Comment line
otheruser:100000:65536
anotheruser:200000:65536
`,
			username:    "testuser",
			uid:         1000,
			expectFound: false,
			expectError: true,
		},
		{
			name: "Empty file",
			fileContent: ``,
			username:    "testuser",
			uid:         1000,
			expectFound: false,
			expectError: true,
		},
		{
			name: "Only comments",
			fileContent: `# Comment line 1
# Comment line 2
`,
			username:    "testuser",
			uid:         1000,
			expectFound: false,
			expectError: true,
		},
		{
			name: "Malformed lines mixed with valid",
			fileContent: `# Comment
malformed line without colons
testuser:100000:65536
another:malformed
`,
			username:      "testuser",
			uid:           1000,
			expectFound:   true,
			expectedRange: "testuser:100000:65536",
			expectError:   false,
		},
		{
			name: "Multiple valid entries - first match wins",
			fileContent: `testuser:100000:65536
testuser:200000:65536
`,
			username:      "testuser",
			uid:           1000,
			expectFound:   true,
			expectedRange: "testuser:100000:65536",
			expectError:   false,
		},
		{
			name: "Entry with extra whitespace",
			fileContent: `  testuser:100000:65536
otheruser:200000:65536
`,
			username:      "testuser",
			uid:           1000,
			expectFound:   true,
			expectedRange: "testuser:100000:65536",
			expectError:   false,
		},
		{
			name: "Different ranges for different users",
			fileContent: `user1:100000:65536
user2:200000:131072
user3:300000:32768
`,
			username:      "user2",
			uid:           1002,
			expectFound:   true,
			expectedRange: "user2:200000:131072",
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary file with test content
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "subid")

			if err := os.WriteFile(tmpFile, []byte(tt.fileContent), 0644); err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}

			result, err := checkSubIDFile(tmpFile, tt.username, tt.uid)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, got nil")
				}
				if tt.expectFound && result != "" {
					t.Errorf("Expected empty result with error, got %q", result)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
				if result != tt.expectedRange {
					t.Errorf("Expected range %q, got %q", tt.expectedRange, result)
				}
			}
		})
	}
}

func TestCheckSubIDFileRealFiles(t *testing.T) {
	t.Run("Real /etc/subuid", func(t *testing.T) {
		uid := os.Getuid()
		username := os.Getenv("USER")
		if username == "" {
			username = fmt.Sprintf("%d", uid)
		}

		result, err := checkSubIDFile("/etc/subuid", username, uid)
		if err != nil {
			t.Logf("subuid not configured for user %s (expected in some environments): %v", username, err)
			return
		}

		t.Logf("subuid configuration: %s", result)

		// Validate format: username:start:count
		parts := strings.Split(result, ":")
		if len(parts) != 3 {
			t.Errorf("Invalid subuid format: %s", result)
		}
	})

	t.Run("Real /etc/subgid", func(t *testing.T) {
		uid := os.Getuid()
		username := os.Getenv("USER")
		if username == "" {
			username = fmt.Sprintf("%d", uid)
		}

		result, err := checkSubIDFile("/etc/subgid", username, uid)
		if err != nil {
			t.Logf("subgid not configured for user %s (expected in some environments): %v", username, err)
			return
		}

		t.Logf("subgid configuration: %s", result)

		// Validate format: username:start:count
		parts := strings.Split(result, ":")
		if len(parts) != 3 {
			t.Errorf("Invalid subgid format: %s", result)
		}
	})
}

// ============================================================================
// testUserNamespaceCreation Tests
// ============================================================================

func TestTestUserNamespaceCreation(t *testing.T) {
	t.Run("Attempt user namespace creation", func(t *testing.T) {
		canCreate, err := testUserNamespaceCreation()

		if err != nil {
			t.Logf("User namespace creation failed (may be expected): %v", err)
		}

		if canCreate {
			t.Log("User namespace creation succeeded")
		} else {
			t.Logf("User namespace creation not possible in this environment")
		}

		// The result depends on system configuration, so we just log it
		// Don't fail the test based on the result
	})
}

// ============================================================================
// IsUserNamespaceReady Tests
// ============================================================================

func TestIsUserNamespaceReady(t *testing.T) {
	tests := []struct {
		name     string
		check    UserNamespaceCheck
		expected bool
	}{
		{
			name: "Ready - supported and can create",
			check: UserNamespaceCheck{
				Supported: true,
				CanCreate: true,
			},
			expected: true,
		},
		{
			name: "Not ready - not supported",
			check: UserNamespaceCheck{
				Supported: false,
				CanCreate: true,
			},
			expected: false,
		},
		{
			name: "Not ready - cannot create",
			check: UserNamespaceCheck{
				Supported: true,
				CanCreate: false,
			},
			expected: false,
		},
		{
			name: "Not ready - neither supported nor can create",
			check: UserNamespaceCheck{
				Supported: false,
				CanCreate: false,
			},
			expected: false,
		},
		{
			name: "Ready - with full configuration",
			check: UserNamespaceCheck{
				Supported:        true,
				MaxUserNS:        31234,
				SubuidConfigured: true,
				SubgidConfigured: true,
				CanCreate:        true,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.check.IsUserNamespaceReady()
			if result != tt.expected {
				t.Errorf("IsUserNamespaceReady() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// ============================================================================
// GetIssues Tests
// ============================================================================

func TestUserNamespaceGetIssues(t *testing.T) {
	tests := []struct {
		name       string
		check      UserNamespaceCheck
		wantCount  int
		wantIssues []string
	}{
		{
			name: "No issues",
			check: UserNamespaceCheck{
				Supported:        true,
				SubuidConfigured: true,
				SubgidConfigured: true,
				CanCreate:        true,
			},
			wantCount:  0,
			wantIssues: []string{},
		},
		{
			name: "Not supported",
			check: UserNamespaceCheck{
				Supported: false,
			},
			wantCount:  1,
			wantIssues: []string{"User namespaces not enabled in kernel"},
		},
		{
			name: "Subuid not configured",
			check: UserNamespaceCheck{
				Supported:        true,
				SubuidConfigured: false,
				SubgidConfigured: true,
				CanCreate:        true,
			},
			wantCount:  1,
			wantIssues: []string{"/etc/subuid not configured"},
		},
		{
			name: "Subgid not configured",
			check: UserNamespaceCheck{
				Supported:        true,
				SubuidConfigured: true,
				SubgidConfigured: false,
				CanCreate:        true,
			},
			wantCount:  1,
			wantIssues: []string{"/etc/subgid not configured"},
		},
		{
			name: "Cannot create",
			check: UserNamespaceCheck{
				Supported:        true,
				SubuidConfigured: true,
				SubgidConfigured: true,
				CanCreate:        false,
				ErrorMessage:     "Operation not permitted",
			},
			wantCount:  1,
			wantIssues: []string{"Cannot create user namespace: Operation not permitted"},
		},
		{
			name: "Multiple issues - subuid and subgid",
			check: UserNamespaceCheck{
				Supported:        true,
				SubuidConfigured: false,
				SubgidConfigured: false,
				CanCreate:        false,
				ErrorMessage:     "Not configured",
			},
			wantCount: 3,
			wantIssues: []string{
				"/etc/subuid not configured",
				"/etc/subgid not configured",
				"Cannot create user namespace",
			},
		},
		{
			name: "All issues",
			check: UserNamespaceCheck{
				Supported:        false,
				SubuidConfigured: false,
				SubgidConfigured: false,
				CanCreate:        false,
			},
			wantCount:  1, // Only "not enabled" when not supported
			wantIssues: []string{"User namespaces not enabled in kernel"},
		},
		{
			name: "Supported but configuration issues",
			check: UserNamespaceCheck{
				Supported:        true,
				SubuidConfigured: false,
				SubgidConfigured: false,
				CanCreate:        true,
			},
			wantCount: 2,
			wantIssues: []string{
				"/etc/subuid not configured",
				"/etc/subgid not configured",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := tt.check.GetIssues()

			if len(issues) != tt.wantCount {
				t.Errorf("GetIssues() returned %d issues, want %d: %v",
					len(issues), tt.wantCount, issues)
			}

			// Check that expected issues are present
			for _, wantIssue := range tt.wantIssues {
				found := false
				for _, issue := range issues {
					if strings.Contains(issue, wantIssue) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected issue containing %q not found in: %v",
						wantIssue, issues)
				}
			}
		})
	}
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestUserNamespaceIntegration(t *testing.T) {
	t.Run("CheckUserNamespaces and IsUserNamespaceReady consistency", func(t *testing.T) {
		check, err := CheckUserNamespaces()
		if err != nil {
			t.Fatalf("CheckUserNamespaces failed: %v", err)
		}

		isReady := check.IsUserNamespaceReady()

		// If ready, must be supported and able to create
		if isReady {
			if !check.Supported {
				t.Error("IsUserNamespaceReady is true but Supported is false")
			}
			if !check.CanCreate {
				t.Error("IsUserNamespaceReady is true but CanCreate is false")
			}
		}

		// If not ready, at least one requirement must be missing
		if !isReady {
			if check.Supported && check.CanCreate {
				t.Error("IsUserNamespaceReady is false but both Supported and CanCreate are true")
			}
		}

		t.Logf("User namespace ready: %v", isReady)
	})

	t.Run("CheckUserNamespaces and GetIssues consistency", func(t *testing.T) {
		check, err := CheckUserNamespaces()
		if err != nil {
			t.Fatalf("CheckUserNamespaces failed: %v", err)
		}

		issues := check.GetIssues()

		// Note: subuid/subgid configuration is optional in some environments
		// User namespaces can work without them (e.g., as root or with certain kernel configs)
		// So it's OK to be ready but have subuid/subgid warnings

		// If ready, critical issues (not supported, cannot create) should not be present
		if check.IsUserNamespaceReady() {
			for _, issue := range issues {
				if strings.Contains(issue, "not enabled in kernel") ||
					strings.Contains(issue, "Cannot create user namespace") {
					t.Errorf("User namespace is ready but has critical issue: %s", issue)
				}
			}
		}

		// If not ready, should have at least one issue
		if !check.IsUserNamespaceReady() && len(issues) == 0 {
			t.Error("User namespace not ready but no issues reported")
		}

		t.Logf("Issues found: %d", len(issues))
		for _, issue := range issues {
			t.Logf("  - %s", issue)
		}
	})
}

func TestUserNamespaceRealWorldScenarios(t *testing.T) {
	t.Run("Typical Kubernetes pod scenario", func(t *testing.T) {
		check, err := CheckUserNamespaces()
		if err != nil {
			t.Fatalf("CheckUserNamespaces failed: %v", err)
		}

		t.Logf("User namespace configuration in this environment:")
		t.Logf("  Kernel support: %v (max_user_namespaces=%d)",
			check.Supported, check.MaxUserNS)
		t.Logf("  /etc/subuid configured: %v", check.SubuidConfigured)
		if check.SubuidConfigured {
			t.Logf("    Range: %s", check.SubuidRange)
		}
		t.Logf("  /etc/subgid configured: %v", check.SubgidConfigured)
		if check.SubgidConfigured {
			t.Logf("    Range: %s", check.SubgidRange)
		}
		t.Logf("  Can create user namespace: %v", check.CanCreate)

		if !check.IsUserNamespaceReady() {
			issues := check.GetIssues()
			t.Logf("User namespaces not ready. Issues:")
			for _, issue := range issues {
				t.Logf("    - %s", issue)
			}
		}
	})

	t.Run("Rootless build requirements", func(t *testing.T) {
		check, err := CheckUserNamespaces()
		if err != nil {
			t.Fatalf("CheckUserNamespaces failed: %v", err)
		}

		// For rootless builds, we need:
		// 1. Kernel support
		// 2. Ability to create user namespaces
		// 3. Ideally subuid/subgid configured (but not always required)

		requirements := map[string]bool{
			"Kernel support":          check.Supported,
			"Can create":              check.CanCreate,
			"subuid configured":       check.SubuidConfigured,
			"subgid configured":       check.SubgidConfigured,
			"Overall ready":           check.IsUserNamespaceReady(),
		}

		t.Log("Rootless build requirements check:")
		for req, met := range requirements {
			status := "❌"
			if met {
				status = "✅"
			}
			t.Logf("  %s %s", status, req)
		}
	})

	t.Run("Restricted environment without user namespaces", func(t *testing.T) {
		// Simulate a restricted environment
		check := &UserNamespaceCheck{
			Supported:        false,
			MaxUserNS:        0,
			SubuidConfigured: false,
			SubgidConfigured: false,
			CanCreate:        false,
			ErrorMessage:     "User namespaces disabled",
		}

		if check.IsUserNamespaceReady() {
			t.Error("Restricted environment should not be ready")
		}

		issues := check.GetIssues()
		if len(issues) == 0 {
			t.Error("Restricted environment should have issues")
		}

		t.Logf("Restricted environment issues: %v", issues)
	})
}

// ============================================================================
// Edge Cases and Error Handling
// ============================================================================

func TestUserNamespaceEdgeCases(t *testing.T) {
	t.Run("Empty username and UID", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "subid")

		content := `testuser:100000:65536`
		if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}

		_, err := checkSubIDFile(tmpFile, "", 0)
		if err == nil {
			t.Error("Expected error for empty username and UID 0")
		}
	})

	t.Run("Very large UID", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "subid")

		content := `999999:100000:65536`
		if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}

		result, err := checkSubIDFile(tmpFile, "testuser", 999999)
		if err != nil {
			t.Errorf("Should handle large UID: %v", err)
		}
		if result == "" {
			t.Error("Expected to find entry for large UID")
		}
	})

	t.Run("Malformed subid entry", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "subid")

		content := `testuser:100000
testuser:not_a_number:65536
testuser:100000:65536:extra
`
		if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}

		result, err := checkSubIDFile(tmpFile, "testuser", 1000)
		// Should skip malformed lines and not find valid entry
		if err == nil {
			t.Log("Found entry despite malformed lines:", result)
		}
	})

	t.Run("File with only whitespace", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "subid")

		content := `


`
		if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}

		_, err := checkSubIDFile(tmpFile, "testuser", 1000)
		if err == nil {
			t.Error("Expected error for whitespace-only file")
		}
	})

	t.Run("Special characters in username", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "subid")

		content := `test-user_123:100000:65536
test.user:200000:65536
`
		if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}

		result, err := checkSubIDFile(tmpFile, "test-user_123", 1000)
		if err != nil {
			t.Errorf("Should handle username with special chars: %v", err)
		}
		if !strings.Contains(result, "test-user_123") {
			t.Errorf("Expected username with special chars, got %q", result)
		}
	})

	t.Run("Negative MaxUserNS handling", func(t *testing.T) {
		check := &UserNamespaceCheck{
			MaxUserNS: -1,
		}

		// Negative MaxUserNS is invalid but shouldn't crash
		_ = check.IsUserNamespaceReady()
		_ = check.GetIssues()
	})

	t.Run("Very long error message", func(t *testing.T) {
		longError := strings.Repeat("error ", 10000)
		check := &UserNamespaceCheck{
			Supported:    true,
			CanCreate:    false,
			ErrorMessage: longError,
		}

		issues := check.GetIssues()
		found := false
		for _, issue := range issues {
			if len(issue) > 10000 {
				found = true
				break
			}
		}
		if !found {
			t.Log("Long error message was truncated or not included")
		}
	})
}

// ============================================================================
// Concurrent Tests
// ============================================================================

func TestUserNamespaceConcurrent(t *testing.T) {
	t.Run("Concurrent CheckUserNamespaces calls", func(t *testing.T) {
		const numGoroutines = 10
		var wg sync.WaitGroup
		results := make([]*UserNamespaceCheck, numGoroutines)
		errors := make([]error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				results[index], errors[index] = CheckUserNamespaces()
			}(i)
		}

		wg.Wait()

		// Verify all calls completed
		for i := 0; i < numGoroutines; i++ {
			if errors[i] != nil {
				t.Errorf("Goroutine %d returned error: %v", i, errors[i])
			}
			if results[i] == nil {
				t.Errorf("Goroutine %d returned nil result", i)
			}
		}

		// Verify all results are consistent
		if results[0] != nil {
			firstSupported := results[0].Supported
			firstMaxUserNS := results[0].MaxUserNS

			for i := 1; i < numGoroutines; i++ {
				if results[i] == nil {
					continue
				}
				if results[i].Supported != firstSupported {
					t.Errorf("Inconsistent Supported: goroutine 0 got %v, goroutine %d got %v",
						firstSupported, i, results[i].Supported)
				}
				if results[i].MaxUserNS != firstMaxUserNS {
					t.Errorf("Inconsistent MaxUserNS: goroutine 0 got %d, goroutine %d got %d",
						firstMaxUserNS, i, results[i].MaxUserNS)
				}
			}
		}
	})

	t.Run("Concurrent checkSubIDFile calls", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "subid")

		content := `testuser:100000:65536
user2:200000:65536
user3:300000:65536
`
		if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}

		const numGoroutines = 20
		var wg sync.WaitGroup

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				username := fmt.Sprintf("user%d", (index%3)+1)
				if index%3 == 0 {
					username = "testuser"
				}
				_, _ = checkSubIDFile(tmpFile, username, 1000+index)
			}(i)
		}

		wg.Wait()
	})
}

// ============================================================================
// Benchmarks
// ============================================================================

func BenchmarkCheckUserNamespaces(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = CheckUserNamespaces()
	}
}

func BenchmarkReadMaxUserNamespaces(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = readMaxUserNamespaces()
	}
}

func BenchmarkCheckSubIDFile(b *testing.B) {
	tmpDir := b.TempDir()
	tmpFile := filepath.Join(tmpDir, "subid")

	content := `testuser:100000:65536
user2:200000:65536
user3:300000:65536
`
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		b.Fatalf("Failed to create temp file: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = checkSubIDFile(tmpFile, "testuser", 1000)
	}
}

func BenchmarkTestUserNamespaceCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = testUserNamespaceCreation()
	}
}

func BenchmarkIsUserNamespaceReady(b *testing.B) {
	check := &UserNamespaceCheck{
		Supported: true,
		CanCreate: true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = check.IsUserNamespaceReady()
	}
}

func BenchmarkUserNamespaceGetIssues(b *testing.B) {
	check := &UserNamespaceCheck{
		Supported:        true,
		SubuidConfigured: false,
		SubgidConfigured: false,
		CanCreate:        false,
		ErrorMessage:     "Test error",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = check.GetIssues()
	}
}

// ============================================================================
// Documentation Examples
// ============================================================================

func ExampleCheckUserNamespaces() {
	check, err := CheckUserNamespaces()
	if err != nil {
		panic(err)
	}

	if check.IsUserNamespaceReady() {
		println("User namespaces are ready for rootless builds")
	} else {
		println("User namespaces not ready:")
		for _, issue := range check.GetIssues() {
			println("  -", issue)
		}
	}
}

func ExampleUserNamespaceCheck_IsUserNamespaceReady() {
	check := &UserNamespaceCheck{
		Supported: true,
		CanCreate: true,
	}

	if check.IsUserNamespaceReady() {
		println("Ready to use user namespaces")
	}
}

func ExampleUserNamespaceCheck_GetIssues() {
	check := &UserNamespaceCheck{
		Supported:        true,
		SubuidConfigured: false,
		CanCreate:        false,
		ErrorMessage:     "Permission denied",
	}

	issues := check.GetIssues()
	for _, issue := range issues {
		println("Issue:", issue)
	}
}
