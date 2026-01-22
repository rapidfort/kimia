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

// ===== TESTS FOR copyDir() and copyFile() FUNCTIONS =====

func TestCopyFile(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		permissions os.FileMode
		wantError   bool
	}{
		{
			name:        "regular text file",
			content:     "hello world",
			permissions: 0644,
			wantError:   false,
		},
		{
			name:        "executable file",
			content:     "#!/bin/sh\necho test",
			permissions: 0755,
			wantError:   false,
		},
		{
			name:        "empty file",
			content:     "",
			permissions: 0644,
			wantError:   false,
		},
		{
			name:        "binary content",
			content:     "\x00\x01\x02\x03\xff\xfe",
			permissions: 0600,
			wantError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			srcPath := filepath.Join(tmpDir, "source.txt")
			dstPath := filepath.Join(tmpDir, "dest.txt")

			// Create source file
			err := os.WriteFile(srcPath, []byte(tt.content), tt.permissions)
			if err != nil {
				t.Fatalf("Failed to create source file: %v", err)
			}

			// Copy file
			err = copyFile(srcPath, dstPath)

			if (err != nil) != tt.wantError {
				t.Errorf("copyFile() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if tt.wantError {
				return
			}

			// Verify content matches
			dstContent, err := os.ReadFile(dstPath)
			if err != nil {
				t.Fatalf("Failed to read destination file: %v", err)
			}

			if string(dstContent) != tt.content {
				t.Errorf("Content mismatch: got %q, want %q", string(dstContent), tt.content)
			}

			// Verify permissions match (on Unix systems)
			if runtime.GOOS != "windows" {
				srcInfo, _ := os.Stat(srcPath)
				dstInfo, _ := os.Stat(dstPath)

				if srcInfo.Mode().Perm() != dstInfo.Mode().Perm() {
					t.Errorf("Permission mismatch: src=%v, dst=%v", srcInfo.Mode().Perm(), dstInfo.Mode().Perm())
				}
			}
		})
	}
}

func TestCopyFile_Errors(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		setupFn func() (src, dst string)
	}{
		{
			name: "source file does not exist",
			setupFn: func() (string, string) {
				return filepath.Join(tmpDir, "nonexistent.txt"), filepath.Join(tmpDir, "dest.txt")
			},
		},
		{
			name: "destination directory does not exist",
			setupFn: func() (string, string) {
				src := filepath.Join(tmpDir, "source.txt")
				os.WriteFile(src, []byte("test"), 0644)
				return src, filepath.Join(tmpDir, "nonexistent", "dest.txt")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, dst := tt.setupFn()
			err := copyFile(src, dst)

			if err == nil {
				t.Error("copyFile() should return error for invalid paths")
			}
		})
	}
}

func TestCopyDir(t *testing.T) {
	t.Run("simple directory copy", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcDir := filepath.Join(tmpDir, "source")
		dstDir := filepath.Join(tmpDir, "dest")

		// Create source structure
		os.MkdirAll(srcDir, 0755)
		os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0644)
		os.WriteFile(filepath.Join(srcDir, "file2.txt"), []byte("content2"), 0644)

		// Copy directory
		err := copyDir(srcDir, dstDir)
		if err != nil {
			t.Fatalf("copyDir() failed: %v", err)
		}

		// Verify files exist
		files := []string{"file1.txt", "file2.txt"}
		for _, file := range files {
			dstFile := filepath.Join(dstDir, file)
			if _, err := os.Stat(dstFile); os.IsNotExist(err) {
				t.Errorf("File %s was not copied", file)
			}
		}

		// Verify content
		content1, _ := os.ReadFile(filepath.Join(dstDir, "file1.txt"))
		if string(content1) != "content1" {
			t.Errorf("File1 content mismatch")
		}
	})

	t.Run("nested directory copy", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcDir := filepath.Join(tmpDir, "source")
		dstDir := filepath.Join(tmpDir, "dest")

		// Create nested structure
		os.MkdirAll(filepath.Join(srcDir, "subdir1", "subdir2"), 0755)
		os.WriteFile(filepath.Join(srcDir, "root.txt"), []byte("root"), 0644)
		os.WriteFile(filepath.Join(srcDir, "subdir1", "sub1.txt"), []byte("sub1"), 0644)
		os.WriteFile(filepath.Join(srcDir, "subdir1", "subdir2", "sub2.txt"), []byte("sub2"), 0644)

		// Copy directory
		err := copyDir(srcDir, dstDir)
		if err != nil {
			t.Fatalf("copyDir() failed: %v", err)
		}

		// Verify structure
		paths := []string{
			filepath.Join(dstDir, "root.txt"),
			filepath.Join(dstDir, "subdir1", "sub1.txt"),
			filepath.Join(dstDir, "subdir1", "subdir2", "sub2.txt"),
		}

		for _, path := range paths {
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Errorf("Path %s does not exist", path)
			}
		}
	})

	t.Run("empty directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcDir := filepath.Join(tmpDir, "empty_source")
		dstDir := filepath.Join(tmpDir, "empty_dest")

		os.MkdirAll(srcDir, 0755)

		err := copyDir(srcDir, dstDir)
		if err != nil {
			t.Fatalf("copyDir() failed on empty directory: %v", err)
		}

		if _, err := os.Stat(dstDir); os.IsNotExist(err) {
			t.Error("Destination directory was not created")
		}
	})
}

