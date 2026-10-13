package engine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/SadeepaNHerath/Spacefold/internal/core"
)

// PrivacyTarget represents a category of forensic or privacy-sensitive data on macOS.
type PrivacyTarget struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Description string   `json:"description"`
	Paths       []string `json:"paths"`
	Size        int64    `json:"size"`
	ItemCount   int      `json:"item_count"`
	IsClipboard bool     `json:"is_clipboard"`
}

// ScanPrivacyTargets audits macOS recent items, QuickLook databases, clipboard, diagnostic crash logs, and browser local storage.
func ScanPrivacyTargets() ([]PrivacyTarget, error) {
	home := core.GetHomeDir()

	type rule struct {
		id          string
		name        string
		category    string
		description string
		patterns    []string
		isClipboard bool
	}

	rules := []rule{
		{
			id:          "recent-items",
			name:        "macOS Recent Items & QuickLook History",
			category:    "Recent Items",
			description: "Recent documents, applications, and server connections (.sfl3 / SharedFileList)",
			patterns: []string{
				filepath.Join(home, "Library", "Application Support", "com.apple.sharedfilelist", "*.sfl*"),
				filepath.Join(home, "Library", "Preferences", "com.apple.recentitems.plist"),
				filepath.Join(home, "Library", "Caches", "com.apple.QuickLook.thumbnailcache"),
				filepath.Join(home, "Library", "Caches", "QuickLook"),
			},
		},
		{
			id:          "clipboard",
			name:        "Terminal & System Clipboard",
			category:    "Clipboard",
			description: "Active system pasteboard buffer containing potentially sensitive copied tokens or passwords",
			patterns:    nil,
			isClipboard: true,
		},
		{
			id:          "diagnostics",
			name:        "Diagnostic Reports & Crash Reporter",
			category:    "Diagnostics",
			description: "Forensic crash dumps and system diagnostic logs revealing app usage patterns",
			patterns: []string{
				filepath.Join(home, "Library", "Logs", "DiagnosticReports", "*"),
				filepath.Join(home, "Library", "Logs", "CrashReporter", "*"),
				"/Library/Logs/DiagnosticReports/*",
				"/Library/Logs/CrashReporter/*",
			},
		},
		{
			id:          "browser-storage",
			name:        "Browser Local Storage & Session State",
			category:    "Browser Storage",
			description: "Web application local databases and session caches (preserves bookmarks, passwords, and logins)",
			patterns: []string{
				// Safari
				filepath.Join(home, "Library", "Safari", "LocalStorage", "*"),
				filepath.Join(home, "Library", "Safari", "Databases", "*"),
				filepath.Join(home, "Library", "Containers", "com.apple.Safari", "Data", "Library", "Caches", "*"),
				// Google Chrome
				filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "Default", "Local Storage", "leveldb", "*"),
				filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "Default", "Session Storage", "*"),
				// Brave
				filepath.Join(home, "Library", "Application Support", "BraveSoftware", "Brave-Browser", "Default", "Local Storage", "leveldb", "*"),
				filepath.Join(home, "Library", "Application Support", "BraveSoftware", "Brave-Browser", "Default", "Session Storage", "*"),
				// Arc
				filepath.Join(home, "Library", "Application Support", "Arc", "User Data", "Default", "Local Storage", "leveldb", "*"),
				filepath.Join(home, "Library", "Application Support", "Arc", "User Data", "Default", "Session Storage", "*"),
			},
		},
	}

	var results []PrivacyTarget

	for _, r := range rules {
		target := PrivacyTarget{
			ID:          r.id,
			Name:        r.name,
			Category:    r.category,
			Description: r.description,
			IsClipboard: r.isClipboard,
		}

		if r.isClipboard {
			// Clipboard is dynamic; assume 1 unit
			target.ItemCount = 1
			results = append(results, target)
			continue
		}

		seen := make(map[string]bool)
		for _, pat := range r.patterns {
			matches := expandWildcard(pat)
			for _, m := range matches {
				if seen[m] {
					continue
				}
				seen[m] = true

				fi, err := os.Lstat(m)
				if err != nil {
					continue
				}

				sz, err := core.FileSize(m)
				if err != nil {
					sz = 0
				}
				if sz == 0 && fi.Mode().IsRegular() {
					sz = fi.Size()
				}

				target.Paths = append(target.Paths, m)
				target.Size += sz
				target.ItemCount++
			}
		}

		results = append(results, target)
	}

	return results, nil
}

// WipePrivacyTargets executes forensic wiping across selected privacy targets.
func WipePrivacyTargets(targets []PrivacyTarget, dryRun bool, logger *core.Logger) (*core.RemovalReport, error) {
	report := &core.RemovalReport{
		Removed: make([]core.RemovedItem, 0),
		Failed:  make([]core.FailedItem, 0),
	}

	for _, tgt := range targets {
		if tgt.IsClipboard {
			if dryRun {
				report.Removed = append(report.Removed, core.RemovedItem{
					Path:      "System Pasteboard Buffer (pbcopy)",
					Size:      0,
					TrashPath: "[dry-run]",
				})
			} else {
				// Flush general pasteboard
				cmd := exec.Command("pbcopy")
				cmd.Stdin = strings.NewReader("")
				_ = cmd.Run()
				report.Removed = append(report.Removed, core.RemovedItem{
					Path:      "System Pasteboard Buffer (pbcopy)",
					Size:      0,
					TrashPath: "[cleared]",
				})
			}
			continue
		}

		if dryRun {
			for _, p := range tgt.Paths {
				sz, _ := core.FileSize(p)
				report.Removed = append(report.Removed, core.RemovedItem{
					Path:      p,
					Size:      sz,
					TrashPath: "[dry-run]",
				})
				report.TotalBytes += sz
			}
			continue
		}

		// Actual removal
		for _, p := range tgt.Paths {
			sz, _ := core.FileSize(p)
			err := wipeSinglePrivacyPath(p)
			if err != nil {
				report.Failed = append(report.Failed, core.FailedItem{
					Path:  p,
					Error: err,
				})
				continue
			}

			report.Removed = append(report.Removed, core.RemovedItem{
				Path:      p,
				Size:      sz,
				TrashPath: "[trashed]",
			})
			report.TotalBytes += sz

			if logger != nil {
				logger.LogDeletion(p, sz, "trash", fmt.Sprintf("sf privacy (%s)", tgt.ID))
			}
		}
	}

	if logger != nil {
		action := "execute"
		if dryRun {
			action = "dry-run"
		}
		logger.LogOperation("privacy", action, "success",
			fmt.Sprintf("Wiped %d items, reclaimed %s", len(report.Removed), core.FormatBytes(report.TotalBytes)))
	}

	return report, nil
}

// wipeSinglePrivacyPath validates safe boundaries and moves forensic items to Trash.
func wipeSinglePrivacyPath(p string) error {
	norm, err := core.NormalizePath(p)
	if err != nil {
		return err
	}
	if norm == "/" || norm == core.GetHomeDir() {
		return core.ErrHomeDirectoryProtected
	}
	if core.IsSystemPath(norm) {
		return core.ErrSystemProtectedPath
	}
	if strings.Contains(norm, "/.ssh") || strings.Contains(norm, "/.gnupg") || strings.Contains(norm, "/Library/Keychains") {
		return core.ErrSensitiveUserPath
	}
	return core.MoveToTrash(norm)
}

