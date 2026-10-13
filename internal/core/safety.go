package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode"
)

// Sentinel safety errors returned by path validation and inode verification.
var (
	// ErrEmptyPath is returned when a path is empty or contains only whitespace.
	ErrEmptyPath = errors.New("path is empty")

	// ErrNotAbsolutePath is returned when a path is not absolute.
	ErrNotAbsolutePath = errors.New("path is not absolute")

	// ErrPathTraversal is returned when a path contains traversal components (..).
	ErrPathTraversal = errors.New("path contains traversal components")

	// ErrControlCharacters is returned when a path contains control characters.
	ErrControlCharacters = errors.New("path contains control characters")

	// ErrSystemProtectedPath is returned when a path is within the system denylist.
	ErrSystemProtectedPath = errors.New("path is system-protected")

	// ErrHomeDirectoryProtected is returned when attempting to delete the user's home directory.
	ErrHomeDirectoryProtected = errors.New("user home directory cannot be deleted")

	// ErrSensitiveUserPath is returned when a path contains protected user configuration or data.
	ErrSensitiveUserPath = errors.New("path contains sensitive user data")

	// ErrAppleBundleProtected is returned when a bundle identifier or path corresponds to Apple components.
	ErrAppleBundleProtected = errors.New("apple system component or bundle ID is protected")

	// ErrSymlinkTargetProtected is returned when a symlink target resides in a protected area.
	ErrSymlinkTargetProtected = errors.New("symlink target is in a protected area")

	// ErrAncestorSymlinkProtected is returned when an ancestor symlink resolves to a protected area.
	ErrAncestorSymlinkProtected = errors.New("ancestor symlink points to a protected area")

	// ErrInodeMismatch is returned when an inode or device changes between verification steps (anti-TOCTOU).
	ErrInodeMismatch = errors.New("inode or device mismatch: file changed during operation")

	// ErrUserWhitelisted is returned when a path is explicitly protected by the user's whitelist.
	ErrUserWhitelisted = errors.New("path is protected by user whitelist")
)

// systemProtectedPrefixes contains macOS system directory trees that must NEVER be deleted.
var systemProtectedPrefixes = []string{
	"/System",
	"/usr/bin",
	"/usr/sbin",
	"/usr/lib",
	"/usr/libexec",
	"/bin",
	"/sbin",
	"/Library/Apple",
	"/Library/Extensions",
	"/Library/Frameworks",
	"/Library/SystemExtensions",
	"/Library/Keychains",
	"/Library/Security",
	"/Library/OSAnalytics",
	"/Library/Managed Preferences",
	"/Library/Preferences/SystemConfiguration",
	"/Library/Filesystems",
	"/Library/Updates",
	"/Library/User Template",
	"/Library/WebServer",
	"/private/var/db",
	"/private/var/protected",
	"/var/db",
	"/var/protected",
	"/cores",
	"/dev",
	"/net",
	"/private/etc",
	"/etc",
	"/usr/local",
}

// systemExactRoots contains shared filesystem root directories that must NEVER be deleted themselves.
var systemExactRoots = []string{
	"/",
	"/Applications",
	"/Library",
	"/Library/Preferences",
	"/Library/Application Support",
	"/Library/Caches",
	"/System",
	"/Users",
	"/Volumes",
	"/usr",
	"/private",
	"/private/var",
	"/var",
	"/tmp",
	"/private/tmp",
}

// sensitiveUserRecursivePrefixes contains directory paths within ~ that are recursively protected.
var sensitiveUserRecursivePrefixes = []string{
	".ssh",
	".gnupg",
	".gpg",
	"Library/Keychains",
	"Library/Mail",
	"Library/Messages",
	"Library/Safari",
	"Library/Accounts",
	"Library/Cookies",
	"Library/Calendars",
	"Library/Reminders",
	"Library/IdentityServices",
	"Library/Application Support/AddressBook",
}

// sensitiveUserFiles contains user dotfiles that must never be deleted.
var sensitiveUserFiles = []string{
	".gitconfig",
	".zshrc",
	".bashrc",
	".bash_profile",
	".profile",
	".zshenv",
	".zprofile",
}

// sensitiveUserExactDirs contains standard user directories where the directory itself is protected,
// but its contents may be cleaned if not otherwise protected.
var sensitiveUserExactDirs = []string{
	"Desktop",
	"Documents",
	"Pictures",
	"Movies",
	"Music",
	"Downloads",
	"Library",
	"Library/Preferences",
	"Library/Application Support",
	"Library/Caches",
	"Library/Containers",
	"Library/Group Containers",
}

// InodeRef binds a file or directory path to its physical device and inode numbers
// to detect time-of-check to time-of-use (TOCTOU) file replacement attacks.
type InodeRef struct {
	Device uint64
	Inode  uint64
	Path   string
}

