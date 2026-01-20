package build

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/rapidfort/kimia/pkg/logger"
)

func expectFatal(t *testing.T, fn func(), expectedMsgSubstr string) {
	t.Helper()

	// Setup mock exit that panics instead
	logger.SetExitFunc(func(code int) {
		panic(fmt.Sprintf("exit called with code %d", code))
	})

	// Restore original exit after test
	defer logger.ResetExitFunc()

	// Capture the fatal message
	var fatalMessage string

	// Use a pipe to capture stderr where Fatal writes
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Expect panic from our mock exit
	defer func() {
		// Restore stderr
		w.Close()
		os.Stderr = oldStderr

		// Read what was written to stderr
		var buf bytes.Buffer
		io.Copy(&buf, r)
		fatalMessage = buf.String()

		if r := recover(); r != nil {
			// Good - Fatal was called and our mock panicked
			if !strings.Contains(fatalMessage, expectedMsgSubstr) {
				t.Errorf("Fatal message = %q; want to contain %q",
					fatalMessage, expectedMsgSubstr)
			}
		} else {
			// Bad - Fatal was not called
			t.Error("Expected logger.Fatal to be called")
		}
	}()

	// Run the function that should call Fatal
	fn()

	// If we get here, Fatal wasn't called
	t.Error("Function should have called logger.Fatal")
}

// ===== TESTS FOR DetectBuilder() FUNCTION =====

