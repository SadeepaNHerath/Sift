package engine

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestSignalWeights(t *testing.T) {
	expectedWeights := map[Signal]float64{
		SignalExactBundleID:      0.95,
		SignalContainerBundleID:  0.90,
		SignalLaunchdProgramPath: 0.85,
		SignalPkgReceipt:         0.80,
		SignalSpotlightBundleID:  0.75,
		SignalPlistContentRef:    0.70,
		SignalSavedStateName:     0.65,
		SignalTeamIDMatch:        0.60,
		SignalVendorPrefix:       0.40,
		SignalAppNameInPath:      0.30,
	}

	for sig, expected := range expectedWeights {
		actual, ok := SignalWeights[sig]
		if !ok {
			t.Errorf("missing weight for signal %s", sig)
			continue
		}
		if math.Abs(actual-expected) > 0.0001 {
			t.Errorf("signal %s: expected weight %f, got %f", sig, expected, actual)
		}
	}
}

func TestCompositeConfidence(t *testing.T) {
	// Empty signals
	if score := CompositeConfidence(nil); score != 0.0 {
		t.Errorf("expected 0.0 for empty signals, got %f", score)
	}

	// Single signal: ExactBundleID (0.95)
	scoreExact := CompositeConfidence([]Signal{SignalExactBundleID})
	if math.Abs(scoreExact-0.95) > 0.0001 {
		t.Errorf("expected 0.95 for ExactBundleID, got %f", scoreExact)
	}

	// Two signals: ExactBundleID (0.95) and AppNameInPath (0.30)
	// Expected: 1 - (1 - 0.95) * (1 - 0.30) = 1 - 0.05 * 0.70 = 1 - 0.035 = 0.965
	scoreCombined := CompositeConfidence([]Signal{SignalExactBundleID, SignalAppNameInPath})
	if math.Abs(scoreCombined-0.965) > 0.0001 {
		t.Errorf("expected 0.965 for combined signals, got %f", scoreCombined)
	}

	// Duplicate signals should be deduplicated
	scoreDup := CompositeConfidence([]Signal{SignalExactBundleID, SignalExactBundleID})
	if math.Abs(scoreDup-0.95) > 0.0001 {
		t.Errorf("expected deduplication to give 0.95, got %f", scoreDup)
	}

	// Weak signals: AppNameInPath (0.30) + VendorPrefix (0.40)
	// 1 - 0.70 * 0.60 = 1 - 0.42 = 0.58
	scoreWeak := CompositeConfidence([]Signal{SignalAppNameInPath, SignalVendorPrefix})
	if math.Abs(scoreWeak-0.58) > 0.0001 {
		t.Errorf("expected 0.58 for weak signals, got %f", scoreWeak)
	}
}

func TestEvaluateConfidence_PromotionRules(t *testing.T) {
	// 1. Weak signals alone NEVER promote to High
	signalsWeak := []Signal{SignalAppNameInPath, SignalVendorPrefix}
	scoreWeak := CompositeConfidence(signalsWeak) // 0.58
	confWeak := EvaluateConfidence(scoreWeak, signalsWeak)
	if confWeak != ConfidenceMedium {
		t.Errorf("weak signals alone (score %f) expected Medium, got %s", scoreWeak, confWeak)
	}

	// Single weak signal below 0.50 threshold
	signalsLow := []Signal{SignalAppNameInPath}
	scoreLow := CompositeConfidence(signalsLow) // 0.30
	confLow := EvaluateConfidence(scoreLow, signalsLow)
	if confLow != ConfidenceLow {
		t.Errorf("AppNameInPath alone (score %f) expected Low, got %s", scoreLow, confLow)
	}

	// 2. High requires ExactBundleID or ContainerBundleID
	// LaunchdProgramPath (0.85) has score 0.85 >= 0.80, but lacks ExactBundleID and ContainerBundleID
	signalsLaunchd := []Signal{SignalLaunchdProgramPath}
	scoreLaunchd := CompositeConfidence(signalsLaunchd)
	confLaunchd := EvaluateConfidence(scoreLaunchd, signalsLaunchd)
	if confLaunchd != ConfidenceMedium {
		t.Errorf("LaunchdProgramPath alone without Exact/Container expected Medium, got %s", confLaunchd)
	}

	// ExactBundleID gives High
	signalsExact := []Signal{SignalExactBundleID}
	scoreExact := CompositeConfidence(signalsExact)
	confExact := EvaluateConfidence(scoreExact, signalsExact)
	if confExact != ConfidenceHigh {
		t.Errorf("ExactBundleID expected High, got %s", confExact)
	}

	// ContainerBundleID gives High
	signalsContainer := []Signal{SignalContainerBundleID}
	scoreContainer := CompositeConfidence(signalsContainer)
	confContainer := EvaluateConfidence(scoreContainer, signalsContainer)
	if confContainer != ConfidenceHigh {
		t.Errorf("ContainerBundleID expected High, got %s", confContainer)
	}
}

