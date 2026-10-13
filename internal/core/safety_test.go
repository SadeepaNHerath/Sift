package core

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestValidatePath_SIPPathsBlocked verifies that macOS System Integrity Protection
// and critical system paths are unconditionally blocked.
func TestValidatePath_SIPPathsBlocked(t *testing.T) {
	sipPaths := []string{
		"/System",
		"/System/Library",
		"/System/Library/CoreServices",
		"/usr/bin",
		"/usr/bin/tar",
		"/usr/sbin",
		"/usr/sbin/systemsetup",
		"/usr/lib",
		"/usr/lib/libSystem.B.dylib",
		"/usr/libexec",
		"/bin",
		"/bin/ls",
		"/sbin",
		"/sbin/mount",
		"/private/var/db",
		"/private/var/protected",
		"/var/db",
		"/private/etc",
		"/etc",
		"/Library/Apple",
		"/Library/Apple/System/Library",
		"/Library/Extensions",
		"/Library/Frameworks",
		"/Library/SystemExtensions",
		"/Library/Keychains",
		"/cores",
		"/dev",
		"/net",
		"/",
		"/Applications",
		"/Library",
		"/Users",
	}

	for _, p := range sipPaths {
		t.Run(p, func(t *testing.T) {
			err := ValidatePathForDeletion(p)
			if err == nil {
				t.Fatalf("expected error for SIP/system path %q, got nil", p)
			}
			if !errors.Is(err, ErrSystemProtectedPath) {
				t.Fatalf("expected ErrSystemProtectedPath for %q, got: %v", p, err)
			}
			if !IsSystemPath(p) {
				t.Errorf("IsSystemPath(%q) should be true", p)
			}
		})
	}
}

// TestValidatePath_HomeDirectoryRootBlocked verifies that the user's home directory root
// cannot be deleted under any circumstances.
func TestValidatePath_HomeDirectoryRootBlocked(t *testing.T) {
	home := GetHomeDir()
	if home == "" {
		t.Skip("skipping test: HOME directory not resolved")
	}

	testCases := []string{
		"~",
		"~/",
		home,
		home + "/",
	}

	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			err := ValidatePathForDeletion(tc)
			if err == nil {
				t.Fatalf("expected error deleting home directory root %q, got nil", tc)
			}
			if !errors.Is(err, ErrHomeDirectoryProtected) && !errors.Is(err, ErrSystemProtectedPath) && !errors.Is(err, ErrSensitiveUserPath) {
				t.Fatalf("expected home protection error for %q, got: %v", tc, err)
			}
		})
	}
}

// TestValidatePath_SensitivePathsBlocked verifies that sensitive user paths (.ssh, .gnupg, Keychains, etc.)
// are blocked from deletion.
func TestValidatePath_SensitivePathsBlocked(t *testing.T) {
	sensitivePaths := []string{
		"~/.ssh",
		"~/.ssh/id_rsa",
		"~/.ssh/id_ed25519.pub",
		"~/.ssh/known_hosts",
		"~/.gnupg",
		"~/.gnupg/pubring.kbx",
		"~/.gpg",
		"~/Library/Keychains",
		"~/Library/Keychains/login.keychain-db",
		"~/Library/Mail",
		"~/Library/Mail/V10",
		"~/Library/Messages",
		"~/Library/Messages/chat.db",
		"~/Library/Safari",
		"~/Library/Safari/Bookmarks.plist",
		"~/Library/Accounts",
		"~/.gitconfig",
		"~/.zshrc",
		"~/.bashrc",
		"~/.bash_profile",
		"~/.profile",
		"~/.zshenv",
		"~/Library/Cookies",
		"~/Library/Calendars",
	}

	for _, p := range sensitivePaths {
		t.Run(p, func(t *testing.T) {
			err := ValidatePathForDeletion(p)
			if err == nil {
				t.Fatalf("expected error for sensitive path %q, got nil", p)
			}
			if !errors.Is(err, ErrSensitiveUserPath) {
				t.Fatalf("expected ErrSensitiveUserPath for %q, got: %v", p, err)
			}
			if !IsSensitiveUserPath(p) {
				t.Errorf("IsSensitiveUserPath(%q) should be true", p)
			}
		})
	}
}