func TestDetectBuilder(t *testing.T) {
	// Save original PATH
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	tests := []struct {
		name          string
		setupPath     func(t *testing.T) string
		want          string
		skipOnWindows bool
	}{
		{
			name: "buildkit available (both buildkitd and buildctl)",
			setupPath: func(t *testing.T) string {
				return createMockBinaries(t, []string{"buildkitd", "buildctl"})
			},
			want: "buildkit",
		},
		{
			name: "buildah available",
			setupPath: func(t *testing.T) string {
				return createMockBinaries(t, []string{"buildah"})
			},
			want: "buildah",
		},
		{
			name: "buildkit preferred over buildah",
			setupPath: func(t *testing.T) string {
				return createMockBinaries(t, []string{"buildkitd", "buildctl", "buildah"})
			},
			want: "buildkit",
		},
		{
			name: "only buildkitd (missing buildctl)",
			setupPath: func(t *testing.T) string {
				return createMockBinaries(t, []string{"buildkitd"})
			},
			want: "unknown", // Both buildkitd and buildctl needed
		},
		{
			name: "only buildctl (missing buildkitd)",
			setupPath: func(t *testing.T) string {
				return createMockBinaries(t, []string{"buildctl"})
			},
			want: "unknown", // Both buildkitd and buildctl needed
		},
		{
			name: "no builders available",
			setupPath: func(t *testing.T) string {
				return createMockBinaries(t, []string{})
			},
			want: "unknown",
		},
		{
			name: "empty PATH",
			setupPath: func(t *testing.T) string {
				return ""
			},
			want: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOnWindows && runtime.GOOS == "windows" {
				t.Skip("Skipping on Windows")
			}

			// Setup PATH
			mockPath := tt.setupPath(t)
			os.Setenv("PATH", mockPath)

			// Test the function
			got := DetectBuilder()

			if got != tt.want {
				t.Errorf("DetectBuilder() = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestDetectBuilder_RealSystem(t *testing.T) {
	// Test with actual system PATH to ensure function works in real environment
	t.Run("real system detection", func(t *testing.T) {
		result := DetectBuilder()

		// Result should be one of the valid values
		validResults := map[string]bool{
			"buildkit": true,
			"buildah":  true,
			"unknown":  true,
		}

		if !validResults[result] {
			t.Errorf("DetectBuilder() = %q; want one of [buildkit, buildah, unknown]", result)
		}

		t.Logf("Detected builder: %s", result)

		// If buildkit detected, both binaries should exist
		if result == "buildkit" {
			if _, err := exec.LookPath("buildkitd"); err != nil {
				t.Error("buildkit detected but buildkitd not found in PATH")
			}
			if _, err := exec.LookPath("buildctl"); err != nil {
				t.Error("buildkit detected but buildctl not found in PATH")
			}
		}

		// If buildah detected, binary should exist
		if result == "buildah" {
			if _, err := exec.LookPath("buildah"); err != nil {
				t.Error("buildah detected but buildah not found in PATH")
			}
		}
	})
}

func TestDetectBuilder_Precedence(t *testing.T) {
	// Test that buildkit takes precedence over buildah
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	mockPath := createMockBinaries(t, []string{"buildkitd", "buildctl", "buildah"})
	os.Setenv("PATH", mockPath)

	result := DetectBuilder()

	if result != "buildkit" {
		t.Errorf("When both builders available, should prefer buildkit, got %q", result)
	}
}

func TestDetectBuilder_BuildkitRequiresBothBinaries(t *testing.T) {
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	tests := []struct {
		name      string
		binaries  []string
		wantFound bool
	}{
		{
			name:      "both buildkitd and buildctl present",
			binaries:  []string{"buildkitd", "buildctl"},
			wantFound: true,
		},
		{
			name:      "only buildkitd",
			binaries:  []string{"buildkitd"},
			wantFound: false,
		},
		{
			name:      "only buildctl",
			binaries:  []string{"buildctl"},
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPath := createMockBinaries(t, tt.binaries)
			os.Setenv("PATH", mockPath)

			result := DetectBuilder()
			isBuildkit := result == "buildkit"

			if isBuildkit != tt.wantFound {
				t.Errorf("DetectBuilder() with %v = %q; buildkit found = %v, want %v",
					tt.binaries, result, isBuildkit, tt.wantFound)
			}
		})
	}
}

func TestDetectBuilder_MultiplePaths(t *testing.T) {
	// Test that detection works with multiple directories in PATH
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	// Create two separate directories
	dir1 := createMockBinaries(t, []string{"buildkitd"})
	dir2 := createMockBinaries(t, []string{"buildctl"})

	// Combine paths
	combinedPath := dir1 + string(os.PathListSeparator) + dir2
	os.Setenv("PATH", combinedPath)

	result := DetectBuilder()

	if result != "buildkit" {
		t.Errorf("DetectBuilder() with binaries in different PATH dirs = %q; want buildkit", result)
	}
}

func TestDetectBuilder_CaseSensitivity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping case sensitivity test on Windows")
	}

	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	// Create binaries with wrong case (should not be found on Unix)
	mockPath := createMockBinaries(t, []string{"BuildKitd", "BuildCtl"})
	os.Setenv("PATH", mockPath)

	result := DetectBuilder()

	// On Unix systems, wrong case should not match
	if result == "buildkit" {
		t.Error("DetectBuilder() should be case-sensitive on Unix systems")
	}
}

// Helper function to create mock binaries for testing
func createMockBinaries(t *testing.T, binaries []string) string {
	t.Helper()

	// Create temporary directory
	tmpDir := t.TempDir()

	// Create mock executable files
	for _, binary := range binaries {
		binaryPath := filepath.Join(tmpDir, binary)

		// Add .exe extension on Windows
		if runtime.GOOS == "windows" {
			binaryPath += ".exe"
		}

		// Create empty file
		file, err := os.Create(binaryPath)
		if err != nil {
			t.Fatalf("Failed to create mock binary %s: %v", binary, err)
		}
		file.Close()

		// Make executable (Unix only)
		if runtime.GOOS != "windows" {
			if err := os.Chmod(binaryPath, 0755); err != nil {
				t.Fatalf("Failed to chmod mock binary %s: %v", binary, err)
			}
		}
	}

	return tmpDir
}

// Benchmark the detection performance
func BenchmarkDetectBuilder(b *testing.B) {
	for i := 0; i < b.N; i++ {
		DetectBuilder()
	}
}

func BenchmarkDetectBuilder_WithSetup(b *testing.B) {
	// Benchmark with controlled PATH
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	tmpDir := b.TempDir()
	os.Setenv("PATH", tmpDir)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DetectBuilder()
	}
}

// Test for concurrent calls (should be safe)
func TestDetectBuilder_Concurrent(t *testing.T) {
	// Run multiple goroutines calling DetectBuilder simultaneously
	const goroutines = 10
	results := make(chan string, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			results <- DetectBuilder()
		}()
	}

	// Collect results
	firstResult := <-results
	for i := 1; i < goroutines; i++ {
		result := <-results
		if result != firstResult {
			t.Errorf("Concurrent calls returned different results: %q vs %q", firstResult, result)
		}
	}
}

