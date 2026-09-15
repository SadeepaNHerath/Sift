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
#   INSTALL_ZIP_URL — direct zip URL (skips GitHub API); must be https://github.com/… or file://…
#   DEST   — install directory (default: ~/Applications)
#   SKIP_CHECKSUM=1 — skip SHA-256 verification (not recommended)
set -euo pipefail

DEST="${DEST:-$HOME/Applications}"
TAG="${TAG:-latest}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "SafeUninstaller installer"
echo "-------------------------"

is_allowed_zip_url() {
  local u="$1"
  [[ -f "$u" ]] && return 0
  [[ "$u" == file://* ]] && return 0
  [[ "$u" == https://github.com/* ]] && return 0
  return 1
}

SHA_URL=""
if [[ -n "${INSTALL_ZIP_URL:-}" ]]; then
  ZIP_URL="$INSTALL_ZIP_URL"
  if ! is_allowed_zip_url "$ZIP_URL"; then
    echo "error: INSTALL_ZIP_URL must be a local path, file://…, or https://github.com/…" >&2
    exit 1
  fi
  if [[ "$ZIP_URL" == https://github.com/* && "$ZIP_URL" == *.zip ]]; then
    SHA_URL="${ZIP_URL}.sha256"
  fi
elif [[ -n "${REPO:-}" ]]; then
  if [[ "$TAG" == "latest" ]]; then
    API="https://api.github.com/repos/${REPO}/releases/latest"
  else
    API="https://api.github.com/repos/${REPO}/releases/tags/${TAG}"
  fi
  echo "Fetching release metadata from $API …"
  META="$(curl -fsSL "$API")"
  ZIP_URL="$(printf '%s' "$META" | python3 -c '
import json,sys
data=json.load(sys.stdin)
for a in data.get("assets", []):
    name=a.get("name","")
    if name.startswith("SafeUninstaller-") and name.endswith(".zip") and not name.endswith(".zip.sha256"):
        print(a["browser_download_url"])
        sys.exit(0)
sys.exit("No SafeUninstaller-*.zip asset found on this release")
')"
  SHA_URL="$(printf '%s' "$META" | python3 -c '
import json,sys
data=json.load(sys.stdin)
for a in data.get("assets", []):
    name=a.get("name","")
    if name.startswith("SafeUninstaller-") and name.endswith(".zip.sha256"):
        print(a["browser_download_url"])
        sys.exit(0)
' || true)"
else
  cat >&2 <<'EOF'
error: set REPO=owner/repo or INSTALL_ZIP_URL=https://github.com/.../SafeUninstaller-x.y.z.zip

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
  if [[ -f "${ZIP_URL}.sha256" ]]; then
    cp "${ZIP_URL}.sha256" "$TMP/SafeUninstaller.zip.sha256"
    SHA_URL="$TMP/SafeUninstaller.zip.sha256"
  fi
elif [[ "$ZIP_URL" == file://* ]]; then
  LOCAL_PATH="${ZIP_URL#file://}"
  LOCAL_PATH="$(printf '%b' "${LOCAL_PATH//%/\\x}")"
  cp "$LOCAL_PATH" "$ZIP_FILE"
  if [[ -f "${LOCAL_PATH}.sha256" ]]; then
    cp "${LOCAL_PATH}.sha256" "$TMP/SafeUninstaller.zip.sha256"
    SHA_URL="$TMP/SafeUninstaller.zip.sha256"
  fi
else
  curl -fL --progress-bar -o "$ZIP_FILE" "$ZIP_URL"
fi

if [[ "${SKIP_CHECKSUM:-0}" != "1" ]]; then
  SHA_FILE="$TMP/SafeUninstaller.zip.sha256"
  if [[ -n "${SHA_URL:-}" && "$SHA_URL" != "$TMP/SafeUninstaller.zip.sha256" ]]; then
    if [[ -f "$SHA_URL" ]]; then
      cp "$SHA_URL" "$SHA_FILE"
    elif [[ "$SHA_URL" == https://github.com/* ]]; then
      echo "Downloading checksum: $SHA_URL"
      if ! curl -fL --progress-bar -o "$SHA_FILE" "$SHA_URL"; then
        echo "error: could not download .sha256 — refuse to install without verification" >&2
        echo "       set SKIP_CHECKSUM=1 to override (not recommended)" >&2
        exit 1
      fi
    fi
  fi
  if [[ -f "$SHA_FILE" ]]; then
    echo "Verifying SHA-256…"
    EXPECTED="$(awk '{print $1}' "$SHA_FILE" | head -n 1)"
    ACTUAL="$(shasum -a 256 "$ZIP_FILE" | awk '{print $1}')"
    if [[ -z "$EXPECTED" || "$EXPECTED" != "$ACTUAL" ]]; then
      echo "error: checksum mismatch" >&2
      echo "  expected: $EXPECTED" >&2
      echo "  actual:   $ACTUAL" >&2
      exit 1
    fi
    echo "Checksum OK."
  elif [[ "$ZIP_URL" == https://github.com/* ]]; then
    echo "error: no .sha256 asset found for this release — refuse to install" >&2
    echo "       set SKIP_CHECKSUM=1 to override (not recommended)" >&2
    exit 1
  else
    echo "warning: no .sha256 beside local zip — skipping verification"
  fi
else
  echo "warning: SKIP_CHECKSUM=1 — integrity not verified"
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

echo "Clearing Gatekeeper quarantine attribute…"
xattr -dr com.apple.quarantine "$APP_DEST" 2>/dev/null || true

if xattr -l "$APP_DEST" 2>/dev/null | grep -q com.apple.quarantine; then
  echo "warning: quarantine attribute may still be present — use Right-click → Open on first launch"
else
  echo "Quarantine attribute cleared."
fi

echo ""
echo "Installed: $APP_DEST"
echo ""
echo "First launch checklist:"
echo "  1. If macOS says the app can’t be opened:"
echo "       Finder → Right-click SafeUninstaller → Open → Open"
echo "     (required once for ad-hoc / unsigned builds — no paid Apple account)"
echo "  2. Grant Full Disk Access:"
echo "       System Settings → Privacy & Security → Full Disk Access"
echo "  3. Empty Trash may ask for Automation (Finder) the first time."
echo ""
if command -v spctl >/dev/null 2>&1; then
  echo "Gatekeeper assess (informational):"
  spctl --assess --type execute -v "$APP_DEST" 2>&1 || true
  echo ""
fi
echo "Note: This build is ad-hoc signed (no paid Apple Developer certificate)."
echo "User Library cleanup works. The privileged /Library helper stays unavailable."
echo "Diagnostics stay on-device (Help → Copy Diagnostics). No telemetry."
echo ""

if command -v open >/dev/null 2>&1; then
  open "$APP_DEST" || true
fi
