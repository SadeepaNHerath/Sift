package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/cespare/xxhash/v2"

	"github.com/SadeepaNHerath/Spacefold/internal/core"
)

// DuplicateFile represents an individual duplicate candidate file.
type DuplicateFile struct {
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
	Dev     uint64    `json:"dev"`
	Inode   uint64    `json:"inode"`
}

// DuplicateGroup represents a group of identical files sharing the exact same content.
type DuplicateGroup struct {
	Hash        string          `json:"hash"`
	Size        int64           `json:"size"`        // Size per individual duplicate file
	Reclaimable int64           `json:"reclaimable"` // Space reclaimable if keeping 1 copy
	Files       []DuplicateFile `json:"files"`
}

// DedupOptions configures the duplicate finder search parameters.
type DedupOptions struct {
	MinSize      int64    // Minimum file size in bytes (default 1MB)
	ExcludedDirs []string // Directory names to skip (e.g. .git, node_modules)
}

// DefaultDedupOptions returns sensible default options for duplicate search.
func DefaultDedupOptions() DedupOptions {
	return DedupOptions{
		MinSize: 1024 * 1024, // 1 MB
		ExcludedDirs: []string{
			".git",
			".Trash",
			"node_modules",
			".cache",
			"DerivedData",
		},
	}
}

// FindDuplicates executes the 3-phase duplicate search across specified search roots.
// Phase 1: Group files by byte size.
// Phase 2: Sparse probing via xxhash on first 4KB, mid 4KB, and last 4KB.
// Phase 3: Full xxhash verification on colliding candidates.
func FindDuplicates(roots []string, opts DedupOptions) ([]DuplicateGroup, error) {
	if len(roots) == 0 {
		home := core.GetHomeDir()
		roots = []string{
			filepath.Join(home, "Downloads"),
			filepath.Join(home, "Desktop"),
			filepath.Join(home, "Documents"),
		}
	}

	excludeMap := make(map[string]bool)
	for _, ex := range opts.ExcludedDirs {
		excludeMap[strings.ToLower(ex)] = true
	}

	// -------------------------------------------------------------
	// Phase 1: Group files by exact byte size
	// -------------------------------------------------------------
	sizeBuckets := make(map[int64][]DuplicateFile)

	for _, root := range roots {
		normRoot, err := core.NormalizePath(root)
		if err != nil {
			continue
		}

		_ = filepath.Walk(normRoot, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil {
				return nil
			}

			// Skip excluded directory names
			if info.IsDir() {
				base := strings.ToLower(info.Name())
				if excludeMap[base] {
					return filepath.SkipDir
				}
				return nil
			}

			// Only evaluate regular files
			if !info.Mode().IsRegular() {
				return nil
			}

			// Skip files smaller than MinSize
			sz := info.Size()
			if sz < opts.MinSize {
				return nil
			}

			var dev, ino uint64
			if stat, ok := info.Sys().(*syscall.Stat_t); ok {
				dev = uint64(stat.Dev)
				ino = stat.Ino
			}

			sizeBuckets[sz] = append(sizeBuckets[sz], DuplicateFile{
				Path:    path,
				Size:    sz,
				ModTime: info.ModTime(),
				Dev:     dev,
				Inode:   ino,
			})

			return nil
		})
	}

	// Filter Phase 1 buckets: keep only buckets with >= 2 candidates
	var phase2Candidates [][]DuplicateFile
	for _, files := range sizeBuckets {
		if len(files) >= 2 {
			phase2Candidates = append(phase2Candidates, files)
		}
	}

	// -------------------------------------------------------------
	// Phase 2: Sparse probing via xxhash (first 4KB + mid 4KB + last 4KB)
	// -------------------------------------------------------------
	var phase3Candidates [][]DuplicateFile

	for _, bucket := range phase2Candidates {
		sparseBuckets := make(map[uint64][]DuplicateFile)

		for _, file := range bucket {
			sparseHash, err := computeSparseHash(file.Path, file.Size)
			if err != nil {
				continue
			}
			sparseBuckets[sparseHash] = append(sparseBuckets[sparseHash], file)
		}

		for _, files := range sparseBuckets {
			if len(files) >= 2 {
				phase3Candidates = append(phase3Candidates, files)
			}
		}
	}

	// -------------------------------------------------------------
	// Phase 3: Full SHA-256 verification on collisions (collision-proof)
	// -------------------------------------------------------------
	fullBuckets := make(map[string][]DuplicateFile)

	for _, bucket := range phase3Candidates {
		for _, file := range bucket {
			fullHash, err := computeFullHash(file.Path)
			if err != nil {
				continue
			}
			fullBuckets[fullHash] = append(fullBuckets[fullHash], file)
		}
	}

	// Form final duplicate groups, filtering out hardlinks sharing same inode
	var groups []DuplicateGroup

	for hashKey, files := range fullBuckets {
		if len(files) < 2 {
			continue
		}

		// Group files and check for unique inodes
		inodeMap := make(map[string]bool)
		var uniqueFiles []DuplicateFile

		for _, f := range files {
			devInoKey := fmt.Sprintf("%d:%d", f.Dev, f.Inode)
			if f.Inode > 0 && inodeMap[devInoKey] {
				// Hardlink pointing to same physical blocks; does not consume additional space
				continue
			}
			if f.Inode > 0 {
				inodeMap[devInoKey] = true
			}
			uniqueFiles = append(uniqueFiles, f)
		}

		if len(uniqueFiles) < 2 {
			continue
		}

		// Sort files by ModTime ascending (oldest first)
		sort.Slice(uniqueFiles, func(i, j int) bool {
			return uniqueFiles[i].ModTime.Before(uniqueFiles[j].ModTime)
		})

		fileSize := uniqueFiles[0].Size
		reclaimable := fileSize * int64(len(uniqueFiles)-1)

		groups = append(groups, DuplicateGroup{
			Hash:        hashKey,
			Size:        fileSize,
			Reclaimable: reclaimable,
			Files:       uniqueFiles,
		})
	}

	// Sort duplicate groups descending by reclaimable space
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Reclaimable > groups[j].Reclaimable
	})

	return groups, nil
}

