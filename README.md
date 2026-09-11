# SafeUninstaller

Intelligent and safe application uninstallation for macOS.

## Features

- Multi-signal **application resource attribution** (bundle ID, vendor prefix, launchd program paths, plist content, Team ID, Spotlight, receipts)
- **High / Medium / Low** confidence scoring with explainable evidence badges
- Orphaned-file search for leftovers of already-deleted apps
- Trash-only removal with post-removal verification and audit reports
- Optional privileged helper (`SMAppService`) for `/Library` cleanup
- Full Disk Access onboarding

## Build

```bash
cd "Mac Issue"
xcodegen generate
xcodebuild -scheme SafeUninstaller -configuration Debug build
xcodebuild -scheme UninstallerCore -destination 'platform=macOS' test
```

Open `SafeUninstaller.xcodeproj` in Xcode to run the app. Grant **Full Disk Access** for complete Library scans.

## Architecture

- `UninstallerCore` — identification, discovery, attribution, orphan search, removal, helper protocol
- `SafeUninstaller` — SwiftUI UI
- `UninstallerHelper` — privileged XPC daemon (requires proper code signing for registration)

## Safety

- Hard denylist for SIP/`com.apple.*`/shared Library roots
- High confidence pre-selected; Medium/Low require review
- Shared-vendor resources flagged as “possibly shared”
