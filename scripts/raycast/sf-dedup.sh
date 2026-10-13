#!/bin/bash

# Required parameters:
# @raycast.schemaVersion 1
# @raycast.title Sift Dedup
# @raycast.mode silent
# @raycast.packageName System Maintenance

# Optional parameters:
# @raycast.icon 🔍
# @raycast.currentDirectoryPath ~

# Documentation:
# @raycast.description Launch Sift 3-Phase Duplicate Finder in terminal
# @raycast.author Sadeepa N Herath

SF_BIN="$(which sf || echo "$HOME/.local/bin/sf")"

if open -Ra "Ghostty" 2>/dev/null; then
  osascript -e 'tell application "Ghostty" to activate' -e "tell application \"System Events\" to keystroke \"$SF_BIN dedup\\n\""
elif open -Ra "iTerm" 2>/dev/null; then
  osascript -e "tell application \"iTerm\" to create window with default profile command \"$SF_BIN dedup\""
else
  osascript -e "tell application \"Terminal\" to do script \"$SF_BIN dedup\"" -e 'tell application "Terminal" to activate'
fi
