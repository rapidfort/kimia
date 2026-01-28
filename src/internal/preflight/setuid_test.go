package preflight

import (
	"os"
	"path/filepath"
	"testing"
)

// ===== TESTS FOR SetuidBinaryCheck Struct =====

func TestSetuidBinaryCheck_Struct(t *testing.T) {
	t.Run("create and verify struct", func(t *testing.T) {
		check := &SetuidBinaryCheck{
			NewuidmapPresent: true,
			NewgidmapPresent: true,
			NewuidmapSetuid:  true,
			NewgidmapSetuid:  true,
			NewuidmapPath:    "/usr/bin/newuidmap",
			NewgidmapPath:    "/usr/bin/newgidmap",
			BothAvailable:    true,
		}

		if !check.NewuidmapPresent {
			t.Error("NewuidmapPresent should be true")
		}
		if !check.NewgidmapPresent {
			t.Error("NewgidmapPresent should be true")
		}
		if !check.NewuidmapSetuid {
			t.Error("NewuidmapSetuid should be true")
		}
		if !check.NewgidmapSetuid {
			t.Error("NewgidmapSetuid should be true")
		}
		if check.NewuidmapPath != "/usr/bin/newuidmap" {
			t.Errorf("NewuidmapPath = %q; want /usr/bin/newuidmap", check.NewuidmapPath)
		}
		if check.NewgidmapPath != "/usr/bin/newgidmap" {
			t.Errorf("NewgidmapPath = %q; want /usr/bin/newgidmap", check.NewgidmapPath)
		}
		if !check.BothAvailable {
			t.Error("BothAvailable should be true")
		}
	})

	t.Run("zero value struct", func(t *testing.T) {
		check := &SetuidBinaryCheck{}

		if check.NewuidmapPresent {
			t.Error("Default NewuidmapPresent should be false")
		}
		if check.NewgidmapPresent {
			t.Error("Default NewgidmapPresent should be false")
		}
		if check.NewuidmapSetuid {
			t.Error("Default NewuidmapSetuid should be false")
		}
		if check.NewgidmapSetuid {
			t.Error("Default NewgidmapSetuid should be false")
		}
		if check.NewuidmapPath != "" {
			t.Error("Default NewuidmapPath should be empty")
		}
		if check.NewgidmapPath != "" {
			t.Error("Default NewgidmapPath should be empty")
		}
		if check.BothAvailable {
			t.Error("Default BothAvailable should be false")
		}
	})
}

// ===== TESTS FOR CheckSetuidBinaries() FUNCTION =====

func TestCheckSetuidBinaries(t *testing.T) {
	t.Run("real system check", func(t *testing.T) {
		result, err := CheckSetuidBinaries()
		if err != nil {
			t.Fatalf("CheckSetuidBinaries() failed: %v", err)
		}

		if result == nil {
			t.Fatal("CheckSetuidBinaries() returned nil result")
		}

		// Log what was found
		t.Logf("newuidmap present: %v", result.NewuidmapPresent)
		t.Logf("newuidmap SETUID: %v", result.NewuidmapSetuid)
		t.Logf("newuidmap path: %s", result.NewuidmapPath)
		t.Logf("newgidmap present: %v", result.NewgidmapPresent)
		t.Logf("newgidmap SETUID: %v", result.NewgidmapSetuid)
		t.Logf("newgidmap path: %s", result.NewgidmapPath)
		t.Logf("Both available: %v", result.BothAvailable)
	})
}

// ===== TESTS FOR HasSetuidBinaries() FUNCTION =====

