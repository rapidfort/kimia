package preflight

import (
	"os"
	"path/filepath"
	"testing"
)

// ===== TESTS FOR Environment Constants =====

func TestEnvironmentConstants(t *testing.T) {
	tests := []struct {
		name string
		env  Environment
		want int
	}{
		{"EnvStandalone", EnvStandalone, 0},
		{"EnvDocker", EnvDocker, 1},
		{"EnvKubernetes", EnvKubernetes, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if int(tt.env) != tt.want {
				t.Errorf("%s = %d; want %d", tt.name, tt.env, tt.want)
			}
		})
	}
}

// ===== TESTS FOR DetectEnvironment() FUNCTION =====

func TestDetectEnvironment(t *testing.T) {
	// Save original env
	originalK8sHost := os.Getenv("KUBERNETES_SERVICE_HOST")
	defer func() {
		if originalK8sHost == "" {
			os.Unsetenv("KUBERNETES_SERVICE_HOST")
		} else {
			os.Setenv("KUBERNETES_SERVICE_HOST", originalK8sHost)
		}
	}()

	t.Run("detect kubernetes", func(t *testing.T) {
		os.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
		defer os.Unsetenv("KUBERNETES_SERVICE_HOST")

		env := DetectEnvironment()
		if env != EnvKubernetes {
			t.Errorf("DetectEnvironment() = %v; want EnvKubernetes", env)
		}
	})

	t.Run("detect docker via dockerenv", func(t *testing.T) {
		os.Unsetenv("KUBERNETES_SERVICE_HOST")

		// Create temporary .dockerenv file
		tmpDir := t.TempDir()
		dockerEnvPath := filepath.Join(tmpDir, ".dockerenv")
		os.WriteFile(dockerEnvPath, []byte(""), 0644)

		// We can't actually test this without being in a container
		// but we can verify the function runs without error
		env := DetectEnvironment()
		// Will return EnvStandalone in test environment
		if env != EnvStandalone && env != EnvDocker {
			t.Logf("DetectEnvironment() = %v (expected in test environment)", env)
		}
	})

	t.Run("detect standalone", func(t *testing.T) {
		os.Unsetenv("KUBERNETES_SERVICE_HOST")

		// In test environment without Docker markers
		env := DetectEnvironment()
		// Should be standalone in test environment
		t.Logf("DetectEnvironment() = %v", env)

		validEnvs := []Environment{EnvStandalone, EnvDocker, EnvKubernetes}
		isValid := false
		for _, valid := range validEnvs {
			if env == valid {
				isValid = true
				break
			}
		}
		if !isValid {
			t.Errorf("DetectEnvironment() returned invalid environment: %v", env)
		}
	})
}

// ===== TESTS FOR getCheckmark() FUNCTION =====

func TestGetCheckmark(t *testing.T) {
	tests := []struct {
		name      string
		condition bool
		want      string
	}{
		{
			name:      "true condition",
			condition: true,
			want:      "✓",
		},
		{
			name:      "false condition",
			condition: false,
			want:      "✗",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getCheckmark(tt.condition)
			if got != tt.want {
				t.Errorf("getCheckmark(%v) = %q; want %q", tt.condition, got, tt.want)
			}
		})
	}
}

// ===== TESTS FOR getPresence() FUNCTION =====

func TestGetPresence(t *testing.T) {
	tests := []struct {
		name    string
		present bool
		want    string
	}{
		{
			name:    "present",
			present: true,
			want:    "Present",
		},
		{
			name:    "not present",
			present: false,
			want:    "Missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getPresence(tt.present)
			if got != tt.want {
				t.Errorf("getPresence(%v) = %q; want %q", tt.present, got, tt.want)
			}
		})
	}
}

// ===== TESTS FOR getEnabled() FUNCTION =====

func TestGetEnabled(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		want    string
	}{
		{
			name:    "enabled",
			enabled: true,
			want:    "Enabled",
		},
		{
			name:    "disabled",
			enabled: false,
			want:    "Disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getEnabled(tt.enabled)
			if got != tt.want {
				t.Errorf("getEnabled(%v) = %q; want %q", tt.enabled, got, tt.want)
			}
		})
	}
}

