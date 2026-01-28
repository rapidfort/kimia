package preflight

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ===== TESTS FOR CheckCapabilities() FUNCTION =====

func TestCheckCapabilities(t *testing.T) {
	// This test runs against the real /proc/self/status
	// It should always pass as the test process has some capabilities
	t.Run("real system check", func(t *testing.T) {
		result, err := CheckCapabilities()
		if err != nil {
			t.Fatalf("CheckCapabilities() failed: %v", err)
		}

		if result == nil {
			t.Fatal("CheckCapabilities() returned nil result")
		}

		// Verify structure is populated
		if result.EffectiveCaps == 0 {
			t.Error("EffectiveCaps should not be 0")
		}

		if len(result.Capabilities) != 4 {
			t.Errorf("Expected 4 capabilities, got %d", len(result.Capabilities))
		}

		// Log current capabilities for debugging
		t.Logf("Current capabilities: 0x%016x", result.EffectiveCaps)
		t.Logf("HasSetUID: %v", result.HasSetUID)
		t.Logf("HasSetGID: %v", result.HasSetGID)
		t.Logf("HasDACOverride: %v", result.HasDACOverride)
	})
}

// ===== TESTS FOR HasRequiredCapabilities() FUNCTION =====

func TestHasRequiredCapabilities(t *testing.T) {
	tests := []struct {
		name       string
		check      *CapabilityCheck
		wantResult bool
	}{
		{
			name: "has all required capabilities",
			check: &CapabilityCheck{
				HasSetUID: true,
				HasSetGID: true,
			},
			wantResult: true,
		},
		{
			name: "missing SETUID",
			check: &CapabilityCheck{
				HasSetUID: false,
				HasSetGID: true,
			},
			wantResult: false,
		},
		{
			name: "missing SETGID",
			check: &CapabilityCheck{
				HasSetUID: true,
				HasSetGID: false,
			},
			wantResult: false,
		},
		{
			name: "missing both",
			check: &CapabilityCheck{
				HasSetUID: false,
				HasSetGID: false,
			},
			wantResult: false,
		},
		{
			name: "has extra capabilities (DAC_OVERRIDE)",
			check: &CapabilityCheck{
				HasSetUID:      true,
				HasSetGID:      true,
				HasDACOverride: true,
			},
			wantResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.check.HasRequiredCapabilities()
			if got != tt.wantResult {
				t.Errorf("HasRequiredCapabilities() = %v; want %v", got, tt.wantResult)
			}
		})
	}
}

// ===== TESTS FOR HasCapability() FUNCTION =====

func TestHasCapability(t *testing.T) {
	// Create test capability check with known values
	check := &CapabilityCheck{
		HasSetUID:      true,
		HasSetGID:      false,
		HasDACOverride: true,
		EffectiveCaps:  (1 << CAP_SETUID) | (1 << CAP_DAC_OVERRIDE) | (1 << CAP_MKNOD),
		Capabilities: []Capability{
			{Name: "CAP_DAC_OVERRIDE", Bit: CAP_DAC_OVERRIDE, Present: true},
			{Name: "CAP_SETUID", Bit: CAP_SETUID, Present: true},
			{Name: "CAP_SETGID", Bit: CAP_SETGID, Present: false},
			{Name: "CAP_MKNOD", Bit: CAP_MKNOD, Present: true},
		},
	}

	tests := []struct {
		name     string
		capName  string
		want     bool
		testDesc string
	}{
		{
			name:     "CAP_SETUID present",
			capName:  "CAP_SETUID",
			want:     true,
			testDesc: "With CAP_ prefix",
		},
		{
			name:     "SETUID present",
			capName:  "SETUID",
			want:     true,
			testDesc: "Without CAP_ prefix",
		},
		{
			name:     "setuid lowercase",
			capName:  "setuid",
			want:     true,
			testDesc: "Lowercase without prefix",
		},
		{
			name:     "cap_setuid lowercase",
			capName:  "cap_setuid",
			want:     true,
			testDesc: "Lowercase with prefix",
		},
		{
			name:     "CAP_SETGID not present",
			capName:  "CAP_SETGID",
			want:     false,
			testDesc: "Missing capability",
		},
		{
			name:     "CAP_DAC_OVERRIDE present",
			capName:  "CAP_DAC_OVERRIDE",
			want:     true,
			testDesc: "DAC_OVERRIDE capability",
		},
		{
			name:     "CAP_MKNOD present",
			capName:  "CAP_MKNOD",
			want:     true,
			testDesc: "MKNOD capability",
		},
		{
			name:     "unknown capability",
			capName:  "CAP_UNKNOWN",
			want:     false,
			testDesc: "Non-existent capability",
		},
		{
			name:     "empty string",
			capName:  "",
			want:     false,
			testDesc: "Empty capability name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := check.HasCapability(tt.capName)
			if got != tt.want {
				t.Errorf("HasCapability(%q) = %v; want %v (%s)",
					tt.capName, got, tt.want, tt.testDesc)
			}
		})
	}
}