func TestHasSetuidBinaries(t *testing.T) {
	tests := []struct {
		name string
		check *SetuidBinaryCheck
		want  bool
	}{
		{
			name: "both available",
			check: &SetuidBinaryCheck{
				NewuidmapPresent: true,
				NewgidmapPresent: true,
				NewuidmapSetuid:  true,
				NewgidmapSetuid:  true,
				BothAvailable:    true,
			},
			want: true,
		},
		{
			name: "missing newuidmap",
			check: &SetuidBinaryCheck{
				NewuidmapPresent: false,
				NewgidmapPresent: true,
				NewuidmapSetuid:  false,
				NewgidmapSetuid:  true,
				BothAvailable:    false,
			},
			want: false,
		},
		{
			name: "missing newgidmap",
			check: &SetuidBinaryCheck{
				NewuidmapPresent: true,
				NewgidmapPresent: false,
				NewuidmapSetuid:  true,
				NewgidmapSetuid:  false,
				BothAvailable:    false,
			},
			want: false,
		},
		{
			name: "missing SETUID on newuidmap",
			check: &SetuidBinaryCheck{
				NewuidmapPresent: true,
				NewgidmapPresent: true,
				NewuidmapSetuid:  false,
				NewgidmapSetuid:  true,
				BothAvailable:    false,
			},
			want: false,
		},
		{
			name: "missing SETUID on newgidmap",
			check: &SetuidBinaryCheck{
				NewuidmapPresent: true,
				NewgidmapPresent: true,
				NewuidmapSetuid:  true,
				NewgidmapSetuid:  false,
				BothAvailable:    false,
			},
			want: false,
		},
		{
			name: "missing everything",
			check: &SetuidBinaryCheck{
				NewuidmapPresent: false,
				NewgidmapPresent: false,
				NewuidmapSetuid:  false,
				NewgidmapSetuid:  false,
				BothAvailable:    false,
			},
			want: false,
		},
		{
			name: "present but no SETUID bits",
			check: &SetuidBinaryCheck{
				NewuidmapPresent: true,
				NewgidmapPresent: true,
				NewuidmapSetuid:  false,
				NewgidmapSetuid:  false,
				BothAvailable:    false,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.check.HasSetuidBinaries()
			if got != tt.want {
				t.Errorf("HasSetuidBinaries() = %v; want %v", got, tt.want)
			}
		})
	}
}

// ===== TESTS FOR GetIssues() FUNCTION =====

func TestGetIssues(t *testing.T) {
	tests := []struct {
		name       string
		check      *SetuidBinaryCheck
		wantCount  int
		wantIssues []string
	}{
		{
			name: "no issues",
			check: &SetuidBinaryCheck{
				NewuidmapPresent: true,
				NewgidmapPresent: true,
				NewuidmapSetuid:  true,
				NewgidmapSetuid:  true,
				NewuidmapPath:    "/usr/bin/newuidmap",
				NewgidmapPath:    "/usr/bin/newgidmap",
				BothAvailable:    true,
			},
			wantCount:  0,
			wantIssues: []string{},
		},
		{
			name: "newuidmap not found",
			check: &SetuidBinaryCheck{
				NewuidmapPresent: false,
				NewgidmapPresent: true,
				NewuidmapSetuid:  false,
				NewgidmapSetuid:  true,
				NewuidmapPath:    "",
				NewgidmapPath:    "/usr/bin/newgidmap",
			},
			wantCount:  1,
			wantIssues: []string{"newuidmap binary not found"},
		},
		{
			name: "newgidmap not found",
			check: &SetuidBinaryCheck{
				NewuidmapPresent: true,
				NewgidmapPresent: false,
				NewuidmapSetuid:  true,
				NewgidmapSetuid:  false,
				NewuidmapPath:    "/usr/bin/newuidmap",
				NewgidmapPath:    "",
			},
			wantCount:  1,
			wantIssues: []string{"newgidmap binary not found"},
		},
		{
			name: "both not found",
			check: &SetuidBinaryCheck{
				NewuidmapPresent: false,
				NewgidmapPresent: false,
				NewuidmapSetuid:  false,
				NewgidmapSetuid:  false,
			},
			wantCount:  2,
			wantIssues: []string{"newuidmap binary not found", "newgidmap binary not found"},
		},
		{
			name: "newuidmap missing SETUID bit",
			check: &SetuidBinaryCheck{
				NewuidmapPresent: true,
				NewgidmapPresent: true,
				NewuidmapSetuid:  false,
				NewgidmapSetuid:  true,
				NewuidmapPath:    "/usr/bin/newuidmap",
				NewgidmapPath:    "/usr/bin/newgidmap",
			},
			wantCount:  1,
			wantIssues: []string{"newuidmap missing SETUID bit: /usr/bin/newuidmap"},
		},
		{
			name: "newgidmap missing SETUID bit",
			check: &SetuidBinaryCheck{
				NewuidmapPresent: true,
				NewgidmapPresent: true,
				NewuidmapSetuid:  true,
				NewgidmapSetuid:  false,
				NewuidmapPath:    "/usr/bin/newuidmap",
				NewgidmapPath:    "/usr/bin/newgidmap",
			},
			wantCount:  1,
			wantIssues: []string{"newgidmap missing SETUID bit: /usr/bin/newgidmap"},
		},
		{
			name: "both missing SETUID bit",
			check: &SetuidBinaryCheck{
				NewuidmapPresent: true,
				NewgidmapPresent: true,
				NewuidmapSetuid:  false,
				NewgidmapSetuid:  false,
				NewuidmapPath:    "/usr/bin/newuidmap",
				NewgidmapPath:    "/usr/bin/newgidmap",
			},
			wantCount: 2,
			wantIssues: []string{
				"newuidmap missing SETUID bit: /usr/bin/newuidmap",
				"newgidmap missing SETUID bit: /usr/bin/newgidmap",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.check.GetIssues()

			if len(got) != tt.wantCount {
				t.Errorf("GetIssues() count = %d; want %d (got: %v)",
					len(got), tt.wantCount, got)
				return
			}

			// Check that all expected issues are present
			for _, expectedIssue := range tt.wantIssues {
				found := false
				for _, gotIssue := range got {
					if gotIssue == expectedIssue {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected issue %q not found in result: %v",
						expectedIssue, got)
				}
			}
		})
	}
}