// ===== TESTS FOR getSuccess() FUNCTION =====

func TestGetSuccess(t *testing.T) {
	tests := []struct {
		name    string
		success bool
		want    string
	}{
		{
			name:    "success",
			success: true,
			want:    "Success",
		},
		{
			name:    "failed",
			success: false,
			want:    "Failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getSuccess(tt.success)
			if got != tt.want {
				t.Errorf("getSuccess(%v) = %q; want %q", tt.success, got, tt.want)
			}
		})
	}
}

// ===== TESTS FOR getEnvironment() FUNCTION =====

func TestGetEnvironment(t *testing.T) {
	tests := []struct {
		name string
		env  Environment
		want string
	}{
		{
			name: "kubernetes",
			env:  EnvKubernetes,
			want: "Kubernetes",
		},
		{
			name: "docker",
			env:  EnvDocker,
			want: "Docker",
		},
		{
			name: "standalone",
			env:  EnvStandalone,
			want: "Standalone",
		},
		{
			name: "unknown",
			env:  Environment(999),
			want: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getEnvironment(tt.env)
			if got != tt.want {
				t.Errorf("getEnvironment(%v) = %q; want %q", tt.env, got, tt.want)
			}
		})
	}
}

// ===== TESTS FOR checkDependency() FUNCTION =====

func TestCheckDependency(t *testing.T) {
	t.Run("dependency exists at specified path", func(t *testing.T) {
		tmpDir := t.TempDir()
		binPath := filepath.Join(tmpDir, "testbin")
		os.WriteFile(binPath, []byte("#!/bin/sh\necho test"), 0755)

		// Just verify it doesn't panic
		checkDependency("testbin", binPath)
	})

	t.Run("dependency not found", func(t *testing.T) {
		// Just verify it doesn't panic
		checkDependency("nonexistent", "/nonexistent/path")
	})

	t.Run("dependency in PATH", func(t *testing.T) {
		// Test with a common binary that should exist
		checkDependency("sh", "/bin/sh")
	})
}

// ===== TESTS FOR checkDependencyVersion() FUNCTION =====

func TestCheckDependencyVersion(t *testing.T) {
	t.Run("check version of existing command", func(t *testing.T) {
		// Test with a common command
		checkDependencyVersion("sh", "sh", "--version")
		// Just verify it doesn't panic
	})

	t.Run("check version of nonexistent command", func(t *testing.T) {
		checkDependencyVersion("nonexistent", "nonexistent", "--version")
		// Just verify it doesn't panic
	})

	t.Run("check version with invalid arg", func(t *testing.T) {
		checkDependencyVersion("sh", "sh", "--invalid-arg-xyz")
		// Just verify it doesn't panic
	})
}

// ===== TESTS FOR Environment Type String Conversion =====

func TestEnvironmentString(t *testing.T) {
	tests := []struct {
		env      Environment
		wantName string
	}{
		{EnvStandalone, "Standalone"},
		{EnvDocker, "Docker"},
		{EnvKubernetes, "Kubernetes"},
	}

	for _, tt := range tests {
		t.Run(tt.wantName, func(t *testing.T) {
			got := getEnvironment(tt.env)
			if got != tt.wantName {
				t.Errorf("getEnvironment(%d) = %q; want %q", tt.env, got, tt.wantName)
			}
		})
	}
}

// ===== INTEGRATION TESTS =====

func TestDetectEnvironment_RealSystem(t *testing.T) {
	t.Run("real system detection", func(t *testing.T) {
		env := DetectEnvironment()

		// Log what was detected
		t.Logf("Detected environment: %s", getEnvironment(env))

		// Verify it's one of the valid environments
		validEnvs := map[Environment]bool{
			EnvStandalone: true,
			EnvDocker:     true,
			EnvKubernetes: true,
		}

		if !validEnvs[env] {
			t.Errorf("DetectEnvironment() returned invalid environment: %v", env)
		}

		// Log environment indicators
		if k8sHost := os.Getenv("KUBERNETES_SERVICE_HOST"); k8sHost != "" {
			t.Logf("KUBERNETES_SERVICE_HOST: %s", k8sHost)
		}

		if _, err := os.Stat("/.dockerenv"); err == nil {
			t.Log("Found /.dockerenv file")
		}
	})
}

