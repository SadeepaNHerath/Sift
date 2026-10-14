package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// ConfigDir returns the XDG-compliant configuration directory for Sift (~/.config/sf/).
func ConfigDir() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "sf")
	}
	return filepath.Join(GetHomeDir(), ".config", "sf")
}

// CacheDir returns the XDG-compliant cache directory for Sift (~/.cache/sf/).
func CacheDir() string {
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return filepath.Join(dir, "sf")
	}
	return filepath.Join(GetHomeDir(), ".cache", "sf")
}

// LogDir returns the standard macOS log directory for Sift (~/Library/Logs/sf/).
func LogDir() string {
	if dir := os.Getenv("SF_LOG_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(GetHomeDir(), "Library", "Logs", "sf")
}

// EnsureDirs creates all required directories with appropriate permissions.
// Directory permissions:
// - Config directory: 0755
// - Cache directory: 0755
// - Log directory: 0700 (strict permissions for audit integrity)
func EnsureDirs() error {
	dirs := []struct {
		path string
		perm os.FileMode
	}{
		{path: ConfigDir(), perm: 0755},
		{path: CacheDir(), perm: 0755},
		{path: LogDir(), perm: 0700},
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d.path, d.perm); err != nil {
			return fmt.Errorf("failed to create directory %q: %w", d.path, err)
		}
	}
	return nil
}

// Whitelist manages protected paths for sf clean.
type Whitelist struct {
	mu    sync.RWMutex
	paths []string
}

// whitelistFile returns the path to the whitelist configuration file.
func whitelistFile() string {
	return filepath.Join(ConfigDir(), "whitelist.json")
}

// LoadWhitelist loads the whitelist from disk, or returns an empty whitelist if none exists.
func LoadWhitelist() (*Whitelist, error) {
	w := &Whitelist{paths: make([]string, 0)}
	data, err := os.ReadFile(whitelistFile())
	if err != nil {
		if os.IsNotExist(err) {
			return w, nil
		}
		return nil, fmt.Errorf("failed to read whitelist file: %w", err)
	}

	// Try loading as []string first, then struct if wrapped
	if err := json.Unmarshal(data, &w.paths); err != nil {
		var wrapper struct {
			Paths []string `json:"paths"`
		}
		if unwrapErr := json.Unmarshal(data, &wrapper); unwrapErr != nil {
			return nil, fmt.Errorf("failed to parse whitelist json: %w", err)
		}
		w.paths = wrapper.Paths
	}

	return w, nil
}