// ===== TESTS FOR IsInKubernetes() FUNCTION =====

func TestIsInKubernetes(t *testing.T) {
	// Save original env
	originalK8sHost := os.Getenv("KUBERNETES_SERVICE_HOST")
	defer func() {
		if originalK8sHost == "" {
			os.Unsetenv("KUBERNETES_SERVICE_HOST")
		} else {
			os.Setenv("KUBERNETES_SERVICE_HOST", originalK8sHost)
		}
	}()

	tests := []struct {
		name      string
		envValue  string
		want      bool
	}{
		{
			name:     "kubernetes detected",
			envValue: "10.96.0.1",
			want:     true,
		},
		{
			name:     "kubernetes with DNS",
			envValue: "kubernetes.default.svc",
			want:     true,
		},
		{
			name:     "not kubernetes (empty)",
			envValue: "",
			want:     false,
		},
		{
			name:     "kubernetes with IP",
			envValue: "192.168.1.1",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv("KUBERNETES_SERVICE_HOST", tt.envValue)
			} else {
				os.Unsetenv("KUBERNETES_SERVICE_HOST")
			}

			got := IsInKubernetes()
			if got != tt.want {
				t.Errorf("IsInKubernetes() = %v; want %v (env=%q)",
					got, tt.want, tt.envValue)
			}
		})
	}
}

// ===== TESTS FOR CanSetuidBinariesWork() FUNCTION =====

func TestCanSetuidBinariesWork(t *testing.T) {
	t.Run("real system check", func(t *testing.T) {
		// This tests against the real system
		canWork := CanSetuidBinariesWork()

		t.Logf("CanSetuidBinariesWork() = %v", canWork)

		// The result depends on system configuration
		// We just verify it returns a boolean without error
	})
}

// ===== INTEGRATION TESTS =====