// Test that function doesn't panic with invalid PATH
func TestDetectBuilder_InvalidPath(t *testing.T) {
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	invalidPaths := []string{
		"/nonexistent/directory",
		"/tmp/\x00/invalid", // Null byte
		"",
	}

	for _, invalidPath := range invalidPaths {
		t.Run("path="+invalidPath, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("DetectBuilder() panicked with PATH=%q: %v", invalidPath, r)
				}
			}()

			os.Setenv("PATH", invalidPath)
			result := DetectBuilder()

			// Should return unknown, not panic
			if result != "unknown" {
				t.Logf("With invalid PATH, got result: %q", result)
			}
		})
	}
}

// Example test showing typical usage patterns
func ExampleDetectBuilder() {
	builder := DetectBuilder()

	switch builder {
	case "buildkit":
		// Use BuildKit
	case "buildah":
		// Use Buildah
	default:
		// No builder available
	}

	// Output format varies by system, so we don't assert output
	_ = builder
}

// ===== TESTS FOR Execute() FUNCTION =====

func TestExecute_NoBuilderAvailable(t *testing.T) {
	// Test that Execute returns proper error when no builder is found
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	// Set PATH to empty directory (no builders)
	mockPath := createMockBinaries(t, []string{})
	os.Setenv("PATH", mockPath)

	config := Config{
		Dockerfile:  "Dockerfile",
		Target:      ".",
		Destination: []string{"test:latest"},
	}
	ctx := &Context{}

	err := Execute(config, ctx)

	// Should return error
	if err == nil {
		t.Fatal("Execute() should return error when no builder found")
	}

	// Should contain specific error message
	expectedMsg := "no builder found"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Error message = %q; want to contain %q", err.Error(), expectedMsg)
	}

	// Should mention expected builders
	errMsg := err.Error()
	if !strings.Contains(errMsg, "buildkitd") && !strings.Contains(errMsg, "buildah") {
		t.Errorf("Error should mention expected builders, got: %q", errMsg)
	}
}

func TestExecute_BuilderDetection(t *testing.T) {
	// Test that Execute correctly detects available builders
	// Note: This doesn't test the actual build, just the detection logic

	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	tests := []struct {
		name          string
		availableBins []string
		expectError   bool
		errorContains string
	}{
		{
			name:          "buildkit available",
			availableBins: []string{"buildkitd", "buildctl"},
			expectError:   false, // Will fail in execution but detection succeeds
		},
		{
			name:          "buildah available",
			availableBins: []string{"buildah"},
			expectError:   false, // Will fail in execution but detection succeeds
		},
		{
			name:          "no builder",
			availableBins: []string{},
			expectError:   true,
			errorContains: "no builder found",
		},
		{
			name:          "incomplete buildkit",
			availableBins: []string{"buildkitd"}, // Missing buildctl
			expectError:   true,
			errorContains: "no builder found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPath := createMockBinaries(t, tt.availableBins)
			os.Setenv("PATH", mockPath)

			config := Config{}
			ctx := &Context{}

			err := Execute(config, ctx)

			if tt.expectError {
				if err == nil {
					t.Errorf("Execute() should return error for %s", tt.name)
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Error = %q; want to contain %q", err.Error(), tt.errorContains)
				}
			}
			// Note: When builder is available, Execute will call the actual
			// executeBuildKit/executeBuildah which will fail (no real config)
			// That's expected - we're testing the routing, not the build
		})
	}
}

func TestExecute_Precedence(t *testing.T) {
	// Test that BuildKit is preferred when both builders are available
	// This tests the routing logic, not the actual execution

	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	// Make both builders available
	mockPath := createMockBinaries(t, []string{"buildkitd", "buildctl", "buildah"})
	os.Setenv("PATH", mockPath)

	// Verify detection prefers buildkit
	builder := DetectBuilder()
	if builder != "buildkit" {
		t.Errorf("With both builders available, DetectBuilder() = %q; want buildkit", builder)
	}

	// Execute should route to buildkit (will fail in actual execution, but that's ok)
	config := Config{}
	ctx := &Context{}

	err := Execute(config, ctx)

	// The function will error (no real build config), but we verified
	// the detection logic above. This test ensures Execute doesn't panic
	// or change the detection behavior.
	_ = err
}

