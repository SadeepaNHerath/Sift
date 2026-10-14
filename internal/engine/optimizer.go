package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/SadeepaNHerath/Spacefold/internal/core"
)

// OptimizeTask represents a single system optimization routine.
type OptimizeTask struct {
	ID           string
	Name         string
	Description  string
	Safe         bool
	RequiresSudo bool
}

// TaskOutcome represents the result of executing an optimization task.
type TaskOutcome struct {
	TaskID  string
	Applied bool
	Details string
	Err     error
}

// TaskCatalog returns the canonical catalog of all supported optimization tasks.
func TaskCatalog() []OptimizeTask {
	return []OptimizeTask{
		{
			ID:           "dns",
			Name:         "Flush macOS DNS Cache",
			Description:  "Flush macOS system DNS cache and reload mDNSResponder daemon",
			Safe:         true,
			RequiresSudo: true,
		},
		{
			ID:           "quicklook",
			Name:         "Reset QuickLook Thumbnail Server",
			Description:  "Reset QuickLook thumbnail cache and reload server daemon",
			Safe:         true,
			RequiresSudo: false,
		},
		{
			ID:           "icons",
			Name:         "Rebuild macOS Icon Services Cache",
			Description:  "Clear corrupted IconServices caches in temporary folders and restart Dock",
			Safe:         true,
			RequiresSudo: false,
		},
		{
			ID:           "sqlite",
			Name:         "Vacuum Safari/Mail SQLite Databases",
			Description:  "Defragment and vacuum Safari and Mail databases (only when apps are closed)",
			Safe:         true,
			RequiresSudo: false,
		},
		{
			ID:           "launchd",
			Name:         "Audit Broken LaunchAgents/LaunchDaemons",
			Description:  "Audit and remove broken LaunchAgents/LaunchDaemons with missing binaries",
			Safe:         true,
			RequiresSudo: false,
		},
		{
			ID:           "dsstore",
			Name:         "Prevent Network & USB .DS_Store Files",
			Description:  "Configure macOS defaults to prevent .DS_Store creation on network and USB volumes",
			Safe:         true,
			RequiresSudo: false,
		},
		{
			ID:           "login_items",
			Name:         "Audit and Prune Broken Login Items",
			Description:  "Audit macOS startup login items and remove entries pointing to missing binaries",
			Safe:         true,
			RequiresSudo: false,
		},
	}
}

// Variables for test interception.
var (
	launchdCustomDirs []string
	sqliteCustomDirs  []string
)

// RunTask executes the requested optimization task by ID.
// If dryRun is true, reports what would be done without modifying the system.
func RunTask(taskID string, dryRun bool, logger *core.Logger) (TaskOutcome, error) {
	switch taskID {
	case "dns":
		return runTaskDNS(dryRun, logger)
	case "quicklook":
		return runTaskQuickLook(dryRun, logger)
	case "icons":
		return runTaskIcons(dryRun, logger)
	case "sqlite":
		return runTaskSQLite(dryRun, logger)
	case "launchd":
		return runTaskLaunchd(dryRun, logger)
	case "dsstore":
		return runTaskDSStore(dryRun, logger)
	case "login_items":
		return runTaskLoginItems(dryRun, logger)
	default:
		return TaskOutcome{
			TaskID:  taskID,
			Applied: false,
			Details: fmt.Sprintf("Unknown optimization task ID: %q", taskID),
			Err:     fmt.Errorf("unknown task ID: %q", taskID),
		}, fmt.Errorf("unknown task ID: %q", taskID)
	}
}

// runTaskDNS flushes macOS DNS cache using dscacheutil and signals mDNSResponder.
func runTaskDNS(dryRun bool, logger *core.Logger) (TaskOutcome, error) {
	const taskID = "dns"
	if dryRun {
		return TaskOutcome{
			TaskID:  taskID,
			Applied: false,
			Details: "Would flush DNS cache (dscacheutil -flushcache) and reload mDNSResponder daemon (killall -HUP mDNSResponder)",
		}, nil
	}

	// 1. Flush DNS cache
	flushCmd := exec.Command("dscacheutil", "-flushcache")
	if out, err := flushCmd.CombinedOutput(); err != nil {
		// Log warning but continue to mDNSResponder restart if possible
		_ = out
	}

	// 2. Restart mDNSResponder
	killCmd := exec.Command("killall", "-HUP", "mDNSResponder")
	if err := killCmd.Run(); err != nil {
		// If running as regular user, try sudo if available
		sudoKill := exec.Command("sudo", "killall", "-HUP", "mDNSResponder")
		_ = sudoKill.Run()
	}

	details := "Flushed macOS DNS cache and reloaded mDNSResponder daemon"
	if logger != nil {
		logger.LogOperation("optimize", taskID, "success", details)
	}

	return TaskOutcome{
		TaskID:  taskID,
		Applied: true,
		Details: details,
	}, nil
}