// TestValidatePath_ValidDeletablePathsAllowed ensures standard cleanable paths are accepted.
func TestValidatePath_ValidDeletablePathsAllowed(t *testing.T) {
	validPaths := []string{
		"~/Library/Caches/com.example.app",
		"~/Library/Application Support/ExampleApp",
		"~/Library/Logs/ExampleApp",
		"~/Library/Preferences/com.example.app.plist",
		"/tmp/sift_test_temp_file.cache",
		"~/Downloads/test_temp_download.zip",
		"~/Desktop/temporary_text_file.txt",
		"~/Documents/scratch_project_notes.txt",
	}

	for _, p := range validPaths {
		t.Run(p, func(t *testing.T) {
			err := ValidatePathForDeletion(p)
			if err != nil {
				t.Fatalf("expected path %q to be allowed, got error: %v", p, err)
			}
		})
	}
}

// TestValidatePath_PathTraversalCaught verifies that directory traversal attempts are caught.
func TestValidatePath_PathTraversalCaught(t *testing.T) {
	traversals := []string{
		"/tmp/../System",
		"/tmp/../usr/bin",
		"/var/tmp/../../etc",
		"~/Library/Caches/../../.ssh",
		"/tmp/something/../../bin",
		"/Library/Application Support/../Apple",
	}

	for _, p := range traversals {
		t.Run(p, func(t *testing.T) {
			err := ValidatePathForDeletion(p)
			if err == nil {
				t.Fatalf("expected error for traversal path %q, got nil", p)
			}
			if !errors.Is(err, ErrPathTraversal) {
				t.Fatalf("expected ErrPathTraversal for %q, got: %v", p, err)
			}
		})
	}
}

// TestValidatePath_ControlCharactersRejected verifies that strings with control characters are rejected.
func TestValidatePath_ControlCharactersRejected(t *testing.T) {
	badPaths := []string{
		"/tmp/file\x00name",
		"/tmp/file\nname",
		"/tmp/file\rname",
		"/tmp/file\tname",
		"~/Library/Caches/\x00test",
	}

	for _, p := range badPaths {
		t.Run(p, func(t *testing.T) {
			err := ValidatePathForDeletion(p)
			if err == nil {
				t.Fatalf("expected error for path with control character %q, got nil", p)
			}
			if !errors.Is(err, ErrControlCharacters) {
				t.Fatalf("expected ErrControlCharacters for %q, got: %v", p, err)
			}
		})
	}
}

// TestValidatePath_EmptyAndRelativeRejected verifies that empty and relative paths are rejected.
func TestValidatePath_EmptyAndRelativeRejected(t *testing.T) {
	t.Run("empty path", func(t *testing.T) {
		if err := ValidatePathForDeletion(""); !errors.Is(err, ErrEmptyPath) {
			t.Fatalf("expected ErrEmptyPath, got: %v", err)
		}
	})

	t.Run("whitespace only path", func(t *testing.T) {
		if err := ValidatePathForDeletion("   "); !errors.Is(err, ErrEmptyPath) {
			t.Fatalf("expected ErrEmptyPath, got: %v", err)
		}
	})

	relativePaths := []string{
		"relative/path/to/file",
		"./some_file.txt",
		"../parent_file.txt",
		"just_a_filename.log",
	}

	for _, p := range relativePaths {
		t.Run(p, func(t *testing.T) {
			err := ValidatePathForDeletion(p)
			if err == nil {
				t.Fatalf("expected relative path %q to be rejected, got nil", p)
			}
			if !errors.Is(err, ErrNotAbsolutePath) && !errors.Is(err, ErrPathTraversal) {
				t.Fatalf("expected ErrNotAbsolutePath or traversal for %q, got: %v", p, err)
			}
		})
	}
}