func TestSetuidBinaryCheck_CompleteWorkflow(t *testing.T) {
	t.Run("complete workflow", func(t *testing.T) {
		// Check binaries
		result, err := CheckSetuidBinaries()
		if err != nil {
			t.Fatalf("CheckSetuidBinaries() failed: %v", err)
		}

		// Check if has binaries
		hasBinaries := result.HasSetuidBinaries()
		t.Logf("Has SETUID binaries: %v", hasBinaries)

		// Get issues
		issues := result.GetIssues()
		t.Logf("Issues count: %d", len(issues))
		for i, issue := range issues {
			t.Logf("  Issue %d: %s", i+1, issue)
		}

		// Verify consistency
		if hasBinaries && len(issues) > 0 {
			t.Error("HasSetuidBinaries() returned true but GetIssues() found issues")
		}

		if !hasBinaries && len(issues) == 0 {
			t.Error("HasSetuidBinaries() returned false but GetIssues() found no issues")
		}
	})

	t.Run("kubernetes detection with setuid check", func(t *testing.T) {
		isK8s := IsInKubernetes()
		t.Logf("Running in Kubernetes: %v", isK8s)

		if isK8s {
			// In Kubernetes, check if binaries can work
			canWork := CanSetuidBinariesWork()
			t.Logf("SETUID binaries can work in K8s: %v", canWork)
		}
	})
}

// ===== TESTS FOR Binary Path Search =====

func TestBinaryPathSearch(t *testing.T) {
	t.Run("common paths checked", func(t *testing.T) {
		// The function checks these paths in order
		commonPaths := []string{
			"/usr/bin/newuidmap",
			"/bin/newuidmap",
			"/usr/local/bin/newuidmap",
		}

		// Check if any exist on the system
		foundAny := false
		for _, path := range commonPaths {
			if _, err := os.Stat(path); err == nil {
				foundAny = true
				t.Logf("Found newuidmap at: %s", path)
				break
			}
		}

		if !foundAny {
			t.Log("No newuidmap binaries found in common paths")
		}
	})
}

// ===== EDGE CASE TESTS =====

func TestSetuidBinaryCheck_EdgeCases(t *testing.T) {
	t.Run("nil check", func(t *testing.T) {
		// Test with zero-value struct (not nil pointer)
		check := &SetuidBinaryCheck{}

		if check.HasSetuidBinaries() {
			t.Error("Zero-value check should not have SETUID binaries")
		}

		issues := check.GetIssues()
		if len(issues) != 2 {
			t.Errorf("Zero-value check should have 2 issues, got %d", len(issues))
		}
	})

	t.Run("partial configuration", func(t *testing.T) {
		// Only newuidmap present
		check := &SetuidBinaryCheck{
			NewuidmapPresent: true,
			NewuidmapSetuid:  true,
			NewuidmapPath:    "/usr/bin/newuidmap",
			NewgidmapPresent: false,
			NewgidmapSetuid:  false,
			BothAvailable:    false,
		}

		if check.HasSetuidBinaries() {
			t.Error("Should not have binaries with only one present")
		}

		issues := check.GetIssues()
		if len(issues) != 1 {
			t.Errorf("Should have 1 issue, got %d", len(issues))
		}
	})

	t.Run("inconsistent BothAvailable flag", func(t *testing.T) {
		// Test when BothAvailable is incorrectly set
		check := &SetuidBinaryCheck{
			NewuidmapPresent: false,
			NewgidmapPresent: false,
			NewuidmapSetuid:  false,
			NewgidmapSetuid:  false,
			BothAvailable:    true, // Incorrectly set to true
		}

		// HasSetuidBinaries trusts BothAvailable
		if !check.HasSetuidBinaries() {
			t.Error("HasSetuidBinaries() should trust BothAvailable flag")
		}

		// But GetIssues should still find problems
		issues := check.GetIssues()
		if len(issues) != 2 {
			t.Errorf("GetIssues() should find 2 issues, got %d", len(issues))
		}
	})
}

// ===== TESTS FOR SETUID Bit Detection =====

func TestSetuidBitDetection(t *testing.T) {
	t.Run("create test file with SETUID bit", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "testbin")

		// Create test file
		if err := os.WriteFile(testFile, []byte("#!/bin/sh\necho test"), 0755); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		// Set SETUID bit
		if err := os.Chmod(testFile, 0755|os.ModeSetuid); err != nil {
			t.Fatalf("Failed to set SETUID bit: %v", err)
		}

		// Check if SETUID bit is set
		info, err := os.Stat(testFile)
		if err != nil {
			t.Fatalf("Failed to stat file: %v", err)
		}

		if info.Mode()&os.ModeSetuid == 0 {
			t.Error("SETUID bit not detected")
		}
	})

	t.Run("test file without SETUID bit", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "testbin")

		// Create test file without SETUID bit
		if err := os.WriteFile(testFile, []byte("#!/bin/sh\necho test"), 0755); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		// Check if SETUID bit is NOT set
		info, err := os.Stat(testFile)
		if err != nil {
			t.Fatalf("Failed to stat file: %v", err)
		}

		if info.Mode()&os.ModeSetuid != 0 {
			t.Error("SETUID bit should not be set")
		}
	})
}