func TestFindAppResourcesWithRoots(t *testing.T) {
	tempDir := t.TempDir()

	// Create mock discovery directories
	appSupportDir := filepath.Join(tempDir, "Library", "Application Support")
	cachesDir := filepath.Join(tempDir, "Library", "Caches")
	prefsDir := filepath.Join(tempDir, "Library", "Preferences")
	savedStateDir := filepath.Join(tempDir, "Library", "Saved Application State")
	containersDir := filepath.Join(tempDir, "Library", "Containers")
	launchAgentsDir := filepath.Join(tempDir, "Library", "LaunchAgents")
	receiptsDir := filepath.Join(tempDir, "Library", "Receipts")

	dirs := []string{appSupportDir, cachesDir, prefsDir, savedStateDir, containersDir, launchAgentsDir, receiptsDir}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("failed to create directory %s: %v", d, err)
		}
	}

	app := AppInfo{
		Name:       "MockStudio",
		BundleID:   "com.mockcompany.studio",
		Executable: "MockStudio",
		Version:    "1.0.0",
		Path:       filepath.Join(tempDir, "MockStudio.app"),
	}

	// 1. Matching resources
	// Application Support folder
	asPath := filepath.Join(appSupportDir, "com.mockcompany.studio")
	_ = os.Mkdir(asPath, 0755)
	_ = os.WriteFile(filepath.Join(asPath, "data.db"), []byte("data"), 0644)

	// Caches folder
	cachePath := filepath.Join(cachesDir, "com.mockcompany.studio")
	_ = os.Mkdir(cachePath, 0755)

	// Preferences plist
	prefPath := filepath.Join(prefsDir, "com.mockcompany.studio.plist")
	_ = os.WriteFile(prefPath, []byte("<plist><string>com.mockcompany.studio</string></plist>"), 0644)

	// Saved Application State folder
	ssPath := filepath.Join(savedStateDir, "com.mockcompany.studio.savedstate")
	_ = os.Mkdir(ssPath, 0755)

	// Containers sandbox directory
	containerPath := filepath.Join(containersDir, "com.mockcompany.studio")
	_ = os.Mkdir(containerPath, 0755)

	// LaunchAgent plist with program argument
	agentPath := filepath.Join(launchAgentsDir, "com.mockcompany.studio.agent.plist")
	_ = os.WriteFile(agentPath, []byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.mockcompany.studio.agent</string>
    <key>Program</key>
    <string>`+app.Path+`</string>
</dict>
</plist>`), 0644)

	// Receipts
	receiptPath := filepath.Join(receiptsDir, "com.mockcompany.studio.bom")
	_ = os.WriteFile(receiptPath, []byte("receipt"), 0644)

	// 2. Unrelated resource (must NOT be attributed)
	otherPath := filepath.Join(appSupportDir, "com.unrelated.tool")
	_ = os.Mkdir(otherPath, 0755)

	roots := []DiscoveryRoot{
		{Path: appSupportDir, Category: CategoryApplicationSupport},
		{Path: cachesDir, Category: CategoryCaches},
		{Path: prefsDir, Category: CategoryPreferences},
		{Path: savedStateDir, Category: CategorySavedState},
		{Path: containersDir, Category: CategoryContainer},
		{Path: launchAgentsDir, Category: CategoryLaunchAgent},
		{Path: receiptsDir, Category: CategoryReceipt},
	}

	resources, err := FindAppResourcesWithRoots(app, roots)
	if err != nil {
		t.Fatalf("FindAppResourcesWithRoots failed: %v", err)
	}

	if len(resources) == 0 {
		t.Fatalf("expected discovered resources, got 0")
	}

	foundPaths := make(map[string]AttributedResource)
	for _, r := range resources {
		foundPaths[r.Path] = r
	}

	// Verify Application Support
	if res, ok := foundPaths[asPath]; !ok {
		t.Errorf("missing Application Support resource %s", asPath)
	} else {
		if res.Category != CategoryApplicationSupport {
			t.Errorf("expected CategoryApplicationSupport, got %s", res.Category)
		}
		if res.Confidence != ConfidenceHigh {
			t.Errorf("expected ConfidenceHigh for exact bundle ID in App Support, got %s", res.Confidence)
		}
	}

	// Verify Container
	if res, ok := foundPaths[containerPath]; !ok {
		t.Errorf("missing Container resource %s", containerPath)
	} else {
		if res.Category != CategoryContainer {
			t.Errorf("expected CategoryContainer, got %s", res.Category)
		}
		if res.Confidence != ConfidenceHigh {
			t.Errorf("expected ConfidenceHigh for Container, got %s", res.Confidence)
		}
	}

	// Verify LaunchAgent
	if res, ok := foundPaths[agentPath]; !ok {
		t.Errorf("missing LaunchAgent resource %s", agentPath)
	} else {
		if res.Category != CategoryLaunchAgent {
			t.Errorf("expected CategoryLaunchAgent, got %s", res.Category)
		}
	}

	// Verify Unrelated is NOT present
	if _, ok := foundPaths[otherPath]; ok {
		t.Errorf("unrelated resource %s was incorrectly attributed!", otherPath)
	}
}

func TestBootoutLaunchd(t *testing.T) {
	tempDir := t.TempDir()
	plistPath := filepath.Join(tempDir, "com.mock.daemon.plist")

	plistContent := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.mock.daemon</string>
    <key>Program</key>
    <string>/usr/bin/true</string>
</dict>
</plist>`

	if err := os.WriteFile(plistPath, []byte(plistContent), 0644); err != nil {
		t.Fatalf("failed to write mock launchd plist: %v", err)
	}

	// BootoutLaunchd should execute cleanly without panicking
	err := BootoutLaunchd(plistPath)
	if err != nil {
		t.Errorf("BootoutLaunchd returned error: %v", err)
	}
}
