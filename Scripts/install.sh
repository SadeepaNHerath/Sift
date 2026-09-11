#!/bin/bash
# Install SafeUninstaller from a GitHub Release (no App Store / no paid Apple account).
#
# One-liner (after publishing a Release and pushing this script):
#   curl -fsSL https://raw.githubusercontent.com/OWNER/REPO/main/Scripts/install.sh | bash
#
# Or locally:
#   REPO=owner/repo TAG=v1.0.0 ./Scripts/install.sh
#
# Env:
#   REPO   — GitHub "owner/repo" (required unless INSTALL_ZIP_URL is set)
#   TAG    — release tag (default: latest)
#   INSTALL_ZIP_URL — direct zip URL (skips GitHub API)
#   DEST   — install directory (default: ~/Applications)
set -euo pipefail

DEST="${DEST:-$HOME/Applications}"
TAG="${TAG:-latest}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "SafeUninstaller installer"
echo "-------------------------"

if [[ -n "${INSTALL_ZIP_URL:-}" ]]; then
  ZIP_URL="$INSTALL_ZIP_URL"
elif [[ -n "${REPO:-}" ]]; then
  if [[ "$TAG" == "latest" ]]; then
    API="https://api.github.com/repos/${REPO}/releases/latest"
  else
    API="https://api.github.com/repos/${REPO}/releases/tags/${TAG}"
  fi
  echo "Fetching release metadata from $API …"
  ZIP_URL="$(curl -fsSL "$API" | python3 -c '
import json,sys
data=json.load(sys.stdin)
for a in data.get("assets", []):
    name=a.get("name","")
    if name.startswith("SafeUninstaller-") and name.endswith(".zip"):
        print(a["browser_download_url"])
        sys.exit(0)
sys.exit("No SafeUninstaller-*.zip asset found on this release")
')"
else
  cat >&2 <<'EOF'
error: set REPO=owner/repo or INSTALL_ZIP_URL=https://.../SafeUninstaller-x.y.z.zip

Examples:
  REPO=yourname/safeuninstaller curl -fsSL https://raw.githubusercontent.com/yourname/safeuninstaller/main/Scripts/install.sh | bash
  INSTALL_ZIP_URL=https://github.com/yourname/safeuninstaller/releases/download/v1.0.0/SafeUninstaller-1.0.0.zip ./Scripts/install.sh
EOF
  exit 1
fi

echo "Downloading: $ZIP_URL"
ZIP_FILE="$TMP/SafeUninstaller.zip"
if [[ -f "$ZIP_URL" ]]; then
  cp "$ZIP_URL" "$ZIP_FILE"
elif [[ "$ZIP_URL" == file://* ]]; then
  LOCAL_PATH="${ZIP_URL#file://}"
  # Decode minimal %20 for spaces in file URLs
  LOCAL_PATH="$(printf '%b' "${LOCAL_PATH//%/\\x}")"
  cp "$LOCAL_PATH" "$ZIP_FILE"
else
  curl -fL --progress-bar -o "$ZIP_FILE" "$ZIP_URL"
fi

echo "Extracting…"
ditto -x -k "$ZIP_FILE" "$TMP/extracted"
APP_SRC="$(find "$TMP/extracted" -maxdepth 2 -name 'SafeUninstaller.app' -type d | head -n 1)"
if [[ -z "$APP_SRC" || ! -d "$APP_SRC" ]]; then
  echo "error: SafeUninstaller.app not found inside the zip" >&2
  exit 1
fi

mkdir -p "$DEST"
APP_DEST="$DEST/SafeUninstaller.app"
if [[ -d "$APP_DEST" ]]; then
  echo "Removing previous install at $APP_DEST"
  rm -rf "$APP_DEST"
fi

ditto "$APP_SRC" "$APP_DEST"

# Clear Gatekeeper quarantine so a downloaded build can launch after user confirmation.
xattr -dr com.apple.quarantine "$APP_DEST" 2>/dev/null || true

echo ""
echo "Installed: $APP_DEST"
echo ""
echo "First launch:"
echo "  • If macOS blocks the app: Right-click SafeUninstaller → Open → Open"
echo "  • Grant Full Disk Access: System Settings → Privacy & Security → Full Disk Access"
echo ""
echo "Note: This build is ad-hoc signed (no paid Apple Developer certificate)."
echo "User Library cleanup works. The privileged /Library helper stays unavailable."
echo ""

if command -v open >/dev/null 2>&1; then
  open "$APP_DEST" || true
fi
