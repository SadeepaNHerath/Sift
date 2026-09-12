#!/bin/bash
# Build an ad-hoc Release zip for GitHub Releases (no paid Apple Developer account required).
#
# Usage:
#   ./Scripts/package-release.sh
#   VERSION=1.0.0 ./Scripts/package-release.sh
#
# For Sparkle updates (recommended for mass distribution):
#   RELEASE_REPO=owner/repo ./Scripts/package-release.sh
# Requires: sparkle-private/ed-private-key.txt and SafeUninstaller/Resources/sparkle-public-ed-key.txt
#   (run Scripts/generate-sparkle-keys.sh once)
#
# Env:
#   RELEASE_REPO / SU_FEED_URL — enable Sparkle embedding + appcast
#   SKIP_SPARKLE=1 — package zip without updates even if RELEASE_REPO is set
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION="${VERSION:-}"
if [[ -z "$VERSION" ]]; then
  VERSION="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' SafeUninstaller/Info.plist 2>/dev/null || echo "1.0.0")"
fi
BUILD_NUMBER="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleVersion' SafeUninstaller/Info.plist 2>/dev/null || echo "1")"

DIST="$ROOT/dist"
STAGE="$DIST/stage"
APP_NAME="SafeUninstaller.app"
ZIP_NAME="SafeUninstaller-${VERSION}.zip"
PUBLIC_KEY_FILE="$ROOT/SafeUninstaller/Resources/sparkle-public-ed-key.txt"
PRIVATE_KEY_FILE="$ROOT/sparkle-private/ed-private-key.txt"

FEED_URL="${SU_FEED_URL:-}"
if [[ -z "$FEED_URL" && -n "${RELEASE_REPO:-}" ]]; then
  FEED_URL="https://github.com/${RELEASE_REPO}/releases/latest/download/appcast.xml"
fi

WANT_SPARKLE=0
if [[ -n "$FEED_URL" && "${SKIP_SPARKLE:-0}" != "1" ]]; then
  WANT_SPARKLE=1
fi

if [[ "$WANT_SPARKLE" == "1" ]]; then
  if [[ ! -f "$PUBLIC_KEY_FILE" ]]; then
    echo "error: missing $PUBLIC_KEY_FILE — run Scripts/generate-sparkle-keys.sh" >&2
    exit 1
  fi
  if [[ ! -f "$PRIVATE_KEY_FILE" ]]; then
    echo "error: missing $PRIVATE_KEY_FILE — run Scripts/generate-sparkle-keys.sh and keep the private key" >&2
    exit 1
  fi
  PUBLIC_KEY="$(tr -d '[:space:]' < "$PUBLIC_KEY_FILE")"
  if [[ -z "$PUBLIC_KEY" ]]; then
    echo "error: public key file is empty" >&2
    exit 1
  fi
fi

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

INFO_PLIST="$STAGE/$APP_NAME/Contents/Info.plist"
if [[ "$WANT_SPARKLE" == "1" ]]; then
  echo "==> Embedding Sparkle feed URL + public key"
  /usr/libexec/PlistBuddy -c "Set :SUFeedURL $FEED_URL" "$INFO_PLIST" 2>/dev/null \
    || /usr/libexec/PlistBuddy -c "Add :SUFeedURL string $FEED_URL" "$INFO_PLIST"
  /usr/libexec/PlistBuddy -c "Set :SUPublicEDKey $PUBLIC_KEY" "$INFO_PLIST" 2>/dev/null \
    || /usr/libexec/PlistBuddy -c "Add :SUPublicEDKey string $PUBLIC_KEY" "$INFO_PLIST"
  /usr/libexec/PlistBuddy -c "Set :SUEnableAutomaticChecks true" "$INFO_PLIST" 2>/dev/null \
    || /usr/libexec/PlistBuddy -c "Add :SUEnableAutomaticChecks bool true" "$INFO_PLIST"
  # Re-sign after Info.plist mutation (ad-hoc).
  codesign --force --deep --sign - "$STAGE/$APP_NAME" 2>/dev/null || true
fi

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