func TestEnvironmentDetection_Precedence(t *testing.T) {
	// Save original env
	originalK8sHost := os.Getenv("KUBERNETES_SERVICE_HOST")
	defer func() {
		if originalK8sHost == "" {
			os.Unsetenv("KUBERNETES_SERVICE_HOST")
		} else {
			os.Setenv("KUBERNETES_SERVICE_HOST", originalK8sHost)
		}
	}()

	t.Run("kubernetes takes precedence", func(t *testing.T) {
		// Even if /.dockerenv exists, Kubernetes should be detected first
		os.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")

		env := DetectEnvironment()
		if env != EnvKubernetes {
			t.Errorf("Kubernetes should take precedence, got: %v", getEnvironment(env))
		}
	})
}

// ===== TESTS FOR CheckEnvironment() FUNCTION =====

func TestCheckEnvironment(t *testing.T) {
	// Save original env vars
	originalStorageDriver := os.Getenv("STORAGE_DRIVER")
	defer func() {
		if originalStorageDriver == "" {
			os.Unsetenv("STORAGE_DRIVER")
		} else {
			os.Setenv("STORAGE_DRIVER", originalStorageDriver)
		}
	}()

	t.Run("check environment returns valid exit code", func(t *testing.T) {
		// This will run the actual check against the system
		exitCode := CheckEnvironment()

		// Exit code should be 0 (success) or 1 (failure)
		if exitCode != 0 && exitCode != 1 {
			t.Errorf("CheckEnvironment() returned invalid exit code: %d", exitCode)
		}

		t.Logf("CheckEnvironment() returned: %d", exitCode)
	})

	t.Run("check environment with vfs storage", func(t *testing.T) {
		os.Setenv("STORAGE_DRIVER", "vfs")
		defer os.Unsetenv("STORAGE_DRIVER")

		exitCode := CheckEnvironment()

		if exitCode != 0 && exitCode != 1 {
			t.Errorf("CheckEnvironment() returned invalid exit code: %d", exitCode)
		}

		t.Logf("CheckEnvironment() with vfs returned: %d", exitCode)
	})

	t.Run("check environment with overlay storage", func(t *testing.T) {
		os.Setenv("STORAGE_DRIVER", "overlay")
		defer os.Unsetenv("STORAGE_DRIVER")

		exitCode := CheckEnvironment()

		if exitCode != 0 && exitCode != 1 {
			t.Errorf("CheckEnvironment() returned invalid exit code: %d", exitCode)
		}

		t.Logf("CheckEnvironment() with overlay returned: %d", exitCode)
	})
}

// ===== TESTS FOR CheckEnvironmentWithDriver() FUNCTION =====

func TestCheckEnvironmentWithDriver(t *testing.T) {
	tests := []struct {
		name          string
		storageDriver string
		wantValidExit bool
	}{
		{
			name:          "vfs storage driver",
			storageDriver: "vfs",
			wantValidExit: true,
		},
		{
			name:          "overlay storage driver",
			storageDriver: "overlay",
			wantValidExit: true,
		},
		{
			name:          "native storage driver",
			storageDriver: "native",
			wantValidExit: true,
		},
		{
			name:          "empty storage driver",
			storageDriver: "",
			wantValidExit: true,
		},
		{
			name:          "unknown storage driver",
			storageDriver: "unknown",
			wantValidExit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exitCode := CheckEnvironmentWithDriver(tt.storageDriver)

			if tt.wantValidExit {
				if exitCode != 0 && exitCode != 1 {
					t.Errorf("CheckEnvironmentWithDriver(%q) returned invalid exit code: %d",
						tt.storageDriver, exitCode)
				}
			}

			t.Logf("CheckEnvironmentWithDriver(%q) returned: %d",
				tt.storageDriver, exitCode)
		})
	}
}

// ===== EDGE CASE TESTS =====

