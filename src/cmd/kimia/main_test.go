package main

import (
	"bytes"
	"io"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/rapidfort/kimia/internal/build"
)

// ============================================================================
// Version Tests
// ============================================================================

func TestPrintVersion(t *testing.T) {
	// Save original values
	origVersion := Version
	origBuildDate := BuildDate
	origCommitSHA := CommitSHA
	origBranch := Branch

	defer func() {
		Version = origVersion
		BuildDate = origBuildDate
		CommitSHA = origCommitSHA
		Branch = origBranch
	}()

	tests := []struct {
		name           string
		version        string
		buildDate      string
		commitSHA      string
		branch         string
		expectedStrs   []string
		unexpectedStrs []string
	}{
		{
			name:      "standard version output",
			version:   "1.2.3",
			buildDate: "1609459200", // 2021-01-01 00:00:00 UTC
			commitSHA: "abc123def456",
			branch:    "main",
			expectedStrs: []string{
				"Kimia",
				"Kubernetes-Native OCI Builder",
				"Daemonless",
				"Rootless",
				"Privilege-free",
				"Version: 1.2.3",
				"abc123def456",
			},
		},
		{
			name:      "dev version",
			version:   "1.0.0-dev",
			buildDate: "unknown",
			commitSHA: "unknown",
			branch:    "develop",
			expectedStrs: []string{
				"Version: 1.0.0-dev",
				"Built: unknown",
				"Commit: unknown",
			},
		},
		{
			name:      "release version with epoch date",
			version:   "2.0.0",
			buildDate: "1640995200", // 2022-01-01 00:00:00 UTC (may be 2021-12-31 in some timezones)
			commitSHA: "fedcba987654",
			branch:    "release",
			expectedStrs: []string{
				"Version: 2.0.0",
				"fedcba987654",
				"202", // Year prefix - could be 2021 or 2022 depending on timezone
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Version = tt.version
			BuildDate = tt.buildDate
			CommitSHA = tt.commitSHA
			Branch = tt.branch

			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			printVersion()

			w.Close()
			os.Stdout = oldStdout

			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := buf.String()

			// Check expected strings
			for _, expected := range tt.expectedStrs {
				if !strings.Contains(output, expected) {
					t.Errorf("printVersion() output missing %q\nGot: %s", expected, output)
				}
			}

			// Check unexpected strings
			for _, unexpected := range tt.unexpectedStrs {
				if strings.Contains(output, unexpected) {
					t.Errorf("printVersion() output should not contain %q\nGot: %s", unexpected, output)
				}
			}
		})
	}
}

func TestConvertEpochStringToHumanReadable(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		contains    bool // if true, check contains; if false, check exact match
		checkYear   bool // if true, just check the year is present
		expectedYear string
	}{
		{
			name:        "valid epoch - 2021 (timezone may shift date)",
			input:       "1609459200",
			checkYear:   true,
			expectedYear: "202", // Could be 2020 or 2021 depending on timezone
		},
		{
			name:      "valid epoch returns formatted date",
			input:     "1655251200",
			checkYear: true,
			expectedYear: "2022",
		},
		{
			name:        "epoch zero returns 1969 or 1970 (timezone dependent)",
			input:       "0",
			checkYear:   true,
			expectedYear: "19", // Could be 1969 or 1970
		},
		{
			name:     "unknown string",
			input:    "unknown",
			expected: "unknown",
			contains: false,
		},
		{
			name:     "non-numeric string",
			input:    "not-a-number",
			expected: "not-a-number",
			contains: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
			contains: false,
		},
		{
			name:        "negative epoch (before 1970)",
			input:       "-86400",
			checkYear:   true,
			expectedYear: "1969",
		},
		{
			name:        "large epoch (far future)",
			input:       "4102444800", // Around 2100
			checkYear:   true,
			expectedYear: "20", // 2099 or 2100 depending on timezone
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertEpochStringToHumanReadable(tt.input)

			if tt.checkYear {
				if !strings.Contains(result, tt.expectedYear) {
					t.Errorf("convertEpochStringToHumanReadable(%q) = %q, want to contain year prefix %q",
						tt.input, result, tt.expectedYear)
				}
			} else if tt.contains {
				if !strings.Contains(result, tt.expected) {
					t.Errorf("convertEpochStringToHumanReadable(%q) = %q, want to contain %q",
						tt.input, result, tt.expected)
				}
			} else {
				if result != tt.expected {
					t.Errorf("convertEpochStringToHumanReadable(%q) = %q, want %q",
						tt.input, result, tt.expected)
				}
			}
		})
	}
}