// ===== TESTS FOR GetMissingCapabilities() FUNCTION =====

func TestGetMissingCapabilities(t *testing.T) {
	tests := []struct {
		name        string
		check       *CapabilityCheck
		wantMissing []string
		wantCount   int
	}{
		{
			name: "no missing capabilities",
			check: &CapabilityCheck{
				HasSetUID: true,
				HasSetGID: true,
			},
			wantMissing: []string{},
			wantCount:   0,
		},
		{
			name: "missing SETUID only",
			check: &CapabilityCheck{
				HasSetUID: false,
				HasSetGID: true,
			},
			wantMissing: []string{"CAP_SETUID"},
			wantCount:   1,
		},
		{
			name: "missing SETGID only",
			check: &CapabilityCheck{
				HasSetUID: true,
				HasSetGID: false,
			},
			wantMissing: []string{"CAP_SETGID"},
			wantCount:   1,
		},
		{
			name: "missing both SETUID and SETGID",
			check: &CapabilityCheck{
				HasSetUID: false,
				HasSetGID: false,
			},
			wantMissing: []string{"CAP_SETUID", "CAP_SETGID"},
			wantCount:   2,
		},
		{
			name: "has extra capabilities but none missing",
			check: &CapabilityCheck{
				HasSetUID:      true,
				HasSetGID:      true,
				HasDACOverride: true,
			},
			wantMissing: []string{},
			wantCount:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.check.GetMissingCapabilities()

			if len(got) != tt.wantCount {
				t.Errorf("GetMissingCapabilities() count = %d; want %d (got: %v)",
					len(got), tt.wantCount, got)
				return
			}

			// Check that all expected missing capabilities are present
			for _, expectedCap := range tt.wantMissing {
				found := false
				for _, gotCap := range got {
					if gotCap == expectedCap {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected missing capability %q not found in result: %v",
						expectedCap, got)
				}
			}
		})
	}
}

// ===== TESTS FOR GetMissingCapabilitiesForStorage() FUNCTION =====