// BindInode stats the target path and records its device and inode numbers.
func BindInode(path string) (InodeRef, error) {
	if strings.TrimSpace(path) == "" {
		return InodeRef{}, ErrEmptyPath
	}
	cleanPath := filepath.Clean(path)
	fi, err := os.Lstat(cleanPath)
	if err != nil {
		return InodeRef{}, fmt.Errorf("failed to stat file for inode binding: %w", err)
	}

	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return InodeRef{}, errors.New("failed to retrieve stat information")
	}

	return InodeRef{
		Device: uint64(stat.Dev),
		Inode:  stat.Ino,
		Path:   cleanPath,
	}, nil
}

// Verify re-stats the bound path and verifies that the physical device and inode
// match the initially recorded values.
func (ref InodeRef) Verify() error {
	if ref.Path == "" {
		return errors.New("empty path in inode ref")
	}

	fi, err := os.Lstat(ref.Path)
	if err != nil {
		return fmt.Errorf("failed to re-stat file for inode verification: %w", err)
	}

	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("failed to retrieve stat information")
	}

	if uint64(stat.Dev) != ref.Device || stat.Ino != ref.Inode {
		return ErrInodeMismatch
	}

	return nil
}

// NormalizePath cleans the path and expands the tilde (~) prefix to the user's home directory.
func NormalizePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", ErrEmptyPath
	}

	if hasControlCharacters(path) {
		return "", ErrControlCharacters
	}

	home := GetHomeDir()

	if path == "~" {
		if home == "" {
			return "", errors.New("unable to determine user home directory")
		}
		return filepath.Clean(home), nil
	}

	if strings.HasPrefix(path, "~/") {
		if home == "" {
			return "", errors.New("unable to determine user home directory")
		}
		return filepath.Clean(filepath.Join(home, path[2:])), nil
	}

	return filepath.Clean(path), nil
}

// ResolveFinalTarget follows symbolic links completely to determine the canonical path.
func ResolveFinalTarget(path string) (string, error) {
	norm, err := NormalizePath(path)
	if err != nil {
		return "", err
	}

	return filepath.EvalSymlinks(norm)
}

// IsProtectedBundleID returns true for any bundle identifier starting with "com.apple."
// or equal to "com.apple".
func IsProtectedBundleID(bundleID string) bool {
	id := strings.ToLower(strings.TrimSpace(bundleID))
	if id == "" {
		return false
	}
	return id == "com.apple" || strings.HasPrefix(id, "com.apple.")
}

// isSystemProtectedPrefix checks if the path matches a system protected prefix tree.
func isSystemProtectedPrefix(p string) bool {
	norm := filepath.Clean(p)
	for _, prefix := range systemProtectedPrefixes {
		if norm == prefix || strings.HasPrefix(norm, prefix+"/") {
			return true
		}
	}
	return false
}

// isSystemExactRoot checks if the path is an exact system or shared folder root.
func isSystemExactRoot(p string) bool {
	norm := filepath.Clean(p)
	for _, root := range systemExactRoots {
		if norm == root {
			return true
		}
	}
	return false
}

// IsSystemPath returns true if the path is in the system denylist (Layer 2).
func IsSystemPath(path string) bool {
	if path == "" {
		return false
	}

	norm, err := NormalizePath(path)
	if err != nil {
		norm = filepath.Clean(path)
	}

	if norm == "/" {
		return true
	}

	home := GetHomeDir()
	if home != "" && norm == filepath.Clean(home) {
		return true
	}

	if isSystemExactRoot(norm) {
		return true
	}

	return isSystemProtectedPrefix(norm)
}

// IsSensitiveUserPath returns true if the path contains sensitive user files or protected directories (Layer 3).
func IsSensitiveUserPath(path string) bool {
	if path == "" {
		return false
	}

	norm, err := NormalizePath(path)
	if err != nil {
		norm = filepath.Clean(path)
	}

	home := GetHomeDir()
	if home == "" {
		return false
	}
	cleanHome := filepath.Clean(home)

	if norm == cleanHome {
		return true
	}

	var rel string
	if norm == cleanHome || strings.HasPrefix(norm, cleanHome+"/") {
		relPart, err := filepath.Rel(cleanHome, norm)
		if err != nil {
			return false
		}
		rel = relPart
	} else if !filepath.IsAbs(norm) {
		rel = norm
	} else {
		// Absolute path outside user's home directory
		return false
	}

	rel = filepath.Clean(rel)

	// 1. Recursive sensitive user directories (both the folder and any child path)
	for _, prefix := range sensitiveUserRecursivePrefixes {
		if rel == prefix || strings.HasPrefix(rel, prefix+"/") {
			return true
		}
	}

	// 2. Sensitive user files
	for _, file := range sensitiveUserFiles {
		if rel == file {
			return true
		}
	}

	// 3. User personal directories (the directories themselves, NOT their contents)
	for _, dir := range sensitiveUserExactDirs {
		if rel == dir {
			return true
		}
	}

	return false
}