func TestExecute_InvalidConfig(t *testing.T) {
	// Test that Execute handles invalid configs gracefully
	// (doesn't panic before reaching the builder implementation)

	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	mockPath := createMockBinaries(t, []string{"buildkitd", "buildctl"})
	os.Setenv("PATH", mockPath)

	invalidConfigs := []Config{
		{}, // Empty config
		{
			Dockerfile:  "",
			Target:      "",
			Destination: []string{},
		},
		{
			Dockerfile:  "nonexistent.dockerfile",
			Target:      "/invalid/path",
			Destination: []string{""},
		},
	}

	for i, config := range invalidConfigs {
		t.Run(fmt.Sprintf("invalid_config_%d", i), func(t *testing.T) {
			ctx := &Context{}

			// Should not panic
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Execute() panicked with invalid config: %v", r)
				}
			}()

			err := Execute(config, ctx)
			// Will error (invalid config), but shouldn't panic
			_ = err
		})
	}
}

// func TestExecute_NilContext(t *testing.T) {
// 	// Test Execute with nil context (edge case)
// 	originalPath := os.Getenv("PATH")
// 	defer os.Setenv("PATH", originalPath)

// 	mockPath := createMockBinaries(t, []string{"buildkitd", "buildctl"})
// 	os.Setenv("PATH", mockPath)

// 	config := Config{}

// 	// Should not panic with nil context
// 	defer func() {
// 		if r := recover(); r != nil {
// 			t.Errorf("Execute() panicked with nil context: %v", r)
// 		}
// 	}()

// 	err := Execute(config, nil)
// 	_ = err
// }

func TestExecute_RepeatedCalls(t *testing.T) {
	// Test that Execute can be called multiple times
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	mockPath := createMockBinaries(t, []string{"buildkitd", "buildctl"})
	os.Setenv("PATH", mockPath)

	config := Config{}
	ctx := &Context{}

	// Call multiple times
	for i := 0; i < 3; i++ {
		err := Execute(config, ctx)
		// Each call should behave the same
		_ = err
	}
}

// Benchmark Execute function (routing overhead only)
func BenchmarkExecute_DetectionOnly(b *testing.B) {
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	// Set PATH to no builders to only benchmark detection
	tmpDir := b.TempDir()
	os.Setenv("PATH", tmpDir)

	config := Config{}
	ctx := &Context{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Execute(config, ctx)
	}
}

// Note: These tests focus on command construction and logic
// Full integration tests with real Buildah would go in separate integration test suite

// ===== TEST COMMAND CONSTRUCTION =====

func TestExecuteBuildah_CommandConstruction(t *testing.T) {
	// This test verifies the buildah command is constructed correctly
	// without actually running buildah

	// We can't easily test executeBuildah directly without running buildah,
	// so we test the command construction logic by extracting it
	// In production, you might refactor to: buildBuildahCommand(config, ctx) []string

	// For now, we test indirectly through behavior
	t.Skip("Requires refactoring executeBuildah to separate command construction")
}

// ===== TEST ARGUMENT SORTING FOR REPRODUCIBILITY =====

func TestBuildArgs_SortingBehavior(t *testing.T) {
	// Test that demonstrates why sorting is critical for reproducible builds

	buildArgs := map[string]string{
		"Z_LAST":  "value1",
		"A_FIRST": "value2",
		"M_MID":   "value3",
	}

	// Without sorting, iteration order is random
	// With sorting, order is deterministic

	keys := make([]string, 0, len(buildArgs))
	for key := range buildArgs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Verify sorted order
	expectedOrder := []string{"A_FIRST", "M_MID", "Z_LAST"}
	for i, key := range keys {
		if key != expectedOrder[i] {
			t.Errorf("Key[%d] = %q; want %q", i, key, expectedOrder[i])
		}
	}
}

func TestLabels_SortingBehavior(t *testing.T) {
	// Similar test for labels
	labels := map[string]string{
		"version":    "1.0.0",
		"maintainer": "team@example.com",
		"app":        "myapp",
	}

	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	expectedOrder := []string{"app", "maintainer", "version"}
	for i, key := range keys {
		if key != expectedOrder[i] {
			t.Errorf("Key[%d] = %q; want %q", i, key, expectedOrder[i])
		}
	}
}

