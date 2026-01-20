package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestPrintHelp(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Call printHelp
	printHelp()

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Test cases - check for expected content
	tests := []struct {
		name        string
		expectedStr string
		description string
	}{
		// Header
		{"title", "Kimia", "Should contain application name"},
		{"subtitle", "Kubernetes-Native OCI", "Should contain subtitle"},
		{"tagline", "Daemonless", "Should contain feature tagline"},

		// Usage section
		{"usage header", "USAGE:", "Should have USAGE section"},
		{"basic usage", "kimia --context", "Should show basic usage"},
		{"check environment", "check-environment", "Should show check-environment command"},

		// Core options
		{"core options header", "CORE OPTIONS:", "Should have CORE OPTIONS section"},
		{"context flag", "--context", "Should document context flag"},
		{"dockerfile flag", "--dockerfile", "Should document dockerfile flag"},
		{"destination flag", "--destination", "Should document destination flag"},
		{"target flag", "--target", "Should document target flag"},

		// Build options
		{"build options header", "BUILD OPTIONS:", "Should have BUILD OPTIONS section"},
		{"build-arg flag", "--build-arg", "Should document build-arg flag"},
		{"label flag", "--label", "Should document label flag"},
		{"no-push flag", "--no-push", "Should document no-push flag"},
		{"cache flag", "--cache", "Should document cache flag"},
		{"storage driver", "--storage-driver", "Should document storage-driver flag"},

		// Reproducible builds
		{"reproducible header", "REPRODUCIBLE BUILDS:", "Should have REPRODUCIBLE BUILDS section"},
		{"reproducible flag", "--reproducible", "Should document reproducible flag"},
		{"timestamp flag", "--timestamp", "Should document timestamp flag"},
		{"source date epoch", "SOURCE_DATE_EPOCH", "Should mention SOURCE_DATE_EPOCH"},

		// Git options
		{"git options header", "GIT OPTIONS:", "Should have GIT OPTIONS section"},
		{"git branch", "--git-branch", "Should document git-branch flag"},
		{"git revision", "--git-revision", "Should document git-revision flag"},

		// Registry options
		{"registry options header", "REGISTRY OPTIONS:", "Should have REGISTRY OPTIONS section"},
		{"insecure flag", "--insecure", "Should document insecure flag"},
		{"push retry", "--push-retry", "Should document push-retry flag"},

		// Authentication
		{"auth header", "AUTHENTICATION:", "Should have AUTHENTICATION section"},
		{"docker config", "config.json", "Should mention Docker config.json"},
		{"credential helpers", "Credential helpers (ecr-login, gcr, acr-env)", "Should mention credential helpers"},

		// Output options
		{"output options header", "OUTPUT OPTIONS:", "Should have OUTPUT OPTIONS section"},
		{"tar path", "--tar-path", "Should document tar-path flag"},
		{"digest file", "--digest-file", "Should document digest-file flag"},

		// Logging
		{"logging header", "LOGGING:", "Should have LOGGING section"},
		{"verbosity flag", "--verbosity", "Should document verbosity flag"},
		{"log timestamp", "--log-timestamp", "Should document log-timestamp flag"},

		// Storage drivers
		{"storage drivers header", "STORAGE DRIVERS:", "Should have STORAGE DRIVERS section"},
		{"overlay driver", "overlay", "Should document overlay driver"},

		// Examples
		{"examples header", "EXAMPLES:", "Should have EXAMPLES section"},
		{"local build example", "Build from local directory", "Should have local build example"},
		{"git build example", "Build from Git repository", "Should have Git build example"},

		// Authentication examples
		{"auth examples header", "AUTHENTICATION EXAMPLES:", "Should have AUTHENTICATION EXAMPLES section"},
		{"docker login example", "docker login", "Should show docker login example"},
		{"kubernetes secret", "kubectl create secret", "Should show Kubernetes secret example"},

		// Environment variables
		{"env vars header", "ENVIRONMENT VARIABLES:", "Should have ENVIRONMENT VARIABLES section"},
		{"docker config env", "DOCKER_CONFIG", "Should document DOCKER_CONFIG"},
		{"docker username env", "DOCKER_USERNAME", "Should document DOCKER_USERNAME"},

		// Version info
		{"version in help", "Version:", "Should show version info"},
		{"github link", "github.com/rapidfort/kimia", "Should have GitHub link"},

		// Help and version flags
		{"help flag", "--help", "Should document help flag"},
		{"version flag", "--version", "Should document version flag"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(output, tt.expectedStr) {
				t.Errorf("printHelp() output missing %q\nDescription: %s",
					tt.expectedStr, tt.description)
			}
		})
	}
}

