# Sift (`sf`)

```text
   ____ ___ _____ _____ 
  / ___|_ _|  ___|_   _|
  \___ \ | || |_    | |  
   ___) || ||  _|   | |  
  |____/|___|_|     |_|  
```

> **Terminal-first macOS maintenance & disk analysis toolkit**

[![GitHub Release](https://img.shields.io/github/v/release/SadeepaNHerath/Sift?style=flat&logo=github&color=388bfd)](https://github.com/SadeepaNHerath/Sift/releases/latest)
[![CI Status](https://github.com/SadeepaNHerath/Sift/actions/workflows/test.yml/badge.svg)](https://github.com/SadeepaNHerath/Sift/actions/workflows/test.yml)
[![Homebrew Tap](https://img.shields.io/badge/Homebrew-SadeepaNHerath%2Ftap-orange?style=flat&logo=homebrew)](https://github.com/SadeepaNHerath/homebrew-tap)
[![Go Version](https://img.shields.io/badge/Go-1.23%2B-00ADD8?style=flat&logo=go)](https://golang.org)
[![Platform](https://img.shields.io/badge/Platform-macOS%20%28arm64%20%7C%20x86__64%29-000000?style=flat&logo=apple)](https://apple.com)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

---

**Sift (`sf`)** is a blazing-fast, terminal-first macOS maintenance, app uninstallation, and disk analysis toolkit. Built in Go, it replaces bloated commercial cleaners, background telemetry daemons, and expensive subscription utilities with a single, standalone binary that puts full control back into your hands.

---

## Table of Contents

- [Why Sift?](#why-sift)
- [Feature Comparison](#feature-comparison)
- [Installation](#installation)
- [Quick Start Guide](#quick-start-guide)
- [Command Reference](#command-reference)
- [Safety Guarantees](#safety-guarantees)
- [Configuration Paths](#configuration-paths)
- [Development](#development)
- [License](#license)

---

## Why Sift?

Commercial Mac cleaners often install persistent background daemons, consume hundreds of megabytes of RAM, send telemetry to third-party servers, and charge ongoing subscription fees. 

**Sift takes the opposite approach:**
- **Zero Background Daemons**: Runs only when you invoke it. Consumes 0 MB of idle RAM.
- **Terminal-First & Interactive**: Launch `sf` for a keyboard-driven TUI, or run specific subcommands with flags in scripts and automation pipelines.
- **Safety First**: Refuses to run directly under `sudo` to protect against unintentional root damage. Features strict SIP and system path denylists, dry-run simulation, and macOS Trash-first deletion.
- **Developer-Centric**: Deep purging for Xcode `DerivedData`, Docker containers/images, Homebrew caches, Gradle, and Rust Cargo artifacts.
- **Touch ID Elevation**: Native PAM integration allows Touch ID verification for elevated operations without typing administrator passwords.

---

## Feature Comparison

| Feature / Capability | Sift (`sf`) | CleanMyMac X | AppCleaner | DaisyDisk | iStat Menus |
| :--- | :---: | :---: | :---: | :---: | :---: |
| **Pricing** | **100% Free & Open Source** | $39.95/yr subscription | Free (Closed Source) | $9.99 one-time | $11.99 one-time |
| **License** | **MIT** | Proprietary | Proprietary | Proprietary | Proprietary |
| **Interface** | **Terminal TUI & CLI** | Cocoa / Electron GUI | Basic Cocoa GUI | GUI Treemap | Menu Bar GUI |
| **Idle Memory Footprint** | **0 MB (No Daemons)** | 400 MB+ (Background) | 0 MB | 0 MB | ~80 MB (Always-on) |
| **Telemetry & Privacy** | **Zero Telemetry (100% Offline)** | Analytics & Tracking | None | Analytics | Analytics |
| **App Uninstaller & Leftovers** | **Yes (Deep Container Scan)** | Yes | Yes (Basic Library) | No | No |
| **Disk Space Analyzer** | **Yes (Multi-threaded)** | Basic | No | Yes (Visual Treemap) | Disk stats only |
| **Developer Artifacts Purge** | **Yes (Xcode, Docker, Brew)** | No | No | No | No |
| **Touch ID for Sudo** | **Yes (Native PAM helper)** | Password dialogs | Password dialogs | Password dialogs | Password dialogs |
| **Scriptable & CI-Friendly** | **Yes (UNIX pipes, stdout)** | No | No | No | No |
| **Safety Path Denylist & Non-Root** | **Strict SIP & Non-Root Guard** | Helper daemon | Basic prompt | Read-only | Read-only |

---

## Installation

### Method 1: Homebrew (Recommended)

```bash
brew tap SadeepaNHerath/tap
brew install sf
```

### Method 2: One-Line Shell Installer

```bash
curl -fsSL https://raw.githubusercontent.com/SadeepaNHerath/Sift/main/install.sh | bash
```

### Method 3: From Source

Requires **Go 1.23+** and standard macOS command-line tools:

```bash
git clone https://github.com/SadeepaNHerath/Sift.git
cd Sift
make all
make install
```

Verify your installation:

```bash
sf --version
```

---

## Quick Start Guide

### Interactive TUI Dashboard

Run `sf` without any arguments in your terminal to launch the interactive dashboard:

```bash
sf
```

The interactive menu displays live macOS metrics (hostname, OS build, architecture, disk usage) and provides keyboard shortcuts:
- `↑` / `k` : Move selection up
- `↓` / `j` : Move selection down
- `1` – `6` : Quick jump to shortcut command
- `↵ Enter` : Execute selected action
- `q` / `Esc` / `Ctrl+C` : Clean exit restoring terminal state

```text
   ____ ___ _____ _____ 
  / ___|_ _|  ___|_   _|
  \___ \ | || |_    | |  
   ___) || ||  _|   | |  
  |____/|___|_|     |_|  
  v1.0.0 • Terminal-first macOS maintenance toolkit

┌────────────────────────────────────────────────────────────────────────┐
│  Host: Mac.local │ OS: macOS 27.0 (arm64) │ Disk: 32.5 GB free / 228.3 GB (85.8% used) │ User: admin │
└────────────────────────────────────────────────────────────────────────┘

  Shortcut Commands:

 ❯ [1] 🧹 Clean      Clean system caches, app logs, and temporary junk
   [2] 🗑️  Uninstall  Safely remove applications and all associated leftovers
   [3] ⚡ Optimize   Rebuild caches, flush DNS, and run system optimizations
   [4] 📊 Analyze    Analyze disk space usage and locate large files
   [5] 🩺 Status     Check system health, disk metrics, and security overview
   [6] 🔥 Purge      Deep purge Xcode, Docker, Homebrew, and dev caches

  ↑/k Up  •  ↓/j Down  •  1-6 Quick Jump  •  ↵ Enter Launch  •  q/Esc Quit
```

> **Note**: When `sf` is invoked in a pipe or non-interactive environment (e.g. `sf | cat`), the interactive menu is bypassed automatically and standard CLI help is returned.

---

## Command Reference

| Command | Shortcut | Description |
| :--- | :---: | :--- |
| [`sf clean`](#sf-clean) | `1` | Scan and clean system, user, and application caches |
| [`sf uninstall`](#sf-uninstall) | `2` | Locate applications and eliminate all associated leftovers |
| [`sf optimize`](#sf-optimize) | `3` | Flush DNS, purge RAM caches, and optimize LaunchServices |
| [`sf analyze`](#sf-analyze) | `4` | High-speed multi-threaded disk usage and large file analyzer |
| [`sf status`](#sf-status) | `5` | Comprehensive overview of system storage, health, and security |
| [`sf purge`](#sf-purge) | `6` | Deep purge developer artifacts (Xcode, Docker, Homebrew, Cargo) |
| [`sf installer`](#sf-installer) | — | Find and remove stale `.dmg`, `.pkg`, and installer archives |
| [`sf touchid`](#sf-touchid) | — | Configure Touch ID PAM authentication for terminal operations |
| [`sf doctor`](#sf-doctor) | — | Diagnose permissions, SIP status, disk access, and environment |
| [`sf history`](#sf-history) | — | Inspect the historical audit log of all maintenance actions |
| [`sf update`](#sf-update) | — | Check for and apply updates to Sift from GitHub releases |
| [`sf completion`](#sf-completion) | — | Generate shell autocompletion scripts |

---

### Command Details

#### `sf clean`
Safely identify and remove user caches, application logs, diagnostic reports, and temporary files.
```bash
sf clean                    # Interactive scan and cleanup
sf clean --dry-run          # Preview items to be cleaned without deleting
sf clean --all              # Clean all safe system and user cache targets
sf clean --user             # Target only ~/Library caches
```

#### `sf uninstall [app...]`
Comprehensively uninstalls macOS `.app` bundles along with all sandboxed Containers, Application Support folders, preferences (`.plist`), crash logs, and launch daemons.
```bash
sf uninstall "Slack"
sf uninstall /Applications/Figma.app --dry-run
sf uninstall "Zoom" --trash # Move leftovers to Trash instead of permanent purge
```

#### `sf optimize`
Runs macOS maintenance scripts, flushes local DNS resolvers, rebuilds the LaunchServices database, and frees purgeable memory.
```bash
sf optimize
sf optimize --dns           # Flush mDNSResponder cache only
sf optimize --ram           # Release inactive system memory
```

#### `sf analyze [path]`
Parallel multi-threaded scanner that breaks down disk usage, identifies large files, and surfaces directory hogs.
```bash
sf analyze                  # Analyze root disk volume
sf analyze ~ --min-size 1GB # Show directories containing files > 1 GB
sf analyze /Users/Shared --depth 3
```

#### `sf status`
Displays instant system diagnostics including APFS volume snapshots, memory breakdown, thermal state, and SIP status.
```bash
sf status
sf status --json            # Output machine-readable JSON format
```

#### `sf purge`
Developer-tailored deep cleaner to reclaim gigabytes of storage from build artifacts.
```bash
sf purge                    # Interactive selection of dev caches
sf purge --xcode            # Clear Xcode DerivedData, Archives, and iOS DeviceSupport
sf purge --docker           # Prune unused Docker images, containers, and build cache
sf purge --brew             # Clear Homebrew download caches and old formula versions
```

#### `sf installer`
Scans `~/Downloads` and cache directories for orphaned DMG disk images, PKG installers, and ISO images.
```bash
sf installer
sf installer --dry-run
```

#### `sf touchid`
Configures `/etc/pam.d/sudo` to enable Touch ID authentication for elevated commands in Terminal sessions.
```bash
sf touchid --status         # Check current Touch ID PAM configuration
sf touchid --enable         # Safely configure Touch ID for sudo
```

#### `sf doctor`
Validates full disk access permissions, checks System Integrity Protection (SIP), inspects terminal permissions, and diagnoses broken symbolic links.
```bash
sf doctor
```

#### `sf history`
Queries the local SQLite audit ledger to display previous operations, files cleaned, and disk space saved.
```bash
sf history
sf history --limit 20
```

#### `sf completion [shell]`
Generate autocompletion scripts for your shell:
```bash
# zsh
sf completion zsh > ~/.zsh/completion/_sf

# bash
sf completion bash > /usr/local/etc/bash_completion.d/sf

# fish
sf completion fish > ~/.config/fish/completions/sf.fish
```

---

## Safety Guarantees

Sift is engineered specifically to prevent data loss and system corruption:

1. **Non-Root Execution Guard**:
   Sift enforces an explicit EUID check that refuses direct execution via `sudo sf`. Running cleaners directly as root can damage system permissions and inadvertently delete protected user data. When elevated privileges are needed, Sift requests them scoped to the specific operation.

2. **System Integrity Protection (SIP) Compliance**:
   Sift respects Apple's SIP boundaries. System directories (`/System`, `/usr/bin`, `/sbin`, `/Library/Apple`) are hard-coded into an immutable safety denylist and will never be touched.

3. **Dry-Run Mode**:
   Every destructive operation supports `--dry-run` to preview all target files, paths, and byte sizes before any action is taken.

4. **Trash-First Strategy**:
   By default, user files and applications are moved to the macOS Trash (`~/.Trash`) rather than permanently unlinked, giving you a safe rollback window.

5. **Local Audit Ledger**:
   All deletions and system changes are recorded in a local SQLite database (`~/.local/share/sf/history.db`) with timestamps, exact paths, and byte counts.

---

## Configuration Paths

Sift adheres to standard XDG and macOS path conventions:

| Purpose | Path |
| :--- | :--- |
| **Configuration** | `~/.config/sf/config.toml` |
| **Audit Logs & History** | `~/.local/share/sf/history.db` |
| **Application Logs** | `~/Library/Logs/sf/` |
| **Temporary Cache** | `~/Library/Caches/sf/` |

---

## Development

Build and test Sift locally:

```bash
# Format code
make fmt

# Static analysis
make vet

# Run all tests with race detector
make test

# Build binary
make build

# Complete check & build
make all
```

---

## License

This project is licensed under the **MIT License**. See the [LICENSE](LICENSE) file for details.

Copyright © 2026 SadeepaNHerath. All rights reserved.
