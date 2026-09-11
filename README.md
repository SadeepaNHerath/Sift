# SafeUninstaller

Intelligent, safety-first application uninstallation for macOS.

**Distribution:** Developer ID + notarization (not Mac App Store — Full Disk Access and a privileged helper are incompatible with App Sandbox).

## Features

- Multi-signal **application resource attribution** with High / Medium / Low confidence and evidence badges
- Orphaned-file (“Leftovers”) search for already-removed apps
- Trash-only removal, confirmation sheets, audit reports (`~/Library/Application Support/SafeUninstaller/Reports`, mode `0700`)
- Optional privileged helper for `/Library` (allowlisted paths only; XPC client code-signing checks)
- First-run Setup for Full Disk Access

## Build

```bash
cd "Mac Issue"
xcodegen generate
xcodebuild -scheme SafeUninstaller -configuration Debug build
xcodebuild -scheme UninstallerCore -destination 'platform=macOS' test
```

Open `SafeUninstaller.xcodeproj` in Xcode. For local Debug builds, ad-hoc signing is fine for **user-domain** cleanup. Privileged helper registration requires a real Team ID.

## Permissions

1. **Full Disk Access** — System Settings → Privacy & Security → Full Disk Access → enable SafeUninstaller. Needed for complete Library scans. The app probes `~/Library/Application Support/com.apple.TCC` (or Shortcuts) — not Mail/Safari.
2. **Privileged Helper (optional)** — System Settings → General → Login Items. Only for root-owned `/Library` leftovers. Disabled in the UI until the app is signed with a Team ID.

## Signing & notarization (release)

1. Set your team in Xcode or `project.yml` (`DEVELOPMENT_TEAM`).
2. Sign **SafeUninstaller**, **UninstallerCore.framework**, and embedded **UninstallerHelper** with the same Developer ID Application identity (hardened runtime).
3. Archive / export the `.app`.
4. Run:

```bash
chmod +x Scripts/notarize.sh
./Scripts/notarize.sh /path/to/SafeUninstaller.app
```

See comments in `Scripts/notarize.sh` for `notarytool` credential setup.

## Architecture

| Target | Role |
|--------|------|
| `UninstallerCore` | Identity, discovery, attribution, orphans, removal, XPC client |
| `SafeUninstaller` | SwiftUI UI |
| `UninstallerHelper` | Standalone privileged daemon (no framework link) |

## Safety model

- Hard denylist for SIP / `com.apple.*` / shared Library roots / `/usr`
- Privileged helper uses a **strict allowlist** (LaunchAgents/Daemons, Application Support leaves, etc.)
- High confidence pre-selected; Medium/Low opt-in; Low collapsed by default
- Symlink leaves refused; launchd labels validated; helper XPC requires matching app code signature (DEBUG soft-fallback for ad-hoc)

## Tests

```bash
xcodebuild -scheme UninstallerCore -destination 'platform=macOS' test
```

Includes attribution fixtures and security allowlist / boundary / orphan-sibling tests.