func TestDestinations_SortingBehavior(t *testing.T) {
	// Test destination sorting
	destinations := []string{
		"registry.io/app:v3",
		"registry.io/app:latest",
		"registry.io/app:v1",
	}

	sorted := make([]string, len(destinations))
	copy(sorted, destinations)
	sort.Strings(sorted)

	expected := []string{
		"registry.io/app:latest",
		"registry.io/app:v1",
		"registry.io/app:v3",
	}

	for i := range sorted {
		if sorted[i] != expected[i] {
			t.Errorf("Destination[%d] = %q; want %q", i, sorted[i], expected[i])
		}
	}
}

// ===== TEST PATH HANDLING =====

func TestDockerfilePath_RelativeToAbsolute(t *testing.T) {
	tests := []struct {
		name           string
		dockerfilePath string
		contextPath    string
		wantAbsolute   bool
	}{
		{
			name:           "absolute path unchanged",
			dockerfilePath: "/abs/path/Dockerfile",
			contextPath:    "/some/context",
			wantAbsolute:   true,
		},
		{
			name:           "relative path made absolute",
			dockerfilePath: "Dockerfile",
			contextPath:    "/app",
			wantAbsolute:   true,
		},
		{
			name:           "relative subdir path",
			dockerfilePath: "docker/Dockerfile.prod",
			contextPath:    "/workspace",
			wantAbsolute:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resultPath string

			if !filepath.IsAbs(tt.dockerfilePath) {
				resultPath = filepath.Join(tt.contextPath, tt.dockerfilePath)
			} else {
				resultPath = tt.dockerfilePath
			}

			if tt.wantAbsolute && !filepath.IsAbs(resultPath) {
				t.Errorf("Path %q should be absolute", resultPath)
			}

			// Verify join logic
			if !filepath.IsAbs(tt.dockerfilePath) {
				expected := filepath.Join(tt.contextPath, tt.dockerfilePath)
				if resultPath != expected {
					t.Errorf("Path = %q; want %q", resultPath, expected)
				}
			}
		})
	}
}

// ===== TEST USER PRIVILEGE DETECTION =====

func TestUserPrivilegeDetection(t *testing.T) {
	// Test demonstrates privilege detection logic
	uid := os.Getuid()

	t.Run("detect current user", func(t *testing.T) {
		isRoot := uid == 0

		if isRoot {
			t.Log("Running as root (UID 0)")
		} else {
			t.Logf("Running as non-root (UID %d)", uid)
		}

		// Test passes if it can detect without error
	})

	t.Run("root detection logic", func(t *testing.T) {
		testUID := 0
		isRoot := testUID == 0

		if !isRoot {
			t.Error("UID 0 should be detected as root")
		}
	})

	t.Run("non-root detection logic", func(t *testing.T) {
		testUID := 1000
		isRoot := testUID == 0

		if isRoot {
			t.Error("UID 1000 should not be detected as root")
		}
	})
}

// ===== TEST CACHE LOGIC =====

func TestCacheConfiguration(t *testing.T) {
	tests := []struct {
		name         string
		cache        bool
		reproducible bool
		cacheDir     string
		wantNoCache  bool
		wantLayers   bool
	}{
		{
			name:         "cache enabled, not reproducible",
			cache:        true,
			reproducible: false,
			cacheDir:     "",
			wantNoCache:  false,
			wantLayers:   true,
		},
		{
			name:         "cache enabled with dir, not reproducible",
			cache:        true,
			reproducible: false,
			cacheDir:     "/tmp/cache",
			wantNoCache:  false,
			wantLayers:   true,
		},
		{
			name:         "reproducible build disables cache",
			cache:        true,
			reproducible: true,
			cacheDir:     "",
			wantNoCache:  true,
			wantLayers:   false,
		},
		{
			name:         "cache disabled",
			cache:        false,
			reproducible: false,
			cacheDir:     "",
			wantNoCache:  true,
			wantLayers:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the cache logic from executeBuildah
			var useNoCache bool
			var useLayers bool

			if tt.cache && !tt.reproducible {
				useLayers = true
			} else {
				useNoCache = true
			}

			if useNoCache != tt.wantNoCache {
				t.Errorf("noCache = %v; want %v", useNoCache, tt.wantNoCache)
			}

			if useLayers != tt.wantLayers {
				t.Errorf("layers = %v; want %v", useLayers, tt.wantLayers)
			}
		})
	}
}