if [[ "$WANT_SPARKLE" == "1" ]]; then
  echo "==> Signing update + writing appcast.xml"
  SIGN_UPDATE=""
  for match in "$HOME/Library/Developer/Xcode/DerivedData"/*/SourcePackages/artifacts/sparkle/Sparkle/bin/sign_update \
               "$DIST/sparkle-tools"/bin/sign_update \
               "$DIST/sparkle-tools"/*/bin/sign_update; do
    if [[ -x "$match" ]]; then SIGN_UPDATE="$match"; break; fi
  done
  if [[ -z "$SIGN_UPDATE" ]]; then
    echo "==> Fetching Sparkle sign_update tool…"
    TOOLS_DIR="$DIST/sparkle-tools"
    mkdir -p "$TOOLS_DIR"
    SPARKLE_VERSION="${SPARKLE_VERSION:-2.7.1}"
    ARCHIVE="$TOOLS_DIR/Sparkle.tar.xz"
    curl -fsSL -o "$ARCHIVE" \
      "https://github.com/sparkle-project/Sparkle/releases/download/${SPARKLE_VERSION}/Sparkle-${SPARKLE_VERSION}.tar.xz"
    tar -xJf "$ARCHIVE" -C "$TOOLS_DIR"
    SIGN_UPDATE="$(find "$TOOLS_DIR" -type f -name sign_update | head -n 1)"
  fi
  if [[ -z "$SIGN_UPDATE" || ! -x "$SIGN_UPDATE" ]]; then
    echo "error: could not locate Sparkle sign_update" >&2
    exit 1
  fi

  # sign_update prints edSignature HTML attribute value
  SIG_LINE="$("$SIGN_UPDATE" "$ZIP_PATH" -f "$PRIVATE_KEY_FILE")"
  # Extract edSignature="..." length="..."
  ED_SIG="$(echo "$SIG_LINE" | sed -n 's/.*edSignature="\([^"]*\)".*/\1/p')"
  LENGTH="$(echo "$SIG_LINE" | sed -n 's/.*length="\([^"]*\)".*/\1/p')"
  if [[ -z "$LENGTH" ]]; then
    LENGTH="$(stat -f%z "$ZIP_PATH" 2>/dev/null || stat -c%s "$ZIP_PATH")"
  fi
  if [[ -z "$ED_SIG" ]]; then
    # Newer sign_update may print only the signature string
    ED_SIG="$(echo "$SIG_LINE" | tr -d '[:space:]')"
  fi

  DOWNLOAD_URL="${SU_DOWNLOAD_URL:-}"
  if [[ -z "$DOWNLOAD_URL" && -n "${RELEASE_REPO:-}" ]]; then
    DOWNLOAD_URL="https://github.com/${RELEASE_REPO}/releases/download/v${VERSION}/${ZIP_NAME}"
  fi
  if [[ -z "$DOWNLOAD_URL" ]]; then
    DOWNLOAD_URL="${ZIP_NAME}"
  fi

  PUB_DATE="$(date -u '+%a, %d %b %Y %H:%M:%S +0000')"
  cat > "$DIST/appcast.xml" <<EOF
<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <channel>
    <title>SafeUninstaller</title>
    <link>${FEED_URL}</link>
    <description>SafeUninstaller updates</description>
    <language>en</language>
    <item>
      <title>Version ${VERSION}</title>
      <pubDate>${PUB_DATE}</pubDate>
      <sparkle:version>${BUILD_NUMBER}</sparkle:version>
      <sparkle:shortVersionString>${VERSION}</sparkle:shortVersionString>
      <sparkle:minimumSystemVersion>14.0</sparkle:minimumSystemVersion>
      <enclosure
        url="${DOWNLOAD_URL}"
        length="${LENGTH}"
        type="application/octet-stream"
        sparkle:edSignature="${ED_SIG}" />
    </item>
  </channel>
</rss>
EOF
  echo "Wrote $DIST/appcast.xml"
fi

echo ""
echo "Packaged: $ZIP_PATH"
echo "SHA-256:  $CHECKSUM"
if [[ "$WANT_SPARKLE" == "1" ]]; then
  echo "Appcast:  $DIST/appcast.xml"
  echo ""
  echo "Upload to GitHub Release v${VERSION}:"
  echo "  • $ZIP_NAME"
  echo "  • $ZIP_NAME.sha256"
  echo "  • appcast.xml  (must be downloadable at: $FEED_URL)"
else
  echo ""
  echo "Sparkle updates were NOT embedded (set RELEASE_REPO=owner/repo to enable)."
  echo "Upload $ZIP_NAME (and the .sha256 file) to a GitHub Release."
fi
echo "Users can install with Scripts/install.sh after you set REPO=owner/name."
