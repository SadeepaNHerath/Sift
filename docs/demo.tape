# VHS tape script for Sift (sf)
# Output configuration
Output docs/assets/demo.gif
Output docs/assets/demo.mp4

# Terminal styling
Set FontSize 14
Set Width 1200
Set Height 720
Set Theme "Catppuccin Mocha"
Set Padding 20
Set TypingSpeed 50ms

# Window setup
Hide
Type "export PATH=$PWD/bin:$PATH"
Enter
Type "clear"
Enter
Show
Sleep 1s

# 1. Show interactive Menu
Type "sf"
Enter
Sleep 3s
Type "j"
Sleep 500ms
Type "j"
Sleep 500ms
Type "q"
Sleep 1s

# 2. Run sf doctor
Type "sf doctor"
Enter
Sleep 3s

# 3. Run APFS clone deduplication
Type "sf dedup --clone --dry-run ~/Downloads"
Enter
Sleep 4s