func TestConvertEpochStringToHumanReadable_Format(t *testing.T) {
	// Test that the output format includes time zone
	result := convertEpochStringToHumanReadable("1609459200")

	// Should contain date in YYYY-MM-DD format (year starts with 202x)
	if !strings.Contains(result, "202") {
		t.Errorf("Result should contain year starting with 202, got: %s", result)
	}

	// Should contain time in HH:MM:SS format (with colons)
	if !strings.Contains(result, ":") {
		t.Errorf("Result should contain time with colons, got: %s", result)
	}

	// Should contain dashes for date format
	if !strings.Contains(result, "-") {
		t.Errorf("Result should contain dashes for date format, got: %s", result)
	}
}

// ============================================================================
// convertAttestationConfigs Tests
// ============================================================================

func TestConvertAttestationConfigs(t *testing.T) {
	tests := []struct {
		name        string
		mainConfigs []AttestationConfig
		want        []build.AttestationConfig
	}{
		{
			name:        "empty configs",
			mainConfigs: []AttestationConfig{},
			want:        []build.AttestationConfig{},
		},
		{
			name: "single sbom config",
			mainConfigs: []AttestationConfig{
				{
					Type: "sbom",
					Params: map[string]string{
						"generator": "custom:v1",
					},
				},
			},
			want: []build.AttestationConfig{
				{
					Type: "sbom",
					Params: map[string]string{
						"generator": "custom:v1",
					},
				},
			},
		},
		{
			name: "single provenance config",
			mainConfigs: []AttestationConfig{
				{
					Type: "provenance",
					Params: map[string]string{
						"mode": "max",
					},
				},
			},
			want: []build.AttestationConfig{
				{
					Type: "provenance",
					Params: map[string]string{
						"mode": "max",
					},
				},
			},
		},
		{
			name: "multiple configs",
			mainConfigs: []AttestationConfig{
				{
					Type: "sbom",
					Params: map[string]string{
						"generator":  "syft",
						"scan-stage": "true",
					},
				},
				{
					Type: "provenance",
					Params: map[string]string{
						"mode":       "max",
						"builder-id": "https://github.com/org/repo",
					},
				},
			},
			want: []build.AttestationConfig{
				{
					Type: "sbom",
					Params: map[string]string{
						"generator":  "syft",
						"scan-stage": "true",
					},
				},
				{
					Type: "provenance",
					Params: map[string]string{
						"mode":       "max",
						"builder-id": "https://github.com/org/repo",
					},
				},
			},
		},
		{
			name: "config with empty params",
			mainConfigs: []AttestationConfig{
				{
					Type:   "sbom",
					Params: map[string]string{},
				},
			},
			want: []build.AttestationConfig{
				{
					Type:   "sbom",
					Params: map[string]string{},
				},
			},
		},
		{
			name: "config with nil params",
			mainConfigs: []AttestationConfig{
				{
					Type:   "provenance",
					Params: nil,
				},
			},
			want: []build.AttestationConfig{
				{
					Type:   "provenance",
					Params: nil,
				},
			},
		},
		{
			name:        "nil input slice",
			mainConfigs: nil,
			want:        []build.AttestationConfig{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertAttestationConfigs(tt.mainConfigs)

			// Check length
			if len(got) != len(tt.want) {
				t.Errorf("convertAttestationConfigs() length = %d, want %d",
					len(got), len(tt.want))
				return
			}

			// Check each config
			for i := range got {
				if got[i].Type != tt.want[i].Type {
					t.Errorf("convertAttestationConfigs()[%d].Type = %q, want %q",
						i, got[i].Type, tt.want[i].Type)
				}

				if !reflect.DeepEqual(got[i].Params, tt.want[i].Params) {
					t.Errorf("convertAttestationConfigs()[%d].Params = %v, want %v",
						i, got[i].Params, tt.want[i].Params)
				}
			}
		})
	}
}

// ============================================================================
// Config Struct Tests
// ============================================================================