// computeSparseHash reads first 4KB, mid 4KB, and last 4KB into xxhash.
func computeSparseHash(path string, size int64) (uint64, error) {
	const chunkSize = 4096
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	hasher := xxhash.New()

	if size <= chunkSize*3 {
		if _, err := io.Copy(hasher, f); err != nil {
			return 0, err
		}
		return hasher.Sum64(), nil
	}

	buf := make([]byte, chunkSize)

	// 1. First 4 KB
	if _, err := io.ReadFull(f, buf); err != nil {
		return 0, err
	}
	_, _ = hasher.Write(buf)

	// 2. Middle 4 KB
	midOffset := (size - chunkSize) / 2
	if _, err := f.Seek(midOffset, io.SeekStart); err != nil {
		return 0, err
	}
	if _, err := io.ReadFull(f, buf); err != nil {
		return 0, err
	}
	_, _ = hasher.Write(buf)

	// 3. Last 4 KB
	lastOffset := size - chunkSize
	if _, err := f.Seek(lastOffset, io.SeekStart); err != nil {
		return 0, err
	}
	if _, err := io.ReadFull(f, buf); err != nil {
		return 0, err
	}
	_, _ = hasher.Write(buf)

	return hasher.Sum64(), nil
}

// computeFullHash reads entire file into SHA-256 in 64KB blocks.
func computeFullHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hasher := sha256.New()
	buf := make([]byte, 64*1024)
	if _, err := io.CopyBuffer(hasher, f, buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// TrashDuplicates safely moves the specified duplicate file paths to Trash.
func TrashDuplicates(paths []string, dryRun bool, logger *core.Logger) (*core.RemovalReport, error) {
	if dryRun {
		dry := core.DryRunRemove(paths)
		report := &core.RemovalReport{TotalBytes: dry.TotalBytes}
		for _, item := range dry.WouldRemove {
			report.Removed = append(report.Removed, core.RemovedItem{
				Path: item.Path,
				Size: item.Size,
			})
		}
		if logger != nil {
			logger.LogOperation("dedup", "dry-run", "success",
				fmt.Sprintf("Would reclaim %s across %d duplicate files", core.FormatBytes(dry.TotalBytes), len(dry.WouldRemove)))
		}
		return report, nil
	}

	report := core.SafeRemoveAll(paths)
	if logger != nil {
		logger.LogOperation("dedup", "execute", "success",
			fmt.Sprintf("Reclaimed %s across %d duplicate files", core.FormatBytes(report.TotalBytes), len(report.Removed)))
		for _, item := range report.Removed {
			logger.LogDeletion(item.Path, item.Size, "trash", "sf dedup")
		}
	}
	return report, nil
}
