#!/bin/bash

# Required parameters:
# @raycast.schemaVersion 1
# @raycast.title Sift Clean
# @raycast.mode fullOutput
# @raycast.packageName System Maintenance

# Optional parameters:
# @raycast.icon 🧹
# @raycast.currentDirectoryPath ~

# Documentation:
# @raycast.description Deep clean macOS system, user, and application caches with Sift
# @raycast.author Sadeepa N Herath

SF_BIN="$(which sf || echo "$HOME/.local/bin/sf")"

if [ ! -x "$SF_BIN" ]; then
  if [ -x "/opt/homebrew/bin/sf" ]; then
    SF_BIN="/opt/homebrew/bin/sf"
  elif [ -x "/usr/local/bin/sf" ]; then
    SF_BIN="/usr/local/bin/sf"
  else
    echo "Error: 'sf' binary not found. Please install Sift or run install.sh."
    exit 1
  fi
fi

exec "$SF_BIN" clean --yes
