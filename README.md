# SafeUninstaller

Intelligent, safety-first application uninstallation for macOS.

**You do not need a paid Apple Developer account or the Mac App Store** to build, share, or install this app. Ad-hoc signed builds clean personal `~/Library` leftovers. A paid Developer ID is only required later if you want Gatekeeper-silent distribution or the optional `/Library` privileged helper.

## Install without App Store

### Option A — Local zip (no GitHub required)

```bash
cd "Mac Issue"   # or your clone path
chmod +x Scripts/package-release.sh Scripts/install.sh
./Scripts/package-release.sh
INSTALL_ZIP_URL="file://$(pwd)/dist/SafeUninstaller-1.0.0.zip" ./Scripts/install.sh
```

Or unzip `dist/SafeUninstaller-1.0.0.zip` and move `SafeUninstaller.app` into **Applications** or **~/Applications**. First launch: if macOS blocks it, **Right-click → Open → Open**. Grant **Full Disk Access** when prompted in Setup.

### Option B — GitHub Releases

1. Upload `SafeUninstaller-1.0.0.zip` (and `.sha256`) to a GitHub Release.
2. Users download the zip, unzip, and move the app into Applications / ~/Applications.
3. Right-click → Open once if Gatekeeper warns; grant Full Disk Access.

### Option C — Terminal install from a published Release

After the Release exists and this repo is on GitHub (replace `OWNER/REPO`):

```bash
REPO=OWNER/REPO \
  curl -fsSL https://raw.githubusercontent.com/OWNER/REPO/main/Scripts/install.sh | bash
```

Or install a specific asset URL:

```bash
INSTALL_ZIP_URL=https://github.com/OWNER/REPO/releases/download/v1.0.0/SafeUninstaller-1.0.0.zip \
  ./Scripts/install.sh
```

The installer places the app in `~/Applications`, clears the quarantine attribute, and opens it.

### What works without a paid Apple account

| Capability | Supported |
|---|---|
| Scan apps + clean `~/Library` leftovers | Yes |
| Move items to Trash + audit reports | Yes |
| Full Disk Access | Yes (user grants it) |
| Double-click with no Gatekeeper warning | No — users Right-click → Open once (or use `install.sh`) |
| Privileged helper for `/Library` | No — needs Developer ID |

## Package a release

On a Mac with Xcode:

```bash
cd "Mac Issue"   # or your clone path
chmod +x Scripts/package-release.sh Scripts/install.sh
./Scripts/package-release.sh
```

Outputs:

- `dist/SafeUninstaller-1.0.0.zip`
- `dist/SafeUninstaller-1.0.0.zip.sha256`

Create a GitHub Release (e.g. tag `v1.0.0`), upload both files, and share the download or terminal one-liner above.

## Features

- **Storage Overview** — free/total space and category breakdown, with Empty Trash
- Multi-signal **application resource attribution** with High / Medium / Low confidence
- Orphaned-file (“Leftovers”) search for already-removed apps
- **Large Files & Media** — Spotlight + FileManager merge for big files (Pictures included; Music/home with FDA); compress videos (AVFoundation HEVC presets) and photos (ImageIO HEIC/JPEG with format choice); originals go to Trash
- **Caches** — clear third-party `~/Library/Caches` leaves safely
- **Developer** — Xcode leftovers, package caches, per-project `node_modules` / Pods / venvs (manual select), plus confirmed `brew cleanup` / `npm cache clean` / `simctl delete unavailable` when those CLIs are installed
- Trash-only removal, confirmation sheets, audit reports (`~/Library/Application Support/SafeUninstaller/Reports`, mode `0700`)
- Optional privileged helper for `/Library` (only when Developer ID–signed)
- First-run Setup for Full Disk Access and Finder Automation (Empty Trash)

Large-file discovery merges Spotlight (`MDQuery`) with a FileManager walk so a partial index never hides unindexed files.
## Build (development)

```bash
cd "Mac Issue"
xcodegen generate
xcodebuild -scheme SafeUninstaller -configuration Debug build
xcodebuild -scheme UninstallerCore -destination 'platform=macOS' test
```

Open `SafeUninstaller.xcodeproj` in Xcode for day-to-day Debug / ad-hoc runs.

## Permissions

1. **Full Disk Access** — System Settings → Privacy & Security → Full Disk Access. Needed for complete Library scans and expanded large-file roots. The app probes `~/Library/Application Support/com.apple.TCC` (or Shortcuts) — not Mail/Safari.
2. **Automation (Finder)** — Empty Trash runs an AppleScript against Finder. macOS may ask once to allow SafeUninstaller to control Finder (Privacy & Security → Automation).
3. **Privileged Helper (optional)** — only for root-owned `/Library` leftovers. Unavailable without a paid Developer ID certificate; user-Library cleanup does not need it.

## Optional later: Developer ID + notarization

If you join the Apple Developer Program later:

1. Set `DEVELOPMENT_TEAM` in `project.yml` / Xcode.
2. Sign app, framework, and helper with **Developer ID Application**.
3. Export the `.app` and run `./Scripts/notarize.sh /path/to/SafeUninstaller.app`.

Until then, use the GitHub zip + `install.sh` path above.

## Architecture

| Target | Role |
|--------|------|
| `UninstallerCore` | Identity, discovery, attribution, orphans, removal, XPC client |
| `SafeUninstaller` | SwiftUI UI |
| `UninstallerHelper` | Standalone privileged daemon (disabled unless properly signed) |

## Safety model

- Hard denylist for SIP / `com.apple.*` / shared Library roots / `/usr`
- Privileged helper uses a **strict allowlist** when enabled
- High confidence pre-selected; Medium/Low opt-in; Low collapsed by default
- Symlink leaves refused; launchd labels validated

## Tests

```bash
xcodebuild -scheme UninstallerCore -destination 'platform=macOS' test
```
