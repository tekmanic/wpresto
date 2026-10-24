// Package publish atomically promotes a built staging tree to the live
// directory that nginx serves, so readers never see a half-written site.
package publish

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// BuildInfo is written to live/build-info.json on every successful build.
type BuildInfo struct {
	BuildID    string        `json:"build_id"`
	StartedAt  time.Time     `json:"started_at"`
	FinishedAt time.Time     `json:"finished_at"`
	Duration   time.Duration `json:"duration"`
	Pages      int           `json:"pages"`
	Assets     int           `json:"assets"`
	Bytes      int64         `json:"bytes"`
	SiteURL    string        `json:"site_url"`
}

// Publish atomically promotes stagingDir to <root>/live.
//
// Layout after a successful publish:
//
//	<root>/live            <- nginx root
//	<root>/build-info.json <- (inside live)
//
// The previous live tree is removed after the swap. If the swap fails, the
// previous live tree is restored.
func Publish(root, stagingDir string, info *BuildInfo) error {
	live := filepath.Join(root, "live")
	old := filepath.Join(root, ".old-"+info.BuildID)

	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("mkdir root %s: %w", root, err)
	}

	// Write build info into the staging tree before the swap.
	if info != nil {
		data, err := json.MarshalIndent(info, "", "  ")
		if err == nil {
			if err := os.WriteFile(filepath.Join(stagingDir, "build-info.json"), data, 0o644); err != nil {
				return fmt.Errorf("write build-info.json: %w", err)
			}
		}
	}

	// Move the current live tree aside.
	hadLive := false
	if _, err := os.Stat(live); err == nil {
		hadLive = true
		if err := os.Rename(live, old); err != nil {
			return fmt.Errorf("rename live->old: %w", err)
		}
	}

	// Promote staging to live.
	if err := os.Rename(stagingDir, live); err != nil {
		if hadLive {
			_ = os.Rename(old, live) // best-effort rollback
		}
		return fmt.Errorf("rename staging->live: %w", err)
	}

	// Remove the previous tree (non-fatal on failure).
	if hadLive {
		if err := os.RemoveAll(old); err != nil {
			fmt.Fprintf(os.Stderr, "warn: could not remove previous tree %s: %v\n", old, err)
		}
	}
	return nil
}

// TreeBytes returns the total size in bytes of all regular files under dir.
func TreeBytes(dir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		}
		return nil
	})
	return total, err
}

// ReadFile reads a whole file, bounded by maxBytes.
func ReadFile(path string, maxBytes int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, maxBytes+1))
}
