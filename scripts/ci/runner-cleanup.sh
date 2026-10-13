#!/bin/bash
set -euo pipefail

# Sift CI/CD Runner Hygiene Script
# Safe automated maintenance script for macOS self-hosted GitHub Actions / GitLab runners.
# Cleans stale toolchains, Docker layers, DerivedData, and verifies runner disk pressure.

echo "▸ [Sift CI] Running automated developer artifact purge..."
sf purge --non-interactive --quiet --all

echo "▸ [Sift CI] Running safe cache cleanup..."
sf clean --yes >/dev/null 2>&1 || true

echo "▸ [Sift CI] Running runner diagnostic health check..."
sf doctor --ci --exit-code

echo "✔ [Sift CI] Runner hygiene check passed successfully."
