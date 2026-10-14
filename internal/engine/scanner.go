package engine

import (
	"container/heap"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/SadeepaNHerath/Spacefold/internal/core"
)

// FileEntry represents a filesystem node (file or directory) with metadata.
type FileEntry struct {
	Path             string      `json:"path"`
	Name             string      `json:"name"`
	Size             int64       `json:"size"`
	IsDir            bool        `json:"is_dir"`
	ModTime          time.Time   `json:"mod_time"`
	ChildrenCount    int         `json:"children_count"`
	InsightsCategory string      `json:"insights_category,omitempty"`
	Children         []FileEntry `json:"children,omitempty"`
}

// DiskScanner performs fast concurrent disk scanning bounded by semaphores
// to prevent I/O thrashing and file descriptor exhaustion.
type DiskScanner struct {
	concurrency int
	sem         chan struct{}
}

// NewDiskScanner initializes a DiskScanner with bounded concurrency.
// If concurrency is <= 0, a sensible default based on CPU cores is chosen.
func NewDiskScanner(concurrency ...int) *DiskScanner {
	workers := 0
	if len(concurrency) > 0 {
		workers = concurrency[0]
	}
	if workers <= 0 {
		workers = runtime.NumCPU() * 2
		if workers < 4 {
			workers = 4
		} else if workers > 32 {
			workers = 32
		}
	}

	return &DiskScanner{
		concurrency: workers,
		sem:         make(chan struct{}, workers),
	}
}

// DefaultDiskScanner provides a shared scanner instance.
var defaultScanner = NewDiskScanner()

// DefaultScanner returns the default shared DiskScanner.
func DefaultScanner() *DiskScanner {
	return defaultScanner
}

func (s *DiskScanner) acquire() {
	s.sem <- struct{}{}
}

func (s *DiskScanner) release() {
	<-s.sem
}

// ScanDirProgressive scans the directory progressively, streaming immediate entries to onEntry.
// Bounded concurrency is used across child directories to prevent I/O thrashing.
func (s *DiskScanner) ScanDirProgressive(path string, onEntry func(FileEntry)) (*FileEntry, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path %q: %w", path, err)
	}

	info, err := os.Lstat(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat path %q: %w", absPath, err)
	}

	// Leaf file case
	if !info.IsDir() {
		entry := FileEntry{
			Path:          absPath,
			Name:          info.Name(),
			Size:          info.Size(),
			IsDir:         false,
			ModTime:       info.ModTime(),
			ChildrenCount: 0,
		}
		if onEntry != nil {
			onEntry(entry)
		}
		return &entry, nil
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %q: %w", absPath, err)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	children := make([]FileEntry, 0, len(entries))
	var totalSize int64

	type dirTask struct {
		path    string
		name    string
		modTime time.Time
	}
	var dirTasks []dirTask

	for _, d := range entries {
		name := d.Name()
		childPath := filepath.Join(absPath, name)

		childInfo, err := d.Info()
		if err != nil {
			continue
		}

		if !d.IsDir() {
			fe := FileEntry{
				Path:          childPath,
				Name:          name,
				Size:          childInfo.Size(),
				IsDir:         false,
				ModTime:       childInfo.ModTime(),
				ChildrenCount: 0,
			}
			mu.Lock()
			children = append(children, fe)
			totalSize += fe.Size
			if onEntry != nil {
				onEntry(fe)
			}
			mu.Unlock()
			continue
		}

		dirTasks = append(dirTasks, dirTask{
			path:    childPath,
			name:    name,
			modTime: childInfo.ModTime(),
		})
	}

	if len(dirTasks) > 0 {
		numWorkers := s.concurrency
		if numWorkers > len(dirTasks) {
			numWorkers = len(dirTasks)
		}
		taskChan := make(chan dirTask, len(dirTasks))
		for _, t := range dirTasks {
			taskChan <- t
		}
		close(taskChan)

		for w := 0; w < numWorkers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for t := range taskChan {
					dirSize, childCount := s.calculateDirStats(t.path)
					fe := FileEntry{
						Path:          t.path,
						Name:          t.name,
						Size:          dirSize,
						IsDir:         true,
						ModTime:       t.modTime,
						ChildrenCount: childCount,
					}

					mu.Lock()
					children = append(children, fe)
					totalSize += dirSize
					if onEntry != nil {
						onEntry(fe)
					}
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
	}

	// Sort children by size descending (largest first)
	sort.Slice(children, func(i, j int) bool {
		if children[i].Size == children[j].Size {
			return children[i].Name < children[j].Name
		}
		return children[i].Size > children[j].Size
	})

	name := info.Name()
	if name == "" || name == "." {
		name = absPath
	}

	rootEntry := &FileEntry{
		Path:          absPath,
		Name:          name,
		Size:          totalSize,
		IsDir:         true,
		ModTime:       info.ModTime(),
		ChildrenCount: len(children),
		Children:      children,
	}

	return rootEntry, nil
}

// calculateDirStats recursively measures the total size and item count of a directory.
// Symlinks are not followed to avoid circular reference loops.
func (s *DiskScanner) calculateDirStats(dirPath string) (int64, int) {
	var totalSize int64
	var count int

	_ = filepath.WalkDir(dirPath, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if p == dirPath {
			return nil
		}

		count++

		// Never traverse into symlinks
		if d.Type()&os.ModeSymlink != 0 {
			if info, err := d.Info(); err == nil {
				totalSize += info.Size()
			}
			return nil
		}

		if !d.IsDir() {
			if info, err := d.Info(); err == nil {
				totalSize += info.Size()
			}
		}
		return nil
	})

	return totalSize, count
}

