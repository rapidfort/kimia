package preflight

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/rapidfort/smithy/pkg/logger"
)

// Capability represents a Linux capability
type Capability struct {
	Name    string
	Bit     uint
	Present bool
}

// CapabilityCheck holds the result of capability detection
type CapabilityCheck struct {
	HasSetUID     bool
	HasSetGID     bool
	EffectiveCaps uint64
	Capabilities  []Capability
}

// Linux capability bit positions
const (
	CAP_SETGID = 6  // bit 6
	CAP_SETUID = 7  // bit 7
)

// CheckCapabilities reads /proc/self/status and parses capabilities
func CheckCapabilities() (*CapabilityCheck, error) {
	logger.Debug("Checking capabilities from /proc/self/status")

	file, err := os.Open("/proc/self/status")
	if err != nil {
		return nil, fmt.Errorf("failed to open /proc/self/status: %v", err)
	}
	defer file.Close()

	var capEffHex string
	scanner := bufio.NewScanner(file)
	
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "CapEff:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				capEffHex = parts[1]
				break
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading /proc/self/status: %v", err)
	}

	if capEffHex == "" {
		return nil, fmt.Errorf("CapEff not found in /proc/self/status")
	}

	// Parse hex string to uint64
	capEff, err := strconv.ParseUint(capEffHex, 16, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CapEff hex value '%s': %v", capEffHex, err)
	}

	logger.Debug("Effective capabilities: 0x%016x", capEff)

	// Check specific capabilities
	hasSetUID := (capEff & (1 << CAP_SETUID)) != 0
	hasSetGID := (capEff & (1 << CAP_SETGID)) != 0

	logger.Debug("CAP_SETUID (bit %d): %v", CAP_SETUID, hasSetUID)
	logger.Debug("CAP_SETGID (bit %d): %v", CAP_SETGID, hasSetGID)

	result := &CapabilityCheck{
		HasSetUID:     hasSetUID,
		HasSetGID:     hasSetGID,
		EffectiveCaps: capEff,
		Capabilities: []Capability{
			{Name: "CAP_SETUID", Bit: CAP_SETUID, Present: hasSetUID},
			{Name: "CAP_SETGID", Bit: CAP_SETGID, Present: hasSetGID},
		},
	}

	return result, nil
}

// HasRequiredCapabilities checks if all required capabilities are present
func (c *CapabilityCheck) HasRequiredCapabilities() bool {
	return c.HasSetUID && c.HasSetGID
}

// GetMissingCapabilities returns a list of missing required capabilities
func (c *CapabilityCheck) GetMissingCapabilities() []string {
	var missing []string
	
	if !c.HasSetUID {
		missing = append(missing, "CAP_SETUID")
	}
	if !c.HasSetGID {
		missing = append(missing, "CAP_SETGID")
	}
	
	return missing
}

// FormatCapabilities returns a formatted string of capabilities for display
func (c *CapabilityCheck) FormatCapabilities() string {
	return fmt.Sprintf("0x%016x", c.EffectiveCaps)
}