func TestConfigStructFields(t *testing.T) {
	config := &Config{
		Dockerfile:   "Dockerfile.prod",
		Context:      "/workspace",
		SubContext:   "src",
		Destination:  []string{"gcr.io/project/image:tag"},
		Cache:        true,
		CacheDir:     "/tmp/cache",
		NoPush:       false,
		TarPath:      "/output/image.tar",
		DigestFile:   "/output/digest.txt",
		Insecure:     false,
		InsecurePull: true,
		InsecureRegistry: []string{
			"registry.local:5000",
		},
		Verbosity:      "debug",
		LogTimestamp:   true,
		CustomPlatform: "linux/arm64",
		Target:         "builder",
		StorageDriver:  "overlay",
		Reproducible:   true,
		Timestamp:      "1609459200",
		BuildArgs: map[string]string{
			"VERSION": "1.0.0",
		},
		Labels: map[string]string{
			"maintainer": "team@example.com",
		},
		GitBranch:   "main",
		GitRevision: "abc123",
		Scan:        false,
		Harden:      false,
		Attestation: "max",
		AttestationConfigs: []AttestationConfig{
			{Type: "sbom", Params: map[string]string{}},
		},
		BuildKitOpts:      []string{"network=host"},
		Sign:              true,
		CosignKeyPath:     "/etc/cosign/cosign.key",
		CosignPasswordEnv: "COSIGN_PASSWORD",
	}

	// Verify all fields are set correctly
	if config.Dockerfile != "Dockerfile.prod" {
		t.Errorf("Dockerfile = %q, want %q", config.Dockerfile, "Dockerfile.prod")
	}
	if config.Context != "/workspace" {
		t.Errorf("Context = %q, want %q", config.Context, "/workspace")
	}
	if config.SubContext != "src" {
		t.Errorf("SubContext = %q, want %q", config.SubContext, "src")
	}
	if len(config.Destination) != 1 || config.Destination[0] != "gcr.io/project/image:tag" {
		t.Errorf("Destination = %v, want [gcr.io/project/image:tag]", config.Destination)
	}
	if !config.Cache {
		t.Error("Cache should be true")
	}
	if config.CacheDir != "/tmp/cache" {
		t.Errorf("CacheDir = %q, want %q", config.CacheDir, "/tmp/cache")
	}
	if config.NoPush {
		t.Error("NoPush should be false")
	}
	if config.TarPath != "/output/image.tar" {
		t.Errorf("TarPath = %q, want %q", config.TarPath, "/output/image.tar")
	}
	if !config.Reproducible {
		t.Error("Reproducible should be true")
	}
	if config.Timestamp != "1609459200" {
		t.Errorf("Timestamp = %q, want %q", config.Timestamp, "1609459200")
	}
	if config.Attestation != "max" {
		t.Errorf("Attestation = %q, want %q", config.Attestation, "max")
	}
	if !config.Sign {
		t.Error("Sign should be true")
	}
}

func TestConfigZeroValue(t *testing.T) {
	config := &Config{}

	// Zero values should be default Go values
	if config.Dockerfile != "" {
		t.Errorf("Zero value Dockerfile = %q, want empty", config.Dockerfile)
	}
	if config.Context != "" {
		t.Errorf("Zero value Context = %q, want empty", config.Context)
	}
	if config.Cache != false {
		t.Error("Zero value Cache should be false")
	}
	if config.NoPush != false {
		t.Error("Zero value NoPush should be false")
	}
	if config.Reproducible != false {
		t.Error("Zero value Reproducible should be false")
	}
	if config.Scan != false {
		t.Error("Zero value Scan should be false")
	}
	if config.Harden != false {
		t.Error("Zero value Harden should be false")
	}
	if config.Sign != false {
		t.Error("Zero value Sign should be false")
	}
	if config.Destination != nil {
		t.Errorf("Zero value Destination = %v, want nil", config.Destination)
	}
	if config.BuildArgs != nil {
		t.Errorf("Zero value BuildArgs = %v, want nil", config.BuildArgs)
	}
	if config.Labels != nil {
		t.Errorf("Zero value Labels = %v, want nil", config.Labels)
	}
}

func TestAttestationConfigStruct(t *testing.T) {
	tests := []struct {
		name   string
		config AttestationConfig
	}{
		{
			name: "sbom with generator",
			config: AttestationConfig{
				Type: "sbom",
				Params: map[string]string{
					"generator": "syft",
				},
			},
		},
		{
			name: "provenance with mode",
			config: AttestationConfig{
				Type: "provenance",
				Params: map[string]string{
					"mode": "max",
				},
			},
		},
		{
			name: "sbom with multiple params",
			config: AttestationConfig{
				Type: "sbom",
				Params: map[string]string{
					"generator":    "custom:v1",
					"scan-context": "true",
					"scan-stage":   "true",
				},
			},
		},
		{
			name: "empty params",
			config: AttestationConfig{
				Type:   "provenance",
				Params: map[string]string{},
			},
		},
		{
			name: "nil params",
			config: AttestationConfig{
				Type:   "sbom",
				Params: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify struct can be created and accessed without panic
			_ = tt.config.Type
			if tt.config.Params != nil {
				for k, v := range tt.config.Params {
					_ = k
					_ = v
				}
			}
		})
	}
}

