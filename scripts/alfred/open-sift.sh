#!/bin/bash
# Alfred Workflow Terminal Dispatcher for Sift (sf)
# Usage: ./open-sift.sh [subcommand...] (e.g. ./open-sift.sh clean or ./open-sift.sh)

CMD="sf $*"
CMD="$(echo "$CMD" | xargs)" # trim whitespace

if open -Ra "Ghostty" 2>/dev/null; then
  osascript -e 'tell application "Ghostty" to activate' -e "tell application \"System Events\" to keystroke \"$CMD\\n\""
elif open -Ra "iTerm" 2>/dev/null; then
  osascript -e "tell application \"iTerm\" to create window with default profile command \"$CMD\""
elif open -Ra "WezTerm" 2>/dev/null; then
  open -a "WezTerm" --args start -- "$CMD"
else
  osascript -e "tell application \"Terminal\" to do script \"$CMD\"" -e 'tell application "Terminal" to activate'
fi