func TestGetMissingCapabilitiesForStorage(t *testing.T) {
	tests := []struct {
		name          string
		check         *CapabilityCheck
		storageDriver string
		wantMissing   []string
		wantCount     int
	}{
		{
			name: "overlay driver with all capabilities",
			check: &CapabilityCheck{
				HasSetUID:     true,
				HasSetGID:     true,
				EffectiveCaps: (1 << CAP_SETUID) | (1 << CAP_SETGID) | (1 << CAP_MKNOD),
				Capabilities: []Capability{
					{Name: "CAP_SETUID", Bit: CAP_SETUID, Present: true},
					{Name: "CAP_SETGID", Bit: CAP_SETGID, Present: true},
					{Name: "CAP_MKNOD", Bit: CAP_MKNOD, Present: true},
				},
			},
			storageDriver: "overlay",
			wantMissing:   []string{},
			wantCount:     0,
		},
		{
			name: "overlay driver missing MKNOD",
			check: &CapabilityCheck{
				HasSetUID:     true,
				HasSetGID:     true,
				EffectiveCaps: (1 << CAP_SETUID) | (1 << CAP_SETGID),
				Capabilities: []Capability{
					{Name: "CAP_SETUID", Bit: CAP_SETUID, Present: true},
					{Name: "CAP_SETGID", Bit: CAP_SETGID, Present: true},
					{Name: "CAP_MKNOD", Bit: CAP_MKNOD, Present: false},
				},
			},
			storageDriver: "overlay",
			wantMissing:   []string{"CAP_MKNOD"},
			wantCount:     1,
		},
		{
			name: "vfs driver doesn't need MKNOD",
			check: &CapabilityCheck{
				HasSetUID:     true,
				HasSetGID:     true,
				EffectiveCaps: (1 << CAP_SETUID) | (1 << CAP_SETGID),
				Capabilities: []Capability{
					{Name: "CAP_SETUID", Bit: CAP_SETUID, Present: true},
					{Name: "CAP_SETGID", Bit: CAP_SETGID, Present: true},
					{Name: "CAP_MKNOD", Bit: CAP_MKNOD, Present: false},
				},
			},
			storageDriver: "vfs",
			wantMissing:   []string{},
			wantCount:     0,
		},
		{
			name: "overlay driver missing SETUID and MKNOD",
			check: &CapabilityCheck{
				HasSetUID:     false,
				HasSetGID:     true,
				EffectiveCaps: (1 << CAP_SETGID),
				Capabilities: []Capability{
					{Name: "CAP_SETUID", Bit: CAP_SETUID, Present: false},
					{Name: "CAP_SETGID", Bit: CAP_SETGID, Present: true},
					{Name: "CAP_MKNOD", Bit: CAP_MKNOD, Present: false},
				},
			},
			storageDriver: "overlay",
			wantMissing:   []string{"CAP_SETUID", "CAP_MKNOD"},
			wantCount:     2,
		},
		{
			name: "overlay driver missing all",
			check: &CapabilityCheck{
				HasSetUID:     false,
				HasSetGID:     false,
				EffectiveCaps: 0,
				Capabilities: []Capability{
					{Name: "CAP_SETUID", Bit: CAP_SETUID, Present: false},
					{Name: "CAP_SETGID", Bit: CAP_SETGID, Present: false},
					{Name: "CAP_MKNOD", Bit: CAP_MKNOD, Present: false},
				},
			},
			storageDriver: "overlay",
			wantMissing:   []string{"CAP_SETUID", "CAP_SETGID", "CAP_MKNOD"},
			wantCount:     3,
		},
		{
			name: "empty storage driver",
			check: &CapabilityCheck{
				HasSetUID:     true,
				HasSetGID:     true,
				EffectiveCaps: (1 << CAP_SETUID) | (1 << CAP_SETGID),
			},
			storageDriver: "",
			wantMissing:   []string{},
			wantCount:     0,
		},
		{
			name: "unknown storage driver",
			check: &CapabilityCheck{
				HasSetUID:     true,
				HasSetGID:     true,
				EffectiveCaps: (1 << CAP_SETUID) | (1 << CAP_SETGID),
			},
			storageDriver: "unknown",
			wantMissing:   []string{},
			wantCount:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.check.GetMissingCapabilitiesForStorage(tt.storageDriver)

			if len(got) != tt.wantCount {
				t.Errorf("GetMissingCapabilitiesForStorage(%q) count = %d; want %d (got: %v)",
					tt.storageDriver, len(got), tt.wantCount, got)
				return
			}

			// Check that all expected missing capabilities are present
			for _, expectedCap := range tt.wantMissing {
				found := false
				for _, gotCap := range got {
					if gotCap == expectedCap {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected missing capability %q not found in result: %v",
						expectedCap, got)
				}
			}
		})
	}
}

// ===== TESTS FOR FormatCapabilities() FUNCTION =====

func TestFormatCapabilities(t *testing.T) {
	tests := []struct {
		name string
		caps uint64
		want string
	}{
		{
			name: "all zeros",
			caps: 0x0000000000000000,
			want: "0x0000000000000000",
		},
		{
			name: "single capability (SETUID)",
			caps: 1 << CAP_SETUID, // bit 7
			want: "0x0000000000000080",
		},
		{
			name: "single capability (SETGID)",
			caps: 1 << CAP_SETGID, // bit 6
			want: "0x0000000000000040",
		},
		{
			name: "both SETUID and SETGID",
			caps: (1 << CAP_SETUID) | (1 << CAP_SETGID),
			want: "0x00000000000000c0",
		},
		{
			name: "all tracked capabilities",
			caps: (1 << CAP_DAC_OVERRIDE) | (1 << CAP_SETGID) | (1 << CAP_SETUID) | (1 << CAP_MKNOD),
			want: "0x00000000080000c2", // bits 1, 6, 7, 27 = 0x2 | 0x40 | 0x80 | 0x8000000
		},
		{
			name: "all bits set",
			caps: 0xffffffffffffffff,
			want: "0xffffffffffffffff",
		},
		{
			name: "typical root capabilities",
			caps: 0x0000003fffffffff, // Common for root
			want: "0x0000003fffffffff",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := &CapabilityCheck{
				EffectiveCaps: tt.caps,
			}

			got := check.FormatCapabilities()
			if got != tt.want {
				t.Errorf("FormatCapabilities() = %s; want %s", got, tt.want)
			}
		})
	}
}