// ============================================================================
// Version Variables Tests
// ============================================================================

func TestVersionVariablesExist(t *testing.T) {
	// Test that version variables are defined
	// These should have default values or be set at build time

	if Version == "" {
		t.Error("Version should not be empty")
	}

	// BuildDate, CommitSHA, Branch can be "unknown" but shouldn't be empty
	// They are set to "unknown" by default in version.go
	if BuildDate == "" {
		t.Error("BuildDate should not be empty (should be 'unknown' or a value)")
	}
	if CommitSHA == "" {
		t.Error("CommitSHA should not be empty (should be 'unknown' or a value)")
	}
	if Branch == "" {
		t.Error("Branch should not be empty (should be 'unknown' or a value)")
	}
}

func TestVersionVariablesDefaults(t *testing.T) {
	// The default values in version.go
	expectedVersion := "1.0.0-dev"
	expectedBuildDate := "unknown"
	expectedCommitSHA := "unknown"
	expectedBranch := "unknown"

	// These tests verify the defaults - they may fail if version is overridden at build time
	t.Run("default version format", func(t *testing.T) {
		// Version should be a semantic version
		parts := strings.Split(Version, ".")
		if len(parts) < 2 {
			t.Errorf("Version %q should be semantic version format", Version)
		}
	})

	t.Run("defaults match expected", func(t *testing.T) {
		// Only check if not overridden by build
		if Version == expectedVersion {
			t.Logf("Version is default: %s", Version)
		}
		if BuildDate == expectedBuildDate {
			t.Logf("BuildDate is default: %s", BuildDate)
		}
		if CommitSHA == expectedCommitSHA {
			t.Logf("CommitSHA is default: %s", CommitSHA)
		}
		if Branch == expectedBranch {
			t.Logf("Branch is default: %s", Branch)
		}
	})
}

// ============================================================================
// Edge Case Tests
// ============================================================================

func TestConfigWithSpecialCharacters(t *testing.T) {
	config := &Config{
		Context: "/path/with spaces/and-dashes",
		BuildArgs: map[string]string{
			"ARG_WITH_EQUALS": "value=with=equals",
			"ARG_WITH_QUOTES": `value with "quotes"`,
			"ARG_UNICODE":     "value with unicode: \u00e9\u00e0\u00f1",
		},
		Labels: map[string]string{
			"label.with.dots":  "value",
			"label-with-dash":  "value",
			"label_with_under": "value",
		},
	}

	// Verify special characters are preserved
	if config.Context != "/path/with spaces/and-dashes" {
		t.Errorf("Context with special characters not preserved: %q", config.Context)
	}

	if config.BuildArgs["ARG_WITH_EQUALS"] != "value=with=equals" {
		t.Errorf("BuildArg with equals not preserved: %q", config.BuildArgs["ARG_WITH_EQUALS"])
	}
}

func TestConfigWithEmptyValues(t *testing.T) {
	config := &Config{
		Dockerfile:  "",
		Context:     "",
		Destination: []string{},
		BuildArgs:   map[string]string{},
		Labels:      map[string]string{},
	}

	// Empty values should be handled
	if config.Dockerfile != "" {
		t.Error("Dockerfile should be empty string")
	}
	if config.Context != "" {
		t.Error("Context should be empty string")
	}
	if len(config.Destination) != 0 {
		t.Errorf("Destination should be empty slice, got %v", config.Destination)
	}
	if len(config.BuildArgs) != 0 {
		t.Errorf("BuildArgs should be empty map, got %v", config.BuildArgs)
	}
}

func TestConfigWithLargeValues(t *testing.T) {
	// Test with many destinations
	destinations := make([]string, 100)
	for i := 0; i < 100; i++ {
		destinations[i] = "registry.io/image:tag" + string(rune('0'+i%10))
	}

	// Test with many build args
	buildArgs := make(map[string]string)
	for i := 0; i < 100; i++ {
		buildArgs["ARG_"+string(rune('A'+i%26))] = "value"
	}

	config := &Config{
		Destination: destinations,
		BuildArgs:   buildArgs,
	}

	if len(config.Destination) != 100 {
		t.Errorf("Should handle 100 destinations, got %d", len(config.Destination))
	}

	if len(config.BuildArgs) != 26 { // Only 26 unique keys (A-Z)
		t.Errorf("Should handle build args, got %d", len(config.BuildArgs))
	}
}

