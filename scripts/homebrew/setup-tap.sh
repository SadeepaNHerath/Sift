#!/usr/bin/env bash
# ==============================================================================
# Sift (sf) — Official Homebrew Tap Setup & Deployment Script
# Initializes, tests, and deploys SadeepaNHerath/homebrew-tap
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
TAP_USER="SadeepaNHerath"
TAP_REPO="homebrew-tap"
FORMULA_SRC="${ROOT_DIR}/Formula/sf.rb"
CORE_SRC="${ROOT_DIR}/Formula/spacefold.rb"

# Color formatting
GREEN="\033[0;32m"
BLUE="\033[0;34m"
YELLOW="\033[0;33m"
RED="\033[0;31m"
BOLD="\033[1m"
RESET="\033[0m"

echo -e "${BLUE}${BOLD}▸ Sift: Homebrew Tap Deployment & Validation${RESET}"
echo -e "  Target: ${BOLD}${TAP_USER}/${TAP_REPO}${RESET}"
echo

# 1. Verify Homebrew formula syntax with brew style
echo -e "${BOLD}[1/4] Auditing formula with brew style...${RESET}"
if command -v brew >/dev/null 2>&1; then
  brew style "${FORMULA_SRC}"
  brew style "${CORE_SRC}"
  echo -e "  ${GREEN}✔ Formula passed RuboCop style check with 0 offenses.${RESET}"
else
  echo -e "  ${YELLOW}⚠ Homebrew not found; skipping local brew style check.${RESET}"
fi

# Exit early if only testing
if [[ "${1:-}" == "--test-only" ]]; then
  echo -e "\n${GREEN}${BOLD}✔ Formula validation completed successfully.${RESET}"
  exit 0
fi

# 2. Check if GitHub CLI is installed and authenticated
echo -e "\n${BOLD}[2/4] Verifying GitHub authentication...${RESET}"
if ! command -v gh >/dev/null 2>&1; then
  echo -e "  ${RED}✗ GitHub CLI (gh) is not installed.${RESET}"
  echo "  Install with: brew install gh && gh auth login"
  exit 1
fi

if ! gh auth token >/dev/null 2>&1; then
  echo -e "  ${RED}✗ GitHub CLI is not authenticated.${RESET}"
  echo "  Run: gh auth login"
  exit 1
fi
echo -e "  ${GREEN}✔ GitHub CLI authenticated as ${TAP_USER}.${RESET}"

# 3. Ensure remote repository exists on GitHub
echo -e "\n${BOLD}[3/4] Ensuring ${TAP_USER}/${TAP_REPO} repository exists...${RESET}"
if gh repo view "${TAP_USER}/${TAP_REPO}" >/dev/null 2>&1; then
  echo -e "  ${GREEN}✔ Remote repository ${TAP_USER}/${TAP_REPO} already exists.${RESET}"
else
  echo -e "  Creating public repository ${BOLD}${TAP_USER}/${TAP_REPO}${RESET}..."
  gh repo create "${TAP_USER}/${TAP_REPO}" --public --description "Official Homebrew Tap for Sift (sf) — macOS Maintenance Toolkit"
  echo -e "  ${GREEN}✔ Repository created successfully.${RESET}"
fi

# 4. Clone, sync formula and README, then push
echo -e "\n${BOLD}[4/4] Syncing formula to tap repository...${RESET}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

git clone "https://github.com/${TAP_USER}/${TAP_REPO}.git" "${TMP_DIR}"
mkdir -p "${TMP_DIR}/Formula"

cp "${FORMULA_SRC}" "${TMP_DIR}/Formula/sf.rb"
cp "${CORE_SRC}" "${TMP_DIR}/Formula/spacefold.rb"

# Generate Tap README
cat > "${TMP_DIR}/README.md" << 'EOF'
# Homebrew Tap for Sift (`sf`)

Official Homebrew tap for [Sift (`sf`)](https://github.com/SadeepaNHerath/Sift) — a terminal-first macOS maintenance, application uninstallation, and disk analysis toolkit.

## Installation

```bash
brew tap SadeepaNHerath/tap
brew install sf
```

Or install directly in a single command:

```bash
brew install SadeepaNHerath/tap/sf
```

## Update

```bash
brew update && brew upgrade sf
```

## Documentation & Issues

For complete documentation, guides, and bug reports, visit the canonical repository:
- **GitHub**: [github.com/SadeepaNHerath/Sift](https://github.com/SadeepaNHerath/Sift)
- **License**: MIT
EOF

cd "${TMP_DIR}"
git add Formula/ README.md
if git diff --staged --quiet; then
  echo -e "  ${GREEN}✔ Tap repository is already up to date with latest formula.${RESET}"
else
  git commit -m "feat: release v2.2.0 formula for Sift (sf)"
  git branch -M main
  git push origin main
  echo -e "  ${GREEN}✔ Successfully pushed latest formula to ${TAP_USER}/${TAP_REPO}.${RESET}"
fi

echo
echo -e "${GREEN}${BOLD}========================================================================${RESET}"
echo -e "${GREEN}${BOLD}✔ Official Homebrew Tap is LIVE!${RESET}"
echo -e "${GREEN}${BOLD}========================================================================${RESET}"
echo -e "Users can now install Sift worldwide via:"
echo -e "  ${BOLD}brew tap ${TAP_USER}/tap${RESET}"
echo -e "  ${BOLD}brew install sf${RESET}"
echo