// TestAppleBundleIDProtection verifies detection of Apple bundle identifiers.
func TestAppleBundleIDProtection(t *testing.T) {
	protected := []string{
		"com.apple.Safari",
		"com.apple.finder",
		"com.apple.SystemPreferences",
		"com.apple.dt.Xcode",
		"com.apple",
		"COM.APPLE.MAIL",
	}

	for _, id := range protected {
		if !IsProtectedBundleID(id) {
			t.Errorf("IsProtectedBundleID(%q) should be true", id)
		}
	}

	unprotected := []string{
		"com.example.app",
		"org.mozilla.firefox",
		"com.google.Chrome",
		"io.github.sift",
		"",
	}

	for _, id := range unprotected {
		if IsProtectedBundleID(id) {
			t.Errorf("IsProtectedBundleID(%q) should be false", id)
		}
	}

	applePaths := []string{
		"~/Library/Caches/com.apple.Safari",
		"~/Library/Preferences/com.apple.finder.plist",
		"~/Library/Application Support/com.apple.Spotlight",
	}

	for _, p := range applePaths {
		t.Run(p, func(t *testing.T) {
			err := ValidatePathForDeletion(p)
			if err == nil {
				t.Fatalf("expected error for Apple path %q, got nil", p)
			}
			if !errors.Is(err, ErrAppleBundleProtected) && !errors.Is(err, ErrSensitiveUserPath) {
				t.Fatalf("expected Apple protection error for %q, got: %v", p, err)
			}
		})
	}
}

// TestInodeBindingAndVerification tests TOCTOU protection by binding and verifying inodes.
func TestInodeBindingAndVerification(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "bind_test.txt")

	if err := os.WriteFile(filePath, []byte("initial content"), 0644); err != nil {
		t.Fatalf("failed to create temp test file: %v", err)
	}

	ref, err := BindInode(filePath)
	if err != nil {
		t.Fatalf("BindInode failed: %v", err)
	}

	if ref.Inode == 0 {
		t.Errorf("expected non-zero inode number")
	}

	if err := ref.Verify(); err != nil {
		t.Fatalf("expected initial verification to succeed, got: %v", err)
	}

	// Case 1: In-place update of content preserves inode
	if err := os.WriteFile(filePath, []byte("updated content"), 0644); err != nil {
		t.Fatalf("failed to update temp file: %v", err)
	}
	if err := ref.Verify(); err != nil {
		t.Fatalf("expected verification to pass on modified file with same inode, got: %v", err)
	}

	// Case 2: Replace file by unlinking and recreating (new inode allocated)
	if err := os.Remove(filePath); err != nil {
		t.Fatalf("failed to remove temp file: %v", err)
	}
	// Recreate another file
	if err := os.WriteFile(filePath, []byte("new file replacement"), 0644); err != nil {
		t.Fatalf("failed to recreate file: %v", err)
	}

	// Re-verify on replaced file - if inode changed, verify must fail
	newRef, err := BindInode(filePath)
	if err != nil {
		t.Fatalf("failed to bind recreated file: %v", err)
	}
	if newRef.Inode != ref.Inode {
		if err := ref.Verify(); !errors.Is(err, ErrInodeMismatch) {
			t.Fatalf("expected ErrInodeMismatch for replaced file, got: %v", err)
		}
	}

	// Case 3: Verify on deleted file
	if err := os.Remove(filePath); err != nil {
		t.Fatalf("failed to delete file: %v", err)
	}
	if err := ref.Verify(); err == nil {
		t.Fatalf("expected error verifying deleted file, got nil")
	}

	// Case 4: Bind on non-existent file
	if _, err := BindInode(filepath.Join(tempDir, "non_existent.txt")); err == nil {
		t.Fatalf("expected error binding non-existent file, got nil")
	}

	// Case 5: Bind on empty path
	if _, err := BindInode(""); !errors.Is(err, ErrEmptyPath) {
		t.Fatalf("expected ErrEmptyPath, got: %v", err)
	}
}

