package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"reflect"
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

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		setupEnv  map[string]string // Environment variables to set
		want      *Config
		wantFatal bool
		wantExit  bool // For --help, --version that exit with 0
	}{
		// Basic flags
		{
			name: "dockerfile flag",
			args: []string{"--dockerfile", "Dockerfile.prod"},
			want: &Config{
				Dockerfile:         "Dockerfile.prod",
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "dockerfile flag with equals",
			args: []string{"--dockerfile=Dockerfile.dev"},
			want: &Config{
				Dockerfile:         "Dockerfile.dev",
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "context flag",
			args: []string{"--context", "/workspace"},
			want: &Config{
				Context:            "/workspace",
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "context-sub-path flag",
			args: []string{"--context-sub-path", "src"},
			want: &Config{
				SubContext:         "src",
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "context-sub-path empty string",
			args: []string{"--context-sub-path="},
			want: &Config{
				SubContext:         "",
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "destination flag",
			args: []string{"--destination", "gcr.io/project/image:tag"},
			want: &Config{
				Destination:        []string{"gcr.io/project/image:tag"},
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "multiple destinations",
			args: []string{
				"--destination", "gcr.io/project/image:tag1",
				"--destination", "docker.io/user/image:tag2",
			},
			want: &Config{
				Destination: []string{
					"gcr.io/project/image:tag1",
					"docker.io/user/image:tag2",
				},
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "cache flag true",
			args: []string{"--cache", "true"},
			want: &Config{
				Cache:              true,
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "cache flag without value",
			args: []string{"--cache"},
			want: &Config{
				Cache:              true,
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "cache-dir flag",
			args: []string{"--cache-dir", "/tmp/cache"},
			want: &Config{
				CacheDir:           "/tmp/cache",
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "no-push flag",
			args: []string{"--no-push"},
			want: &Config{
				NoPush:             true,
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "insecure flag",
			args: []string{"--insecure"},
			want: &Config{
				Insecure:           true,
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "verbosity flag",
			args: []string{"--verbosity", "debug"},
			want: &Config{
				Verbosity:          "debug",
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "reproducible flag",
			args: []string{"--reproducible"},
			want: &Config{
				Reproducible:       true,
				Timestamp:          "0",
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "reproducible with SOURCE_DATE_EPOCH",
			args: []string{"--reproducible"},
			setupEnv: map[string]string{
				"SOURCE_DATE_EPOCH": "1609459200",
			},
			want: &Config{
				Reproducible:       true,
				Timestamp:          "1609459200",
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "explicit timestamp",
			args: []string{"--timestamp", "1234567890"},
			want: &Config{
				Reproducible:       true,
				Timestamp:          "1234567890",
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "attestation min",
			args: []string{"--attestation", "min"},
			want: &Config{
				Attestation:        "min",
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "attestation without value defaults to min",
			args: []string{"--attestation"},
			want: &Config{
				Attestation:        "min",
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "sign flag",
			args: []string{"--sign", "--attestation", "min"},
			want: &Config{
				Sign:               true,
				Attestation:        "min",
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "build-arg single",
			args: []string{"--build-arg", "VERSION=1.0.0"},
			want: &Config{
				BuildArgs: map[string]string{
					"VERSION": "1.0.0",
				},
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "build-arg multiple",
			args: []string{
				"--build-arg", "VERSION=1.0.0",
				"--build-arg", "ENV=production",
			},
			want: &Config{
				BuildArgs: map[string]string{
					"VERSION": "1.0.0",
					"ENV":     "production",
				},
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "build-arg with equals format",
			args: []string{"--build-arg=APP_NAME=myapp"},
			want: &Config{
				BuildArgs: map[string]string{
					"APP_NAME": "myapp",
				},
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "label single",
			args: []string{"--label", "version=1.0.0"},
			want: &Config{
				BuildArgs: make(map[string]string),
				Labels: map[string]string{
					"version": "1.0.0",
				},
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "label multiple",
			args: []string{
				"--label", "version=1.0.0",
				"--label", "maintainer=team@example.com",
			},
			want: &Config{
				BuildArgs: make(map[string]string),
				Labels: map[string]string{
					"version":    "1.0.0",
					"maintainer": "team@example.com",
				},
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "attest sbom",
			args: []string{"--attest", "type=sbom,generator=custom:v1"},
			want: &Config{
				AttestationConfigs: []AttestationConfig{
					{
						Type: "sbom",
						Params: map[string]string{
							"generator": "custom:v1",
						},
					},
				},
				BuildArgs:         make(map[string]string),
				Labels:            make(map[string]string),
				Verbosity:         "info",
				InsecureRegistry:  []string{},
				Destination:       []string{},
				BuildKitOpts:      []string{},
				CosignKeyPath:     "/etc/cosign/cosign.key",
				CosignPasswordEnv: "COSIGN_PASSWORD",
			},
		},
		{
			name: "attest provenance",
			args: []string{"--attest", "type=provenance,mode=max"},
			want: &Config{
				AttestationConfigs: []AttestationConfig{
					{
						Type: "provenance",
						Params: map[string]string{
							"mode": "max",
						},
					},
				},
				BuildArgs:         make(map[string]string),
				Labels:            make(map[string]string),
				Verbosity:         "info",
				InsecureRegistry:  []string{},
				Destination:       []string{},
				BuildKitOpts:      []string{},
				CosignKeyPath:     "/etc/cosign/cosign.key",
				CosignPasswordEnv: "COSIGN_PASSWORD",
			},
		},
		{
			name: "multiple attest",
			args: []string{
				"--attest", "type=sbom,generator=syft",
				"--attest", "type=provenance,mode=min",
			},
			want: &Config{
				AttestationConfigs: []AttestationConfig{
					{
						Type: "sbom",
						Params: map[string]string{
							"generator": "syft",
						},
					},
					{
						Type: "provenance",
						Params: map[string]string{
							"mode": "min",
						},
					},
				},
				BuildArgs:         make(map[string]string),
				Labels:            make(map[string]string),
				Verbosity:         "info",
				InsecureRegistry:  []string{},
				Destination:       []string{},
				BuildKitOpts:      []string{},
				CosignKeyPath:     "/etc/cosign/cosign.key",
				CosignPasswordEnv: "COSIGN_PASSWORD",
			},
		},
		{
			name: "buildkit-opt single",
			args: []string{"--buildkit-opt", "network=host"},
			want: &Config{
				BuildKitOpts:       []string{"network=host"},
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "buildkit-opt multiple",
			args: []string{
				"--buildkit-opt", "network=host",
				"--buildkit-opt", "security=insecure",
			},
			want: &Config{
				BuildKitOpts:       []string{"network=host", "security=insecure"},
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "push-retry",
			args: []string{"--push-retry", "5"},
			want: &Config{
				PushRetry:          5,
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "push-retry with equals",
			args: []string{"--push-retry=3"},
			want: &Config{
				PushRetry:          3,
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "image-download-retry",
			args: []string{"--image-download-retry", "10"},
			want: &Config{
				ImageDownloadRetry: 10,
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "insecure-registry single",
			args: []string{"--insecure-registry", "registry.local:5000"},
			want: &Config{
				InsecureRegistry:   []string{"registry.local:5000"},
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "insecure-registry multiple",
			args: []string{
				"--insecure-registry", "registry1.local:5000",
				"--insecure-registry", "registry2.local:5001",
			},
			want: &Config{
				InsecureRegistry: []string{
					"registry1.local:5000",
					"registry2.local:5001",
				},
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "tar-path",
			args: []string{"--tar-path", "/tmp/image.tar"},
			want: &Config{
				TarPath:            "/tmp/image.tar",
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "digest-file",
			args: []string{"--digest-file", "/tmp/digest.txt"},
			want: &Config{
				DigestFile:         "/tmp/digest.txt",
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "storage-driver",
			args: []string{"--storage-driver", "overlay2"},
			want: &Config{
				StorageDriver:      "overlay2",
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "custom-platform",
			args: []string{"--custom-platform", "linux/arm64"},
			want: &Config{
				CustomPlatform:     "linux/arm64",
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "target",
			args: []string{"--target", "builder"},
			want: &Config{
				Target:             "builder",
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "git flags",
			args: []string{
				"--git-branch", "main",
				"--git-revision", "abc123",
			},
			want: &Config{
				GitBranch:          "main",
				GitRevision:        "abc123",
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "insecure-pull",
			args: []string{"--insecure-pull"},
			want: &Config{
				InsecurePull:       true,
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "log-timestamp",
			args: []string{"--log-timestamp"},
			want: &Config{
				LogTimestamp:       true,
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "scan flag",
			args: []string{"--scan"},
			want: &Config{
				Scan:               true,
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "harden flag",
			args: []string{"--harden"},
			want: &Config{
				Harden:             true,
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "cosign-key custom path",
			args: []string{"--cosign-key", "/path/to/key"},
			want: &Config{
				CosignKeyPath:      "/path/to/key",
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},
		{
			name: "cosign-password-env custom",
			args: []string{"--cosign-password-env", "MY_COSIGN_PASS"},
			want: &Config{
				CosignPasswordEnv:  "MY_COSIGN_PASS",
				BuildArgs:          make(map[string]string),
				Labels:             make(map[string]string),
				Verbosity:          "info",
				InsecureRegistry:   []string{},
				Destination:        []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
			},
		},
		{
			name: "complex configuration",
			args: []string{
				"--dockerfile", "Dockerfile",
				"--context", "/app",
				"--destination", "gcr.io/project/image:v1",
				"--cache", "true",
				"--no-push",
				"--verbosity", "debug",
				"--build-arg", "VERSION=1.0",
				"--label", "env=prod",
				"--push-retry", "3",
			},
			want: &Config{
				Dockerfile:  "Dockerfile",
				Context:     "/app",
				Destination: []string{"gcr.io/project/image:v1"},
				Cache:       true,
				NoPush:      true,
				Verbosity:   "debug",
				PushRetry:   3,
				BuildArgs: map[string]string{
					"VERSION": "1.0",
				},
				Labels: map[string]string{
					"env": "prod",
				},
				InsecureRegistry:   []string{},
				AttestationConfigs: []AttestationConfig{},
				BuildKitOpts:       []string{},
				CosignKeyPath:      "/etc/cosign/cosign.key",
				CosignPasswordEnv:  "COSIGN_PASSWORD",
			},
		},

		// Invalid cases
		{
			name:      "invalid attestation mode",
			args:      []string{"--attestation", "invalid"},
			wantFatal: true,
		},
		{
			name:      "sign without attestation",
			args:      []string{"--sign"},
			wantFatal: true,
		},
		{
			name:      "attest without value",
			args:      []string{"--attest"},
			wantFatal: true,
		},
		{
			name:      "attest with invalid format",
			args:      []string{"--attest", "invalid"},
			wantFatal: true,
		},
		{
			name:      "attest with missing type",
			args:      []string{"--attest", "generator=custom"},
			wantFatal: true,
		},
		{
			name:      "buildkit-opt without value",
			args:      []string{"--buildkit-opt"},
			wantFatal: true,
		},
		{
			name:      "cosign-key without value",
			args:      []string{"--cosign-key"},
			wantFatal: true,
		},
		{
			name:      "cosign-password-env without value",
			args:      []string{"--cosign-password-env"},
			wantFatal: true,
		},
		{
			name:      "push-retry invalid value",
			args:      []string{"--push-retry", "invalid"},
			wantFatal: true,
		},
		{
			name:      "image-download-retry invalid value",
			args:      []string{"--image-download-retry", "not-a-number"},
			wantFatal: true,
		},

		// Exit cases (--help, --version, no args)
		{
			name:     "help flag",
			args:     []string{"--help"},
			wantExit: true,
		},
		{
			name:     "version flag",
			args:     []string{"--version"},
			wantExit: true,
		},
		{
			name:     "no arguments shows help",
			args:     []string{},
			wantExit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment variables
			if len(tt.setupEnv) > 0 {
				for key, value := range tt.setupEnv {
					oldValue := os.Getenv(key)
					os.Setenv(key, value)
					defer os.Setenv(key, oldValue)
				}
			}

			if tt.wantFatal {
				// Test cases that should call logger.Fatal
				logger.SetExitFunc(func(code int) {
					panic("Fatal called")
				})
				defer logger.ResetExitFunc()

				defer func() {
					if r := recover(); r == nil {
						t.Errorf("parseArgs(%v) should have called logger.Fatal", tt.args)
					}
				}()

				parseArgs(tt.args)
				t.Errorf("parseArgs(%v) should have called logger.Fatal", tt.args)

			} else if tt.wantExit {
				// Test cases that should exit (--help, --version, no args)
				logger.SetExitFunc(func(code int) {
					if code != 0 {
						t.Errorf("Expected exit code 0, got %d", code)
					}
					panic("Exit called")
				})
				defer logger.ResetExitFunc()

				defer func() {
					if r := recover(); r == nil {
						t.Errorf("parseArgs(%v) should have called os.Exit", tt.args)
					}
				}()

				parseArgs(tt.args)

			} else {
				// Test valid cases
				got := parseArgs(tt.args)

				// Compare fields
				if got.Dockerfile != tt.want.Dockerfile {
					t.Errorf("Dockerfile = %q; want %q", got.Dockerfile, tt.want.Dockerfile)
				}
				if got.Context != tt.want.Context {
					t.Errorf("Context = %q; want %q", got.Context, tt.want.Context)
				}
				if got.SubContext != tt.want.SubContext {
					t.Errorf("SubContext = %q; want %q", got.SubContext, tt.want.SubContext)
				}
				if !reflect.DeepEqual(got.Destination, tt.want.Destination) {
					t.Errorf("Destination = %v; want %v", got.Destination, tt.want.Destination)
				}
				if got.Cache != tt.want.Cache {
					t.Errorf("Cache = %v; want %v", got.Cache, tt.want.Cache)
				}
				if got.CacheDir != tt.want.CacheDir {
					t.Errorf("CacheDir = %q; want %q", got.CacheDir, tt.want.CacheDir)
				}
				if got.NoPush != tt.want.NoPush {
					t.Errorf("NoPush = %v; want %v", got.NoPush, tt.want.NoPush)
				}
				if got.Insecure != tt.want.Insecure {
					t.Errorf("Insecure = %v; want %v", got.Insecure, tt.want.Insecure)
				}
				if got.Verbosity != tt.want.Verbosity {
					t.Errorf("Verbosity = %q; want %q", got.Verbosity, tt.want.Verbosity)
				}
				if got.Reproducible != tt.want.Reproducible {
					t.Errorf("Reproducible = %v; want %v", got.Reproducible, tt.want.Reproducible)
				}
				if got.Timestamp != tt.want.Timestamp {
					t.Errorf("Timestamp = %q; want %q", got.Timestamp, tt.want.Timestamp)
				}
				if got.Attestation != tt.want.Attestation {
					t.Errorf("Attestation = %q; want %q", got.Attestation, tt.want.Attestation)
				}
				if got.Sign != tt.want.Sign {
					t.Errorf("Sign = %v; want %v", got.Sign, tt.want.Sign)
				}
				if got.CosignKeyPath != tt.want.CosignKeyPath {
					t.Errorf("CosignKeyPath = %q; want %q", got.CosignKeyPath, tt.want.CosignKeyPath)
				}
				if got.CosignPasswordEnv != tt.want.CosignPasswordEnv {
					t.Errorf("CosignPasswordEnv = %q; want %q", got.CosignPasswordEnv, tt.want.CosignPasswordEnv)
				}
				if !reflect.DeepEqual(got.BuildArgs, tt.want.BuildArgs) {
					t.Errorf("BuildArgs = %v; want %v", got.BuildArgs, tt.want.BuildArgs)
				}
				if !reflect.DeepEqual(got.Labels, tt.want.Labels) {
					t.Errorf("Labels = %v; want %v", got.Labels, tt.want.Labels)
				}
				if !reflect.DeepEqual(got.InsecureRegistry, tt.want.InsecureRegistry) {
					t.Errorf("InsecureRegistry = %v; want %v", got.InsecureRegistry, tt.want.InsecureRegistry)
				}
				if !reflect.DeepEqual(got.BuildKitOpts, tt.want.BuildKitOpts) {
					t.Errorf("BuildKitOpts = %v; want %v", got.BuildKitOpts, tt.want.BuildKitOpts)
				}
				if got.PushRetry != tt.want.PushRetry {
					t.Errorf("PushRetry = %d; want %d", got.PushRetry, tt.want.PushRetry)
				}
				if got.ImageDownloadRetry != tt.want.ImageDownloadRetry {
					t.Errorf("ImageDownloadRetry = %d; want %d", got.ImageDownloadRetry, tt.want.ImageDownloadRetry)
				}
				if got.TarPath != tt.want.TarPath {
					t.Errorf("TarPath = %q; want %q", got.TarPath, tt.want.TarPath)
				}
				if got.DigestFile != tt.want.DigestFile {
					t.Errorf("DigestFile = %q; want %q", got.DigestFile, tt.want.DigestFile)
				}
				if got.StorageDriver != tt.want.StorageDriver {
					t.Errorf("StorageDriver = %q; want %q", got.StorageDriver, tt.want.StorageDriver)
				}
				if got.CustomPlatform != tt.want.CustomPlatform {
					t.Errorf("CustomPlatform = %q; want %q", got.CustomPlatform, tt.want.CustomPlatform)
				}
				if got.Target != tt.want.Target {
					t.Errorf("Target = %q; want %q", got.Target, tt.want.Target)
				}
				if got.GitBranch != tt.want.GitBranch {
					t.Errorf("GitBranch = %q; want %q", got.GitBranch, tt.want.GitBranch)
				}
				if got.GitRevision != tt.want.GitRevision {
					t.Errorf("GitRevision = %q; want %q", got.GitRevision, tt.want.GitRevision)
				}
				if got.InsecurePull != tt.want.InsecurePull {
					t.Errorf("InsecurePull = %v; want %v", got.InsecurePull, tt.want.InsecurePull)
				}
				if got.LogTimestamp != tt.want.LogTimestamp {
					t.Errorf("LogTimestamp = %v; want %v", got.LogTimestamp, tt.want.LogTimestamp)
				}
				if got.Scan != tt.want.Scan {
					t.Errorf("Scan = %v; want %v", got.Scan, tt.want.Scan)
				}
				if got.Harden != tt.want.Harden {
					t.Errorf("Harden = %v; want %v", got.Harden, tt.want.Harden)
				}

				// Check AttestationConfigs
				if len(got.AttestationConfigs) != len(tt.want.AttestationConfigs) {
					t.Errorf("AttestationConfigs length = %d; want %d",
						len(got.AttestationConfigs), len(tt.want.AttestationConfigs))
				} else {
					for i := range got.AttestationConfigs {
						if got.AttestationConfigs[i].Type != tt.want.AttestationConfigs[i].Type {
							t.Errorf("AttestationConfigs[%d].Type = %q; want %q",
								i, got.AttestationConfigs[i].Type, tt.want.AttestationConfigs[i].Type)
						}
						if !reflect.DeepEqual(got.AttestationConfigs[i].Params, tt.want.AttestationConfigs[i].Params) {
							t.Errorf("AttestationConfigs[%d].Params = %v; want %v",
								i, got.AttestationConfigs[i].Params, tt.want.AttestationConfigs[i].Params)
						}
					}
				}
			}
		})
	}
}

func TestParseBool(t *testing.T) {
	// Test valid inputs
	t.Run("valid inputs", func(t *testing.T) {
		tests := []struct {
			name  string
			input string
			want  bool
		}{
			// true-ish values
			{"lower true", "true", true},
			{"upper TRUE", "TRUE", true},
			{"mixed True", "TrUe", true},
			{"yes", "yes", true},
			{"YES", "YES", true},
			{"1", "1", true},
			{"on", "on", true},
			{"ON", "ON", true},

			// false-ish values
			{"lower false", "false", false},
			{"upper FALSE", "FALSE", false},
			{"mixed False", "FaLsE", false},
			{"no", "no", false},
			{"NO", "NO", false},
			{"0", "0", false},
			{"off", "off", false},
			{"OFF", "OFF", false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := parseBool(tt.input)
				if got != tt.want {
					t.Errorf("parseBool(%q) = %v; want %v", tt.input, got, tt.want)
				}
			})
		}
	})

	// Test invalid inputs
	t.Run("invalid inputs", func(t *testing.T) {
		invalidInputs := []string{
			"maybe",
			"",
			"invalid",
			"2",
			"yep",
			"nope",
			"123",
			"abc",
		}

		for _, input := range invalidInputs {
			t.Run(input, func(t *testing.T) {
				// Mock logger.Fatal to panic instead of exit
				logger.SetExitFunc(func(code int) {
					panic("Fatal called")
				})
				defer logger.ResetExitFunc()

				// Expect panic
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("parseBool(%q) should have called logger.Fatal", input)
					}
				}()

				// This should call Fatal and panic
				parseBool(input)
			})
		}
	})
}

func TestParseInt(t *testing.T) {
	// Test valid inputs
	t.Run("valid inputs", func(t *testing.T) {
		tests := []struct {
			input string
			want  int
		}{
			{"0", 0},
			{"42", 42},
			{"-13", -13},
			{"123456", 123456},
		}

		for _, tt := range tests {
			got := parseInt(tt.input)
			if got != tt.want {
				t.Errorf("parseInt(%q) = %v; want %v", tt.input, got, tt.want)
			}
		}
	})

	// Test invalid inputs
	t.Run("invalid inputs", func(t *testing.T) {
		invalidInputs := []string{"1.5", "abc", "", "12.34", "not-a-number"}

		for _, input := range invalidInputs {
			t.Run(input, func(t *testing.T) {
				// Mock exit
				logger.SetExitFunc(func(code int) {
					panic("Fatal called")
				})
				defer logger.ResetExitFunc()

				// Expect panic
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("parseInt(%q) should have called Fatal", input)
					}
				}()

				parseInt(input)
			})
		}
	})
}

func TestParseBuildArg(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		wantKey string
		wantVal string
	}{
		{
			name:    "key with value",
			arg:     "label=abc",
			wantKey: "label",
			wantVal: "abc",
		},
		{
			name:    "key only no value",
			arg:     "FOO",
			wantKey: "FOO",
			wantVal: "",
		},
		{
			name:    "value with equals inside",
			arg:     "FOO=bar=baz",
			wantKey: "FOO",
			wantVal: "bar=baz", // because SplitN(..., 2)
		},
		{
			name:    "empty value after equals",
			arg:     "FOO=",
			wantKey: "FOO",
			wantVal: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{BuildArgs: make(map[string]string)}

			parseBuildArg(tt.arg, cfg)

			got, ok := cfg.BuildArgs[tt.wantKey]
			if !ok {
				t.Fatalf("expected key %q to be present in BuildArgs", tt.wantKey)
			}
			if got != tt.wantVal {
				t.Fatalf("for arg %q: BuildArgs[%q] = %q, want %q",
					tt.arg, tt.wantKey, got, tt.wantVal)
			}
		})
	}
}

func TestParseLabel(t *testing.T) {
	tests := []struct {
		name    string
		label   string
		wantKey string
		wantVal string
	}{
		{
			name:    "key with value",
			label:   "label=abc",
			wantKey: "label",
			wantVal: "abc",
		},
		// {
		// 	name:    "key only no value",
		// 	label:   "FOO",
		// 	wantKey: "FOO",
		// 	wantVal: "",
		// },
		{
			name:    "value with equals inside",
			label:   "FOO=bar=baz",
			wantKey: "FOO",
			wantVal: "bar=baz", // because SplitN(..., 2)
		},
		{
			name:    "empty value after equals",
			label:   "FOO=",
			wantKey: "FOO",
			wantVal: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{BuildArgs: make(map[string]string)}

			parseBuildArg(tt.label, cfg)

			got, ok := cfg.BuildArgs[tt.wantKey]
			if !ok {
				t.Fatalf("expected key %q to be present in BuildArgs", tt.wantKey)
			}
			if got != tt.wantVal {
				t.Fatalf("for arg %q: BuildArgs[%q] = %q, want %q",
					tt.label, tt.wantKey, got, tt.wantVal)
			}
		})
	}
}

func TestParseAttestationConfig(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantType   string
		wantParams map[string]string
		wantFatal  bool   // true if this should call logger.Fatal
		wantErrMsg string // expected error message substring (only checked if wantFatal=true)
	}{
		// Valid inputs
		{
			name:       "sbom type only",
			input:      "type=sbom",
			wantType:   "sbom",
			wantParams: map[string]string{},
			wantFatal:  false,
		},
		{
			name:       "provenance type only",
			input:      "type=provenance",
			wantType:   "provenance",
			wantParams: map[string]string{},
			wantFatal:  false,
		},
		{
			name:     "sbom with single parameter",
			input:    "type=sbom,generator=custom:v1",
			wantType: "sbom",
			wantParams: map[string]string{
				"generator": "custom:v1",
			},
			wantFatal: false,
		},
		{
			name:     "sbom with multiple parameters",
			input:    "type=sbom,generator=custom:v1,scan-stage=true",
			wantType: "sbom",
			wantParams: map[string]string{
				"generator":  "custom:v1",
				"scan-stage": "true",
			},
			wantFatal: false,
		},
		{
			name:     "provenance with parameters",
			input:    "type=provenance,builder=kaniko,mode=max",
			wantType: "provenance",
			wantParams: map[string]string{
				"builder": "kaniko",
				"mode":    "max",
			},
			wantFatal: false,
		},
		{
			name:     "parameters with spaces (should be trimmed)",
			input:    "type=sbom, generator=custom:v1 , scan-stage=true ",
			wantType: "sbom",
			wantParams: map[string]string{
				"generator":  "custom:v1",
				"scan-stage": "true",
			},
			wantFatal: false,
		},
		{
			name:     "type at end",
			input:    "generator=custom:v1,scan-stage=true,type=sbom",
			wantType: "sbom",
			wantParams: map[string]string{
				"generator":  "custom:v1",
				"scan-stage": "true",
			},
			wantFatal: false,
		},
		{
			name:     "parameters with equals in value",
			input:    "type=sbom,url=https://example.com?key=value",
			wantType: "sbom",
			wantParams: map[string]string{
				"url": "https://example.com?key=value",
			},
			wantFatal: false,
		},
		{
			name:     "complex parameter values",
			input:    "type=provenance,image=gcr.io/my-project/my-image:v1.0.0",
			wantType: "provenance",
			wantParams: map[string]string{
				"image": "gcr.io/my-project/my-image:v1.0.0",
			},
			wantFatal: false,
		},
		{
			name:     "boolean-like parameters",
			input:    "type=sbom,enabled=true,disabled=false,verbose=1",
			wantType: "sbom",
			wantParams: map[string]string{
				"enabled":  "true",
				"disabled": "false",
				"verbose":  "1",
			},
			wantFatal: false,
		},

		// Invalid inputs
		{
			name:       "missing type",
			input:      "generator=custom:v1",
			wantFatal:  true,
			wantErrMsg: "--attest must include",
		},
		{
			name:       "invalid type",
			input:      "type=invalid",
			wantFatal:  true,
			wantErrMsg: "type must be 'sbom' or 'provenance'",
		},
		{
			name:       "malformed parameter",
			input:      "type=sbom,badparam",
			wantFatal:  true,
			wantErrMsg: "Invalid attestation parameter",
		},
		{
			name:       "empty type value",
			input:      "type=",
			wantFatal:  true,
			wantErrMsg: "[FATAL] --attest must include 'type=sbom' or 'type=provenance'\n",
		},
		{
			name:       "only parameters no type",
			input:      "generator=v1,builder=v2",
			wantFatal:  true,
			wantErrMsg: "--attest must include",
		},
		{
			name:       "empty parameter in middle",
			input:      "type=sbom,,generator=custom",
			wantFatal:  true,
			wantErrMsg: "Invalid attestation parameter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantFatal {
				// Test invalid inputs that should call logger.Fatal
				expectFatal(t, func() {
					parseAttestationConfig(tt.input)
				}, tt.wantErrMsg)
			} else {
				// Test valid inputs
				config := parseAttestationConfig(tt.input)

				// Check type
				if config.Type != tt.wantType {
					t.Errorf("Type = %q; want %q", config.Type, tt.wantType)
				}

				// Check params map length
				if len(config.Params) != len(tt.wantParams) {
					t.Errorf("Params length = %d; want %d",
						len(config.Params), len(tt.wantParams))
				}

				// Check each parameter
				for key, wantValue := range tt.wantParams {
					gotValue, exists := config.Params[key]
					if !exists {
						t.Errorf("Parameter %q not found in Params", key)
						continue
					}
					if gotValue != wantValue {
						t.Errorf("Params[%q] = %q; want %q", key, gotValue, wantValue)
					}
				}

				// Check no extra parameters
				for key := range config.Params {
					if _, exists := tt.wantParams[key]; !exists {
						t.Errorf("Unexpected parameter %q = %q", key, config.Params[key])
					}
				}
			}
		})
	}
}