func TestPrintHelp_BuilderSpecific(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printHelp()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Check for builder-specific content
	// Note: The actual builder detection happens at runtime
	// This test just ensures the output is consistent

	t.Run("contains builder indication", func(t *testing.T) {
		hasBuildah := strings.Contains(output, "Buildah")
		hasBuildkit := strings.Contains(output, "Buildkit")

		if !hasBuildah && !hasBuildkit {
			t.Error("Output should indicate either Buildah or Buildkit builder")
		}

		if hasBuildah && hasBuildkit {
			// Both mentions are okay as long as it's clear which is active
			t.Log("Output mentions both builders (this is okay)")
		}
	})

	t.Run("has attestation section for buildkit", func(t *testing.T) {
		if strings.Contains(output, "Buildkit") {
			if !strings.Contains(output, "ATTESTATION") {
				t.Error("Buildkit output should include ATTESTATION section")
			}
			if !strings.Contains(output, "--attest") {
				t.Error("Buildkit output should document --attest flag")
			}
		}
	})
}

func TestPrintVersionInfo(t *testing.T) {
	// Save original values
	origVersion := Version
	origBuildDate := BuildDate
	origCommitSHA := CommitSHA

	// Set test values
	Version = "1.2.3-test"
	BuildDate = "1609459200" // 2021-01-01 00:00:00 UTC
	CommitSHA = "abc123def456"

	// Restore after test
	defer func() {
		Version = origVersion
		BuildDate = origBuildDate
		CommitSHA = origCommitSHA
	}()

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printVersionInfo()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Test expected content
	tests := []struct {
		name        string
		expectedStr string
	}{
		{"contains version label", "Version:"},
		{"contains version value", "1.2.3-test"},
		{"contains built label", "Built:"},
		{"contains date", "2021-01-01"},
		{"contains commit label", "Commit:"},
		{"contains commit value", "abc123def456"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(output, tt.expectedStr) {
				t.Errorf("printVersionInfo() output missing %q\nGot: %s",
					tt.expectedStr, output)
			}
		})
	}
}

func TestPrintVersionInfo_Format(t *testing.T) {
	// Test that output is on a single line
	Version = "1.0.0"
	BuildDate = "1609459200"
	CommitSHA = "abc123"

	defer func() {
		Version = "1.0.0-dev"
		BuildDate = "unknown"
		CommitSHA = "unknown"
	}()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printVersionInfo()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 {
		t.Errorf("printVersionInfo() should output single line, got %d lines", len(lines))
	}
}