// TestNormalizePath_TildeExpansion tests path normalization and tilde expansion.
func TestNormalizePath_TildeExpansion(t *testing.T) {
	home := GetHomeDir()
	if home == "" {
		t.Skip("skipping test: HOME directory not resolved")
	}

	normHome, err := NormalizePath("~")
	if err != nil {
		t.Fatalf("NormalizePath(~) failed: %v", err)
	}
	if normHome != filepath.Clean(home) {
		t.Errorf("NormalizePath(~) = %q; want %q", normHome, filepath.Clean(home))
	}

	normSub, err := NormalizePath("~/Library/Caches")
	if err != nil {
		t.Fatalf("NormalizePath(~/Library/Caches) failed: %v", err)
	}
	expectedSub := filepath.Join(home, "Library", "Caches")
	if normSub != expectedSub {
		t.Errorf("NormalizePath(~/Library/Caches) = %q; want %q", normSub, expectedSub)
	}

	normClean, err := NormalizePath("~/foo/../bar")
	if err != nil {
		t.Fatalf("NormalizePath(~/foo/../bar) failed: %v", err)
	}
	expectedClean := filepath.Join(home, "bar")
	if normClean != expectedClean {
		t.Errorf("NormalizePath(~/foo/../bar) = %q; want %q", normClean, expectedClean)
	}
}