// ===== TESTS FOR Capability Constants =====

func TestCapabilityConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant uint
		want     uint
	}{
		{"CAP_DAC_OVERRIDE", CAP_DAC_OVERRIDE, 1},
		{"CAP_SETGID", CAP_SETGID, 6},
		{"CAP_SETUID", CAP_SETUID, 7},
		{"CAP_MKNOD", CAP_MKNOD, 27},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.want {
				t.Errorf("%s = %d; want %d", tt.name, tt.constant, tt.want)
			}
		})
	}
}

// ===== TESTS FOR Capability Bit Manipulation =====

func TestCapabilityBitManipulation(t *testing.T) {
	t.Run("verify bit positions", func(t *testing.T) {
		// Test that capability bits are correctly positioned
		dacOverride := uint64(1 << CAP_DAC_OVERRIDE)
		setgid := uint64(1 << CAP_SETGID)
		setuid := uint64(1 << CAP_SETUID)
		mknod := uint64(1 << CAP_MKNOD)

		if dacOverride != 0x2 { // bit 1 = 2^1
			t.Errorf("CAP_DAC_OVERRIDE bit value = 0x%x; want 0x2", dacOverride)
		}
		if setgid != 0x40 { // bit 6 = 2^6
			t.Errorf("CAP_SETGID bit value = 0x%x; want 0x40", setgid)
		}
		if setuid != 0x80 { // bit 7 = 2^7
			t.Errorf("CAP_SETUID bit value = 0x%x; want 0x80", setuid)
		}
		if mknod != 0x8000000 { // bit 27 = 2^27
			t.Errorf("CAP_MKNOD bit value = 0x%x; want 0x8000000", mknod)
		}
	})

	t.Run("capability detection logic", func(t *testing.T) {
		// Test the bit checking logic used in CheckCapabilities
		caps := uint64((1 << CAP_SETUID) | (1 << CAP_SETGID))

		hasSetUID := (caps & (1 << CAP_SETUID)) != 0
		hasSetGID := (caps & (1 << CAP_SETGID)) != 0
		hasMknod := (caps & (1 << CAP_MKNOD)) != 0

		if !hasSetUID {
			t.Error("SETUID should be detected")
		}
		if !hasSetGID {
			t.Error("SETGID should be detected")
		}
		if hasMknod {
			t.Error("MKNOD should not be detected")
		}
	})
}

// ===== TESTS FOR Capability Struct =====

func TestCapabilityStruct(t *testing.T) {
	t.Run("create and verify capability", func(t *testing.T) {
		cap := Capability{
			Name:    "CAP_SETUID",
			Bit:     CAP_SETUID,
			Present: true,
		}

		if cap.Name != "CAP_SETUID" {
			t.Errorf("Name = %q; want CAP_SETUID", cap.Name)
		}
		if cap.Bit != CAP_SETUID {
			t.Errorf("Bit = %d; want %d", cap.Bit, CAP_SETUID)
		}
		if !cap.Present {
			t.Error("Present should be true")
		}
	})
}

// ===== TESTS FOR CapabilityCheck Struct =====

func TestCapabilityCheckStruct(t *testing.T) {
	t.Run("create and verify capability check", func(t *testing.T) {
		check := &CapabilityCheck{
			HasSetUID:      true,
			HasSetGID:      false,
			HasDACOverride: true,
			EffectiveCaps:  0x1234567890abcdef,
			Capabilities: []Capability{
				{Name: "CAP_SETUID", Bit: CAP_SETUID, Present: true},
			},
		}

		if !check.HasSetUID {
			t.Error("HasSetUID should be true")
		}
		if check.HasSetGID {
			t.Error("HasSetGID should be false")
		}
		if !check.HasDACOverride {
			t.Error("HasDACOverride should be true")
		}
		if check.EffectiveCaps != 0x1234567890abcdef {
			t.Errorf("EffectiveCaps = 0x%x; want 0x1234567890abcdef", check.EffectiveCaps)
		}
		if len(check.Capabilities) != 1 {
			t.Errorf("Capabilities length = %d; want 1", len(check.Capabilities))
		}
	})
}

