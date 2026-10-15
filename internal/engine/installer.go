package engine

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SadeepaNHerath/Sift/internal/core"
)

// directInstallerExts contains file extensions for standalone macOS installers and disk images.
var directInstallerExts = map[string]bool{
	".dmg":  true,
	".pkg":  true,
	".mpkg": true,
	".iso":  true,
	".xip":  true,
}

// InstallerFile represents a discovered installer package, disk image, or installer archive.
type InstallerFile struct {
	Path        string
	Name        string
	Extension   string
	Size        int64
	ModTime     time.Time
	InArchive   bool
	ArchivePath string
}

// DefaultInstallerScanDirs returns the default directories checked for installer files.
func DefaultInstallerScanDirs() []string {
	home := core.GetHomeDir()
	if home == "" {
		home, _ = os.UserHomeDir()
	}

	candidates := []string{
		filepath.Join(home, "Downloads"),
		filepath.Join(home, "Desktop"),
		filepath.Join(home, "Documents"),
		filepath.Join(home, "Library", "Caches", "Homebrew"),
		filepath.Join(home, "Library", "Caches", "Homebrew", "downloads"),
		// Apple Mail downloads
		filepath.Join(home, "Library", "Containers", "com.apple.mail", "Data", "Library", "Mail Downloads"),
		filepath.Join(home, "Library", "Mail Downloads"),
		// Telegram downloads
		filepath.Join(home, "Downloads", "Telegram Desktop"),
		filepath.Join(home, "Library", "Group Containers", "7484557A3.group.ph.telegra.Telegraph", "Data", "tmp"),
		filepath.Join(home, "Library", "Containers", "ru.keepcoder.Telegram", "Data", "Downloads"),
		filepath.Join(home, "Library", "Application Support", "Telegram Desktop"),
	}

	var existing []string
	for _, cand := range candidates {
		if fi, err := os.Stat(cand); err == nil && fi.IsDir() {
			existing = append(existing, cand)
		}
	}
	return existing
}

// isZipInstaller inspects a .zip archive and reports whether it contains an application bundle (.app),
// a package (.pkg), or a disk image (.dmg).
func isZipInstaller(zipPath string) (bool, string) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return false, ""
	}
	defer r.Close()

	for _, f := range r.File {
		cleanName := filepath.Clean(f.Name)
		lower := strings.ToLower(cleanName)
		ext := filepath.Ext(lower)

		if ext == ".pkg" || ext == ".dmg" {
			return true, filepath.Base(cleanName)
		}

		parts := strings.Split(filepath.ToSlash(lower), "/")
		for _, part := range parts {
			if strings.HasSuffix(part, ".app") {
				return true, part
			}
		}
	}

	return false, ""
}

// ScanInstallers scans default macOS locations for installer packages, disk images, and installer zip archives.
func ScanInstallers() ([]InstallerFile, error) {
	return ScanInstallersDirs(DefaultInstallerScanDirs())
}

