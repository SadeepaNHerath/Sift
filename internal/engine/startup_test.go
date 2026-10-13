package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLaunchService_ValidAndZombie(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Valid service (binary exists)
	binPath := filepath.Join(tempDir, "mydaemon")
	_ = os.WriteFile(binPath, []byte("#!/bin/sh\nexit 0"), 0755)

	validPlist := filepath.Join(tempDir, "com.example.valid.plist")
	validXML := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.example.valid</string>
	<key>Program</key>
	<string>` + binPath + `</string>
</dict>
</plist>`
	_ = os.WriteFile(validPlist, []byte(validXML), 0644)

	svc1, err := parseLaunchService(validPlist, LevelUserAgent, "gui/501")
	if err != nil {
		t.Fatalf("failed to parse valid plist: %v", err)
	}
	if svc1.Label != "com.example.valid" {
		t.Errorf("expected label com.example.valid, got %s", svc1.Label)
	}
	if svc1.IsZombie {
		t.Errorf("expected IsZombie=false for existing binary")
	}

	// 2. Zombie service (binary deleted)
	zombiePlist := filepath.Join(tempDir, "com.example.zombie.plist")
	zombieXML := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.example.zombie</string>
	<key>Program</key>
	<string>/Applications/DeletedApp.app/Contents/MacOS/agent</string>
</dict>
</plist>`
	_ = os.WriteFile(zombiePlist, []byte(zombieXML), 0644)

	svc2, err := parseLaunchService(zombiePlist, LevelUserAgent, "gui/501")
	if err != nil {
		t.Fatalf("failed to parse zombie plist: %v", err)
	}
	if svc2.Label != "com.example.zombie" {
		t.Errorf("expected label com.example.zombie, got %s", svc2.Label)
	}
	if !svc2.IsZombie {
		t.Errorf("expected IsZombie=true for missing binary")
	}

	// 3. Remove zombie plist
	err = RemoveZombiePlist(svc2, nil)
	if err != nil {
		t.Fatalf("RemoveZombiePlist failed: %v", err)
	}
	if _, err := os.Stat(zombiePlist); !os.IsNotExist(err) {
		t.Errorf("zombie plist was not moved to trash")
	}
}