// ScanDirProgressive is a convenience wrapper using DefaultScanner.
func ScanDirProgressive(path string, onEntry func(FileEntry)) (*FileEntry, error) {
	return DefaultScanner().ScanDirProgressive(path, onEntry)
}

// topFilesHeap implements container/heap as a min-heap for FileEntry items.
type topFilesHeap []FileEntry

func (h topFilesHeap) Len() int           { return len(h) }
func (h topFilesHeap) Less(i, j int) bool { return h[i].Size < h[j].Size }
func (h topFilesHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *topFilesHeap) Push(x any)        { *h = append(*h, x.(FileEntry)) }
func (h *topFilesHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// ScanTopFiles identifies the Top-N largest files rooted at root using a min-heap.
// Concurrency is bounded by semaphores across root subtrees.
func (s *DiskScanner) ScanTopFiles(root string, count int) ([]FileEntry, error) {
	if count <= 0 {
		count = 20
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve root %q: %w", root, err)
	}

	rootInfo, err := os.Lstat(absRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to stat root %q: %w", absRoot, err)
	}

	if !rootInfo.IsDir() {
		return []FileEntry{{
			Path:          absRoot,
			Name:          rootInfo.Name(),
			Size:          rootInfo.Size(),
			IsDir:         false,
			ModTime:       rootInfo.ModTime(),
			ChildrenCount: 0,
		}}, nil
	}

	entries, err := os.ReadDir(absRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to read root directory %q: %w", absRoot, err)
	}

	var rootFiles []FileEntry
	var subDirs []string

	for _, d := range entries {
		p := filepath.Join(absRoot, d.Name())
		if d.IsDir() {
			subDirs = append(subDirs, p)
		} else {
			if info, err := d.Info(); err == nil && d.Type()&os.ModeSymlink == 0 {
				rootFiles = append(rootFiles, FileEntry{
					Path:          p,
					Name:          d.Name(),
					Size:          info.Size(),
					IsDir:         false,
					ModTime:       info.ModTime(),
					ChildrenCount: 0,
				})
			}
		}
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	allCandidates := append([]FileEntry{}, rootFiles...)

	for _, subDir := range subDirs {
		wg.Add(1)
		go func(dir string) {
			defer wg.Done()
			s.acquire()
			defer s.release()

			localHeap := &topFilesHeap{}
			heap.Init(localHeap)

			_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if d.IsDir() {
					return nil
				}
				if d.Type()&os.ModeSymlink != 0 {
					return nil
				}

				info, err := d.Info()
				if err != nil {
					return nil
				}

				entry := FileEntry{
					Path:          p,
					Name:          d.Name(),
					Size:          info.Size(),
					IsDir:         false,
					ModTime:       info.ModTime(),
					ChildrenCount: 0,
				}

				if localHeap.Len() < count {
					heap.Push(localHeap, entry)
				} else if entry.Size > (*localHeap)[0].Size {
					heap.Pop(localHeap)
					heap.Push(localHeap, entry)
				}
				return nil
			})

			mu.Lock()
			for _, item := range *localHeap {
				allCandidates = append(allCandidates, item)
			}
			mu.Unlock()
		}(subDir)
	}

	wg.Wait()

	// Consolidate into global min-heap
	globalHeap := &topFilesHeap{}
	heap.Init(globalHeap)

	for _, fe := range allCandidates {
		if globalHeap.Len() < count {
			heap.Push(globalHeap, fe)
		} else if fe.Size > (*globalHeap)[0].Size {
			heap.Pop(globalHeap)
			heap.Push(globalHeap, fe)
		}
	}

	// Extract in descending order
	result := make([]FileEntry, globalHeap.Len())
	for i := len(result) - 1; i >= 0; i-- {
		result[i] = heap.Pop(globalHeap).(FileEntry)
	}

	return result, nil
}

