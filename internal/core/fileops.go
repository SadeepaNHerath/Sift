package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// RemovalReport captures the outcome of a batch removal operation.
type RemovalReport struct {
	Removed    []RemovedItem
	Failed     []FailedItem
	TotalBytes int64
}

// RemovedItem represents an item successfully moved to the trash.
type RemovedItem struct {
	Path      string
	Size      int64
	TrashPath string
}

// FailedItem represents an item that could not be safely removed.
type FailedItem struct {
	Path  string
	Error error
}

// DryRunReport captures what would be deleted without executing any deletion.
type DryRunReport struct {
	WouldRemove []DryRunItem
	Blocked     []BlockedItem
	TotalBytes  int64
}

// DryRunItem represents an item validated and eligible for deletion.
type DryRunItem struct {
	Path string
	Size int64
}

// BlockedItem represents an item that failed safety validation or is missing.
type BlockedItem struct {
	Path   string
	Reason string
}

var trashVerbosePattern = regexp.MustCompile(`# Moved ".*?" to "(.*?)"`)

// MoveToTrash moves a file or directory to macOS Trash.
// Uses a 3-tier fallback strategy:
//
//	Tier 1: /usr/bin/trash (native macOS 15+ CLI tool)
//	Tier 2: Rename directly to ~/.Trash/ with collision handling
//	Tier 3: AppleScript Finder fallback
func MoveToTrash(path string) error {
	_, err := moveToTrashWithDest(path)
	return err
}

// moveToTrashWithDest moves the file or directory to Trash and returns the destination path.
func moveToTrashWithDest(path string) (string, error) {
	normPath, err := NormalizePath(path)
	if err != nil {
		return "", err
	}

	if _, err := os.Lstat(normPath); err != nil {
		return "", fmt.Errorf("item does not exist: %w", err)
	}

	var tierErrors []string

	// Tier 1: /usr/bin/trash (macOS 15+)
	if fi, err := os.Stat("/usr/bin/trash"); err == nil && !fi.IsDir() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, "/usr/bin/trash", "-v", normPath)
		output, runErr := cmd.CombinedOutput()
		cancel()
		if runErr == nil {
			dest := parseTrashDestination(string(output))
			if dest == "" {
				dest = filepath.Join(getTrashDir(), filepath.Base(normPath))
			}
			return dest, nil
		}
		tierErrors = append(tierErrors, fmt.Sprintf("tier 1 (/usr/bin/trash): %v (%s)", runErr, strings.TrimSpace(string(output))))
	} else {
		tierErrors = append(tierErrors, "tier 1 (/usr/bin/trash): binary not available")
	}

	// Tier 2: Rename to ~/.Trash/ with collision handling
	trashDir := getTrashDir()
	if err := os.MkdirAll(trashDir, 0700); err == nil {
		destPath := ResolveTrashCollision(trashDir, filepath.Base(normPath))
		if renameErr := os.Rename(normPath, destPath); renameErr == nil {
			return destPath, nil
		} else if isCrossDeviceError(renameErr) {
			if copyErr := moveCrossDevice(normPath, destPath); copyErr == nil {
				return destPath, nil
			} else {
				tierErrors = append(tierErrors, fmt.Sprintf("tier 2 cross-device copy: %v", copyErr))
			}
		} else {
			tierErrors = append(tierErrors, fmt.Sprintf("tier 2 (rename to ~/.Trash): %v", renameErr))
		}
	} else {
		tierErrors = append(tierErrors, fmt.Sprintf("tier 2 (mkdir ~/.Trash): %v", err))
	}

	// Tier 3: AppleScript Finder fallback
	script := fmt.Sprintf(`tell application "Finder" to delete (POSIX file %q as alias)`, normPath)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	output, scriptErr := cmd.CombinedOutput()
	cancel()
	if scriptErr == nil {
		dest := filepath.Join(getTrashDir(), filepath.Base(normPath))
		return dest, nil
	}
	tierErrors = append(tierErrors, fmt.Sprintf("tier 3 (osascript Finder): %v (%s)", scriptErr, strings.TrimSpace(string(output))))

	return "", fmt.Errorf("failed to move %q to trash: %s", normPath, strings.Join(tierErrors, "; "))
}

// SafeRemove validates the path through the 5-layer safety system,
// binds its physical inode (anti-TOCTOU), and moves it to macOS Trash.
func SafeRemove(path string) error {
	normPath, err := NormalizePath(path)
	if err != nil {
		return err
	}

	// Layer 1-4 Safety checks
	if err := ValidatePathForDeletion(normPath); err != nil {
		return err
	}

	// Check existence
	if _, err := os.Lstat(normPath); err != nil {
		return fmt.Errorf("target does not exist: %w", err)
	}

	// Layer 5: Bind Inode
	ref, err := BindInode(normPath)
	if err != nil {
		return fmt.Errorf("failed to bind inode before deletion: %w", err)
	}

	// Verify Inode immediately before deletion
	if err := ref.Verify(); err != nil {
		return fmt.Errorf("inode verification failed: %w", err)
	}

	return MoveToTrash(normPath)
}

