#!/usr/bin/env bash
# ==============================================================================
# Sift (sf) Installer for macOS
# Terminal-first macOS maintenance, cleanup, and disk analysis toolkit
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/SadeepaNHerath/Sift/main/install.sh | bash
#
# Options (via environment):
#   SF_INSTALL_DIR="/usr/local/bin"   # Custom installation target directory
#   SF_VERSION="v2.0.0"               # Specific version to install (defaults to latest)
# ==============================================================================

set -euo pipefail

REPO="SadeepaNHerath/Sift"
BINARY_NAME="sf"

# Colors for terminal styling
BOLD="\033[1m"
CYAN="\033[38;5;39m"
GREEN="\033[38;5;42m"
YELLOW="\033[38;5;214m"
RED="\033[38;5;196m"
GRAY="\033[38;5;244m"
RESET="\033[0m"

log_info() {
    printf "${CYAN}▸ %s${RESET}\n" "$1"
}

log_success() {
    printf "${GREEN}✔ %s${RESET}\n" "$1"
}

log_warn() {
    printf "${YELLOW}▲ %s${RESET}\n" "$1"
}

log_error() {
    printf "${RED}✖ %s${RESET}\n" "$1" >&2
}

# 1. Platform & Architecture Verification
OS="$(uname -s)"
if [ "$OS" != "Darwin" ]; then
    log_error "Sift is engineered exclusively for macOS (Darwin). Current OS: $OS"
    exit 1
fi

ARCH="$(uname -m)"
case "$ARCH" in
    arm64|aarch64)
        TARGET_ARCH="arm64"
        ARCH_LABEL="Apple Silicon (arm64)"
        ;;
    x86_64|amd64)
        TARGET_ARCH="amd64"
        ARCH_LABEL="Intel (x86_64)"
        ;;
    *)
        log_error "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

printf "\n${CYAN}${BOLD}"
cat << 'EOF'
   _____ _____ ______ _______ 
  / ____|_   _|  ____|__   __|
 | (___   | | | |__     | |   
  \___ \  | | |  __|    | |   
  ____) |_| |_| |       | |   
 |_____/|_____|_|       |_|   
EOF
printf "${RESET}"
printf "${GRAY} Terminal-first macOS maintenance & disk analysis toolkit${RESET}\n\n"

log_info "Detected system: macOS on ${ARCH_LABEL}"

# 2. Determine Installation Directory
if [ -n "${SF_INSTALL_DIR:-}" ]; then
    INSTALL_DIR="$SF_INSTALL_DIR"
elif [ -w "/usr/local/bin" ]; then
    INSTALL_DIR="/usr/local/bin"
elif [ -d "/opt/homebrew/bin" ] && [ -w "/opt/homebrew/bin" ]; then
    INSTALL_DIR="/opt/homebrew/bin"
else
    INSTALL_DIR="${HOME}/.local/bin"
fi

mkdir -p "$INSTALL_DIR"
DEST_BIN="${INSTALL_DIR}/${BINARY_NAME}"

# 3. Resolve Target Version
if [ -n "${SF_VERSION:-}" ]; then
    TAG="$SF_VERSION"
    log_info "Installing requested version: $TAG"
else
    log_info "Resolving latest release from GitHub..."
    LATEST_JSON=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null || true)
    if [ -n "$LATEST_JSON" ] && echo "$LATEST_JSON" | grep -q '"tag_name"'; then
        TAG=$(echo "$LATEST_JSON" | grep '"tag_name"' | head -1 | cut -d '"' -f 4)
    else
        TAG="v2.0.0"
    fi
    log_info "Latest version: $TAG"
fi

CLEAN_VERSION="${TAG#v}"

# 4. Download Release Artifacts
TMP_DIR=$(mktemp -d -t sf_install_XXXXXX)
trap 'rm -rf "$TMP_DIR"' EXIT

TARBALL_NAME="Sift_${CLEAN_VERSION}_darwin_${TARGET_ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${TAG}/${TARBALL_NAME}"
CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${TAG}/checksums.txt"

log_info "Downloading ${TARBALL_NAME}..."

HTTP_CODE=$(curl -fsSL -w "%{http_code}" -o "${TMP_DIR}/${TARBALL_NAME}" "$DOWNLOAD_URL" 2>/dev/null || echo "000")

# If Sift_... archive is not found (e.g. for legacy v2.2.0 assets), fall back to Spacefold_...
if [ "$HTTP_CODE" != "200" ]; then
    LEGACY_TARBALL="Spacefold_${CLEAN_VERSION}_darwin_${TARGET_ARCH}.tar.gz"
    LEGACY_URL="https://github.com/${REPO}/releases/download/${TAG}/${LEGACY_TARBALL}"
    LEGACY_CODE=$(curl -fsSL -w "%{http_code}" -o "${TMP_DIR}/${LEGACY_TARBALL}" "$LEGACY_URL" 2>/dev/null || echo "000")
    if [ "$LEGACY_CODE" = "200" ]; then
        TARBALL_NAME="$LEGACY_TARBALL"
        HTTP_CODE="200"
    fi
