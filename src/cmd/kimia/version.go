package main

import (
	"fmt"
	"strconv"
	"time"
)

// Version information set at build time
var (
	Version   = "1.0.0-dev"
	BuildDate = "unknown"
	CommitSHA = "unknown"
	Branch    = "unknown"
)

func printVersion() {
	fmt.Println("Kimia – Kubernetes-Native OCI Builder")
	fmt.Println("Daemonless. Rootless. Privilege-free. Fully OCI-compliant.")
	fmt.Println()
	fmt.Printf("Version: %s\n", Version)
	fmt.Printf("Built: %s\n", convertEpochStringToHumanReadable(BuildDate))
	fmt.Printf("Commit: %s\n", CommitSHA)
}

func convertEpochStringToHumanReadable(epochStr string) string {
	epoch, err := strconv.ParseFloat(epochStr, 64)
	if err != nil {
		return epochStr // Return as-is if not a valid epoch
	}

	t := time.Unix(int64(epoch), 0).Local()
	return t.Format("2006-01-02 15:04:05 MST")
}