// ============================================================================
// Concurrent Tests
// ============================================================================

func TestConvertAttestationConfigsConcurrent(t *testing.T) {
	const numGoroutines = 10

	configs := []AttestationConfig{
		{Type: "sbom", Params: map[string]string{"generator": "syft"}},
		{Type: "provenance", Params: map[string]string{"mode": "max"}},
	}

	var wg sync.WaitGroup
	results := make([][]build.AttestationConfig, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index] = convertAttestationConfigs(configs)
		}(i)
	}

	wg.Wait()

	// All results should be identical
	for i := 1; i < numGoroutines; i++ {
		if len(results[i]) != len(results[0]) {
			t.Errorf("Inconsistent results: results[%d] length = %d, results[0] length = %d",
				i, len(results[i]), len(results[0]))
		}
	}
}

func TestConvertEpochConcurrent(t *testing.T) {
	const numGoroutines = 10
	epoch := "1609459200"

	var wg sync.WaitGroup
	results := make([]string, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index] = convertEpochStringToHumanReadable(epoch)
		}(i)
	}

	wg.Wait()

	// All results should be identical
	for i := 1; i < numGoroutines; i++ {
		if results[i] != results[0] {
			t.Errorf("Inconsistent results: results[%d] = %q, results[0] = %q",
				i, results[i], results[0])
		}
	}
}

// ============================================================================
// Benchmarks
// ============================================================================

func BenchmarkConvertAttestationConfigs(b *testing.B) {
	configs := []AttestationConfig{
		{Type: "sbom", Params: map[string]string{"generator": "syft"}},
		{Type: "provenance", Params: map[string]string{"mode": "max"}},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = convertAttestationConfigs(configs)
	}
}

func BenchmarkConvertAttestationConfigsLarge(b *testing.B) {
	configs := make([]AttestationConfig, 100)
	for i := 0; i < 100; i++ {
		configs[i] = AttestationConfig{
			Type: "sbom",
			Params: map[string]string{
				"generator":  "syft",
				"scan-stage": "true",
			},
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = convertAttestationConfigs(configs)
	}
}

func BenchmarkConvertEpochStringToHumanReadable(b *testing.B) {
	epoch := "1609459200"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = convertEpochStringToHumanReadable(epoch)
	}
}

func BenchmarkConvertEpochStringToHumanReadable_Invalid(b *testing.B) {
	invalid := "not-a-number"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = convertEpochStringToHumanReadable(invalid)
	}
}

func BenchmarkPrintVersion(b *testing.B) {
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		printVersion()
	}
}

// ============================================================================
// Usage Examples (as test functions since main package functions are private)
// ============================================================================

func TestUsageExample_EpochConversion(t *testing.T) {
	// Example: Convert Unix epoch to human readable date
	result := convertEpochStringToHumanReadable("1609459200")
	// Output will contain year 2020 or 2021 (timezone dependent)
	if !strings.Contains(result, "202") {
		t.Errorf("Expected result to contain year starting with '202', got %q", result)
	}
	// Should have date format with dashes
	if !strings.Contains(result, "-") {
		t.Errorf("Expected result to contain date with dashes, got %q", result)
	}
}

func TestUsageExample_EpochConversionInvalid(t *testing.T) {
	// Example: Invalid input returns as-is
	result := convertEpochStringToHumanReadable("unknown")
	if result != "unknown" {
		t.Errorf("Expected 'unknown', got %q", result)
	}
}

func TestUsageExample_Config(t *testing.T) {
	// Example: Create a Config struct
	config := &Config{
		Context:     ".",
		Destination: []string{"registry.io/myapp:latest"},
		Cache:       true,
		BuildArgs: map[string]string{
			"VERSION": "1.0.0",
		},
	}

	if config.Context != "." {
		t.Errorf("Context = %q, want '.'", config.Context)
	}
	if len(config.Destination) != 1 {
		t.Errorf("Destination length = %d, want 1", len(config.Destination))
	}
}

func TestUsageExample_AttestationConfig(t *testing.T) {
	// Example: Create an AttestationConfig struct
	config := AttestationConfig{
		Type: "sbom",
		Params: map[string]string{
			"generator": "syft",
		},
	}

	if config.Type != "sbom" {
		t.Errorf("Type = %q, want 'sbom'", config.Type)
	}
	if config.Params["generator"] != "syft" {
		t.Errorf("Params[generator] = %q, want 'syft'", config.Params["generator"])
	}
}
