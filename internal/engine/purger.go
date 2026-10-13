package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/SadeepaNHerath/Spacefold/internal/core"
)

// KnownArtifactDirs defines common developer dependencies and build artifact directory names.
var KnownArtifactDirs = []string{
	"node_modules",
	"target",
	"build",
	"dist",
	".venv",
	"venv",
	"Pods",
	"DerivedData",
	".next",
	".nuxt",
	"vendor",
	".turbo",
	".pytest_cache",
}

// knownArtifactMap provides O(1) case-insensitive lookup for known artifact directory names.
var knownArtifactMap = func() map[string]string {
	m := make(map[string]string, len(KnownArtifactDirs))
	for _, dir := range KnownArtifactDirs {
		m[strings.ToLower(dir)] = dir
	}
	return m
}()

// IsKnownArtifactDir returns the canonical artifact type name if the directory name matches,
// or an empty string otherwise.
func IsKnownArtifactDir(name string) string {
	if canonical, ok := knownArtifactMap[strings.ToLower(name)]; ok {
		return canonical
	}
	return ""
}

// PurgeTarget represents a discovered developer build artifact or dependency directory.
type PurgeTarget struct {
	Path         string
	ProjectName  string
	ArtifactType string
	Size         int64
	ModTime      time.Time
	SafeToPurge  bool
	IsOldEnough  bool
	GitRepo      bool
	GitDirty     bool
	Warning      string
}

// gitStatusResult caches git status queries for a given project directory.
type gitStatusResult struct {
	isRepo  bool
	isDirty bool
	err     error
}

var (
	gitCacheMu sync.Mutex
	gitCache   = make(map[string]gitStatusResult)
)

// checkGitStatus inspects whether a directory is inside a git repository and whether it has uncommitted changes.
func checkGitStatus(dir string) (bool, bool, error) {
	cleanDir := filepath.Clean(dir)

	gitCacheMu.Lock()
	if cached, ok := gitCache[cleanDir]; ok {
		gitCacheMu.Unlock()
		return cached.isRepo, cached.isDirty, cached.err
	}
	gitCacheMu.Unlock()

	gitPath, err := exec.LookPath("git")
	if err != nil {
		res := gitStatusResult{isRepo: false, isDirty: false, err: nil}
		gitCacheMu.Lock()
		gitCache[cleanDir] = res
		gitCacheMu.Unlock()
		return false, false, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, gitPath, "-C", cleanDir, "rev-parse", "--is-inside-work-tree")
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		res := gitStatusResult{isRepo: false, isDirty: false, err: nil}
		gitCacheMu.Lock()
		gitCache[cleanDir] = res
		gitCacheMu.Unlock()
		return false, false, nil
	}

	// It's a git repo; now check if dirty
	statusCtx, statusCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer statusCancel()

	statusCmd := exec.CommandContext(statusCtx, gitPath, "-C", cleanDir, "status", "--porcelain")
	statusOut, statusErr := statusCmd.Output()
	isDirty := false
	if statusErr == nil && len(strings.TrimSpace(string(statusOut))) > 0 {
		isDirty = true
	}

	res := gitStatusResult{isRepo: true, isDirty: isDirty, err: statusErr}
	gitCacheMu.Lock()
	gitCache[cleanDir] = res
	gitCacheMu.Unlock()

	return true, isDirty, statusErr
}

// ClearGitCache clears the cached git status entries (useful for testing).
func ClearGitCache() {
	gitCacheMu.Lock()
	gitCache = make(map[string]gitStatusResult)
	gitCacheMu.Unlock()
}

// DefaultPurgeRoots returns a list of standard developer root directories that exist on this system.
func DefaultPurgeRoots() []string {
	var roots []string
	home := core.GetHomeDir()
	if home == "" {
		home, _ = os.UserHomeDir()
	}

	candidates := []string{
		filepath.Join(home, "Developer"),
		filepath.Join(home, "Projects"),
		filepath.Join(home, "Development"),
		filepath.Join(home, "workspace"),
		filepath.Join(home, "code"),
		filepath.Join(home, "src"),
		filepath.Join(home, "Library", "Developer", "Xcode", "DerivedData"),
	}

	for _, cand := range candidates {
		if fi, err := os.Stat(cand); err == nil && fi.IsDir() {
			roots = append(roots, cand)
		}
	}

	cwd, err := os.Getwd()
	if err == nil && cwd != "" {
		alreadyPresent := false
		for _, r := range roots {
			if r == cwd {
				alreadyPresent = true
				break
			}
		}
		if !alreadyPresent {
			roots = append(roots, cwd)
		}
	}

	if len(roots) == 0 && cwd != "" {
		roots = []string{cwd}
	}

	return roots
}