// runTaskQuickLook resets the QuickLook thumbnail server and cache.
func runTaskQuickLook(dryRun bool, logger *core.Logger) (TaskOutcome, error) {
	const taskID = "quicklook"
	if dryRun {
		return TaskOutcome{
			TaskID:  taskID,
			Applied: false,
			Details: "Would reset QuickLook thumbnail cache (qlmanage -r cache) and restart server (qlmanage -r)",
		}, nil
	}

	// 1. Reset thumbnail cache
	cmdCache := exec.Command("qlmanage", "-r", "cache")
	_ = cmdCache.Run()

	// 2. Reset QuickLook server
	cmdServer := exec.Command("qlmanage", "-r")
	_ = cmdServer.Run()

	details := "Reset QuickLook thumbnail cache and reloaded server daemon"
	if logger != nil {
		logger.LogOperation("optimize", taskID, "success", details)
	}

	return TaskOutcome{
		TaskID:  taskID,
		Applied: true,
		Details: details,
	}, nil
}

// runTaskIcons rebuilds the macOS Icon Services Cache.
func runTaskIcons(dryRun bool, logger *core.Logger) (TaskOutcome, error) {
	const taskID = "icons"
	if dryRun {
		return TaskOutcome{
			TaskID:  taskID,
			Applied: false,
			Details: "Would clear IconServices caches in temporary directories and restart Dock",
		}, nil
	}

	removedCount := 0

	// Locate IconServices cache directories in $TMPDIR and subpaths
	tmpDir := filepath.Clean(os.TempDir())
	matches, err := filepath.Glob(filepath.Join(tmpDir, "com.apple.iconservices*"))
	if err == nil {
		for _, m := range matches {
			base := filepath.Base(m)
			if !strings.HasPrefix(base, "com.apple.iconservices") {
				continue
			}
			// Verify it is not a symlink to an outside directory
			if fi, lerr := os.Lstat(m); lerr == nil && fi.Mode()&os.ModeSymlink == 0 {
				if err := os.RemoveAll(m); err == nil {
					removedCount++
				}
			}
		}
	}

	// Also check Darwin C cache folder if accessible: $TMPDIR/../C/com.apple.iconservices*
	parentC := filepath.Clean(filepath.Join(tmpDir, "..", "C"))
	cMatches, err := filepath.Glob(filepath.Join(parentC, "com.apple.iconservices*"))
	if err == nil {
		for _, m := range cMatches {
			base := filepath.Base(m)
			if !strings.HasPrefix(base, "com.apple.iconservices") {
				continue
			}
			if fi, lerr := os.Lstat(m); lerr == nil && fi.Mode()&os.ModeSymlink == 0 {
				if err := os.RemoveAll(m); err == nil {
					removedCount++
				}
			}
		}
	}

	// Restart Dock to reload icon cache
	_ = exec.Command("killall", "Dock").Run()

	details := fmt.Sprintf("Cleared %d IconServices cache directories and restarted Dock", removedCount)
	if logger != nil {
		logger.LogOperation("optimize", taskID, "success", details)
	}

	return TaskOutcome{
		TaskID:  taskID,
		Applied: true,
		Details: details,
	}, nil
}

// isProcessRunning checks if an application is currently running by process name.
func isProcessRunning(name string) bool {
	cmd := exec.Command("pgrep", "-x", name)
	return cmd.Run() == nil
}

