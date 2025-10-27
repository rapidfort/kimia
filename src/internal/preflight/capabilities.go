package preflight

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/rapidfort/kimia/pkg/logger"
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
	HasDACOverride bool 
	EffectiveCaps uint64
	Capabilities  []Capability
}

// Linux capability bit positions
const (
	CAP_DAC_OVERRIDE = 1  // bit 1 - bypass file read, write, and execute permission checks
	CAP_SETGID = 6  // bit 6
	CAP_SETUID = 7  // bit 7
	CAP_MKNOD  = 27 // bit 27 - CREATE special files (needed for overlay)
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
	hasDACOverride := (capEff & (1 << CAP_DAC_OVERRIDE)) != 0  // Added
	hasSetUID := (capEff & (1 << CAP_SETUID)) != 0
	hasSetGID := (capEff & (1 << CAP_SETGID)) != 0
	hasMknod := (capEff & (1 << CAP_MKNOD)) != 0

	logger.Debug("CAP_DAC_OVERRIDE (bit %d): %v", CAP_DAC_OVERRIDE, hasDACOverride)  // Added
	logger.Debug("CAP_SETUID (bit %d): %v", CAP_SETUID, hasSetUID)
	logger.Debug("CAP_SETGID (bit %d): %v", CAP_SETGID, hasSetGID)
	logger.Debug("CAP_MKNOD (bit %d): %v", CAP_MKNOD, hasMknod)

	result := &CapabilityCheck{
		HasSetUID:     hasSetUID,
		HasSetGID:     hasSetGID,
		HasDACOverride: hasDACOverride,  // Added
		EffectiveCaps: capEff,
		Capabilities: []Capability{
			{Name: "CAP_DAC_OVERRIDE", Bit: CAP_DAC_OVERRIDE, Present: hasDACOverride},  // Added
			{Name: "CAP_SETUID", Bit: CAP_SETUID, Present: hasSetUID},
			{Name: "CAP_SETGID", Bit: CAP_SETGID, Present: hasSetGID},
			{Name: "CAP_MKNOD", Bit: CAP_MKNOD, Present: hasMknod},
		},
	}

	return result, nil
}

// HasRequiredCapabilities checks if all required capabilities are present
func (c *CapabilityCheck) HasRequiredCapabilities() bool {
	return c.HasSetUID && c.HasSetGID
}

// HasCapability checks if a specific capability is present by name
func (c *CapabilityCheck) HasCapability(capName string) bool {
	// Normalize the capability name (handle with or without CAP_ prefix)
	capName = strings.ToUpper(capName)
	if !strings.HasPrefix(capName, "CAP_") {
		capName = "CAP_" + capName
	}

	// Check against stored capabilities
	for _, cap := range c.Capabilities {
		if cap.Name == capName {
			return cap.Present
		}
	}

	// For backward compatibility, also check by bit position
	switch capName {
	case "CAP_DAC_OVERRIDE":  // Added
		return c.HasDACOverride
	case "CAP_SETUID":
		return c.HasSetUID
	case "CAP_SETGID":
		return c.HasSetGID
	case "CAP_MKNOD":
		return (c.EffectiveCaps & (1 << CAP_MKNOD)) != 0
	default:
		logger.Debug("Unknown capability requested: %s", capName)
		return false
	}
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

// GetMissingCapabilitiesForStorage returns missing capabilities for a specific storage driver
func (c *CapabilityCheck) GetMissingCapabilitiesForStorage(storageDriver string) []string {
	var missing []string

	// SETUID and SETGID are always required
	if !c.HasSetUID {
		missing = append(missing, "CAP_SETUID")
	}
	if !c.HasSetGID {
		missing = append(missing, "CAP_SETGID")
	}

	// MKNOD only required for overlay
	if storageDriver == "overlay" && !c.HasCapability("CAP_MKNOD") {
		missing = append(missing, "CAP_MKNOD")
	}

	return missing
}

// FormatCapabilities returns a formatted string of capabilities for display
func (c *CapabilityCheck) FormatCapabilities() string {
	return fmt.Sprintf("0x%016x", c.EffectiveCaps)
}