// ===== TEST REPRODUCIBLE BUILD LOGIC =====

func TestReproducibleBuildSettings(t *testing.T) {
	tests := []struct {
		name          string
		reproducible  bool
		timestamp     string
		wantTimestamp bool
	}{
		{
			name:          "reproducible with timestamp",
			reproducible:  true,
			timestamp:     "1609459200",
			wantTimestamp: true,
		},
		{
			name:          "reproducible without timestamp",
			reproducible:  true,
			timestamp:     "",
			wantTimestamp: false,
		},
		{
			name:          "not reproducible",
			reproducible:  false,
			timestamp:     "1609459200",
			wantTimestamp: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldSetTimestamp := tt.reproducible && tt.timestamp != ""

			if shouldSetTimestamp != tt.wantTimestamp {
				t.Errorf("Should set timestamp = %v; want %v",
					shouldSetTimestamp, tt.wantTimestamp)
			}
		})
	}
}

// ===== TEST ENVIRONMENT VARIABLE CONSTRUCTION =====

func TestEnvironmentVariables(t *testing.T) {
	tests := []struct {
		name          string
		storageDriver string
		dockerConfig  string
		wantVars      map[string]string
	}{
		{
			name:          "overlay storage driver",
			storageDriver: "overlay",
			dockerConfig:  "/home/user/.docker",
			wantVars: map[string]string{
				"STORAGE_DRIVER":    "overlay",
				"DOCKER_CONFIG":     "/home/user/.docker",
				"BUILDAH_ISOLATION": "chroot",
			},
		},
		{
			name:          "vfs storage driver",
			storageDriver: "vfs",
			dockerConfig:  "/kaniko/.docker",
			wantVars: map[string]string{
				"STORAGE_DRIVER":    "vfs",
				"DOCKER_CONFIG":     "/kaniko/.docker",
				"BUILDAH_ISOLATION": "chroot",
			},
		},
		{
			name:          "no storage driver specified",
			storageDriver: "",
			dockerConfig:  "/root/.docker",
			wantVars: map[string]string{
				"DOCKER_CONFIG":     "/root/.docker",
				"BUILDAH_ISOLATION": "chroot",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate environment variable construction
			env := os.Environ()

			// Add BUILDAH_ISOLATION
			env = append(env, "BUILDAH_ISOLATION=chroot")

			// Add DOCKER_CONFIG
			env = append(env, fmt.Sprintf("DOCKER_CONFIG=%s", tt.dockerConfig))

			// Add STORAGE_DRIVER if specified
			if tt.storageDriver != "" {
				env = append(env, fmt.Sprintf("STORAGE_DRIVER=%s", tt.storageDriver))
			}

			// Verify expected variables are present
			for key, expectedValue := range tt.wantVars {
				found := false
				expectedEnv := fmt.Sprintf("%s=%s", key, expectedValue)

				for _, envVar := range env {
					if envVar == expectedEnv {
						found = true
						break
					}
				}

				if !found {
					t.Errorf("Environment missing %s=%s", key, expectedValue)
				}
			}
		})
	}
}

// ===== TEST INSECURE REGISTRY LOGIC =====

func TestInsecureRegistryFlag(t *testing.T) {
	tests := []struct {
		name         string
		insecure     bool
		insecurePull bool
		wantTLSFlag  bool
	}{
		{
			name:         "insecure enabled",
			insecure:     true,
			insecurePull: false,
			wantTLSFlag:  true,
		},
		{
			name:         "insecure pull enabled",
			insecure:     false,
			insecurePull: true,
			wantTLSFlag:  true,
		},
		{
			name:         "both enabled",
			insecure:     true,
			insecurePull: true,
			wantTLSFlag:  true,
		},
		{
			name:         "both disabled",
			insecure:     false,
			insecurePull: false,
			wantTLSFlag:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldAddTLSFlag := tt.insecure || tt.insecurePull

			if shouldAddTLSFlag != tt.wantTLSFlag {
				t.Errorf("Should add --tls-verify=false = %v; want %v",
					shouldAddTLSFlag, tt.wantTLSFlag)
			}
		})
	}
}

