#!/bin/bash
# Build an ad-hoc Release zip for GitHub Releases (no paid Apple Developer account required).
# Usage:
#   ./Scripts/package-release.sh
#   VERSION=1.0.0 ./Scripts/package-release.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION="${VERSION:-}"
if [[ -z "$VERSION" ]]; then
  VERSION="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' SafeUninstaller/Info.plist 2>/dev/null || echo "1.0.0")"
fi

DIST="$ROOT/dist"
STAGE="$DIST/stage"
APP_NAME="SafeUninstaller.app"
ZIP_NAME="SafeUninstaller-${VERSION}.zip"

echo "==> Generating Xcode project (if needed)"
if command -v xcodegen >/dev/null 2>&1; then
  xcodegen generate
fi

echo "==> Building Release (ad-hoc signing)"
xcodebuild \
  -scheme SafeUninstaller \
  -configuration Release \
  -destination 'platform=macOS' \
  -derivedDataPath "$DIST/DerivedData" \
  CODE_SIGN_IDENTITY="-" \
  CODE_SIGNING_ALLOWED=YES \
  build

BUILT_APP="$DIST/DerivedData/Build/Products/Release/$APP_NAME"
if [[ ! -d "$BUILT_APP" ]]; then
  echo "error: built app not found at $BUILT_APP" >&2
  exit 1
fi

echo "==> Staging $APP_NAME"
rm -rf "$STAGE"
mkdir -p "$STAGE"
ditto "$BUILT_APP" "$STAGE/$APP_NAME"
xattr -cr "$STAGE/$APP_NAME" 2>/dev/null || true

echo "==> Zipping"
mkdir -p "$DIST"
ZIP_PATH="$DIST/$ZIP_NAME"
rm -f "$ZIP_PATH"
(
  cd "$STAGE"
  ditto -c -k --keepParent "$APP_NAME" "$ZIP_PATH"
)

CHECKSUM="$(shasum -a 256 "$ZIP_PATH" | awk '{print $1}')"
echo "$CHECKSUM  $ZIP_NAME" > "$DIST/$ZIP_NAME.sha256"

echo ""
echo "Packaged: $ZIP_PATH"
echo "SHA-256:  $CHECKSUM"
echo ""
echo "Upload $ZIP_NAME (and the .sha256 file) to a GitHub Release."
echo "Users can install with Scripts/install.sh after you set REPO=owner/name."