// ScanTopFiles is a convenience wrapper using DefaultScanner.
func ScanTopFiles(root string, count int) ([]FileEntry, error) {
	return DefaultScanner().ScanTopFiles(root, count)
}

// insightTarget defines known hidden macOS developer caches and space hogs.
type insightTarget struct {
	category string
	getPath  func(home string) string
}

// ScanInsights detects hidden macOS space hogs:
//   - Xcode DerivedData (~/Library/Developer/Xcode/DerivedData)
//   - iOS Simulator runtimes (~/Library/Developer/CoreSimulator/Devices)
//   - Docker Desktop images (~/Library/Containers/com.docker.docker)
//   - Android SDK / emulator images (~/Library/Android/sdk)
//   - Rust Cargo registry (~/.cargo)
//   - Go module cache (~/go/pkg/mod)
//   - Gradle / Maven caches (~/.gradle, ~/.m2)
//   - Time Machine local snapshots (via tmutil listlocalsnapshots)
func (s *DiskScanner) ScanInsights() ([]FileEntry, error) {
	home := core.GetHomeDir()
	if home == "" {
		home, _ = os.UserHomeDir()
	}

	targets := []insightTarget{
		{
			category: "Xcode DerivedData",
			getPath: func(h string) string {
				return filepath.Join(h, "Library", "Developer", "Xcode", "DerivedData")
			},
		},
		{
			category: "iOS Simulator Runtimes",
			getPath: func(h string) string {
				return filepath.Join(h, "Library", "Developer", "CoreSimulator", "Devices")
			},
		},
		{
			category: "Docker Desktop",
			getPath: func(h string) string {
				return filepath.Join(h, "Library", "Containers", "com.docker.docker")
			},
		},
		{
			category: "Android SDK",
			getPath: func(h string) string {
				return filepath.Join(h, "Library", "Android", "sdk")
			},
		},
		{
			category: "Rust Cargo Registry",
			getPath: func(h string) string {
				return filepath.Join(h, ".cargo")
			},
		},
		{
			category: "Go Module Cache",
			getPath: func(h string) string {
				if gopath := os.Getenv("GOPATH"); gopath != "" {
					return filepath.Join(gopath, "pkg", "mod")
				}
				return filepath.Join(h, "go", "pkg", "mod")
			},
		},
		{
			category: "Gradle Cache",
			getPath: func(h string) string {
				return filepath.Join(h, ".gradle")
			},
		},
		{
			category: "Maven Cache",
			getPath: func(h string) string {
				return filepath.Join(h, ".m2")
			},
		},
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var results []FileEntry

	for _, target := range targets {
		p := target.getPath(home)
		if p == "" {
			continue
		}

		info, err := os.Lstat(p)
		if err != nil {
			continue
		}

		wg.Add(1)
		go func(t insightTarget, targetPath string, fi os.FileInfo) {
			defer wg.Done()
			s.acquire()
			defer s.release()

			size, count := s.calculateDirStats(targetPath)
			fe := FileEntry{
				Path:             targetPath,
				Name:             fi.Name(),
				Size:             size,
				IsDir:            fi.IsDir(),
				ModTime:          fi.ModTime(),
				ChildrenCount:    count,
				InsightsCategory: t.category,
			}

			mu.Lock()
			results = append(results, fe)
			mu.Unlock()
		}(target, p, info)
	}

	// Also inspect Time Machine local snapshots via tmutil
	wg.Add(1)
	go func() {
		defer wg.Done()
		cmd := exec.Command("tmutil", "listlocalsnapshots", "/")
		out, err := cmd.Output()
		if err != nil {
			return
		}

		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "com.apple.TimeMachine.") {
				fe := FileEntry{
					Path:             line,
					Name:             line,
					Size:             0,
					IsDir:            false,
					ModTime:          time.Now(),
					ChildrenCount:    0,
					InsightsCategory: "Time Machine Local Snapshots",
				}
				mu.Lock()
				results = append(results, fe)
				mu.Unlock()
			}
		}
	}()

	wg.Wait()

	// Sort insights descending by size
	sort.Slice(results, func(i, j int) bool {
		if results[i].Size == results[j].Size {
			return results[i].Name < results[j].Name
		}
		return results[i].Size > results[j].Size
	})

	return results, nil
}

// ScanInsights is a convenience wrapper using DefaultScanner.
func ScanInsights() ([]FileEntry, error) {
	return DefaultScanner().ScanInsights()
}