// ValidatePathForDeletion validates a candidate path through the 5-layer safety architecture:
//
// Layer 1: Structural path checks (non-empty, absolute, no traversal components, no control characters).
// Layer 2: System Denylist matching (/System, /usr, /Library/Apple, etc.).
// Layer 3: Sensitive User Path Denylist matching (~/.ssh, Keychain, Mail, personal folder roots).
// Layer 4: Apple Bundle ID Protection (com.apple.* components).
// Layer 5: Ancestor and target symlink resolution to prevent traversal via symlink redirection.
func ValidatePathForDeletion(path string) error {
	// Check 1: Non-empty path
	if strings.TrimSpace(path) == "" {
		return ErrEmptyPath
	}

	// Check 3: No control characters
	if hasControlCharacters(path) {
		return ErrControlCharacters
	}

	// Check 2: No path traversal components (/../)
	if hasPathTraversal(path) {
		return ErrPathTraversal
	}

	// Normalize tilde expansion if present
	normalized, err := NormalizePath(path)
	if err != nil {
		return err
	}

	// Check 1 (continued): Path must be absolute
	if !filepath.IsAbs(normalized) {
		return ErrNotAbsolutePath
	}

	// Layer 2: System Denylist check
	if normalized == "/" {
		return ErrSystemProtectedPath
	}

	home := GetHomeDir()
	if home != "" && normalized == filepath.Clean(home) {
		return ErrHomeDirectoryProtected
	}

	if IsSystemPath(normalized) {
		return ErrSystemProtectedPath
	}

	// Layer 3: Sensitive User Path Denylist check
	if IsSensitiveUserPath(normalized) {
		return ErrSensitiveUserPath
	}

	// Layer 4: Apple Bundle ID Protection check on path segments
	if hasAppleBundleComponent(normalized) {
		return ErrAppleBundleProtected
	}

	// Check user whitelist if configured
	if wl, err := LoadWhitelist(); err == nil && wl != nil && wl.Contains(normalized) {
		return ErrUserWhitelisted
	}

	// Layer 1 & 5: Walk ancestor symlinks to prevent symlinked parent traversal.
	// Check that ancestors do not redirect out of safe boundaries into protected system or user trees.
	curr := filepath.Dir(normalized)
	for curr != "/" && curr != "." {
		if info, err := os.Lstat(curr); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				resolvedParent, err := filepath.EvalSymlinks(curr)
				if err == nil {
					// Check if resolved parent is a protected tree
					if isSystemProtectedPrefix(resolvedParent) || IsSensitiveUserPath(resolvedParent) {
						return ErrAncestorSymlinkProtected
					}
					rel, relErr := filepath.Rel(curr, normalized)
					if relErr == nil {
						reconstructed := filepath.Join(resolvedParent, rel)
						if isSystemProtectedPrefix(reconstructed) || IsSensitiveUserPath(reconstructed) {
							return ErrAncestorSymlinkProtected
						}
						if hasAppleBundleComponent(reconstructed) {
							return ErrAppleBundleProtected
						}
					}
				}
			}
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}

	// Layer 1 Check 4: Resolve symlink leaf - real target must not be in protected areas
	if lstat, err := os.Lstat(normalized); err == nil {
		if lstat.Mode()&os.ModeSymlink != 0 {
			realTarget, err := filepath.EvalSymlinks(normalized)
			if err == nil {
				if IsSystemPath(realTarget) {
					return ErrSymlinkTargetProtected
				}
				if IsSensitiveUserPath(realTarget) {
					return ErrSymlinkTargetProtected
				}
				if hasAppleBundleComponent(realTarget) {
					return ErrAppleBundleProtected
				}
			}
		}
	}

	return nil
}

// hasControlCharacters checks if the string contains ASCII control characters or null bytes.
func hasControlCharacters(s string) bool {
	for _, r := range s {
		if r == 0 || r == '\n' || r == '\r' || r == '\t' || unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// hasPathTraversal checks if any directory segment is "..".
func hasPathTraversal(path string) bool {
	clean := filepath.ToSlash(path)
	parts := strings.Split(clean, "/")
	for _, part := range parts {
		if part == ".." {
			return true
		}
	}
	return false
}

// hasAppleBundleComponent checks if any segment of the path matches an Apple bundle identifier.
func hasAppleBundleComponent(path string) bool {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, part := range parts {
		if part == "" {
			continue
		}
		if IsProtectedBundleID(part) {
			return true
		}
		stem := strings.TrimSuffix(part, filepath.Ext(part))
		if IsProtectedBundleID(stem) {
			return true
		}
	}
	return false
}
