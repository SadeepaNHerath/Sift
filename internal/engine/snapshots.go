package engine

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// APFSSnapshot represents an APFS local Time Machine snapshot.
type APFSSnapshot struct {
	Name      string    `json:"name"`
	Timestamp time.Time `json:"timestamp,omitempty"`
	DateStr   string    `json:"date_str"`
}

// Command execution hooks for testability.
var (
	execTmutilList = func(mountPoint string) ([]byte, error) {
		return exec.Command("tmutil", "listlocalsnapshots", mountPoint).CombinedOutput()
	}

	execTmutilThin = func(mountPoint string, bytes int64, urgency int) ([]byte, error) {
		return exec.Command("tmutil", "thinlocalsnapshots", mountPoint, fmt.Sprintf("%d", bytes), fmt.Sprintf("%d", urgency)).CombinedOutput()
	}
)

// ParseLocalSnapshots parses raw tmutil listlocalsnapshots stdout into structured APFSSnapshot entries.
func ParseLocalSnapshots(rawOutput []byte) []APFSSnapshot {
	var results []APFSSnapshot
	scanner := bufio.NewScanner(bytes.NewReader(rawOutput))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "Snapshots for") {
			continue
		}

		if strings.HasPrefix(line, "com.apple.TimeMachine.") || strings.HasSuffix(line, ".local") {
			snap := APFSSnapshot{
				Name: line,
			}

			// Extract date tokens if matching com.apple.TimeMachine.YYYY-MM-DD-HHMMSS.local
			clean := strings.TrimPrefix(line, "com.apple.TimeMachine.")
			clean = strings.TrimSuffix(clean, ".local")
			if t, err := time.Parse("2006-01-02-150405", clean); err == nil {
				snap.Timestamp = t
				snap.DateStr = t.Format("2006-01-02 15:04:05")
			} else {
				snap.DateStr = clean
			}

			results = append(results, snap)
		}
	}

	return results
}

// ListLocalSnapshots queries tmutil for local snapshots on the root filesystem.
func ListLocalSnapshots() ([]APFSSnapshot, error) {
	out, err := execTmutilList("/")
	if err != nil {
		return nil, fmt.Errorf("tmutil listlocalsnapshots failed: %w", err)
	}
	return ParseLocalSnapshots(out), nil
}

// ThinLocalSnapshots requests tmutil to thin local snapshots on the root filesystem.
// urgency is 1 (low) to 4 (aggressive, reclaim all possible purgeable space).
func ThinLocalSnapshots(bytesToReclaim int64, urgency int) (string, error) {
	if bytesToReclaim <= 0 {
		bytesToReclaim = 100 * 1000 * 1000 * 1000 // default 100 GB in base-10
	}
	if urgency < 1 || urgency > 4 {
		urgency = 4
	}

	out, err := execTmutilThin("/", bytesToReclaim, urgency)
	msg := strings.TrimSpace(string(out))
	if err != nil {
		return msg, fmt.Errorf("tmutil thinlocalsnapshots failed: %w (output: %s)", err, msg)
	}
	return msg, nil
}