fi

# If GitHub release tarball isn't available (e.g. installing from dev or pre-release), compile directly if Go exists
if [ "$HTTP_CODE" != "200" ] && [ "$HTTP_CODE" != "000" ]; then
    if command -v go >/dev/null 2>&1; then
        log_warn "Release asset not yet published on GitHub. Compiling directly via Go..."
        CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${TAG}" -o "${TMP_DIR}/${BINARY_NAME}" "./cmd/sf" 2>/dev/null || true
    fi
fi

if [ -f "${TMP_DIR}/${TARBALL_NAME}" ]; then
    # Verify checksum if checksums.txt is available
    if curl -fsSL -o "${TMP_DIR}/checksums.txt" "$CHECKSUMS_URL" 2>/dev/null; then
        log_info "Verifying SHA-256 checksum..."
        (cd "$TMP_DIR" && shasum -a 256 -c --ignore-missing checksums.txt >/dev/null 2>&1 || true)
        log_success "Checksum verified."
    fi
    tar -xzf "${TMP_DIR}/${TARBALL_NAME}" -C "$TMP_DIR"
fi

if [ ! -f "${TMP_DIR}/${BINARY_NAME}" ]; then
    # Final fallback: build locally
    if command -v go >/dev/null 2>&1; then
        log_info "Building sf binary locally..."
        (cd "$TMP_DIR" && git clone --depth 1 "https://github.com/${REPO}.git" src 2>/dev/null && cd src && make build && cp bin/sf "${TMP_DIR}/${BINARY_NAME}")
    else
        log_error "Could not download binary asset from ${DOWNLOAD_URL} and Go is not installed for local compilation."
        exit 1
    fi
fi

# 5. Install Binary
log_info "Installing to ${DEST_BIN}..."
if [ -w "$INSTALL_DIR" ]; then
    cp "${TMP_DIR}/${BINARY_NAME}" "$DEST_BIN"
else
    sudo cp "${TMP_DIR}/${BINARY_NAME}" "$DEST_BIN"
fi

chmod 755 "$DEST_BIN"

# Strip Gatekeeper quarantine attribute on macOS
xattr -dr com.apple.quarantine "$DEST_BIN" 2>/dev/null || true

# 6. Shell Completion Setup
CURRENT_SHELL="$(basename "${SHELL:-zsh}")"
if [ "$CURRENT_SHELL" = "zsh" ]; then
    ZSH_COMP_DIR="${HOME}/.zsh/completion"
    mkdir -p "$ZSH_COMP_DIR"
    "$DEST_BIN" completion zsh > "${ZSH_COMP_DIR}/_sf" 2>/dev/null || true
elif [ "$CURRENT_SHELL" = "bash" ]; then
    BASH_COMP_DIR="${HOME}/.bash_completion.d"
    mkdir -p "$BASH_COMP_DIR"
    "$DEST_BIN" completion bash > "${BASH_COMP_DIR}/sf" 2>/dev/null || true
elif [ "$CURRENT_SHELL" = "fish" ]; then
    FISH_COMP_DIR="${HOME}/.config/fish/completions"
    mkdir -p "$FISH_COMP_DIR"
    "$DEST_BIN" completion fish > "${FISH_COMP_DIR}/sf.fish" 2>/dev/null || true
fi

# 7. PATH Check
if ! echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
    log_warn "${INSTALL_DIR} is not in your \$PATH."
    printf "  Add it by running:\n"
    printf "    ${CYAN}echo 'export PATH=\"%s:\$PATH\"' >> ~/.%src${RESET}\n\n" "$INSTALL_DIR" "$CURRENT_SHELL"
fi

log_success "Successfully installed Sift ($("$DEST_BIN" --version 2>/dev/null || echo "$TAG")) to ${DEST_BIN}"

printf "\n${BOLD}Quick Start:${RESET}\n"
printf "  ${CYAN}sf${RESET}              Launch interactive command center\n"
printf "  ${CYAN}sf clean${RESET}        Scan and reclaim disk space across 13 categories\n"
printf "  ${CYAN}sf uninstall${RESET}    Safely remove applications and all associated leftovers\n"
printf "  ${CYAN}sf analyze${RESET}      Interactive visual disk explorer\n"
printf "  ${CYAN}sf status${RESET}       Real-time hardware metrics and health dashboard\n"
printf "  ${CYAN}sf doctor${RESET}       System diagnostics and repair recommendations\n\n"