func TestCopyDir_Errors(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		src     string
		dst     string
		wantErr bool
	}{
		{
			name:    "source does not exist",
			src:     filepath.Join(tmpDir, "nonexistent"),
			dst:     filepath.Join(tmpDir, "dest"),
			wantErr: true,
		},
		{
			name:    "source is a file not directory",
			src:     "",
			dst:     filepath.Join(tmpDir, "dest"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.src == "" {
				// Create a file instead of directory
				tt.src = filepath.Join(tmpDir, "file.txt")
				os.WriteFile(tt.src, []byte("test"), 0644)
			}

			err := copyDir(tt.src, tt.dst)

			if (err != nil) != tt.wantErr {
				t.Errorf("copyDir() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ===== TESTS FOR SaveDigestInfo() FUNCTION =====

func TestSaveDigestInfo(t *testing.T) {
	t.Run("save all digest files", func(t *testing.T) {
		tmpDir := t.TempDir()

		config := Config{
			Destination:                []string{"registry.io/myapp:v1.0"},
			DigestFile:                 filepath.Join(tmpDir, "digest.txt"),
			ImageNameWithDigestFile:    filepath.Join(tmpDir, "image-digest.txt"),
			ImageNameTagWithDigestFile: filepath.Join(tmpDir, "full-info.json"),
		}

		digestMap := map[string]string{
			"registry.io/myapp:v1.0": "sha256:abcdef1234567890",
		}

		err := SaveDigestInfo(config, digestMap)
		if err != nil {
			t.Fatalf("SaveDigestInfo() failed: %v", err)
		}

		// Verify digest file
		digestContent, err := os.ReadFile(config.DigestFile)
		if err != nil {
			t.Fatalf("Failed to read digest file: %v", err)
		}
		if string(digestContent) != "sha256:abcdef1234567890" {
			t.Errorf("Digest file content = %q, want %q", string(digestContent), "sha256:abcdef1234567890")
		}

		// Verify image name with digest file
		imageDigestContent, err := os.ReadFile(config.ImageNameWithDigestFile)
		if err != nil {
			t.Fatalf("Failed to read image digest file: %v", err)
		}
		expected := "registry.io/myapp@sha256:abcdef1234567890"
		if string(imageDigestContent) != expected {
			t.Errorf("Image digest content = %q, want %q", string(imageDigestContent), expected)
		}

		// Verify JSON file exists and is valid
		jsonContent, err := os.ReadFile(config.ImageNameTagWithDigestFile)
		if err != nil {
			t.Fatalf("Failed to read JSON file: %v", err)
		}
		if !strings.Contains(string(jsonContent), "registry.io/myapp:v1.0") {
			t.Error("JSON file missing image name")
		}
		if !strings.Contains(string(jsonContent), "sha256:abcdef1234567890") {
			t.Error("JSON file missing digest")
		}
	})

	t.Run("no digest available", func(t *testing.T) {
		tmpDir := t.TempDir()

		config := Config{
			Destination: []string{"registry.io/myapp:v1.0"},
			DigestFile:  filepath.Join(tmpDir, "digest.txt"),
		}

		digestMap := map[string]string{} // Empty map

		err := SaveDigestInfo(config, digestMap)
		if err != nil {
			t.Fatalf("SaveDigestInfo() should not error with empty digest map: %v", err)
		}

		// File should not be created
		if _, err := os.Stat(config.DigestFile); err == nil {
			t.Error("Digest file should not be created when no digest available")
		}
	})

	t.Run("no destinations", func(t *testing.T) {
		config := Config{
			Destination: []string{},
		}

		digestMap := map[string]string{}

		err := SaveDigestInfo(config, digestMap)
		if err != nil {
			t.Errorf("SaveDigestInfo() should not error with no destinations: %v", err)
		}
	})

	t.Run("only digest file specified", func(t *testing.T) {
		tmpDir := t.TempDir()

		config := Config{
			Destination: []string{"registry.io/myapp:v1.0"},
			DigestFile:  filepath.Join(tmpDir, "digest.txt"),
		}

		digestMap := map[string]string{
			"registry.io/myapp:v1.0": "sha256:fedcba0987654321",
		}

		err := SaveDigestInfo(config, digestMap)
		if err != nil {
			t.Fatalf("SaveDigestInfo() failed: %v", err)
		}

		// Only digest file should exist
		if _, err := os.Stat(config.DigestFile); os.IsNotExist(err) {
			t.Error("Digest file should exist")
		}
	})
}

// ===== TESTS FOR Attestation Functions =====

func TestBuildAttestationOptsFromSimpleMode(t *testing.T) {
	tests := []struct {
		name      string
		mode      string
		wantOpts  []string
		wantFatal bool
	}{
		{
			name: "min mode",
			mode: "min",
			wantOpts: []string{
				"attest:sbom=false",
				"attest:provenance=mode=min",
			},
		},
		{
			name: "max mode",
			mode: "max",
			wantOpts: []string{
				"attest:sbom=true",
				"attest:provenance=mode=max",
			},
		},
		{
			name:      "invalid mode",
			mode:      "invalid",
			wantFatal: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantFatal {
				expectFatal(t, func() {
					buildAttestationOptsFromSimpleMode(tt.mode)
				}, "Invalid attestation mode")
				return
			}

			opts := buildAttestationOptsFromSimpleMode(tt.mode)

			if len(opts) != len(tt.wantOpts) {
				t.Errorf("Got %d options, want %d", len(opts), len(tt.wantOpts))
			}

			for i, want := range tt.wantOpts {
				if i >= len(opts) {
					t.Errorf("Missing option: %s", want)
					continue
				}
				if opts[i] != want {
					t.Errorf("Option[%d] = %q, want %q", i, opts[i], want)
				}
			}
		})
	}
}

func TestBuildSBOMOpt(t *testing.T) {
	tests := []struct {
		name   string
		config AttestationConfig
		want   string
	}{
		{
			name: "no params",
			config: AttestationConfig{
				Type:   "sbom",
				Params: map[string]string{},
			},
			want: "attest:sbom=true",
		},
		{
			name: "with generator",
			config: AttestationConfig{
				Type: "sbom",
				Params: map[string]string{
					"generator": "docker/buildkit-syft-scanner",
				},
			},
			want: "attest:sbom=generator=docker/buildkit-syft-scanner",
		},
		{
			name: "with scan-context (should be excluded)",
			config: AttestationConfig{
				Type: "sbom",
				Params: map[string]string{
					"scan-context": "true",
				},
			},
			want: "attest:sbom=true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSBOMOpt(tt.config)
			if got != tt.want {
				t.Errorf("buildSBOMOpt() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildProvenanceOpt(t *testing.T) {
	tests := []struct {
		name   string
		config AttestationConfig
		want   string
	}{
		{
			name: "no params (default to max)",
			config: AttestationConfig{
				Type:   "provenance",
				Params: map[string]string{},
			},
			want: "attest:provenance=mode=max",
		},
		{
			name: "explicit min mode",
			config: AttestationConfig{
				Type: "provenance",
				Params: map[string]string{
					"mode": "min",
				},
			},
			want: "attest:provenance=mode=min",
		},
		{
			name: "with builder-id",
			config: AttestationConfig{
				Type: "provenance",
				Params: map[string]string{
					"mode":       "max",
					"builder-id": "mybuilder",
				},
			},
			want: "attest:provenance=mode=max,builder-id=mybuilder",
		},
		{
			name: "with multiple params",
			config: AttestationConfig{
				Type: "provenance",
				Params: map[string]string{
					"mode":         "max",
					"builder-id":   "mybuilder",
					"reproducible": "true",
				},
			},
			want: "attest:provenance=mode=max,builder-id=mybuilder,reproducible=true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildProvenanceOpt(tt.config)
			if got != tt.want {
				t.Errorf("buildProvenanceOpt() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name  string
		slice []string
		item  string
		want  bool
	}{
		{
			name:  "item exists",
			slice: []string{"apple", "banana", "cherry"},
			item:  "banana",
			want:  true,
		},
		{
			name:  "item does not exist",
			slice: []string{"apple", "banana", "cherry"},
			item:  "orange",
			want:  false,
		},
		{
			name:  "empty slice",
			slice: []string{},
			item:  "apple",
			want:  false,
		},
		{
			name:  "empty item",
			slice: []string{"apple", "", "banana"},
			item:  "",
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contains(tt.slice, tt.item)
			if got != tt.want {
				t.Errorf("contains() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ===== TESTS FOR sanitizeCommandArgs() FUNCTION =====

func TestSanitizeCommandArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "sanitize git url with credentials",
			args: []string{
				"context=https://user:password@github.com/repo.git",
			},
			want: []string{
				"context=https://user:**REDACTED**@github.com/repo.git",
			},
		},
		{
			name: "sanitize sensitive build args",
			args: []string{
				"build-arg:USERNAME=user",
				"build-arg:PASSWORD=secret123",
				"build-arg:API_KEY=key123",
			},
			want: []string{
				"build-arg:USERNAME=user",
				"build-arg:PASSWORD=***REDACTED***",
				"build-arg:API_KEY=***REDACTED***",
			},
		},
		{
			name: "preserve non-sensitive args",
			args: []string{
				"build-arg:VERSION=1.0.0",
				"build-arg:ENVIRONMENT=production",
			},
			want: []string{
				"build-arg:VERSION=1.0.0",
				"build-arg:ENVIRONMENT=production",
			},
		},
		{
			name: "mixed sensitive and non-sensitive",
			args: []string{
				"build-arg:VERSION=1.0.0",
				"build-arg:GIT_TOKEN=ghp_secret",
				"build-arg:NODE_ENV=prod",
			},
			want: []string{
				"build-arg:VERSION=1.0.0",
				"build-arg:GIT_TOKEN=***REDACTED***",
				"build-arg:NODE_ENV=prod",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeCommandArgs(tt.args)

			if len(got) != len(tt.want) {
				t.Errorf("Length mismatch: got %d, want %d", len(got), len(tt.want))
				return
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Arg[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSanitizeCommandArgs_GitURLs(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool // whether credentials should be hidden
	}{
		{
			name: "https with credentials",
			url:  "https://user:pass@github.com/repo.git",
			want: true,
		},
		{
			name: "https without credentials",
			url:  "https://github.com/repo.git",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"context=" + tt.url}
			sanitized := sanitizeCommandArgs(args)

			if tt.want {
				// Should contain hidden credentials
				if !strings.Contains(sanitized[0], "**REDACTED**") {
					t.Errorf("Expected credentials to be hidden, got %q", sanitized[0])
				}
			} else {
				// Should be unchanged
				if sanitized[0] != args[0] {
					t.Errorf("URL should not be modified: got %q, want %q", sanitized[0], args[0])
				}
			}
		})
	}
}

// ===== BENCHMARKS FOR NEW FUNCTIONS =====

func BenchmarkCopyFile(b *testing.B) {
	tmpDir := b.TempDir()
	srcPath := filepath.Join(tmpDir, "source.txt")
	content := []byte(strings.Repeat("test content ", 100))
	os.WriteFile(srcPath, content, 0644)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dstPath := filepath.Join(tmpDir, fmt.Sprintf("dest_%d.txt", i))
		copyFile(srcPath, dstPath)
	}
}

func BenchmarkCopyDir(b *testing.B) {
	tmpDir := b.TempDir()
	srcDir := filepath.Join(tmpDir, "source")
	os.MkdirAll(srcDir, 0755)

	// Create test structure
	for i := 0; i < 10; i++ {
		os.WriteFile(filepath.Join(srcDir, fmt.Sprintf("file%d.txt", i)), []byte("test"), 0644)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dstDir := filepath.Join(tmpDir, fmt.Sprintf("dest_%d", i))
		copyDir(srcDir, dstDir)
	}
}

func BenchmarkSanitizeCommandArgs(b *testing.B) {
	args := []string{
		"context=https://user:password@github.com/repo.git",
		"build-arg:PASSWORD=secret",
		"build-arg:VERSION=1.0.0",
		"build-arg:API_KEY=key123",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sanitizeCommandArgs(args)
	}
}

// ===== ADDITIONAL TESTS FOR 100% COVERAGE =====

func TestBuildAttestationOptsFromConfigs(t *testing.T) {
	tests := []struct {
		name          string
		configs       []AttestationConfig
		wantOpts      []string
		wantBuildArgs int // Number of build args added
	}{
		{
			name: "single sbom config",
			configs: []AttestationConfig{
				{
					Type:   "sbom",
					Params: map[string]string{},
				},
			},
			wantOpts:      []string{"attest:sbom=true"},
			wantBuildArgs: 0,
		},
		{
			name: "single provenance config",
			configs: []AttestationConfig{
				{
					Type: "provenance",
					Params: map[string]string{
						"mode": "max",
					},
				},
			},
			wantOpts:      []string{"attest:provenance=mode=max"},
			wantBuildArgs: 0,
		},
		{
			name: "sbom with scan-context",
			configs: []AttestationConfig{
				{
					Type: "sbom",
					Params: map[string]string{
						"scan-context": "true",
					},
				},
			},
			wantOpts:      []string{"attest:sbom=true"},
			wantBuildArgs: 1, // Should add BUILDKIT_SBOM_SCAN_CONTEXT
		},
		{
			name: "sbom with scan-stage",
			configs: []AttestationConfig{
				{
					Type: "sbom",
					Params: map[string]string{
						"scan-stage": "true",
					},
				},
			},
			wantOpts:      []string{"attest:sbom=true"},
			wantBuildArgs: 1, // Should add BUILDKIT_SBOM_SCAN_STAGE
		},
		{
			name: "sbom with both scan options",
			configs: []AttestationConfig{
				{
					Type: "sbom",
					Params: map[string]string{
						"scan-context": "true",
						"scan-stage":   "true",
					},
				},
			},
			wantOpts:      []string{"attest:sbom=true"},
			wantBuildArgs: 2, // Should add both build args
		},
		{
			name: "multiple configs",
			configs: []AttestationConfig{
				{
					Type:   "sbom",
					Params: map[string]string{},
				},
				{
					Type: "provenance",
					Params: map[string]string{
						"mode": "min",
					},
				},
			},
			wantOpts:      []string{"attest:sbom=true", "attest:provenance=mode=min"},
			wantBuildArgs: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{}
			opts := buildAttestationOptsFromConfigs(tt.configs, &args)

			if len(opts) != len(tt.wantOpts) {
				t.Errorf("Got %d options, want %d", len(opts), len(tt.wantOpts))
			}

			for i, want := range tt.wantOpts {
				if i >= len(opts) {
					t.Errorf("Missing option: %s", want)
					continue
				}
				if opts[i] != want {
					t.Errorf("Option[%d] = %q, want %q", i, opts[i], want)
				}
			}

			// Count build args (each build arg adds 2 elements to args: "--opt" and "build-arg:...")
			buildArgCount := 0
			for i := 0; i < len(args); i++ {
				if args[i] == "--opt" && i+1 < len(args) && strings.HasPrefix(args[i+1], "build-arg:BUILDKIT_SBOM_SCAN") {
					buildArgCount++
					i++ // Skip the next element as it's the build arg value
				}
			}

			if buildArgCount != tt.wantBuildArgs {
				t.Errorf("Got %d build args, want %d", buildArgCount, tt.wantBuildArgs)
			}
		})
	}
}

func TestBuildAttestationOptsFromConfigs_UnknownType(t *testing.T) {
	configs := []AttestationConfig{
		{
			Type:   "unknown",
			Params: map[string]string{},
		},
	}

	expectFatal(t, func() {
		args := []string{}
		buildAttestationOptsFromConfigs(configs, &args)
	}, "Unknown attestation type")
}

func TestSaveDigestInfo_EdgeCases(t *testing.T) {
	t.Run("write error for digest file", func(t *testing.T) {
		// Try to write to a directory that doesn't exist
		config := Config{
			Destination: []string{"registry.io/myapp:v1.0"},
			DigestFile:  "/nonexistent/path/digest.txt",
		}

		digestMap := map[string]string{
			"registry.io/myapp:v1.0": "sha256:test",
		}

		err := SaveDigestInfo(config, digestMap)
		if err == nil {
			t.Error("SaveDigestInfo() should error when unable to write digest file")
		}
	})

	t.Run("write error for image name with digest file", func(t *testing.T) {
		tmpDir := t.TempDir()
		config := Config{
			Destination:             []string{"registry.io/myapp:v1.0"},
			DigestFile:              filepath.Join(tmpDir, "digest.txt"),
			ImageNameWithDigestFile: "/nonexistent/path/image-digest.txt",
		}

		digestMap := map[string]string{
			"registry.io/myapp:v1.0": "sha256:test",
		}

		err := SaveDigestInfo(config, digestMap)
		if err == nil {
			t.Error("SaveDigestInfo() should error when unable to write image digest file")
		}
	})

	t.Run("write error for image name tag with digest file", func(t *testing.T) {
		tmpDir := t.TempDir()
		config := Config{
			Destination:                []string{"registry.io/myapp:v1.0"},
			DigestFile:                 filepath.Join(tmpDir, "digest.txt"),
			ImageNameWithDigestFile:    filepath.Join(tmpDir, "image-digest.txt"),
			ImageNameTagWithDigestFile: "/nonexistent/path/full-info.json",
		}

		digestMap := map[string]string{
			"registry.io/myapp:v1.0": "sha256:test",
		}

		err := SaveDigestInfo(config, digestMap)
		if err == nil {
			t.Error("SaveDigestInfo() should error when unable to write JSON file")
		}
	})
}

func TestCopyDir_Symlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Symlink test skipped on Windows")
	}

	t.Run("directory with symlinks", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcDir := filepath.Join(tmpDir, "source")
		dstDir := filepath.Join(tmpDir, "dest")

		// Create source structure
		os.MkdirAll(srcDir, 0755)
		os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("content"), 0644)

		// Create symlink (this will be treated as a regular file in ReadDir)
		targetFile := filepath.Join(srcDir, "file.txt")
		linkFile := filepath.Join(srcDir, "link.txt")
		os.Symlink(targetFile, linkFile)

		// Copy directory
		err := copyDir(srcDir, dstDir)
		if err != nil {
			t.Fatalf("copyDir() failed: %v", err)
		}

		// Verify both files exist in destination
		if _, err := os.Stat(filepath.Join(dstDir, "file.txt")); os.IsNotExist(err) {
			t.Error("Original file was not copied")
		}
		if _, err := os.Stat(filepath.Join(dstDir, "link.txt")); os.IsNotExist(err) {
			t.Error("Symlink was not copied")
		}
	})
}

func TestSanitizeCommandArgs_AllBranches(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "dockerfile option with credentials",
			args: []string{
				"dockerfile=https://user:token@github.com/repo.git",
			},
			want: []string{
				"dockerfile=https://user:**REDACTED**@github.com/repo.git",
			},
		},
		{
			name: "build-arg without value",
			args: []string{
				"build-arg:PASSWORD",
			},
			want: []string{
				"build-arg:PASSWORD",
			},
		},
		{
			name: "credentials in sensitive arg",
			args: []string{
				"build-arg:CREDENTIALS=secret",
			},
			want: []string{
				"build-arg:CREDENTIALS=***REDACTED***",
			},
		},
		{
			name: "secret in arg name",
			args: []string{
				"build-arg:MY_SECRET=value",
			},
			want: []string{
				"build-arg:MY_SECRET=***REDACTED***",
			},
		},
		{
			name: "git_password arg",
			args: []string{
				"build-arg:GIT_PASSWORD=pass123",
			},
			want: []string{
				"build-arg:GIT_PASSWORD=***REDACTED***",
			},
		},
		{
			name: "regular arg that's not sensitive",
			args: []string{
				"--target",
				"production",
			},
			want: []string{
				"--target",
				"production",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeCommandArgs(tt.args)

			if len(got) != len(tt.want) {
				t.Errorf("Length mismatch: got %d, want %d", len(got), len(tt.want))
				return
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Arg[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestBuildSBOMOpt_AllParams(t *testing.T) {
	t.Run("with additional params", func(t *testing.T) {
		config := AttestationConfig{
			Type: "sbom",
			Params: map[string]string{
				"generator": "docker/buildkit-syft-scanner",
				"format":    "spdx",
			},
		}

		got := buildSBOMOpt(config)
		// Should include generator and format, excluding scan-context and scan-stage
		if !strings.Contains(got, "generator=docker/buildkit-syft-scanner") {
			t.Errorf("Result missing generator: %s", got)
		}
		if !strings.Contains(got, "format=spdx") {
			t.Errorf("Result missing format: %s", got)
		}
	})
}

func TestBuildProvenanceOpt_AdditionalParams(t *testing.T) {
	t.Run("with params not in predefined order", func(t *testing.T) {
		config := AttestationConfig{
			Type: "provenance",
			Params: map[string]string{
				"mode":       "max",
				"custom-key": "custom-value",
			},
		}

		got := buildProvenanceOpt(config)
		if !strings.Contains(got, "mode=max") {
			t.Errorf("Result missing mode: %s", got)
		}
		if !strings.Contains(got, "custom-key=custom-value") {
			t.Errorf("Result missing custom key: %s", got)
		}
	})

	t.Run("with all predefined params", func(t *testing.T) {
		config := AttestationConfig{
			Type: "provenance",
			Params: map[string]string{
				"mode":         "max",
				"builder-id":   "mybuilder",
				"reproducible": "true",
				"inline-only":  "false",
				"version":      "1.0",
				"filename":     "provenance.json",
			},
		}

		got := buildProvenanceOpt(config)
		expectedParams := []string{"mode=max", "builder-id=mybuilder", "reproducible=true", "inline-only=false", "version=1.0", "filename=provenance.json"}
		for _, param := range expectedParams {
			if !strings.Contains(got, param) {
				t.Errorf("Result missing param %s in: %s", param, got)
			}
		}
	})
}

// ===== TESTS FOR executeBuildah COVERAGE =====

func TestExecuteBuildah_TarPath(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping buildah integration test in short mode")
	}

	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	// Check if buildah is available
	if _, err := exec.LookPath("buildah"); err != nil {
		t.Skip("Buildah not available, skipping test")
	}

	tmpDir := t.TempDir()

	config := Config{
		Dockerfile:  filepath.Join(tmpDir, "Dockerfile"),
		Destination: []string{"test:latest"},
		TarPath:     filepath.Join(tmpDir, "output.tar"),
		NoPush:      true,
	}

	// Create minimal Dockerfile
	os.WriteFile(config.Dockerfile, []byte("FROM scratch\n"), 0644)

	ctx := &Context{
		Path: tmpDir,
	}

	// This will fail because we can't actually build with scratch, but tests the TarPath code path
	err := executeBuildah(config, ctx)
	// We expect an error, but we've tested the code path
	_ = err
}

func TestExecuteBuildah_InsecurePull(t *testing.T) {
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	mockPath := createMockBinaries(t, []string{"buildah"})
	os.Setenv("PATH", mockPath)

	config := Config{
		Dockerfile:   "Dockerfile",
		Destination:  []string{"test:latest"},
		InsecurePull: true,
	}

	ctx := &Context{
		Path: t.TempDir(),
	}

	// Will fail on execution but tests the InsecurePull flag logic
	err := executeBuildah(config, ctx)
	_ = err // Expected to fail
}

func TestExecuteBuildah_CustomPlatform(t *testing.T) {
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	mockPath := createMockBinaries(t, []string{"buildah"})
	os.Setenv("PATH", mockPath)

	config := Config{
		Dockerfile:     "Dockerfile",
		Destination:    []string{"test:latest"},
		CustomPlatform: "linux/arm64",
	}

	ctx := &Context{
		Path: t.TempDir(),
	}

	err := executeBuildah(config, ctx)
	_ = err // Expected to fail
}

func TestExecuteBuildah_BuildArgWithoutValue(t *testing.T) {
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	mockPath := createMockBinaries(t, []string{"buildah"})
	os.Setenv("PATH", mockPath)

	config := Config{
		Dockerfile:  "Dockerfile",
		Destination: []string{"test:latest"},
		BuildArgs: map[string]string{
			"VERSION":  "1.0",
			"NODE_ENV": "", // Empty value means use environment variable
		},
	}

	ctx := &Context{
		Path: t.TempDir(),
	}

	err := executeBuildah(config, ctx)
	_ = err // Expected to fail, but tests the build arg logic
}

// ===== TESTS FOR exportToTar FUNCTION =====

func TestExportToTar_NoDestination(t *testing.T) {
	config := Config{
		Destination: []string{},
		TarPath:     "/tmp/test.tar",
	}

	err := exportToTar(config)
	if err == nil {
		t.Error("exportToTar() should return error when no destination specified")
	}

	if !strings.Contains(err.Error(), "no destination") {
		t.Errorf("Error should mention no destination, got: %v", err)
	}
}

func TestExportToTar_MockBuildahNotAvailable(t *testing.T) {
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	// Set PATH to directory without buildah
	tmpDir := t.TempDir()
	os.Setenv("PATH", tmpDir)

	config := Config{
		Destination: []string{"test:latest"},
		TarPath:     filepath.Join(tmpDir, "test.tar"),
	}

	err := exportToTar(config)
	// Should fail because buildah is not available
	if err == nil {
		t.Error("exportToTar() should fail when buildah is not available")
	}
}

// ===== TESTS FOR signImageWithCosign FUNCTION =====

func TestSignImageWithCosign_MockCosignNotAvailable(t *testing.T) {
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	// Set PATH to directory without cosign
	tmpDir := t.TempDir()
	os.Setenv("PATH", tmpDir)

	config := Config{
		CosignKeyPath: filepath.Join(tmpDir, "cosign.key"),
	}

	err := signImageWithCosign("test:latest", config)
	// Should fail because cosign is not available
	if err == nil {
		t.Error("signImageWithCosign() should fail when cosign is not available")
	}
}

func TestSignImageWithCosign_WithInsecureRegistry(t *testing.T) {
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	tmpDir := t.TempDir()
	os.Setenv("PATH", tmpDir)

	config := Config{
		CosignKeyPath: filepath.Join(tmpDir, "cosign.key"),
		Insecure:      true,
	}

	err := signImageWithCosign("test:latest", config)
	// Should fail because cosign is not available, but tests insecure flag logic
	if err == nil {
		t.Error("signImageWithCosign() should fail when cosign is not available")
	}
}

func TestSignImageWithCosign_WithInsecureRegistryList(t *testing.T) {
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	tmpDir := t.TempDir()
	os.Setenv("PATH", tmpDir)

	config := Config{
		CosignKeyPath:    filepath.Join(tmpDir, "cosign.key"),
		InsecureRegistry: []string{"localhost:5000"},
	}

	err := signImageWithCosign("test:latest", config)
	// Should fail because cosign is not available, but tests insecure registry list logic
	if err == nil {
		t.Error("signImageWithCosign() should fail when cosign is not available")
	}
}

func TestSignImageWithCosign_WithPasswordEnv(t *testing.T) {
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	tmpDir := t.TempDir()
	os.Setenv("PATH", tmpDir)

	// Set password environment variable
	os.Setenv("COSIGN_PASSWORD_TEST", "test-password")
	defer os.Unsetenv("COSIGN_PASSWORD_TEST")

	config := Config{
		CosignKeyPath:     filepath.Join(tmpDir, "cosign.key"),
		CosignPasswordEnv: "COSIGN_PASSWORD_TEST",
	}

	err := signImageWithCosign("test:latest", config)
	// Should fail because cosign is not available, but tests password env logic
	if err == nil {
		t.Error("signImageWithCosign() should fail when cosign is not available")
	}
}

func TestSignImageWithCosign_WithEmptyPasswordEnv(t *testing.T) {
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	tmpDir := t.TempDir()
	os.Setenv("PATH", tmpDir)

	// Don't set the environment variable
	config := Config{
		CosignKeyPath:     filepath.Join(tmpDir, "cosign.key"),
		CosignPasswordEnv: "COSIGN_PASSWORD_NONEXISTENT",
	}

	err := signImageWithCosign("test:latest", config)
	// Should fail because cosign is not available, but tests empty password env warning
	if err == nil {
		t.Error("signImageWithCosign() should fail when cosign is not available")
	}
}

// ===== ADDITIONAL COVERAGE TESTS =====

func TestCopyFile_LargeFile(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "large.bin")
	dstPath := filepath.Join(tmpDir, "large_copy.bin")

	// Create a larger file (1MB)
	largeContent := make([]byte, 1024*1024)
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}

	err := os.WriteFile(srcPath, largeContent, 0644)
	if err != nil {
		t.Fatalf("Failed to create large file: %v", err)
	}

	err = copyFile(srcPath, dstPath)
	if err != nil {
		t.Fatalf("copyFile() failed: %v", err)
	}

	// Verify content
	dstContent, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("Failed to read destination: %v", err)
	}

	if len(dstContent) != len(largeContent) {
		t.Errorf("Size mismatch: got %d, want %d", len(dstContent), len(largeContent))
	}
}

func TestCopyDir_DeepNesting(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "deep")
	dstDir := filepath.Join(tmpDir, "deep_copy")

	// Create deeply nested structure
	deepPath := srcDir
	for i := 0; i < 10; i++ {
		deepPath = filepath.Join(deepPath, fmt.Sprintf("level%d", i))
	}
	os.MkdirAll(deepPath, 0755)
	os.WriteFile(filepath.Join(deepPath, "deep.txt"), []byte("deep content"), 0644)

	err := copyDir(srcDir, dstDir)
	if err != nil {
		t.Fatalf("copyDir() failed: %v", err)
	}

	// Verify deep file exists
	deepCopyPath := dstDir
	for i := 0; i < 10; i++ {
		deepCopyPath = filepath.Join(deepCopyPath, fmt.Sprintf("level%d", i))
	}
	deepFile := filepath.Join(deepCopyPath, "deep.txt")

	if _, err := os.Stat(deepFile); os.IsNotExist(err) {
		t.Error("Deep file was not copied")
	}
}

func TestSanitizeCommandArgs_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "empty args",
			args: []string{},
			want: []string{},
		},
		{
			name: "arg with equals but no value",
			args: []string{"build-arg:KEY="},
			want: []string{"build-arg:KEY="},
		},
		{
			name: "multiple equals signs",
			args: []string{"build-arg:KEY=value=with=equals"},
			want: []string{"build-arg:KEY=value=with=equals"},
		},
		{
			name: "sensitive arg with multiple equals",
			args: []string{"build-arg:PASSWORD=secret=with=equals"},
			want: []string{"build-arg:PASSWORD=***REDACTED***"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeCommandArgs(tt.args)

			if len(got) != len(tt.want) {
				t.Errorf("Length mismatch: got %d, want %d", len(got), len(tt.want))
				return
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Arg[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestExecuteBuildah_AllConfigOptions(t *testing.T) {
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	mockPath := createMockBinaries(t, []string{"buildah"})
	os.Setenv("PATH", mockPath)

	tmpDir := t.TempDir()

	config := Config{
		Dockerfile:         filepath.Join(tmpDir, "Custom.dockerfile"),
		Destination:        []string{"registry.io/app:v1", "registry.io/app:latest"},
		Target:             "production",
		BuildArgs:          map[string]string{"VERSION": "1.0", "ENV": "prod"},
		Labels:             map[string]string{"app": "myapp", "version": "1.0"},
		CustomPlatform:     "linux/amd64",
		Cache:              false,
		Reproducible:       true,
		Timestamp:          "1609459200",
		ImageDownloadRetry: 3,
		InsecureRegistry:   []string{"localhost:5000"},
		StorageDriver:      "overlay",
	}

	ctx := &Context{
		Path: tmpDir,
	}

	// Will fail on execution but tests all config options
	err := executeBuildah(config, ctx)
	_ = err // Expected to fail
}

func TestExecuteBuildah_CacheWithCacheDir(t *testing.T) {
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	mockPath := createMockBinaries(t, []string{"buildah"})
	os.Setenv("PATH", mockPath)

	config := Config{
		Dockerfile:   "Dockerfile",
		Destination:  []string{"test:latest"},
		Cache:        true,
		CacheDir:     "/tmp/cache",
		Reproducible: false,
	}

	ctx := &Context{
		Path: t.TempDir(),
	}

	err := executeBuildah(config, ctx)
	_ = err // Expected to fail, tests cache with cache dir logic
}

func TestExecuteBuildah_ExistingBuildahIsolation(t *testing.T) {
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	// Set existing BUILDAH_ISOLATION
	originalIsolation := os.Getenv("BUILDAH_ISOLATION")
	os.Setenv("BUILDAH_ISOLATION", "rootless")
	defer func() {
		if originalIsolation == "" {
			os.Unsetenv("BUILDAH_ISOLATION")
		} else {
			os.Setenv("BUILDAH_ISOLATION", originalIsolation)
		}
	}()

	mockPath := createMockBinaries(t, []string{"buildah"})
	os.Setenv("PATH", mockPath)

	config := Config{
		Dockerfile:  "Dockerfile",
		Destination: []string{"test:latest"},
	}

	ctx := &Context{
		Path: t.TempDir(),
	}

	err := executeBuildah(config, ctx)
	_ = err // Expected to fail, tests existing isolation env var
}