// runTaskSQLite defragments and vacuums Safari and Mail SQLite databases.
func runTaskSQLite(dryRun bool, logger *core.Logger) (TaskOutcome, error) {
	const taskID = "sqlite"

	home := core.GetHomeDir()
	if home == "" {
		home, _ = os.UserHomeDir()
	}

	var searchPaths []string
	if len(sqliteCustomDirs) > 0 {
		for _, dir := range sqliteCustomDirs {
			matches, _ := filepath.Glob(filepath.Join(dir, "*.db"))
			searchPaths = append(searchPaths, matches...)
		}
	} else {
		// Safari databases
		safariRunning := isProcessRunning("Safari")
		if !safariRunning {
			safariDBs := []string{
				filepath.Join(home, "Library", "Safari", "History.db"),
				filepath.Join(home, "Library", "Safari", "CloudHistory.db"),
				filepath.Join(home, "Library", "Safari", "FaviconCache.db"),
				filepath.Join(home, "Library", "Safari", "ReadingList.db"),
			}
			for _, db := range safariDBs {
				if fi, err := os.Stat(db); err == nil && !fi.IsDir() && fi.Size() > 0 {
					searchPaths = append(searchPaths, db)
				}
			}
		}

		// Mail databases
		mailRunning := isProcessRunning("Mail")
		if !mailRunning {
			mailGlob := filepath.Join(home, "Library", "Mail", "V*", "MailData", "Envelope Index*")
			if matches, err := filepath.Glob(mailGlob); err == nil {
				for _, db := range matches {
					if fi, err := os.Stat(db); err == nil && !fi.IsDir() && fi.Size() > 0 {
						searchPaths = append(searchPaths, db)
					}
				}
			}
		}
	}

	if dryRun {
		return TaskOutcome{
			TaskID:  taskID,
			Applied: false,
			Details: fmt.Sprintf("Would vacuum %d SQLite databases (Safari running: %t, Mail running: %t)",
				len(searchPaths), isProcessRunning("Safari"), isProcessRunning("Mail")),
		}, nil
	}

	vacuumedCount := 0
	for _, db := range searchPaths {
		// Execute sqlite3 <db> "VACUUM;"
		cmd := exec.Command("sqlite3", db, "VACUUM;")
		if err := cmd.Run(); err == nil {
			vacuumedCount++
		}
	}

	details := fmt.Sprintf("Successfully vacuumed %d SQLite databases", vacuumedCount)
	if logger != nil {
		logger.LogOperation("optimize", taskID, "success", details)
	}

	return TaskOutcome{
		TaskID:  taskID,
		Applied: true,
		Details: details,
	}, nil
}

// extractPlistExecutable extracts the binary target of a launchd plist file.
func extractPlistExecutable(plistPath string) (string, error) {
	// Attempt fast conversion to JSON via plutil
	out, err := exec.Command("plutil", "-convert", "json", "-o", "-", plistPath).Output()
	if err == nil {
		var data struct {
			Program          string   `json:"Program"`
			ProgramArguments []string `json:"ProgramArguments"`
		}
		if err := json.Unmarshal(out, &data); err == nil {
			if strings.TrimSpace(data.Program) != "" {
				return strings.TrimSpace(data.Program), nil
			}
			if len(data.ProgramArguments) > 0 && strings.TrimSpace(data.ProgramArguments[0]) != "" {
				return strings.TrimSpace(data.ProgramArguments[0]), nil
			}
		}
	}

	// Fallback to text reading for XML plists
	content, err := os.ReadFile(plistPath)
	if err != nil {
		return "", err
	}

	reProg := regexp.MustCompile(`<key>Program</key>\s*<string>([^<]+)</string>`)
	if m := reProg.FindSubmatch(content); len(m) > 1 {
		return strings.TrimSpace(string(m[1])), nil
	}

	reArgs := regexp.MustCompile(`<key>ProgramArguments</key>\s*<array>\s*<string>([^<]+)</string>`)
	if m := reArgs.FindSubmatch(content); len(m) > 1 {
		return strings.TrimSpace(string(m[1])), nil
	}

	return "", errors.New("no Program or ProgramArguments found in plist")
}

// isExecutableMissing checks whether the specified binary path exists.
func isExecutableMissing(target string) bool {
	if target == "" {
		return false
	}

	// Absolute path
	if filepath.IsAbs(target) {
		_, err := os.Stat(target)
		return errors.Is(err, os.ErrNotExist)
	}

	// Relative or binary name on PATH
	_, err := exec.LookPath(target)
	return err != nil
}

// runTaskLaunchd audits and removes broken LaunchAgents/LaunchDaemons.
func runTaskLaunchd(dryRun bool, logger *core.Logger) (TaskOutcome, error) {
	const taskID = "launchd"

	dirs := launchdCustomDirs
	if len(dirs) == 0 {
		home := core.GetHomeDir()
		if home == "" {
			home, _ = os.UserHomeDir()
		}
		dirs = []string{
			filepath.Join(home, "Library", "LaunchAgents"),
			"/Library/LaunchAgents",
			"/Library/LaunchDaemons",
		}
	}

	var brokenPlists []string
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".plist") {
				continue
			}

			plistPath := filepath.Join(dir, entry.Name())
			execPath, err := extractPlistExecutable(plistPath)
			if err != nil {
				continue
			}

			if isExecutableMissing(execPath) {
				brokenPlists = append(brokenPlists, plistPath)
			}
		}
	}

	if dryRun {
		return TaskOutcome{
			TaskID:  taskID,
			Applied: false,
			Details: fmt.Sprintf("Audited launch services: found %d broken LaunchAgents/LaunchDaemons", len(brokenPlists)),
		}, nil
	}

	removedCount := 0
	for _, p := range brokenPlists {
		// Attempt to unload service before removal
		_ = exec.Command("launchctl", "unload", p).Run()

		fi, err := os.Stat(p)
		size := int64(0)
		if err == nil {
			size = fi.Size()
		}

		if err := core.SafeRemove(p); err == nil {
			removedCount++
			if logger != nil {
				logger.LogDeletion(p, size, "trash", "optimize:launchd")
			}
		} else if logger != nil {
			logger.LogOperation("optimize", "launchd:blocked", "warning", fmt.Sprintf("Failed to safely remove broken plist %s: %v", p, err))
		}
	}

	details := fmt.Sprintf("Audited launch services: removed %d broken LaunchAgents/LaunchDaemons", removedCount)
	if logger != nil {
		logger.LogOperation("optimize", taskID, "success", details)
	}

	return TaskOutcome{
		TaskID:  taskID,
		Applied: true,
		Details: details,
	}, nil
}