// SafeRemoveAll validates and removes multiple paths.
// Returns a detailed report of successes, failures, and bytes freed.
func SafeRemoveAll(paths []string) *RemovalReport {
	report := &RemovalReport{
		Removed: make([]RemovedItem, 0),
		Failed:  make([]FailedItem, 0),
	}

	for _, p := range paths {
		normPath, err := NormalizePath(p)
		if err != nil {
			report.Failed = append(report.Failed, FailedItem{Path: p, Error: err})
			continue
		}

		if err := ValidatePathForDeletion(normPath); err != nil {
			report.Failed = append(report.Failed, FailedItem{Path: normPath, Error: err})
			continue
		}

		size, err := FileSize(normPath)
		if err != nil {
			report.Failed = append(report.Failed, FailedItem{Path: normPath, Error: err})
			continue
		}

		ref, err := BindInode(normPath)
		if err != nil {
			report.Failed = append(report.Failed, FailedItem{Path: normPath, Error: err})
			continue
		}

		if err := ref.Verify(); err != nil {
			report.Failed = append(report.Failed, FailedItem{Path: normPath, Error: err})
			continue
		}

		trashPath, err := moveToTrashWithDest(normPath)
		if err != nil {
			report.Failed = append(report.Failed, FailedItem{Path: normPath, Error: err})
			continue
		}

		report.Removed = append(report.Removed, RemovedItem{
			Path:      normPath,
			Size:      size,
			TrashPath: trashPath,
		})
		report.TotalBytes += size
	}

	return report
}

// DryRunRemove validates candidate paths without deleting them.
// Returns a report detailing what would be removed and what would be blocked.
func DryRunRemove(paths []string) *DryRunReport {
	report := &DryRunReport{
		WouldRemove: make([]DryRunItem, 0),
		Blocked:     make([]BlockedItem, 0),
	}

	for _, p := range paths {
		normPath, err := NormalizePath(p)
		if err != nil {
			report.Blocked = append(report.Blocked, BlockedItem{Path: p, Reason: err.Error()})
			continue
		}

		if err := ValidatePathForDeletion(normPath); err != nil {
			report.Blocked = append(report.Blocked, BlockedItem{Path: normPath, Reason: err.Error()})
			continue
		}

		size, err := FileSize(normPath)
		if err != nil {
			report.Blocked = append(report.Blocked, BlockedItem{Path: normPath, Reason: err.Error()})
			continue
		}

		report.WouldRemove = append(report.WouldRemove, DryRunItem{
			Path: normPath,
			Size: size,
		})
		report.TotalBytes += size
	}

	return report
}

// FileSize returns the size in bytes of a file or directory tree.
func FileSize(path string) (int64, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return 0, err
	}

	if !fi.IsDir() {
		return fi.Size(), nil
	}

	var total int64
	err = filepath.WalkDir(path, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err == nil {
				total += info.Size()
			}
		}
		return nil
	})

	return total, err
}

// FormatBytes formats a byte count into a human-readable string (e.g., "1.2 GB", "456 MB").
func FormatBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}

	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	val := float64(bytes)
	idx := 0
	for val >= 1024 && idx < len(units)-1 {
		val /= 1024
		idx++
	}

	if val == math.Floor(val) {
		return fmt.Sprintf("%.0f %s", val, units[idx])
	}

	formatted := fmt.Sprintf("%.1f", val)
	formatted = strings.TrimSuffix(formatted, ".0")
	return fmt.Sprintf("%s %s", formatted, units[idx])
}

// ResolveTrashCollision generates a non-colliding destination path in trashDir.
// If baseName already exists in trashDir, appends " 1", " 2", etc. before the extension.
func ResolveTrashCollision(trashDir, baseName string) string {
	target := filepath.Join(trashDir, baseName)
	if _, err := os.Lstat(target); os.IsNotExist(err) {
		return target
	}

	ext := filepath.Ext(baseName)
	stem := strings.TrimSuffix(baseName, ext)

	for i := 1; i <= 100000; i++ {
		candidate := filepath.Join(trashDir, fmt.Sprintf("%s %d%s", stem, i, ext))
		if _, err := os.Lstat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}

	return filepath.Join(trashDir, fmt.Sprintf("%s_%d%s", stem, os.Getpid(), ext))
}

// getTrashDir returns the ~/.Trash path for the current user.
func getTrashDir() string {
	home := GetHomeDir()
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return filepath.Join(home, ".Trash")
}

// parseTrashDestination parses the moved destination path from /usr/bin/trash -v output.
func parseTrashDestination(output string) string {
	matches := trashVerbosePattern.FindStringSubmatch(output)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// isCrossDeviceError checks if the error indicates a cross-device filesystem move.
func isCrossDeviceError(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}

// moveCrossDevice handles cross-device file and directory moves by copying and removing.
func moveCrossDevice(src, dst string) error {
	fi, err := os.Lstat(src)
	if err != nil {
		return err
	}

	if fi.IsDir() {
		if err := copyDirectory(src, dst); err != nil {
			return err
		}
		return os.RemoveAll(src)
	}

	if fi.Mode()&os.ModeSymlink != 0 {
		linkTarget, err := os.Readlink(src)
		if err != nil {
			return err
		}
		if err := os.Symlink(linkTarget, dst); err != nil {
			return err
		}
		return os.Remove(src)
	}

	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

// copyFile copies a regular file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	fi, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fi.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// copyDirectory recursively copies a directory tree from src to dst.
func copyDirectory(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode().Perm()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDirectory(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}
