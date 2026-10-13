package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/SadeepaNHerath/Spacefold/internal/core"
)

// ServiceLevel distinguishes user vs global vs system launchd jobs.
type ServiceLevel string

const (
	LevelUserAgent    ServiceLevel = "User Agent"    // ~/Library/LaunchAgents
	LevelGlobalAgent  ServiceLevel = "Global Agent"  // /Library/LaunchAgents
	LevelSystemDaemon ServiceLevel = "System Daemon" // /Library/LaunchDaemons
)

// LaunchService represents a macOS LaunchAgent or LaunchDaemon.
type LaunchService struct {
	Label      string       `json:"label"`
	Level      ServiceLevel `json:"level"`
	PlistPath  string       `json:"plist_path"`
	BinaryPath string       `json:"binary_path"`
	Loaded     bool         `json:"loaded"`
	PID        int          `json:"pid"`
	LastExit   int          `json:"last_exit"`
	IsZombie   bool         `json:"is_zombie"`
	RunAtLoad  bool         `json:"run_at_load"`
	KeepAlive  bool         `json:"keep_alive"`
	Domain     string       `json:"domain"`
}

type plistData struct {
	Label            string      `json:"Label"`
	Program          string      `json:"Program"`
	ProgramArguments []string    `json:"ProgramArguments"`
	RunAtLoad        interface{} `json:"RunAtLoad"`
	KeepAlive        interface{} `json:"KeepAlive"`
	Disabled         interface{} `json:"Disabled"`
}

// ScanLaunchServices scans ~/Library/LaunchAgents, /Library/LaunchAgents, and /Library/LaunchDaemons.
func ScanLaunchServices() ([]LaunchService, error) {
	home := core.GetHomeDir()
	uid := os.Getuid()

	targets := []struct {
		dir    string
		level  ServiceLevel
		domain string
	}{
		{
			dir:    filepath.Join(home, "Library", "LaunchAgents"),
			level:  LevelUserAgent,
			domain: fmt.Sprintf("gui/%d", uid),
		},
		{
			dir:    "/Library/LaunchAgents",
			level:  LevelGlobalAgent,
			domain: fmt.Sprintf("gui/%d", uid),
		},
		{
			dir:    "/Library/LaunchDaemons",
			level:  LevelSystemDaemon,
			domain: "system",
		},
	}

	loadedMap := getLoadedLaunchdJobs()
	var services []LaunchService

	for _, tgt := range targets {
		entries, err := os.ReadDir(tgt.dir)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".plist") {
				continue
			}

			plistPath := filepath.Join(tgt.dir, e.Name())
			svc, err := parseLaunchService(plistPath, tgt.level, tgt.domain)
			if err != nil {
				continue
			}

			// Cross-reference with launchctl list
			if status, ok := loadedMap[svc.Label]; ok {
				svc.Loaded = true
				svc.PID = status.PID
				svc.LastExit = status.LastExit
			}

			services = append(services, svc)
		}
	}

	return services, nil
}

type launchStatus struct {
	PID      int
	LastExit int
}

func getLoadedLaunchdJobs() map[string]launchStatus {
	out, err := core.RunCommandWithTimeout(5*time.Second, "launchctl", "list")
	if err != nil {
		return map[string]launchStatus{}
	}

	m := make(map[string]launchStatus)
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		label := strings.TrimSpace(parts[2])
		if label == "" || label == "Label" {
			continue
		}

		pidStr := strings.TrimSpace(parts[0])
		pid := 0
		if pidStr != "-" {
			pid, _ = strconv.Atoi(pidStr)
		}

		statusStr := strings.TrimSpace(parts[1])
		lastExit, _ := strconv.Atoi(statusStr)

		m[label] = launchStatus{
			PID:      pid,
			LastExit: lastExit,
		}
	}
	return m
}

func parseLaunchService(plistPath string, level ServiceLevel, domain string) (LaunchService, error) {
	svc := LaunchService{
		PlistPath: plistPath,
		Level:     level,
		Domain:    domain,
	}

	var data plistData

	// Attempt JSON extraction via plutil
	out, err := exec.Command("plutil", "-convert", "json", "-o", "-", plistPath).Output()
	if err == nil {
		_ = json.Unmarshal(out, &data)
	} else {
		// Fallback simple parsing for raw files or test mocks
		content, readErr := os.ReadFile(plistPath)
		if readErr != nil {
			return svc, readErr
		}
		strContent := string(content)
		data.Label = extractXMLTag(strContent, "Label")
		data.Program = extractXMLTag(strContent, "Program")
	}

	svc.Label = data.Label
	if svc.Label == "" {
		svc.Label = strings.TrimSuffix(filepath.Base(plistPath), ".plist")
	}

	// Determine binary path
	bin := data.Program
	if bin == "" && len(data.ProgramArguments) > 0 {
		bin = data.ProgramArguments[0]
	}
	svc.BinaryPath = bin

	// Check for Zombie condition (missing backing binary)
	if bin != "" {
		cleanBin := bin
		if strings.HasPrefix(cleanBin, "~/") {
			cleanBin = filepath.Join(core.GetHomeDir(), cleanBin[2:])
		}
		if _, err := os.Stat(cleanBin); os.IsNotExist(err) {
			svc.IsZombie = true
		}
	}

	return svc, nil
}

func extractXMLTag(xml, tag string) string {
	startTag := "<key>" + tag + "</key>"
	idx := strings.Index(xml, startTag)
	if idx == -1 {
		return ""
	}
	after := xml[idx+len(startTag):]
	sIdx := strings.Index(after, "<string>")
	if sIdx == -1 {
		return ""
	}
	eIdx := strings.Index(after[sIdx:], "</string>")
	if eIdx == -1 {
		return ""
	}
	return strings.TrimSpace(after[sIdx+8 : sIdx+eIdx])
}

// ToggleLaunchService starts/bootstraps or stops/bootouts a launchd service.
func ToggleLaunchService(svc LaunchService, enable bool) error {
	if enable {
		// Try modern bootstrap
		cmd := exec.Command("launchctl", "bootstrap", svc.Domain, svc.PlistPath)
		if err := cmd.Run(); err != nil {
			// Fallback legacy load
			return exec.Command("launchctl", "load", svc.PlistPath).Run()
		}
		return nil
	}

	// Disable / bootout
	cmd := exec.Command("launchctl", "bootout", fmt.Sprintf("%s/%s", svc.Domain, svc.Label))
	if err := cmd.Run(); err != nil {
		// Fallback legacy unload
		return exec.Command("launchctl", "unload", svc.PlistPath).Run()
	}
	return nil
}

// RemoveZombiePlist moves an orphaned launchd plist file to Trash.
func RemoveZombiePlist(svc LaunchService, logger *core.Logger) error {
	if !svc.IsZombie {
		return fmt.Errorf("service %s is not a zombie", svc.Label)
	}

	// If currently loaded, boot it out first
	_ = ToggleLaunchService(svc, false)

	err := core.SafeRemove(svc.PlistPath)
	if err != nil {
		return err
	}

	if logger != nil {
		sz, _ := core.FileSize(svc.PlistPath)
		logger.LogDeletion(svc.PlistPath, sz, "trash", fmt.Sprintf("sf startup (zombie %s)", svc.Label))
	}

	return nil
}
