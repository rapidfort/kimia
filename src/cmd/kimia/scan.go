package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/rapidfort/kimia/pkg/logger"
)

// prepareScanRootfs creates an empty directory for BuildKit type=local export.
func prepareScanRootfs() (string, error) {
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		homeDir = "/home/kimia"
	}
	dir := filepath.Join(homeDir, ".kimia", "scan-rootfs")
	if err := os.RemoveAll(dir); err != nil {
		return "", fmt.Errorf("clear scan rootfs: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create scan rootfs: %w", err)
	}
	return dir, nil
}

// runRapidFortScan exports a merged rootfs if needed and invokes RapidFort scan.sh
// with RF_OCI_ROOTFS so rfscan does not need docker or podman.
func runRapidFortScan(config *Config, builder, scanRootfs string) error {
	imageName := config.Destination[0]
	if !strings.Contains(imageName, ":") && !strings.Contains(imageName, "@") {
		imageName += ":latest"
	}

	rootfs := scanRootfs
	var cleanup func()
	if builder == "buildah" {
		var err error
		rootfs, cleanup, err = exportBuildahRootfs(config.Destination[0])
		if err != nil {
			return err
		}
		if cleanup != nil {
			defer cleanup()
		}
	}

	if _, err := os.Stat(rootfs); err != nil {
		return fmt.Errorf("scan rootfs not found at %s: %w", rootfs, err)
	}

	rfRoot, bashPath, scanScript, err := findRapidFortCLI()
	if err != nil {
		return err
	}
	logger.Info("Using RapidFort CLI at %s", rfRoot)

	// Keep inspect.json outside the scanned tree (sibling of the export dir).
	inspectPath := filepath.Join(filepath.Dir(scanRootfs), "inspect.json")
	if err := writeScanInspect(inspectPath, imageName, rootfs, config.CustomPlatform); err != nil {
		return fmt.Errorf("write inspect.json: %w", err)
	}

	logger.Info("Scanning %s (rootfs %s)", imageName, rootfs)
	cmd := exec.Command(bashPath, scanScript, imageName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Dir = rfRoot
	cmd.Env = append(os.Environ(),
		"RF_OCI_ROOTFS="+rootfs,
		"RF_OCI_INSPECT="+inspectPath,
	)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rfscan failed: %w", err)
	}
	logger.Info("Scan completed for %s", imageName)
	return nil
}

func findRapidFortCLI() (root, bashPath, scanScript string, err error) {
	candidates := []string{}
	if p, lookErr := exec.LookPath("rfscan"); lookErr == nil {
		candidates = append(candidates, filepath.Dir(p))
	}
	if home, homeErr := os.UserHomeDir(); homeErr == nil {
		candidates = append(candidates, filepath.Join(home, "rapidfort"))
	}
	if home := os.Getenv("HOME"); home != "" {
		candidates = append(candidates, filepath.Join(home, "rapidfort"))
	}
	candidates = append(candidates, "/usr/local/bin/rapidfort", "/home/kimia/rapidfort")

	seen := map[string]bool{}
	for _, dir := range candidates {
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		script := filepath.Join(dir, "scan.sh")
		bash := filepath.Join(dir, "tools", "bash")
		if st, statErr := os.Stat(script); statErr == nil && !st.IsDir() {
			if _, bashErr := os.Stat(bash); bashErr != nil {
				if p, lookErr := exec.LookPath("bash"); lookErr == nil {
					bash = p
				} else {
					continue
				}
			}
			return dir, bash, script, nil
		}
	}
	return "", "", "", fmt.Errorf("RapidFort CLI not found (rfscan/scan.sh). Rebuild kimia with RF_APP_HOST set so the CLI is installed into the image")
}

func exportBuildahRootfs(image string) (string, func(), error) {
	fromCmd := exec.Command("buildah", "from", image)
	fromOut, err := fromCmd.CombinedOutput()
	if err != nil {
		return "", nil, fmt.Errorf("buildah from %s: %w (%s)", image, err, strings.TrimSpace(string(fromOut)))
	}
	ctr := strings.TrimSpace(string(fromOut))
	cleanup := func() {
		_ = exec.Command("buildah", "umount", ctr).Run()
		_ = exec.Command("buildah", "rm", ctr).Run()
	}

	mountCmd := exec.Command("buildah", "mount", ctr)
	mountOut, err := mountCmd.CombinedOutput()
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("buildah mount: %w (%s)", err, strings.TrimSpace(string(mountOut)))
	}
	mnt := strings.TrimSpace(string(mountOut))
	if mnt == "" {
		cleanup()
		return "", nil, fmt.Errorf("buildah mount returned empty path")
	}
	logger.Info("Buildah scan rootfs: %s", mnt)
	return mnt, cleanup, nil
}

func writeScanInspect(path, imageRef, rootfs, platform string) error {
	arch := runtime.GOARCH
	if platform != "" {
		// linux/amd64 -> amd64
		if i := strings.LastIndex(platform, "/"); i >= 0 {
			arch = platform[i+1:]
		} else {
			arch = platform
		}
	}

	size, err := rootfsSize(rootfs)
	if err != nil {
		return err
	}

	sum := sha256.Sum256([]byte(imageRef + "\n" + rootfs))
	id := "sha256:" + hex.EncodeToString(sum[:])

	inspect := []map[string]interface{}{{
		"Id":           id,
		"RepoTags":     []string{imageRef},
		"RepoDigests":  []string{},
		"Created":      time.Now().UTC().Format(time.RFC3339Nano),
		"Size":         size,
		"VirtualSize":  size,
		"Architecture": arch,
		"Os":           "linux",
		"Config":       map[string]interface{}{},
		"RootFS": map[string]interface{}{
			"Type":   "layers",
			"Layers": []string{},
		},
	}}

	data, err := json.MarshalIndent(inspect, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func rootfsSize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total, err
}
