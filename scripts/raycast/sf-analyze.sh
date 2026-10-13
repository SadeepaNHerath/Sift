#!/bin/bash

# Required parameters:
# @raycast.schemaVersion 1
# @raycast.title Sift Analyze
# @raycast.mode silent
# @raycast.packageName System Maintenance

# Optional parameters:
# @raycast.icon 📊
# @raycast.currentDirectoryPath ~

# Documentation:
# @raycast.description Launch Sift Disk Space Analyzer in terminal
# @raycast.author Sadeepa N Herath

SF_BIN="$(which sf || echo "$HOME/.local/bin/sf")"

# Open in preferred terminal (Ghostty, iTerm, WezTerm, Terminal)
if open -Ra "Ghostty" 2>/dev/null; then
  osascript -e 'tell application "Ghostty" to activate' -e "tell application \"System Events\" to keystroke \"$SF_BIN analyze\\n\""
elif open -Ra "iTerm" 2>/dev/null; then
  osascript -e "tell application \"iTerm\" to create window with default profile command \"$SF_BIN analyze\""
else
  osascript -e "tell application \"Terminal\" to do script \"$SF_BIN analyze\"" -e 'tell application "Terminal" to activate'
fi