func TestPrintHelp_Structure(t *testing.T) {
	// Test that help output has proper structure
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printHelp()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	t.Run("has proper section ordering", func(t *testing.T) {
		// Check that sections appear in expected order
		usagePos := strings.Index(output, "USAGE:")
		corePos := strings.Index(output, "CORE OPTIONS:")
		buildPos := strings.Index(output, "BUILD OPTIONS:")
		examplesPos := strings.Index(output, "EXAMPLES:")

		if usagePos == -1 || corePos == -1 || buildPos == -1 || examplesPos == -1 {
			t.Fatal("Missing required sections")
		}

		if usagePos > corePos {
			t.Error("USAGE should come before CORE OPTIONS")
		}
		if corePos > buildPos {
			t.Error("CORE OPTIONS should come before BUILD OPTIONS")
		}
		if examplesPos < buildPos {
			t.Error("EXAMPLES should come after BUILD OPTIONS")
		}
	})

	t.Run("has version info at end", func(t *testing.T) {
		lines := strings.Split(output, "\n")
		lastLines := strings.Join(lines[len(lines)-10:], "\n")

		if !strings.Contains(lastLines, "Version:") {
			t.Error("Version info should appear near the end")
		}
	})

	t.Run("has github link", func(t *testing.T) {
		if !strings.Contains(output, "github.com/rapidfort/kimia") {
			t.Error("Should have GitHub link at end")
		}
	})
}

func TestPrintHelp_FlagDocumentation(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printHelp()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Test that important flags are documented with descriptions
	requiredFlags := []struct {
		flag        string
		description string
	}{
		{"--context", "should explain what context is"},
		{"--destination", "should explain destination format"},
		{"--dockerfile", "should explain dockerfile path"},
		{"--build-arg", "should explain build arguments"},
		{"--cache", "should explain caching"},
		{"--reproducible", "should explain reproducible builds"},
	}

	for _, rf := range requiredFlags {
		t.Run("documents "+rf.flag, func(t *testing.T) {
			flagPos := strings.Index(output, rf.flag)
			if flagPos == -1 {
				t.Errorf("Flag %q not found in help output", rf.flag)
				return
			}

			// Check that there's some description text after the flag
			afterFlag := output[flagPos:]
			nextNewline := strings.Index(afterFlag, "\n")
			if nextNewline == -1 {
				nextNewline = len(afterFlag)
			}
			flagLine := afterFlag[:nextNewline]

			// Flag line should be more than just the flag name
			if len(flagLine) < len(rf.flag)+10 {
				t.Errorf("Flag %q appears to have no description: %q", rf.flag, flagLine)
			}
		})
	}
}

func TestPrintHelp_Examples(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printHelp()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Check for practical examples
	examples := []string{
		"kimia --context=.",                  // Local build
		"kimia --context=https://github.com", // Git build
		"--build-arg",                        // Build args example
		"--label",                            // Labels example
		"docker login",                       // Auth example
		"kubectl create secret",              // K8s example
	}

	for _, example := range examples {
		t.Run("has example: "+example, func(t *testing.T) {
			if !strings.Contains(output, example) {
				t.Errorf("Help should include example with %q", example)
			}
		})
	}
}

func TestPrintHelp_NoErrorOutput(t *testing.T) {
	// Capture both stdout and stderr
	oldStdout := os.Stdout
	oldStderr := os.Stderr

	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()

	os.Stdout = wOut
	os.Stderr = wErr

	printHelp()

	wOut.Close()
	wErr.Close()

	os.Stdout = oldStdout
	os.Stderr = oldStderr

	// Read stderr
	var errBuf bytes.Buffer
	io.Copy(&errBuf, rErr)
	stderrOutput := errBuf.String()

	// Read stdout (to drain the pipe)
	var outBuf bytes.Buffer
	io.Copy(&outBuf, rOut)

	t.Run("no output to stderr", func(t *testing.T) {
		if stderrOutput != "" {
			t.Errorf("printHelp() should not write to stderr, got: %q", stderrOutput)
		}
	})
}

// Benchmark to ensure help doesn't take too long
func BenchmarkPrintHelp(b *testing.B) {
	// Redirect output to discard
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	for i := 0; i < b.N; i++ {
		printHelp()
	}
}

func BenchmarkPrintVersionInfo(b *testing.B) {
	oldStdout := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = oldStdout }()

	for i := 0; i < b.N; i++ {
		printVersionInfo()
	}
}