// writeAtomicFile writes data to a temporary file in the same directory, syncs, and renames it atomically.
func writeAtomicFile(targetPath string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(targetPath)
	tmpFile, err := os.CreateTemp(dir, ".sf_atomic_*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return err
	}
	if err := tmpFile.Chmod(perm); err != nil {
		tmpFile.Close()
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	return os.Rename(tmpName, targetPath)
}

// Contains checks if the given path is in the whitelist or is protected by a whitelisted ancestor directory.
// On macOS APFS, comparisons are case-insensitive.
func (w *Whitelist) Contains(path string) bool {
	if w == nil {
		return false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()

	cleanTarget := filepath.Clean(path)
	for _, p := range w.paths {
		cleanP := filepath.Clean(p)
		if strings.EqualFold(cleanTarget, cleanP) {
			return true
		}
		// If cleanP is an ancestor of cleanTarget, path is protected
		if cleanP != "/" {
			prefix := cleanP + string(filepath.Separator)
			if len(cleanTarget) > len(prefix) && strings.EqualFold(cleanTarget[:len(prefix)], prefix) {
				return true
			}
		}
	}
	return false
}

// Add appends a new path to the whitelist if not already present.
func (w *Whitelist) Add(path string) error {
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	if cleanPath == "" || cleanPath == "." {
		return fmt.Errorf("invalid path: %q", path)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	for _, p := range w.paths {
		if strings.EqualFold(filepath.Clean(p), cleanPath) {
			return nil // already whitelisted
		}
	}

	w.paths = append(w.paths, cleanPath)
	return nil
}

// Remove deletes a path from the whitelist.
func (w *Whitelist) Remove(path string) error {
	cleanPath := filepath.Clean(strings.TrimSpace(path))

	w.mu.Lock()
	defer w.mu.Unlock()

	idx := -1
	for i, p := range w.paths {
		if strings.EqualFold(filepath.Clean(p), cleanPath) {
			idx = i
			break
		}
	}

	if idx == -1 {
		return fmt.Errorf("path %q not found in whitelist", path)
	}

	w.paths = append(w.paths[:idx], w.paths[idx+1:]...)
	return nil
}

// Save writes the whitelist to disk with secure permissions (0600) using atomic replacement.
func (w *Whitelist) Save() error {
	if err := EnsureDirs(); err != nil {
		return err
	}

	w.mu.RLock()
	paths := make([]string, len(w.paths))
	copy(paths, w.paths)
	w.mu.RUnlock()

	data, err := json.MarshalIndent(paths, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode whitelist: %w", err)
	}

	if err := writeAtomicFile(whitelistFile(), data, 0600); err != nil {
		return fmt.Errorf("failed to save whitelist file: %w", err)
	}
	return nil
}

// List returns a copy of all whitelisted paths.
func (w *Whitelist) List() []string {
	if w == nil {
		return nil
	}
	w.mu.RLock()
	defer w.mu.RUnlock()

	res := make([]string, len(w.paths))
	copy(res, w.paths)
	return res
}

// Preferences stores user settings for Sift.
type Preferences struct {
	ShowCat     bool   `json:"show_cat"`
	CPUCores    int    `json:"cpu_cores"`
	ColorScheme string `json:"color_scheme"`
}

// preferencesFile returns the path to preferences.json.
func preferencesFile() string {
	return filepath.Join(ConfigDir(), "preferences.json")
}

// defaultPreferences creates a Preferences struct with sane defaults.
func defaultPreferences() *Preferences {
	cores := runtime.NumCPU()
	if cores < 1 {
		cores = 1
	}
	return &Preferences{
		ShowCat:     true,
		CPUCores:    cores,
		ColorScheme: "default",
	}
}

// LoadPreferences loads user settings from disk or returns defaults if not found.
func LoadPreferences() (*Preferences, error) {
	prefs := defaultPreferences()

	targetFile := preferencesFile()
	data, err := os.ReadFile(targetFile)
	if err != nil {
		// Fallback check for config.json if preferences.json is missing
		altFile := filepath.Join(ConfigDir(), "config.json")
		var altErr error
		data, altErr = os.ReadFile(altFile)
		if altErr != nil {
			if os.IsNotExist(err) && os.IsNotExist(altErr) {
				return prefs, nil
			}
			return nil, fmt.Errorf("failed to read preferences file: %w", err)
		}
	}

	if err := json.Unmarshal(data, prefs); err != nil {
		return nil, fmt.Errorf("failed to parse preferences: %w", err)
	}

	if prefs.CPUCores <= 0 {
		prefs.CPUCores = runtime.NumCPU()
	}
	if prefs.ColorScheme == "" {
		prefs.ColorScheme = "default"
	}

	return prefs, nil
}

// Save writes user settings to disk using atomic replacement.
func (p *Preferences) Save() error {
	if err := EnsureDirs(); err != nil {
		return err
	}

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode preferences: %w", err)
	}

	if err := writeAtomicFile(preferencesFile(), data, 0644); err != nil {
		return fmt.Errorf("failed to save preferences file: %w", err)
	}
	return nil
}

// purgePathsFile returns the path to purge_paths.json.
func purgePathsFile() string {
	return filepath.Join(ConfigDir(), "purge_paths.json")
}

// DefaultPurgePaths returns standard development project root directories on macOS:
// ~/Projects, ~/dev, ~/GitHub, ~/Code, ~/repos.
func DefaultPurgePaths() []string {
	home := GetHomeDir()
	return []string{
		filepath.Join(home, "Projects"),
		filepath.Join(home, "dev"),
		filepath.Join(home, "GitHub"),
		filepath.Join(home, "Code"),
		filepath.Join(home, "repos"),
	}
}

// LoadPurgePaths loads configured project root paths from disk, or returns defaults.
func LoadPurgePaths() ([]string, error) {
	data, err := os.ReadFile(purgePathsFile())
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultPurgePaths(), nil
		}
		return nil, fmt.Errorf("failed to read purge paths file: %w", err)
	}

	var paths []string
	if err := json.Unmarshal(data, &paths); err != nil {
		return nil, fmt.Errorf("failed to parse purge paths: %w", err)
	}
	return paths, nil
}

// SavePurgePaths saves custom project root paths to disk using atomic replacement.
func SavePurgePaths(paths []string) error {
	if err := EnsureDirs(); err != nil {
		return err
	}

	data, err := json.MarshalIndent(paths, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode purge paths: %w", err)
	}

	if err := writeAtomicFile(purgePathsFile(), data, 0644); err != nil {
		return fmt.Errorf("failed to save purge paths file: %w", err)
	}
	return nil
}