// inspectArtifactDir calculates the total size in bytes and finds the most recent modification time
// among all entries within the given directory tree.
func inspectArtifactDir(targetPath string) (int64, time.Time, error) {
	fi, err := os.Lstat(targetPath)
	if err != nil {
		return 0, time.Time{}, err
	}

	var totalSize int64
	maxModTime := fi.ModTime()

	err = filepath.WalkDir(targetPath, func(_ string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		if !d.IsDir() {
			totalSize += info.Size()
		}
		if info.ModTime().After(maxModTime) {
			maxModTime = info.ModTime()
		}
		return nil
	})

	return totalSize, maxModTime, err
}

// ScanPurgeTargets scans the given root directories for known developer build artifacts and dependencies.
// If roots is empty, default developer locations are scanned.
// minAge specifies the minimum elapsed time since modification for an artifact to be considered old enough.
// If includeEmpty is false, empty artifact directories (0 bytes) are skipped.
func ScanPurgeTargets(roots []string, minAge time.Duration, includeEmpty bool) ([]PurgeTarget, error) {
	if len(roots) == 0 {
		roots = DefaultPurgeRoots()
	}

	var targets []PurgeTarget
	seenPaths := make(map[string]bool)

	for _, root := range roots {
		normRoot, err := core.NormalizePath(root)
		if err != nil {
			continue
		}

		rootInfo, err := os.Lstat(normRoot)
		if err != nil || !rootInfo.IsDir() {
			continue
		}

		// Check if the root directory itself is a known artifact directory
		if artType := IsKnownArtifactDir(filepath.Base(normRoot)); artType != "" {
			if !seenPaths[normRoot] {
				seenPaths[normRoot] = true
				target, ok := evaluatePurgeTarget(normRoot, artType, minAge, includeEmpty)
				if ok {
					targets = append(targets, target)
				}
			}
			continue
		}

		// Recursively walk subdirectories
		_ = filepath.WalkDir(normRoot, func(currentPath string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}

			if !d.IsDir() {
				return nil
			}

			// Don't treat the root itself as a child
			if currentPath == normRoot {
				return nil
			}

			name := d.Name()

			// Skip version control and trash directories entirely
			if name == ".git" || name == ".Trash" || name == ".svn" || name == ".hg" {
				return filepath.SkipDir
			}

			// Skip user personal libraries that are protected unless explicitly scanning inside them
			if name == "Library" && filepath.Dir(currentPath) == core.GetHomeDir() {
				return filepath.SkipDir
			}

			// Check if this directory is a known artifact directory
			if artType := IsKnownArtifactDir(name); artType != "" {
				if !seenPaths[currentPath] {
					seenPaths[currentPath] = true
					target, ok := evaluatePurgeTarget(currentPath, artType, minAge, includeEmpty)
					if ok {
						targets = append(targets, target)
					}
				}
				// Crucial: Do not recurse into known artifact directories
				return filepath.SkipDir
			}

			return nil
		})
	}

	return targets, nil
}

// evaluatePurgeTarget builds a PurgeTarget with safety, age, and git status analysis.
func evaluatePurgeTarget(targetPath, artifactType string, minAge time.Duration, includeEmpty bool) (PurgeTarget, bool) {
	size, modTime, err := inspectArtifactDir(targetPath)
	if err != nil {
		return PurgeTarget{}, false
	}

	if !includeEmpty && size == 0 {
		return PurgeTarget{}, false
	}

	// Determine ProjectName from the parent directory
	parentDir := filepath.Dir(targetPath)
	projectName := filepath.Base(parentDir)
	if projectName == "" || projectName == "." || projectName == "/" {
		projectName = "root"
	}

	// Age check
	isOldEnough := true
	if minAge > 0 {
		isOldEnough = time.Since(modTime) >= minAge
	}

	// Git status check
	isRepo, isDirty, _ := checkGitStatus(parentDir)

	// Five-layer safety check
	valErr := core.ValidatePathForDeletion(targetPath)

	var warnings []string
	if valErr != nil {
		warnings = append(warnings, fmt.Sprintf("protected path: %v", valErr))
	}
	if !isOldEnough {
		warnings = append(warnings, fmt.Sprintf("recently modified (< %s)", formatPurgeDuration(minAge)))
	}
	if isDirty {
		warnings = append(warnings, "git repository has uncommitted changes")
	}

	// SafeToPurge requires passing path validation, meeting minimum age, and having clean git working tree
	safeToPurge := (valErr == nil) && isOldEnough && !isDirty

	target := PurgeTarget{
		Path:         targetPath,
		ProjectName:  projectName,
		ArtifactType: artifactType,
		Size:         size,
		ModTime:      modTime,
		SafeToPurge:  safeToPurge,
		IsOldEnough:  isOldEnough,
		GitRepo:      isRepo,
		GitDirty:     isDirty,
		Warning:      strings.Join(warnings, "; "),
	}

	return target, true
}

