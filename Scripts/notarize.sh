#!/bin/bash
# Notarize a Developer ID–signed SafeUninstaller.app
# Prerequisites:
#   - App signed with Developer ID Application (same Team ID on app, framework, helper)
#   - App-specific password or keychain profile for notarytool
#   - xcrun notarytool stored credentials, e.g.:
#       xcrun notarytool store-credentials "SafeUninstaller-notary" \
#         --apple-id "you@example.com" --team-id "TEAMID" --password "app-specific-password"
set -euo pipefail

APP="${1:-}"
PROFILE="${NOTARY_PROFILE:-SafeUninstaller-notary}"

if [[ -z "$APP" || ! -d "$APP" ]]; then
  echo "Usage: $0 /path/to/SafeUninstaller.app"
  exit 1
fi

APP="$(cd "$(dirname "$APP")" && pwd)/$(basename "$APP")"
ZIP="${TMPDIR:-/tmp}/SafeUninstaller-notarize.zip"

echo "Zipping $APP …"
rm -f "$ZIP"
ditto -c -k --keepParent "$APP" "$ZIP"

echo "Submitting to Apple notary service (profile: $PROFILE) …"
xcrun notarytool submit "$ZIP" --keychain-profile "$PROFILE" --wait

echo "Stapling …"
xcrun stapler staple "$APP"
xcrun stapler validate "$APP"

echo "Done. Verify with: spctl --assess --verbose --type execute \"$APP\""