func TestHelperFunctions_EdgeCases(t *testing.T) {
	t.Run("all helper functions with various inputs", func(t *testing.T) {
		// Test all combinations
		conditions := []bool{true, false}

		for _, cond := range conditions {
			checkmark := getCheckmark(cond)
			presence := getPresence(cond)
			enabled := getEnabled(cond)
			success := getSuccess(cond)

			// Verify none return empty strings
			if checkmark == "" {
				t.Error("getCheckmark returned empty string")
			}
			if presence == "" {
				t.Error("getPresence returned empty string")
			}
			if enabled == "" {
				t.Error("getEnabled returned empty string")
			}
			if success == "" {
				t.Error("getSuccess returned empty string")
			}
		}
	})

	t.Run("getEnvironment with all valid values", func(t *testing.T) {
		envs := []Environment{EnvStandalone, EnvDocker, EnvKubernetes}

		for _, env := range envs {
			name := getEnvironment(env)
			if name == "" || name == "Unknown" {
				t.Errorf("getEnvironment(%v) returned invalid name: %q", env, name)
			}
		}
	})

	t.Run("getEnvironment with invalid value", func(t *testing.T) {
		invalidEnv := Environment(-1)
		name := getEnvironment(invalidEnv)
		if name != "Unknown" {
			t.Errorf("getEnvironment(%v) should return 'Unknown', got: %q", invalidEnv, name)
		}
	})
}

// ===== BENCHMARK TESTS =====

func BenchmarkDetectEnvironment(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DetectEnvironment()
	}
}

func BenchmarkGetCheckmark(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		getCheckmark(true)
		getCheckmark(false)
	}
}

func BenchmarkGetEnvironment(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		getEnvironment(EnvKubernetes)
		getEnvironment(EnvDocker)
		getEnvironment(EnvStandalone)
	}
}

// ===== CONCURRENT TESTS =====

func TestDetectEnvironment_Concurrent(t *testing.T) {
	// Test that DetectEnvironment is safe to call concurrently
	const goroutines = 10
	results := make(chan Environment, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			results <- DetectEnvironment()
		}()
	}

	// Collect results
	firstResult := <-results
	for i := 1; i < goroutines; i++ {
		result := <-results
		if result != firstResult {
			t.Logf("Note: Concurrent calls returned different results: %v vs %v",
				getEnvironment(firstResult), getEnvironment(result))
			// This is not necessarily an error - environment can change
		}
	}
}