// runTaskDSStore prevents .DS_Store file creation on Network and USB volumes.
func runTaskDSStore(dryRun bool, logger *core.Logger) (TaskOutcome, error) {
	const taskID = "dsstore"
	if dryRun {
		return TaskOutcome{
			TaskID:  taskID,
			Applied: false,
			Details: "Would configure defaults: DSDontWriteNetworkStores=true, DSDontWriteUSBStores=true",
		}, nil
	}

	cmdNet := exec.Command("defaults", "write", "com.apple.desktopservices", "DSDontWriteNetworkStores", "-bool", "true")
	_ = cmdNet.Run()

	cmdUSB := exec.Command("defaults", "write", "com.apple.desktopservices", "DSDontWriteUSBStores", "-bool", "true")
	_ = cmdUSB.Run()

	details := "Configured macOS defaults to prevent .DS_Store creation on Network and USB volumes"
	if logger != nil {
		logger.LogOperation("optimize", taskID, "success", details)
	}

	return TaskOutcome{
		TaskID:  taskID,
		Applied: true,
		Details: details,
	}, nil
}

// brokenLoginItem tracks a detected broken login item.
type brokenLoginItem struct {
	Name string
	Path string
}

// auditLoginItems queries macOS System Events for broken login items.
func auditLoginItems() ([]brokenLoginItem, error) {
	// First check count of login items
	scriptCount := `tell application "System Events" to return count of login items`
	outCount, err := exec.Command("osascript", "-e", scriptCount).Output()
	if err != nil {
		return nil, err
	}

	countStr := strings.TrimSpace(string(outCount))
	count, err := strconv.Atoi(countStr)
	if err != nil || count == 0 {
		return nil, nil
	}

	var broken []brokenLoginItem
	for i := 1; i <= count; i++ {
		script := fmt.Sprintf(`tell application "System Events"
			set itm to login item %d
			return (name of itm) & "|||" & (path of itm)
		end tell`, i)

		out, err := exec.Command("osascript", "-e", script).Output()
		if err != nil {
			continue
		}

		parts := strings.Split(strings.TrimSpace(string(out)), "|||")
		if len(parts) >= 2 {
			name := strings.TrimSpace(parts[0])
			p := strings.TrimSpace(parts[1])

			if p != "" {
				if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
					broken = append(broken, brokenLoginItem{Name: name, Path: p})
				}
			}
		}
	}

	return broken, nil
}

// runTaskLoginItems audits and prunes broken Login Items.
func runTaskLoginItems(dryRun bool, logger *core.Logger) (TaskOutcome, error) {
	const taskID = "login_items"

	broken, err := auditLoginItems()
	if err != nil {
		// Non-fatal if AppleScript / System Events is unavailable or denied TCC
		details := "Audited login items (no broken items detected or AppleScript unavailable)"
		return TaskOutcome{
			TaskID:  taskID,
			Applied: !dryRun,
			Details: details,
		}, nil
	}

	if dryRun {
		return TaskOutcome{
			TaskID:  taskID,
			Applied: false,
			Details: fmt.Sprintf("Audited login items: found %d broken startup items", len(broken)),
		}, nil
	}

	prunedCount := 0
	for _, item := range broken {
		script := fmt.Sprintf(`tell application "System Events"
			delete (first login item whose name is %q)
		end tell`, item.Name)

		if err := exec.Command("osascript", "-e", script).Run(); err == nil {
			prunedCount++
		}
	}

	details := fmt.Sprintf("Audited login items: pruned %d broken startup items", prunedCount)
	if logger != nil {
		logger.LogOperation("optimize", taskID, "success", details)
	}

	return TaskOutcome{
		TaskID:  taskID,
		Applied: true,
		Details: details,
	}, nil
}
