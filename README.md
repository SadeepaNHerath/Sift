# SafeUninstaller

Intelligent, safety-first application uninstallation and low-storage toolkit for macOS.

**You do not need a paid Apple Developer account or the Mac App Store** to build, share, or install this app. Ad-hoc signed builds clean personal `~/Library` leftovers, compress media, and clear caches. A paid Developer ID is only required later if you want Gatekeeper-silent distribution or the optional `/Library` privileged helper.

## Distribute without Apple Developer ID (checklist)

1. **One-time Sparkle keys** (for auto-update):
   ```bash
   chmod +x Scripts/generate-sparkle-keys.sh
   ./Scripts/generate-sparkle-keys.sh
   ```
   Commit `SafeUninstaller/Resources/sparkle-public-ed-key.txt`. Keep `sparkle-private/ed-private-key.txt` secret (gitignored).
2. **Package**:
   ```bash
   chmod +x Scripts/package-release.sh Scripts/install.sh
   RELEASE_REPO=OWNER/REPO ./Scripts/package-release.sh
   ```
   Or without updates: `./Scripts/package-release.sh` (Sparkle stays disabled until feed keys are embedded).
3. **GitHub Release** — upload `SafeUninstaller-x.y.z.zip`, `.sha256`, and `appcast.xml` (appcast must be reachable at `…/releases/latest/download/appcast.xml`).
4. **Users install** via `Scripts/install.sh` or manual unzip → Right-click → Open once → grant Full Disk Access (+ Automation for Empty Trash).
5. **Help → Copy Diagnostics** if something goes wrong (on-device log only; no telemetry).

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

1. Upload `SafeUninstaller-1.0.0.zip`, `.sha256`, and `appcast.xml` to a GitHub Release.
2. Users download the zip, unzip, and move the app into Applications / ~/Applications — or use Option C.
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
| Sparkle auto-update (EdDSA + GitHub Releases) | Yes (after keys + appcast) |
| Local diagnostics (Help → Copy Diagnostics) | Yes |
| Full Disk Access | Yes (user grants it) |
| Double-click with no Gatekeeper warning | No — users Right-click → Open once (or use `install.sh`) |
| Privileged helper for `/Library` | No — needs Developer ID |

## Package a release

```bash
cd "Mac Issue"
chmod +x Scripts/package-release.sh Scripts/install.sh Scripts/generate-sparkle-keys.sh
# With Sparkle updates:
RELEASE_REPO=OWNER/REPO ./Scripts/package-release.sh
# Without updates:
./Scripts/package-release.sh
```

Outputs:

- `dist/SafeUninstaller-1.0.0.zip`
- `dist/SafeUninstaller-1.0.0.zip.sha256`
- `dist/appcast.xml` (when `RELEASE_REPO` / `SU_FEED_URL` is set)

**Note:** New ad-hoc builds may still need Right-click → Open once after an update (Gatekeeper). Sparkle does not require a paid Apple account.

## Features

- **Storage Overview** — free/total space and category breakdown (time-budgeted samples, `~` when approximate), with Empty Trash
- Multi-signal **application resource attribution** with High / Medium / Low confidence
- Orphaned-file (“Leftovers”) search for already-removed apps
- **Large Files & Media** — Spotlight + FileManager merge; video sample dry-run estimates + AVFoundation compress; ImageIO HEIC/JPEG
- **Caches** / **Developer** cleanups + confirmed brew/npm/simctl CLIs
- Sparkle auto-update when feed + EdDSA public key are packaged
- On-device diagnostics log; Help → Copy Diagnostics
- Trash-only removal, confirmation sheets, audit reports
- Optional privileged helper for `/Library` (only when Developer ID–signed)
- First-run Setup for unsigned-build notes, FDA, Automation

Large-file discovery merges Spotlight (`MDQuery`) with a FileManager walk so a partial index never hides unindexed files.

## Build (development)

```bash
cd "Mac Issue"
xcodegen generate
xcodebuild -scheme SafeUninstaller -configuration Debug build
xcodebuild -scheme UninstallerCore -destination 'platform=macOS' test
xcodebuild -scheme SafeUninstaller -destination 'platform=macOS' test
```

Open `SafeUninstaller.xcodeproj` in Xcode for day-to-day Debug / ad-hoc runs.

## Permissions

1. **Full Disk Access** — System Settings → Privacy & Security → Full Disk Access. Needed for complete Library scans and expanded large-file roots.
2. **Automation (Finder)** — Empty Trash runs an AppleScript against Finder. macOS may ask once (Privacy & Security → Automation).
3. **Privileged Helper (optional)** — only for root-owned `/Library` leftovers. Unavailable without a paid Developer ID certificate.

Diagnostics are written to `~/Library/Logs/SafeUninstaller/app.log` and never uploaded by the app.

## Optional later: Developer ID + notarization

If you join the Apple Developer Program later:

1. Set `DEVELOPMENT_TEAM` in `project.yml` / Xcode.
2. Sign app, framework, and helper with **Developer ID Application**.
3. Export the `.app` and run `./Scripts/notarize.sh /path/to/SafeUninstaller.app`.

Until then, use the GitHub zip + `install.sh` path above.

## Architecture

| Target | Role |
|--------|------|
| `UninstallerCore` | Identity, discovery, attribution, orphans, removal, storage toolkit, diagnostics |
| `SafeUninstaller` | SwiftUI UI + Sparkle updater |
| `UninstallerHelper` | Standalone privileged daemon (disabled unless properly signed) |

## Safety model

- Hard denylist for SIP / `com.apple.*` / shared Library roots / `/usr`
- Privileged helper uses a **strict allowlist** when enabled
- High confidence pre-selected; Medium/Low opt-in; Low collapsed by default
- Symlink leaves refused; launchd labels validated

## Tests

```bash
xcodebuild -scheme UninstallerCore -destination 'platform=macOS' test
# UI smoke (ad-hoc signing — disable hardened runtime for the runner):
xcodebuild -scheme SafeUninstaller -destination 'platform=macOS' \
  CODE_SIGN_IDENTITY=- CODE_SIGNING_REQUIRED=NO ENABLE_HARDENED_RUNTIME=NO \
  -only-testing:SafeUninstallerUITests test
```