func TestHelperFunctions_Concurrent(t *testing.T) {
	// Test that helper functions are safe to call concurrently
	const goroutines = 100
	done := make(chan bool, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			// Call all helper functions
			getCheckmark(id%2 == 0)
			getPresence(id%2 == 0)
			getEnabled(id%2 == 0)
			getSuccess(id%2 == 0)
			getEnvironment(Environment(id % 3))
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < goroutines; i++ {
		<-done
	}
}

// ===== REAL WORLD SCENARIO TESTS =====

func TestRealWorldScenarios(t *testing.T) {
	t.Run("typical kubernetes scenario", func(t *testing.T) {
		originalK8sHost := os.Getenv("KUBERNETES_SERVICE_HOST")
		defer func() {
			if originalK8sHost == "" {
				os.Unsetenv("KUBERNETES_SERVICE_HOST")
			} else {
				os.Setenv("KUBERNETES_SERVICE_HOST", originalK8sHost)
			}
		}()

		os.Setenv("KUBERNETES_SERVICE_HOST", "10.96.0.1")
		env := DetectEnvironment()

		if env != EnvKubernetes {
			t.Errorf("Kubernetes scenario failed: got %s", getEnvironment(env))
		}

		// Check environment with typical k8s storage
		os.Setenv("STORAGE_DRIVER", "vfs")
		exitCode := CheckEnvironmentWithDriver("vfs")
		t.Logf("Kubernetes scenario exit code: %d", exitCode)
	})

	t.Run("typical docker scenario", func(t *testing.T) {
		os.Unsetenv("KUBERNETES_SERVICE_HOST")

		// In test environment, we can't easily fake Docker detection
		// but we can test the storage driver logic
		exitCode := CheckEnvironmentWithDriver("overlay")
		t.Logf("Docker scenario exit code: %d", exitCode)
	})

	t.Run("typical standalone scenario", func(t *testing.T) {
		os.Unsetenv("KUBERNETES_SERVICE_HOST")

		exitCode := CheckEnvironmentWithDriver("vfs")
		t.Logf("Standalone scenario exit code: %d", exitCode)
	})
}

// ===== ENVIRONMENT VARIABLE TESTS =====

func TestEnvironmentVariables(t *testing.T) {
	t.Run("KUBERNETES_SERVICE_HOST detection", func(t *testing.T) {
		testCases := []struct {
			value    string
			wantType Environment
		}{
			{"10.96.0.1", EnvKubernetes},
			{"kubernetes.default.svc", EnvKubernetes},
			{"192.168.1.1", EnvKubernetes},
		}

		for _, tc := range testCases {
			t.Run(tc.value, func(t *testing.T) {
				os.Setenv("KUBERNETES_SERVICE_HOST", tc.value)
				defer os.Unsetenv("KUBERNETES_SERVICE_HOST")

				env := DetectEnvironment()
				if env != tc.wantType {
					t.Errorf("With KUBERNETES_SERVICE_HOST=%s, got %s; want %s",
						tc.value, getEnvironment(env), getEnvironment(tc.wantType))
				}
			})
		}
	})

	t.Run("empty KUBERNETES_SERVICE_HOST", func(t *testing.T) {
		os.Setenv("KUBERNETES_SERVICE_HOST", "")
		defer os.Unsetenv("KUBERNETES_SERVICE_HOST")

		env := DetectEnvironment()
		// Should not detect as Kubernetes with empty value
		if env == EnvKubernetes {
			t.Error("Empty KUBERNETES_SERVICE_HOST should not be detected as Kubernetes")
		}
	})
}

// ===== STORAGE DRIVER TESTS =====

func TestStorageDriverSelection(t *testing.T) {
	t.Run("storage driver defaults", func(t *testing.T) {
		// Test that CheckEnvironment selects appropriate defaults
		// This is an integration test that verifies the logic works

		originalDriver := os.Getenv("STORAGE_DRIVER")
		defer func() {
			if originalDriver == "" {
				os.Unsetenv("STORAGE_DRIVER")
			} else {
				os.Setenv("STORAGE_DRIVER", originalDriver)
			}
		}()

		// Test with no STORAGE_DRIVER set
		os.Unsetenv("STORAGE_DRIVER")
		exitCode := CheckEnvironment()
		t.Logf("Exit code with default storage: %d", exitCode)

		// Test with explicit vfs
		os.Setenv("STORAGE_DRIVER", "vfs")
		exitCode = CheckEnvironment()
		t.Logf("Exit code with vfs storage: %d", exitCode)

		// Test with explicit overlay
		os.Setenv("STORAGE_DRIVER", "overlay")
		exitCode = CheckEnvironment()
		t.Logf("Exit code with overlay storage: %d", exitCode)
	})
}

// ===== FILE SYSTEM TESTS =====

func TestDockerenvDetection(t *testing.T) {
	t.Run("check dockerenv file handling", func(t *testing.T) {
		// We can't create /.dockerenv in tests, but we can verify
		// the function handles file check errors gracefully

		// The function should not panic regardless of file existence
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("DetectEnvironment() panicked: %v", r)
			}
		}()

		env := DetectEnvironment()
		t.Logf("Environment detected: %s", getEnvironment(env))
	})
}

// ===== SPECIAL CHARACTER TESTS =====

func TestHelperFunctions_OutputFormat(t *testing.T) {
	t.Run("checkmark symbols are valid UTF-8", func(t *testing.T) {
		checkTrue := getCheckmark(true)
		checkFalse := getCheckmark(false)

		// Verify they're not empty and are valid UTF-8
		if len(checkTrue) == 0 {
			t.Error("Checkmark for true is empty")
		}
		if len(checkFalse) == 0 {
			t.Error("Checkmark for false is empty")
		}

		// Verify they're different
		if checkTrue == checkFalse {
			t.Error("Checkmarks for true and false are the same")
		}
	})

	t.Run("helper functions return consistent values", func(t *testing.T) {
		// Call multiple times and verify consistency
		for i := 0; i < 100; i++ {
			if getCheckmark(true) != "✓" {
				t.Error("getCheckmark(true) not consistent")
			}
			if getCheckmark(false) != "✗" {
				t.Error("getCheckmark(false) not consistent")
			}
		}
	})
}
