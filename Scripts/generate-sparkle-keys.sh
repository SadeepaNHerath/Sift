#!/bin/bash
# Generate Sparkle EdDSA keys (no Apple Developer account required).
# Public key → SafeUninstaller/Resources/sparkle-public-ed-key.txt (safe to commit)
# Private key → sparkle-private/ed-private-key.txt (gitignored — never commit)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

PUBLIC_OUT="$ROOT/SafeUninstaller/Resources/sparkle-public-ed-key.txt"
PRIVATE_DIR="$ROOT/sparkle-private"
PRIVATE_OUT="$PRIVATE_DIR/ed-private-key.txt"
TOOLS_DIR="$ROOT/dist/sparkle-tools"
SPARKLE_VERSION="${SPARKLE_VERSION:-2.7.1}"

mkdir -p "$PRIVATE_DIR" "$TOOLS_DIR" "$(dirname "$PUBLIC_OUT")"

if [[ -f "$PRIVATE_OUT" && -f "$PUBLIC_OUT" ]]; then
  echo "Keys already exist:"
  echo "  public:  $PUBLIC_OUT"
  echo "  private: $PRIVATE_OUT"
  exit 0
fi

GENERATE_KEYS=""
shopt -s nullglob
CANDIDATES=(
  "$TOOLS_DIR"/bin/generate_keys
  "$TOOLS_DIR"/*/bin/generate_keys
  "$HOME"/Library/Developer/Xcode/DerivedData/*/SourcePackages/artifacts/sparkle/Sparkle/bin/generate_keys
)
for match in "${CANDIDATES[@]}"; do
  if [[ -x "$match" ]]; then
    GENERATE_KEYS="$match"
    break
  fi
done
shopt -u nullglob

if [[ -z "$GENERATE_KEYS" ]]; then
  echo "==> Downloading Sparkle $SPARKLE_VERSION tools…"
  ARCHIVE="$TOOLS_DIR/Sparkle.tar.xz"
  curl -fsSL -o "$ARCHIVE" \
    "https://github.com/sparkle-project/Sparkle/releases/download/${SPARKLE_VERSION}/Sparkle-${SPARKLE_VERSION}.tar.xz"
  tar -xJf "$ARCHIVE" -C "$TOOLS_DIR"
  GENERATE_KEYS="$(find "$TOOLS_DIR" -type f -name generate_keys | head -n 1)"
fi

if [[ -z "$GENERATE_KEYS" || ! -x "$GENERATE_KEYS" ]]; then
  echo "error: could not locate Sparkle generate_keys" >&2
  exit 1
fi

echo "==> Generating EdDSA key pair with $GENERATE_KEYS"
# -f writes private key to the given path (Sparkle 2)
OUT="$("$GENERATE_KEYS" -f "$PRIVATE_OUT" 2>&1)"
echo "$OUT"
chmod 600 "$PRIVATE_OUT" 2>/dev/null || true

PUBLIC="$(echo "$OUT" | sed -n 's/.*SUPublicEDKey)[[:space:]]*is:[[:space:]]*//p' | head -n 1 | tr -d '[:space:]')"
if [[ -z "$PUBLIC" ]]; then
  PUBLIC="$(echo "$OUT" | sed -n 's/.*Public key.*(for SUPublicEDKey).*[[:space:]]//p' | head -n 1 | tr -d '[:space:]')"
fi
if [[ -z "$PUBLIC" ]]; then
  PUBLIC="$(echo "$OUT" | grep -Eo '[A-Za-z0-9+/]{40,}={0,2}' | head -n 1 || true)"
fi

if [[ -z "$PUBLIC" ]]; then
  echo "error: could not parse public key from generate_keys output" >&2
  echo "$OUT" >&2
  exit 1
fi

printf '%s\n' "$PUBLIC" > "$PUBLIC_OUT"
echo "Wrote public key  → $PUBLIC_OUT"
echo "Wrote private key → $PRIVATE_OUT"
echo ""
echo "Commit the public key. Never commit sparkle-private/."
echo "Next: RELEASE_REPO=owner/repo ./Scripts/package-release.sh"
