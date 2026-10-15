package engine

import (
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/SadeepaNHerath/Sift/internal/core"
)

// SystemDataItem represents an identified component of macOS "System Data".
type SystemDataItem struct {
	Category  string `json:"category"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	ItemCount int    `json:"item_count"`
	CleanHint string `json:"clean_hint,omitempty"`
}

// SystemDataReport summarizes macOS System Data components and categorized consumption.
type SystemDataReport struct {
	Items      []SystemDataItem `json:"items"`
	TotalBytes int64            `json:"total_bytes"`
	Categories map[string]int64 `json:"categories"`
}

type systemDataTarget struct {
	category  string
	name      string
	cleanHint string
	getPath   func(home string) string
}

// ScanSystemData inspects known locations contributing to macOS "System Data".
func ScanSystemData() (*SystemDataReport, error) {
	home := core.GetHomeDir()
	if home == "" {
		home, _ = os.UserHomeDir()
	}

	targets := []systemDataTarget{
		{
			category:  "Developer",
			name:      "Xcode DerivedData",
			cleanHint: "sf purge --xcode",
			getPath:   func(h string) string { return filepath.Join(h, "Library", "Developer", "Xcode", "DerivedData") },
		},
		{
			category:  "Developer",
			name:      "Xcode Archives",
			cleanHint: "sf purge --xcode",
			getPath:   func(h string) string { return filepath.Join(h, "Library", "Developer", "Xcode", "Archives") },
		},
		{
			category:  "Developer",
			name:      "iOS Simulator Devices",
			cleanHint: "xcrun simctl erase all",
			getPath:   func(h string) string { return filepath.Join(h, "Library", "Developer", "CoreSimulator", "Devices") },
		},
		{
			category:  "Developer",
			name:      "Android SDK / Images",
			cleanHint: "sf clean",
			getPath:   func(h string) string { return filepath.Join(h, "Library", "Android", "sdk") },
		},
		{
			category:  "Developer",
			name:      "Rust Cargo Registry",
			cleanHint: "cargo cache --autoclean",
			getPath:   func(h string) string { return filepath.Join(h, ".cargo") },
		},
		{
			category:  "Developer",
			name:      "Go Module Cache",
			cleanHint: "go clean -modcache",
			getPath: func(h string) string {
				if gp := os.Getenv("GOPATH"); gp != "" {
					return filepath.Join(gp, "pkg", "mod")
				}
				return filepath.Join(h, "go", "pkg", "mod")
			},
		},
		{
			category:  "Developer",
			name:      "Gradle Caches",
			cleanHint: "sf clean",
			getPath:   func(h string) string { return filepath.Join(h, ".gradle", "caches") },
		},
		{
			category:  "Developer",
			name:      "Maven Repository",
			cleanHint: "sf clean",
			getPath:   func(h string) string { return filepath.Join(h, ".m2", "repository") },
		},

		{
			category:  "Virtualization",
			name:      "Docker Desktop Data",
			cleanHint: "docker system prune -a",
			getPath:   func(h string) string { return filepath.Join(h, "Library", "Containers", "com.docker.docker") },
		},
		{
			category:  "Virtualization",
			name:      "OrbStack Machines & Disks",
			cleanHint: "orb delete",
			getPath:   func(h string) string { return filepath.Join(h, ".orbstack") },
		},
		{
			category:  "Virtualization",
			name:      "Colima VM Disks",
			cleanHint: "colima delete",
			getPath:   func(h string) string { return filepath.Join(h, ".colima") },
		},

		{
			category:  "Package Managers",
			name:      "Homebrew Downloads",
			cleanHint: "brew cleanup -s",
			getPath:   func(h string) string { return filepath.Join(h, "Library", "Caches", "Homebrew") },
		},
		{
			category:  "Package Managers",
			name:      "npm Cache",
			cleanHint: "npm cache clean --force",
			getPath:   func(h string) string { return filepath.Join(h, ".npm") },
		},
		{
			category:  "Package Managers",
			name:      "pip Cache",
			cleanHint: "pip cache purge",
			getPath:   func(h string) string { return filepath.Join(h, ".cache", "pip") },
		},
		{
			category:  "Package Managers",
			name:      "CocoaPods Cache",
			cleanHint: "pod cache clean --all",
			getPath:   func(h string) string { return filepath.Join(h, "Library", "Caches", "CocoaPods") },
		},

		{
			category:  "System & Logs",
			name:      "User Logs & Diagnostics",
			cleanHint: "sf clean",
			getPath:   func(h string) string { return filepath.Join(h, "Library", "Logs") },
		},
		{
			category:  "System & Logs",
			name:      "Crash Reports",
			cleanHint: "sf clean",
			getPath:   func(h string) string { return filepath.Join(h, "Library", "DiagnosticReports") },
		},
	}

	var mu sync.Mutex
	var items []SystemDataItem
	var wg sync.WaitGroup

	for _, target := range targets {
		p := target.getPath(home)
		if p == "" {
			continue
		}

		if _, err := os.Stat(p); err != nil {
			continue
		}

		wg.Add(1)
		go func(t systemDataTarget, targetPath string) {
			defer wg.Done()
			size, count := dirStatsFast(targetPath)
			if size > 0 {
				mu.Lock()
				items = append(items, SystemDataItem{
					Category:  t.category,
					Name:      t.name,
					Path:      targetPath,
					Size:      size,
					ItemCount: count,
					CleanHint: t.cleanHint,
				})
				mu.Unlock()
			}
		}(target, p)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		count, totalBytes, err := core.GetTrashSize()
		if err == nil && totalBytes > 0 {
			mu.Lock()
			items = append(items, SystemDataItem{
				Category:  "Trash",
				Name:      "User Trash",
				Path:      core.GetTrashDir(),
				Size:      totalBytes,
				ItemCount: count,
				CleanHint: "sf clean --trash",
			})
			mu.Unlock()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		userCaches := filepath.Join(home, "Library", "Caches")
		entries, err := os.ReadDir(userCaches)
		if err != nil {
			return
		}

		type dirEntry struct {
			name string
			path string
		}
		var appCaches []dirEntry
		for _, e := range entries {
			if e.IsDir() {
				// Skip Homebrew and CocoaPods as they are already handled
				if e.Name() == "Homebrew" || e.Name() == "CocoaPods" {
					continue
				}
				appCaches = append(appCaches, dirEntry{
					name: e.Name(),
					path: filepath.Join(userCaches, e.Name()),
				})
			}
		}

		var subWg sync.WaitGroup
		for _, ac := range appCaches {
			subWg.Add(1)
			go func(d dirEntry) {
				defer subWg.Done()
				sz, cnt := dirStatsFast(d.path)
				// Include substantial app caches (over 100 MB)
				if sz >= 100*1000*1000 {
					mu.Lock()
					items = append(items, SystemDataItem{
						Category:  "Application Caches",
						Name:      d.name + " Cache",
						Path:      d.path,
						Size:      sz,
						ItemCount: cnt,
						CleanHint: "sf clean",
					})
					mu.Unlock()
				}
			}(ac)
		}
		subWg.Wait()
	}()

	wg.Wait()

	sort.Slice(items, func(i, j int) bool {
		return items[i].Size > items[j].Size
	})

	catMap := make(map[string]int64)
	var total int64
	for _, itm := range items {
		total += itm.Size
		catMap[itm.Category] += itm.Size
	}

	return &SystemDataReport{
		Items:      items,
		TotalBytes: total,
		Categories: catMap,
	}, nil
}

// dirStatsFast computes directory size and file count efficiently.
func dirStatsFast(root string) (int64, int) {
	var totalSize int64
	var count int

	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			totalSize += info.Size()
			count++
		}
		return nil
	})

	return totalSize, count
}
