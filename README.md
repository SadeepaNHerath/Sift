# SafeUninstaller

Intelligent, safety-first application uninstallation for macOS.

**You do not need a paid Apple Developer account or the Mac App Store** to build, share, or install this app. Ad-hoc signed builds clean personal `~/Library` leftovers. A paid Developer ID is only required later if you want Gatekeeper-silent distribution or the optional `/Library` privileged helper.

## Install without App Store

### Option A — Download from GitHub Releases

1. Build a zip (see [Package a release](#package-a-release)) and upload `SafeUninstaller-1.0.0.zip` to a GitHub Release.
2. Users download the zip, unzip, and move `SafeUninstaller.app` into **Applications** or **~/Applications**.
3. First launch: if macOS says the app can’t be opened, **Right-click → Open → Open**.
4. Grant **Full Disk Access**: System Settings → Privacy & Security → Full Disk Access → enable SafeUninstaller.

### Option B — Terminal install

After the Release exists and this repo is on GitHub:

```bash
REPO=YOUR_GITHUB_USER/YOUR_REPO \
  curl -fsSL https://raw.githubusercontent.com/YOUR_GITHUB_USER/YOUR_REPO/main/Scripts/install.sh | bash
```

Or install a specific asset URL:

```bash
INSTALL_ZIP_URL=https://github.com/YOUR_GITHUB_USER/YOUR_REPO/releases/download/v1.0.0/SafeUninstaller-1.0.0.zip \
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
- **Large Files & Media** — Spotlight-backed scan of big files; compress videos (AVFoundation HEVC presets) and photos (ImageIO HEIC/JPEG); originals go to Trash
- **Caches** — clear third-party `~/Library/Caches` leaves safely
- **Developer** — Xcode leftovers, package caches, per-project `node_modules` / Pods / venvs (manual select), plus confirmed `brew cleanup` / `npm cache clean` / `simctl delete unavailable` when those CLIs are installed
- Trash-only removal, confirmation sheets, audit reports (`~/Library/Application Support/SafeUninstaller/Reports`, mode `0700`)
- Optional privileged helper for `/Library` (only when Developer ID–signed)
- First-run Setup for Full Disk Access

Large-file discovery prefers Spotlight (`MDQuery`) for speed, then falls back to a FileManager walk when the index is cold or empty.
## Build (development)

```bash
cd "Mac Issue"
xcodegen generate
xcodebuild -scheme SafeUninstaller -configuration Debug build
xcodebuild -scheme UninstallerCore -destination 'platform=macOS' test
```

Open `SafeUninstaller.xcodeproj` in Xcode for day-to-day Debug / ad-hoc runs.

## Permissions

1. **Full Disk Access** — System Settings → Privacy & Security → Full Disk Access. Needed for complete Library scans. The app probes `~/Library/Application Support/com.apple.TCC` (or Shortcuts) — not Mail/Safari.
2. **Privileged Helper (optional)** — only for root-owned `/Library` leftovers. Unavailable without a paid Developer ID certificate; user-Library cleanup does not need it.

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