// ScanInstallersDirs scans the provided directory paths for installer packages.
func ScanInstallersDirs(dirs []string) ([]InstallerFile, error) {
	var results []InstallerFile
	seenPaths := make(map[string]bool)

	for _, dir := range dirs {
		normDir, err := core.NormalizePath(dir)
		if err != nil {
			continue
		}

		fi, err := os.Lstat(normDir)
		if err != nil || !fi.IsDir() {
			continue
		}

		_ = filepath.WalkDir(normDir, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}

			name := d.Name()

			if d.IsDir() {
				// Don't skip root dir
				if path == normDir {
					return nil
				}

				// Skip version control and trash
				if name == ".git" || name == ".Trash" || name == "node_modules" {
					return filepath.SkipDir
				}

				// Skip inside app bundles or frameworks
				lowerName := strings.ToLower(name)
				if strings.HasSuffix(lowerName, ".app") || strings.HasSuffix(lowerName, ".framework") {
					return filepath.SkipDir
				}

				// Limit search depth to max 4 levels below scan root
				rel, err := filepath.Rel(normDir, path)
				if err == nil && strings.Count(filepath.ToSlash(rel), "/") >= 4 {
					return filepath.SkipDir
				}
				return nil
			}

			if seenPaths[path] {
				return nil
			}

			lowerName := strings.ToLower(name)
			ext := filepath.Ext(lowerName)

			if directInstallerExts[ext] {
				info, err := d.Info()
				if err != nil {
					return nil
				}
				seenPaths[path] = true
				results = append(results, InstallerFile{
					Path:        path,
					Name:        name,
					Extension:   filepath.Ext(name),
					Size:        info.Size(),
					ModTime:     info.ModTime(),
					InArchive:   false,
					ArchivePath: "",
				})
				return nil
			}

			if ext == ".zip" {
				info, err := d.Info()
				if err != nil {
					return nil
				}
				isInst, _ := isZipInstaller(path)
				if isInst {
					seenPaths[path] = true
					results = append(results, InstallerFile{
						Path:        path,
						Name:        name,
						Extension:   filepath.Ext(name),
						Size:        info.Size(),
						ModTime:     info.ModTime(),
						InArchive:   true,
						ArchivePath: path,
					})
				}
				return nil
			}

			return nil
		})
	}

	return results, nil
}

// RemoveInstallers safely moves the specified installer files to macOS Trash.
// If dryRun is true, no files are removed and a preview report is returned.
// Validates all paths using core.ValidatePathForDeletion and moves them using core.SafeRemove.
func RemoveInstallers(files []InstallerFile, dryRun bool, logger *core.Logger) (*core.RemovalReport, error) {
	report := &core.RemovalReport{
		Removed: make([]core.RemovedItem, 0),
		Failed:  make([]core.FailedItem, 0),
	}

	if logger != nil {
		mode := "execute"
		if dryRun {
			mode = "dry-run"
		}
		logger.LogOperation("installer", "batch", "start",
			fmt.Sprintf("removing %d installer files (%s)", len(files), mode))
	}

	seenPaths := make(map[string]bool)

	for _, file := range files {
		normPath, err := core.NormalizePath(file.Path)
		if err != nil {
			report.Failed = append(report.Failed, core.FailedItem{
				Path:  file.Path,
				Error: err,
			})
			if logger != nil {
				logger.LogOperation("installer", file.Path, "failed", err.Error())
			}
			continue
		}

		if seenPaths[normPath] {
			continue
		}
		seenPaths[normPath] = true

		// Five-layer safety check
		if err := core.ValidatePathForDeletion(normPath); err != nil {
			report.Failed = append(report.Failed, core.FailedItem{
				Path:  normPath,
				Error: err,
			})
			if logger != nil {
				logger.LogOperation("installer", normPath, "blocked", err.Error())
			}
			continue
		}

		if dryRun {
			report.Removed = append(report.Removed, core.RemovedItem{
				Path: normPath,
				Size: file.Size,
			})
			report.TotalBytes += file.Size
			if logger != nil {
				logger.LogOperation("installer", normPath, "dry-run",
					fmt.Sprintf("would remove %s", core.FormatBytes(file.Size)))
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
				logger.LogOperation("installer", normPath, "failed", err.Error())
			}
			continue
		}

		trashDir := filepath.Join(core.GetHomeDir(), ".Trash")
		report.Removed = append(report.Removed, core.RemovedItem{
			Path:      normPath,
			Size:      file.Size,
			TrashPath: filepath.Join(trashDir, filepath.Base(normPath)),
		})
		report.TotalBytes += file.Size

		if logger != nil {
			logger.LogDeletion(normPath, file.Size, "trash", "installer")
			logger.LogOperation("installer", normPath, "success",
				fmt.Sprintf("reclaimed %s", core.FormatBytes(file.Size)))
		}
	}

	if logger != nil {
		logger.LogOperation("installer", "batch", "complete",
			fmt.Sprintf("removed %d, failed %d, reclaimed %s",
				len(report.Removed), len(report.Failed), core.FormatBytes(report.TotalBytes)))
	}

	return report, nil
}