// ===== INTEGRATION TESTS =====

func TestCapabilityCheckIntegration(t *testing.T) {
	t.Run("complete workflow", func(t *testing.T) {
		// Simulate a capability check result
		check := &CapabilityCheck{
			HasSetUID:      true,
			HasSetGID:      true,
			HasDACOverride: false,
			EffectiveCaps:  (1 << CAP_SETUID) | (1 << CAP_SETGID) | (1 << CAP_MKNOD),
			Capabilities: []Capability{
				{Name: "CAP_DAC_OVERRIDE", Bit: CAP_DAC_OVERRIDE, Present: false},
				{Name: "CAP_SETUID", Bit: CAP_SETUID, Present: true},
				{Name: "CAP_SETGID", Bit: CAP_SETGID, Present: true},
				{Name: "CAP_MKNOD", Bit: CAP_MKNOD, Present: true},
			},
		}

		// Test HasRequiredCapabilities
		if !check.HasRequiredCapabilities() {
			t.Error("Should have required capabilities")
		}

		// Test HasCapability
		if !check.HasCapability("SETUID") {
			t.Error("Should have SETUID")
		}
		if check.HasCapability("DAC_OVERRIDE") {
			t.Error("Should not have DAC_OVERRIDE")
		}

		// Test GetMissingCapabilities
		missing := check.GetMissingCapabilities()
		if len(missing) != 0 {
			t.Errorf("Should have no missing required capabilities, got: %v", missing)
		}

		// Test GetMissingCapabilitiesForStorage
		overlayMissing := check.GetMissingCapabilitiesForStorage("overlay")
		if len(overlayMissing) != 0 {
			t.Errorf("Should have no missing capabilities for overlay, got: %v", overlayMissing)
		}

		vfsMissing := check.GetMissingCapabilitiesForStorage("vfs")
		if len(vfsMissing) != 0 {
			t.Errorf("Should have no missing capabilities for vfs, got: %v", vfsMissing)
		}

		// Test FormatCapabilities
		formatted := check.FormatCapabilities()
		if !strings.HasPrefix(formatted, "0x") {
			t.Errorf("Formatted capabilities should start with 0x, got: %s", formatted)
		}
	})

	t.Run("insufficient capabilities workflow", func(t *testing.T) {
		check := &CapabilityCheck{
			HasSetUID:      false,
			HasSetGID:      false,
			HasDACOverride: false,
			EffectiveCaps:  0,
			Capabilities:   []Capability{},
		}

		// Should not have required capabilities
		if check.HasRequiredCapabilities() {
			t.Error("Should not have required capabilities")
		}

		// Should report missing capabilities
		missing := check.GetMissingCapabilities()
		if len(missing) != 2 {
			t.Errorf("Should have 2 missing capabilities, got: %d", len(missing))
		}

		// Should report missing capabilities for overlay
		overlayMissing := check.GetMissingCapabilitiesForStorage("overlay")
		if len(overlayMissing) != 3 {
			t.Errorf("Should have 3 missing capabilities for overlay, got: %d", len(overlayMissing))
		}
	})
}

// ===== EDGE CASE TESTS =====

func TestEdgeCases(t *testing.T) {
	t.Run("nil capability check", func(t *testing.T) {
		var check *CapabilityCheck
		// These should panic if not handled properly, but they're not
		// Let's test with zero-value struct instead
		check = &CapabilityCheck{}

		if check.HasRequiredCapabilities() {
			t.Error("Zero-value check should not have required capabilities")
		}

		missing := check.GetMissingCapabilities()
		if len(missing) != 2 {
			t.Errorf("Zero-value check should have 2 missing capabilities, got: %d", len(missing))
		}
	})

	t.Run("capability name variations", func(t *testing.T) {
		check := &CapabilityCheck{
			HasSetUID: true,
			Capabilities: []Capability{
				{Name: "CAP_SETUID", Bit: CAP_SETUID, Present: true},
			},
		}

		// Test various name formats
		variations := []string{
			"CAP_SETUID",
			"cap_setuid",
			"SETUID",
			"setuid",
			"Setuid",
			"SetuId",
		}

		for _, variation := range variations {
			if !check.HasCapability(variation) {
				t.Errorf("HasCapability(%q) should return true", variation)
			}
		}
	})

	t.Run("empty capabilities list", func(t *testing.T) {
		check := &CapabilityCheck{
			HasSetUID:     true,
			Capabilities:  []Capability{},
			EffectiveCaps: (1 << CAP_SETUID),
		}

		// Should fall back to checking EffectiveCaps
		if !check.HasCapability("SETUID") {
			t.Error("Should detect SETUID from EffectiveCaps fallback")
		}
	})
}