// TestSymlinkResolution tests symlink chasing and blocking symlinks pointing to protected targets.
func TestSymlinkResolution(t *testing.T) {
	tempDir := t.TempDir()

	targetFile := filepath.Join(tempDir, "real_target.txt")
	if err := os.WriteFile(targetFile, []byte("target data"), 0644); err != nil {
		t.Fatalf("failed to write target file: %v", err)
	}

	symlinkPath := filepath.Join(tempDir, "symlink_test")
	if err := os.Symlink(targetFile, symlinkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	// Test ResolveFinalTarget resolves correctly
	resolved, err := ResolveFinalTarget(symlinkPath)
	if err != nil {
		t.Fatalf("ResolveFinalTarget failed: %v", err)
	}
	realTargetEval, _ := filepath.EvalSymlinks(targetFile)
	if resolved != realTargetEval {
		t.Errorf("ResolveFinalTarget = %q; want %q", resolved, realTargetEval)
	}

	// Test Symlink pointing to /System is blocked by ValidatePathForDeletion
	symlinkToSystem := filepath.Join(tempDir, "symlink_to_system")
	if err := os.Symlink("/System", symlinkToSystem); err == nil {
		err := ValidatePathForDeletion(symlinkToSystem)
		if err == nil {
			t.Fatalf("expected ValidatePathForDeletion to block symlink to /System, got nil")
		}
		if !errors.Is(err, ErrSymlinkTargetProtected) && !errors.Is(err, ErrSystemProtectedPath) {
			t.Fatalf("expected protected symlink target error, got: %v", err)
		}
	}

	// Test Symlink pointing to ~/.ssh is blocked
	home := GetHomeDir()
	if home != "" {
		sshPath := filepath.Join(home, ".ssh")
		symlinkToSSH := filepath.Join(tempDir, "symlink_to_ssh")
		if err := os.Symlink(sshPath, symlinkToSSH); err == nil {
			err := ValidatePathForDeletion(symlinkToSSH)
			if err == nil {
				t.Fatalf("expected ValidatePathForDeletion to block symlink to ~/.ssh, got nil")
			}
			if !errors.Is(err, ErrSymlinkTargetProtected) && !errors.Is(err, ErrSensitiveUserPath) {
				t.Fatalf("expected sensitive symlink target error, got: %v", err)
			}
		}
	}
}

// TestPathsWithTrailingSlashes verifies that paths with trailing slashes are properly normalized and evaluated.
func TestPathsWithTrailingSlashes(t *testing.T) {
	blockedTrailing := []string{
		"/System/",
		"/usr/bin/",
		"~/",
		"~/Desktop/",
		"~/.ssh/",
	}

	for _, p := range blockedTrailing {
		t.Run(p, func(t *testing.T) {
			err := ValidatePathForDeletion(p)
			if err == nil {
				t.Fatalf("expected path with trailing slash %q to be blocked, got nil", p)
			}
		})
	}

	allowedTrailing := []string{
		"~/Library/Caches/com.example.app/",
		"/tmp/test_dir/",
	}

	for _, p := range allowedTrailing {
		t.Run(p, func(t *testing.T) {
			err := ValidatePathForDeletion(p)
			if err != nil {
				t.Fatalf("expected allowed path with trailing slash %q to pass, got error: %v", p, err)
			}
		})
	}
}

// TestPersonalDirectoriesProtected verifies that user personal directory roots are blocked,
// but files inside them can be deleted.
func TestPersonalDirectoriesProtected(t *testing.T) {
	personalRoots := []string{
		"~/Desktop",
		"~/Documents",
		"~/Pictures",
		"~/Movies",
		"~/Music",
		"~/Downloads",
	}

	for _, root := range personalRoots {
		t.Run("root:"+root, func(t *testing.T) {
			err := ValidatePathForDeletion(root)
			if err == nil {
				t.Fatalf("expected personal directory root %q to be blocked, got nil", root)
			}
			if !errors.Is(err, ErrSensitiveUserPath) {
				t.Fatalf("expected ErrSensitiveUserPath for %q, got: %v", root, err)
			}
		})
	}

	personalContents := []string{
		"~/Desktop/temporary_screenshot.png",
		"~/Documents/scratch_draft.txt",
		"~/Pictures/cache_thumbnail.jpg",
		"~/Movies/temp_clip.mov",
		"~/Music/scratch_recording.wav",
		"~/Downloads/old_installer.dmg",
	}

	for _, item := range personalContents {
		t.Run("content:"+item, func(t *testing.T) {
			err := ValidatePathForDeletion(item)
			if err != nil {
				t.Fatalf("expected personal directory item %q to be allowed, got error: %v", item, err)
			}
		})
	}
}

// TestLibrarySubdirectoriesDeletable verifies that shared Library root directories are protected,
// but application-specific subdirectories inside Caches and Application Support are deletable.
func TestLibrarySubdirectoriesDeletable(t *testing.T) {
	libraryRoots := []string{
		"~/Library",
		"~/Library/Preferences",
		"~/Library/Application Support",
		"~/Library/Caches",
		"~/Library/Containers",
		"~/Library/Group Containers",
		"/Library",
		"/Library/Preferences",
		"/Library/Application Support",
		"/Library/Caches",
	}

	for _, root := range libraryRoots {
		t.Run("root:"+root, func(t *testing.T) {
			err := ValidatePathForDeletion(root)
			if err == nil {
				t.Fatalf("expected shared Library root %q to be blocked, got nil", root)
			}
		})
	}

	deletableSubdirs := []string{
		"~/Library/Caches/com.example.app",
		"~/Library/Application Support/ExampleApp",
		"~/Library/Logs/ExampleApp",
		"~/Library/Caches/org.videolan.vlc",
	}

	for _, sub := range deletableSubdirs {
		t.Run("sub:"+sub, func(t *testing.T) {
			err := ValidatePathForDeletion(sub)
			if err != nil {
				t.Fatalf("expected deletable Library subdir %q to be allowed, got: %v", sub, err)
			}
		})
	}
}