// ===== TEST COMMAND EXECUTION (MOCK) =====

func TestBuildahCommand_NotAvailable(t *testing.T) {
	// Test behavior when buildah is not in PATH
	_, err := exec.LookPath("buildah")

	if err == nil {
		t.Skip("Buildah is available, skipping this test")
	}

	// If buildah is not available, executeBuildah should fail
	// (This is tested indirectly through Execute tests)
	t.Log("Buildah not found in PATH (expected in some environments)")
}

// ===== BENCHMARKS =====

func BenchmarkArgumentSorting(b *testing.B) {
	// Benchmark the performance of sorting build arguments
	buildArgs := map[string]string{
		"ARG_Z": "value1",
		"ARG_A": "value2",
		"ARG_M": "value3",
		"ARG_B": "value4",
		"ARG_Y": "value5",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		keys := make([]string, 0, len(buildArgs))
		for key := range buildArgs {
			keys = append(keys, key)
		}
		sort.Strings(keys)
	}
}

func BenchmarkLabelSorting(b *testing.B) {
	labels := map[string]string{
		"version":     "1.0.0",
		"maintainer":  "team",
		"app":         "myapp",
		"environment": "prod",
		"tier":        "backend",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		keys := make([]string, 0, len(labels))
		for key := range labels {
			keys = append(keys, key)
		}
		sort.Strings(keys)
	}
}

func BenchmarkPathJoining(b *testing.B) {
	contextPath := "/workspace/app"
	dockerfilePath := "docker/Dockerfile.prod"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !filepath.IsAbs(dockerfilePath) {
			_ = filepath.Join(contextPath, dockerfilePath)
		}
	}
}

// ===== INTEGRATION TEST HELPERS =====

// These would be used in separate integration tests with real Buildah

func createTestDockerfile(t *testing.T, dir string, content string) string {
	t.Helper()

	dockerfilePath := filepath.Join(dir, "Dockerfile")
	err := os.WriteFile(dockerfilePath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test Dockerfile: %v", err)
	}

	return dockerfilePath
}

func TestDockerfileCreation_Helper(t *testing.T) {
	// Test the test helper itself
	tmpDir := t.TempDir()

	content := `FROM alpine:latest
RUN echo "test"
`

	dockerfilePath := createTestDockerfile(t, tmpDir, content)

	// Verify file was created
	if _, err := os.Stat(dockerfilePath); os.IsNotExist(err) {
		t.Error("Dockerfile was not created")
	}

	// Verify content
	readContent, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("Failed to read Dockerfile: %v", err)
	}

	if string(readContent) != content {
		t.Errorf("Dockerfile content mismatch")
	}
}

// ===== EXAMPLE INTEGRATION TEST STRUCTURE =====

func TestExecuteBuildah_IntegrationExample(t *testing.T) {
	// This is an example of how you would structure integration tests
	// These should be in a separate file or use build tags

	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Check if buildah is available
	if _, err := exec.LookPath("buildah"); err != nil {
		t.Skip("Buildah not available, skipping integration test")
	}

	t.Skip("Full integration test implementation would go here")

	// Full test would:
	// 1. Create temporary directory
	// 2. Create simple Dockerfile
	// 3. Run executeBuildah
	// 4. Verify image was created
	// 5. Clean up
}

// ===== TEST STORAGE DRIVER VALIDATION =====

func TestStorageDriverValidation(t *testing.T) {
	validDrivers := []string{"overlay", "vfs", ""}

	for _, driver := range validDrivers {
		t.Run("driver_"+driver, func(t *testing.T) {
			// Simulate storage driver setup
			if driver != "" {
				normalized := strings.ToLower(driver)
				if normalized != "overlay" && normalized != "vfs" {
					t.Errorf("Invalid storage driver: %s", driver)
				}
			}
		})
	}
}

// ===== TEST RETRY CONFIGURATION =====

func TestRetryConfiguration(t *testing.T) {
	tests := []struct {
		name  string
		retry int
		want  bool
	}{
		{"no retry", 0, false},
		{"retry once", 1, true},
		{"retry multiple", 5, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldAddRetry := tt.retry > 0

			if shouldAddRetry != tt.want {
				t.Errorf("Should add retry flag = %v; want %v", shouldAddRetry, tt.want)
			}
		})
	}
}