// ===== BENCHMARK TESTS =====

func BenchmarkHasCapability(b *testing.B) {
	check := &CapabilityCheck{
		HasSetUID: true,
		HasSetGID: true,
		Capabilities: []Capability{
			{Name: "CAP_SETUID", Bit: CAP_SETUID, Present: true},
			{Name: "CAP_SETGID", Bit: CAP_SETGID, Present: true},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		check.HasCapability("SETUID")
	}
}

func BenchmarkGetMissingCapabilities(b *testing.B) {
	check := &CapabilityCheck{
		HasSetUID: false,
		HasSetGID: false,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		check.GetMissingCapabilities()
	}
}

func BenchmarkGetMissingCapabilitiesForStorage(b *testing.B) {
	check := &CapabilityCheck{
		HasSetUID:     true,
		HasSetGID:     true,
		EffectiveCaps: (1 << CAP_SETUID) | (1 << CAP_SETGID),
		Capabilities: []Capability{
			{Name: "CAP_MKNOD", Bit: CAP_MKNOD, Present: false},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		check.GetMissingCapabilitiesForStorage("overlay")
	}
}

func BenchmarkFormatCapabilities(b *testing.B) {
	check := &CapabilityCheck{
		EffectiveCaps: 0x1234567890abcdef,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		check.FormatCapabilities()
	}
}

// ===== HELPER FUNCTIONS FOR TESTING =====

// createMockProcStatus creates a temporary file with mock /proc/self/status content
func createMockProcStatus(t *testing.T, capEffHex string) string {
	t.Helper()

	tmpDir := t.TempDir()
	statusPath := filepath.Join(tmpDir, "status")

	content := fmt.Sprintf(`Name:	test
Umask:	0022
State:	R (running)
Tgid:	12345
Ngid:	0
Pid:	12345
PPid:	1
TracerPid:	0
Uid:	0	0	0	0
Gid:	0	0	0	0
FDSize:	256
Groups:
NStgid:	12345
NSpid:	12345
NSpgid:	12345
NSsid:	12345
VmPeak:	   12345 kB
VmSize:	   12345 kB
VmLck:	       0 kB
VmPin:	       0 kB
VmHWM:	    1234 kB
VmRSS:	    1234 kB
RssAnon:	     123 kB
RssFile:	    1111 kB
RssShmem:	       0 kB
VmData:	    1234 kB
VmStk:	     132 kB
VmExe:	       4 kB
VmLib:	    1234 kB
VmPTE:	      48 kB
VmSwap:	       0 kB
HugetlbPages:	       0 kB
CoreDumping:	0
Threads:	1
SigQ:	0/12345
SigPnd:	0000000000000000
ShdPnd:	0000000000000000
SigBlk:	0000000000000000
SigIgn:	0000000000000000
SigCgt:	0000000000000000
CapInh:	0000000000000000
CapPrm:	%s
CapEff:	%s
CapBnd:	0000003fffffffff
CapAmb:	0000000000000000
NoNewPrivs:	0
Seccomp:	0
Speculation_Store_Bypass:	vulnerable
Cpus_allowed:	ff
Cpus_allowed_list:	0-7
Mems_allowed:	1
Mems_allowed_list:	0
voluntary_ctxt_switches:	1
nonvoluntary_ctxt_switches:	0
`, capEffHex, capEffHex)

	if err := os.WriteFile(statusPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create mock status file: %v", err)
	}

	return statusPath
}

// Note: Tests for CheckCapabilities() with mock data would require refactoring
// the function to accept a file path parameter, or using build tags for testing.
// The current implementation reads from /proc/self/status directly.