// ===== CONCURRENT TESTS =====

func TestConcurrent(t *testing.T) {
	t.Run("concurrent CheckSetuidBinaries calls", func(t *testing.T) {
		const goroutines = 10
		results := make(chan *SetuidBinaryCheck, goroutines)
		errors := make(chan error, goroutines)

		for i := 0; i < goroutines; i++ {
			go func() {
				result, err := CheckSetuidBinaries()
				if err != nil {
					errors <- err
					return
				}
				results <- result
			}()
		}

		// Collect results
		for i := 0; i < goroutines; i++ {
			select {
			case err := <-errors:
				t.Errorf("Goroutine failed: %v", err)
			case result := <-results:
				if result == nil {
					t.Error("Received nil result")
				}
			}
		}
	})

	t.Run("concurrent IsInKubernetes calls", func(t *testing.T) {
		const goroutines = 100
		done := make(chan bool, goroutines)

		for i := 0; i < goroutines; i++ {
			go func() {
				IsInKubernetes()
				done <- true
			}()
		}

		// Wait for all goroutines
		for i := 0; i < goroutines; i++ {
			<-done
		}
	})
}

// ===== BENCHMARK TESTS =====

func BenchmarkCheckSetuidBinaries(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CheckSetuidBinaries()
	}
}

func BenchmarkHasSetuidBinaries(b *testing.B) {
	check := &SetuidBinaryCheck{
		NewuidmapPresent: true,
		NewgidmapPresent: true,
		NewuidmapSetuid:  true,
		NewgidmapSetuid:  true,
		BothAvailable:    true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		check.HasSetuidBinaries()
	}
}

func BenchmarkGetIssues(b *testing.B) {
	check := &SetuidBinaryCheck{
		NewuidmapPresent: true,
		NewgidmapPresent: false,
		NewuidmapSetuid:  true,
		NewgidmapSetuid:  false,
		NewuidmapPath:    "/usr/bin/newuidmap",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		check.GetIssues()
	}
}

func BenchmarkIsInKubernetes(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsInKubernetes()
	}
}

func BenchmarkCanSetuidBinariesWork(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CanSetuidBinariesWork()
	}
}

// ===== REAL WORLD SCENARIO TESTS =====

