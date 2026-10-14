# Sift (`sf`) 75+ Star Community Launch Playbook

A structured, battle-tested launch kit to promote Sift (`sf`) across developer communities, drive adoption via Homebrew Tap, and surpass the **75+ GitHub star threshold** required for upstream acceptance into `homebrew/core`.

---

## 1. Value Proposition & Positioning

### The Narrative Hook
Most Mac cleaning tools (CleanMyMac, DaisyDisk, MacKeeper) have become bloated commercial software:
- They run background helper daemons that consume 300–500 MB of RAM.
- They send telemetry and analytics to third-party tracking servers.
- They charge expensive $40/year subscriptions.
- They have **zero awareness of modern developer caches** (Ollama models, Hugging Face blobs, Docker buildx layers, Xcode DerivedData, vLLM/PyTorch compilation caches).

**Sift is the open-source, terminal-first alternative:**
- **Zero Daemons**: 0 MB idle memory footprint.
- **Offline & Private**: Zero telemetry, zero analytics, MIT licensed.
- **AI-Aware**: The only CLI cleaner targeting local LLM weights (~/.ollama, HF Hub, LM Studio).
- **Fast & Interactive**: Native Bubble Tea TUI with live system metrics and Vim keybindings.
- **Hardware-Accelerated Dedup**: 3-phase duplicate finder using xxhash sparse probing and SHA-256.
- **Safety First**: Non-root guard, SIP denylist, and anti-TOCTOU Trash-first staging.

---

## 2. Platform 1: Hacker News (Show HN)

### Submission Details
- **Target URL**: `https://github.com/SadeepaNHerath/Sift`
- **Best Timing**: Tuesday or Wednesday at 13:00 UTC (8:00 AM EST)
- **Title**:
  ```text
  Show HN: Sift – A terminal-first, open source macOS cleaner written in Go
  ```

### Post Text / First Comment (Post immediately after submitting)
```markdown
Hi HN! I built Sift (sf) because I was tired of commercial Mac cleaners running background daemons, eating 400 MB of RAM, and charging subscriptions just to clear system caches.

As a developer, my biggest disk hogs weren't just browser downloads—they were Ollama blobs, Hugging Face models, Docker buildx caches, and Xcode DerivedData. Existing cleaners don't know what an AI worktree or PyTorch kernel cache is.

Key features:
1. Terminal-First TUI: Built with Charmbracelet Bubble Tea. Real-time Apple Silicon metrics, disk pressure graphs, and Vim-style navigation.
2. AI & LLM Cache Purger: Audits Ollama (~/.ollama/models), HF Hub, LM Studio, Jan, and PyTorch kernels.
3. 3-Phase Duplicate Finder: Fast sparse probing (first/middle/last 4KB with xxhash) + full SHA-256 verification.
4. Native Touch ID for Sudo: Configures /etc/pam.d/sudo_local atomically with backup rollback.
5. Safety Architecture: Refuses to run as sudo (anti-privilege escalation), respects SIP, and stages deletions in macOS Trash (~/.Trash).

Install via Homebrew:
$ brew tap SadeepaNHerath/tap
$ brew install sf

Code and architecture: https://github.com/SadeepaNHerath/Sift
Feedback and suggestions welcome!
```

---

## 3. Platform 2: Reddit

### Subreddit 1: `r/mac` and `r/macapps`
- **Title**: `I built an open-source, terminal-first Mac cleaner and disk analyzer to replace CleanMyMac`
- **Post Content**:
```markdown
Hey everyone!

I wanted a clean, fast way to maintain my Mac without installing commercial utilities that run background daemons and charge $40/year subscriptions.

So I wrote **Sift (`sf`)** in Go. It’s 100% free, open-source (MIT), and runs completely offline with zero telemetry.

What it does:
- 🧹 **Deep Clean**: Scans 14 distinct macOS cache categories (app caches, browser disk caches, dev tools, diagnostic crash reports).
- 🗑️ **App Uninstaller**: Finds orphaned application support containers, preferences, and caches when you remove an app.
- 📊 **Disk Explorer**: Interactive TUI to drill down into directory sizes in real time.
- 🐳 **AI & Dev Purge**: Specifically targets Ollama blobs, Hugging Face caches, Docker layers, and Xcode DerivedData.
- ⚡ **Optimize**: Rebuilds QuickLook, flushes DNS, and tunes APFS local snapshots.
- 🔐 **Touch ID**: Enables Touch ID authentication for sudo with atomic PAM backups.

Install via Homebrew:
```bash
brew tap SadeepaNHerath/tap
brew install sf
```

GitHub repository: https://github.com/SadeepaNHerath/Sift

Would love your thoughts and feature requests!
```