// PurgeTargets removes the specified purge targets.
// If dryRun is true, no files are modified, and the report reflects what would be removed.
// When dryRun is false, uses core.ValidatePathForDeletion and core.SafeRemove (which moves to macOS Trash).
// High-level operations and deletions are logged to logger if non-nil.
func PurgeTargets(targets []PurgeTarget, dryRun bool, logger *core.Logger) (*core.RemovalReport, error) {
	report := &core.RemovalReport{
		Removed: make([]core.RemovedItem, 0),
		Failed:  make([]core.FailedItem, 0),
	}

	if logger != nil {
		mode := "execute"
		if dryRun {
			mode = "dry-run"
		}
		logger.LogOperation("purge", "batch", "start",
			fmt.Sprintf("processing %d targets (%s)", len(targets), mode))
	}

	for _, target := range targets {
		normPath, err := core.NormalizePath(target.Path)
		if err != nil {
			report.Failed = append(report.Failed, core.FailedItem{
				Path:  target.Path,
				Error: err,
			})
			if logger != nil {
				logger.LogOperation("purge", target.Path, "failed", err.Error())
			}
			continue
		}

		// Always run 5-layer safety validation before any operation
		if err := core.ValidatePathForDeletion(normPath); err != nil {
			report.Failed = append(report.Failed, core.FailedItem{
				Path:  normPath,
				Error: err,
			})
			if logger != nil {
				logger.LogOperation("purge", normPath, "blocked", err.Error())
			}
			continue
		}

		if dryRun {
			report.Removed = append(report.Removed, core.RemovedItem{
				Path: normPath,
				Size: target.Size,
			})
			report.TotalBytes += target.Size
			if logger != nil {
				logger.LogOperation("purge", normPath, "dry-run",
					fmt.Sprintf("would reclaim %s", core.FormatBytes(target.Size)))
			}
			continue
		}

		// Perform safe removal (anti-TOCTOU inode check + macOS trash)
		if err := core.SafeRemove(normPath); err != nil {
			report.Failed = append(report.Failed, core.FailedItem{
				Path:  normPath,
				Error: err,
			})
			if logger != nil {
				logger.LogOperation("purge", normPath, "failed", err.Error())
			}
			continue
		}

		trashDir := filepath.Join(core.GetHomeDir(), ".Trash")
		report.Removed = append(report.Removed, core.RemovedItem{
			Path:      normPath,
			Size:      target.Size,
			TrashPath: filepath.Join(trashDir, filepath.Base(normPath)),
		})
		report.TotalBytes += target.Size

		if logger != nil {
			logger.LogDeletion(normPath, target.Size, "trash", "purge")
			logger.LogOperation("purge", normPath, "success",
				fmt.Sprintf("reclaimed %s", core.FormatBytes(target.Size)))
		}
	}

	if logger != nil {
		logger.LogOperation("purge", "batch", "complete",
			fmt.Sprintf("removed %d, failed %d, reclaimed %s",
				len(report.Removed), len(report.Failed), core.FormatBytes(report.TotalBytes)))
	}

	return report, nil
}

// formatPurgeDuration formats a time.Duration into a concise human-readable form (e.g., "7d", "24h").
func formatPurgeDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	days := int(d.Hours() / 24)
	if days > 0 {
		return fmt.Sprintf("%dd", days)
	}
	hours := int(d.Hours())
	if hours > 0 {
		return fmt.Sprintf("%dh", hours)
	}
	mins := int(d.Minutes())
	if mins > 0 {
		return fmt.Sprintf("%dm", mins)
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}