func TestSetuidRealWorldScenarios(t *testing.T) {
	t.Run("typical kubernetes pod scenario", func(t *testing.T) {
		// Save original env
		originalK8sHost := os.Getenv("KUBERNETES_SERVICE_HOST")
		defer func() {
			if originalK8sHost == "" {
				os.Unsetenv("KUBERNETES_SERVICE_HOST")
			} else {
				os.Setenv("KUBERNETES_SERVICE_HOST", originalK8sHost)
			}
		}()

		os.Setenv("KUBERNETES_SERVICE_HOST", "10.96.0.1")

		// In Kubernetes
		if !IsInKubernetes() {
			t.Error("Should detect Kubernetes environment")
		}

		// Check binaries
		result, _ := CheckSetuidBinaries()
		if result != nil {
			t.Logf("Kubernetes pod - SETUID binaries: %v", result.HasSetuidBinaries())
			t.Logf("Kubernetes pod - Can work: %v", CanSetuidBinariesWork())
		}
	})

	t.Run("typical docker container scenario", func(t *testing.T) {
		os.Unsetenv("KUBERNETES_SERVICE_HOST")

		// Not in Kubernetes
		if IsInKubernetes() {
			t.Error("Should not detect Kubernetes environment")
		}

		// Check binaries
		result, _ := CheckSetuidBinaries()
		if result != nil {
			t.Logf("Docker container - SETUID binaries: %v", result.HasSetuidBinaries())

			if result.HasSetuidBinaries() {
				t.Logf("Docker container - Can work: %v", CanSetuidBinariesWork())
			}
		}
	})

	t.Run("missing binaries scenario", func(t *testing.T) {
		// Simulate missing binaries
		check := &SetuidBinaryCheck{
			NewuidmapPresent: false,
			NewgidmapPresent: false,
		}

		if check.HasSetuidBinaries() {
			t.Error("Should not have binaries when missing")
		}

		issues := check.GetIssues()
		if len(issues) != 2 {
			t.Errorf("Should report 2 missing binaries, got %d issues", len(issues))
		}
	})

	t.Run("binaries present but no SETUID bit scenario", func(t *testing.T) {
		check := &SetuidBinaryCheck{
			NewuidmapPresent: true,
			NewgidmapPresent: true,
			NewuidmapSetuid:  false,
			NewgidmapSetuid:  false,
			NewuidmapPath:    "/usr/bin/newuidmap",
			NewgidmapPath:    "/usr/bin/newgidmap",
		}

		if check.HasSetuidBinaries() {
			t.Error("Should not work without SETUID bits")
		}

		issues := check.GetIssues()
		if len(issues) != 2 {
			t.Errorf("Should report 2 SETUID bit issues, got %d issues", len(issues))
		}
	})
}

// ===== ENVIRONMENT VARIABLE TESTS =====

func TestKubernetesDetection_VariousValues(t *testing.T) {
	originalK8sHost := os.Getenv("KUBERNETES_SERVICE_HOST")
	defer func() {
		if originalK8sHost == "" {
			os.Unsetenv("KUBERNETES_SERVICE_HOST")
		} else {
			os.Setenv("KUBERNETES_SERVICE_HOST", originalK8sHost)
		}
	}()

	testCases := []struct {
		value string
		want  bool
		desc  string
	}{
		{"10.96.0.1", true, "IP address"},
		{"kubernetes.default.svc", true, "DNS name"},
		{"kubernetes.default.svc.cluster.local", true, "Full DNS name"},
		{"192.168.1.1", true, "Private IP"},
		{"", false, "Empty string"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			if tc.value != "" {
				os.Setenv("KUBERNETES_SERVICE_HOST", tc.value)
			} else {
				os.Unsetenv("KUBERNETES_SERVICE_HOST")
			}

			got := IsInKubernetes()
			if got != tc.want {
				t.Errorf("IsInKubernetes() with %q = %v; want %v",
					tc.value, got, tc.want)
			}
		})
	}
}

// ===== CONSISTENCY TESTS =====

func TestBothAvailableConsistency(t *testing.T) {
	t.Run("verify BothAvailable logic", func(t *testing.T) {
		// The BothAvailable flag should match the actual conditions
		tests := []struct {
			name     string
			check    *SetuidBinaryCheck
			expected bool
		}{
			{
				name: "all true",
				check: &SetuidBinaryCheck{
					NewuidmapPresent: true,
					NewgidmapPresent: true,
					NewuidmapSetuid:  true,
					NewgidmapSetuid:  true,
				},
				expected: true,
			},
			{
				name: "missing newuidmap present",
				check: &SetuidBinaryCheck{
					NewuidmapPresent: false,
					NewgidmapPresent: true,
					NewuidmapSetuid:  false,
					NewgidmapSetuid:  true,
				},
				expected: false,
			},
			{
				name: "missing newgidmap present",
				check: &SetuidBinaryCheck{
					NewuidmapPresent: true,
					NewgidmapPresent: false,
					NewuidmapSetuid:  true,
					NewgidmapSetuid:  false,
				},
				expected: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Manually calculate what BothAvailable should be
				calculated := tt.check.NewuidmapPresent &&
					tt.check.NewgidmapPresent &&
					tt.check.NewuidmapSetuid &&
					tt.check.NewgidmapSetuid

				if calculated != tt.expected {
					t.Errorf("Calculated BothAvailable = %v; want %v", calculated, tt.expected)
				}
			})
		}
	})
}