### Subreddit 2: `r/golang`
- **Title**: `Sift (sf): High-performance macOS maintenance toolkit built with Go and Bubble Tea`
- **Focus**: Concurrency, worker pool directory scanners, APFS case insensitivity, TOCTOU safety validation, and Charmbracelet Bubble Tea UI architecture.

### Subreddit 3: `r/commandline`
- **Title**: `Sift (sf) – Terminal-first disk maintenance and duplicate finder for macOS`

---

## 4. Platform 3: X / Twitter Launch Thread

### Tweet 1 (Hook + Demo Video/GIF)
> Introducing Sift (`sf`): A terminal-first macOS maintenance & disk analysis toolkit built for developers.
>
> Replaces bloated commercial cleaners with a single, fast binary. Zero daemons. 0 MB idle RAM. 100% offline.
>
> 🍺 Install: `brew tap SadeepaNHerath/tap && brew install sf`
> 🧵👇

### Tweet 2 (AI & Modern Bloat)
> Modern developers lose 50 GB - 200 GB+ to local AI models without realizing it.
>
> Sift is the first Mac CLI to specifically audit:
> - Ollama blobs (`~/.ollama/models`)
> - Hugging Face Hub caches
> - LM Studio & Jan weights
> - PyTorch / Triton compilation caches

### Tweet 3 (Interactive TUI)
> Built with @charmcli Bubble Tea:
> - Real-time Apple Silicon CPU & memory pressure
> - 3-phase duplicate file finder (xxhash sparse probe + SHA-256)
> - Dynamic viewport windowing and Vim keybindings (`j`/`k`, 1-9 shortcuts)

### Tweet 4 (Safety Architecture)
> Safety is non-negotiable on macOS:
> 🛡️ Refuses `sudo sf` to prevent root privilege escalation
> 🛡️ Staged in `~/.Trash` (never raw unlinks)
> 🛡️ Atomic PAM Touch ID configuration with automated rollback
> 🛡️ Immutable SIP denylist

### Tweet 5 (CTA)
> Sift is 100% free and open-source under MIT.
>
> Star the repo on GitHub: https://github.com/SadeepaNHerath/Sift
> Let me know what you think! 🚀

---

## 5. Platform 4: Curated GitHub Lists (Instant Backlinks & Stars)

Submit a Pull Request to these high-traffic curated lists:
1. **awesome-mac**: `https://github.com/jaywcjlove/awesome-mac` (Section: *Utilities* / *Command Line Tools*)
2. **awesome-go**: `https://github.com/avelino/awesome-go` (Section: *Command Line Interfaces*)
3. **awesome-cli-apps**: `https://github.com/agarrharr/awesome-cli-apps`

### Sample PR Title & Description
- **Title**: `Add Sift: Terminal-first macOS maintenance and disk analysis toolkit`
- **Link**: `- [Sift](https://github.com/SadeepaNHerath/Sift) - Terminal-first macOS maintenance, app uninstallation, and disk analysis toolkit.`

---

## 6. Milestone Checklist to Homebrew Core

- [ ] **Step 1**: Deploy `SadeepaNHerath/homebrew-tap` via `./scripts/homebrew/setup-tap.sh`.
- [ ] **Step 2**: Publish launch posts on Hacker News (Show HN), Reddit (`r/mac`, `r/commandline`), and X.
- [ ] **Step 3**: Monitor GitHub Stars counter until reaching **75 stars**.
- [ ] **Step 4**: Submit PR to `Homebrew/homebrew-core` using `Formula/sift.rb` (or `Formula/sf.rb`).
