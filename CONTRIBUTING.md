# Contributing to Sift (`sf`)

Thank you for your interest in contributing to **Sift**! We welcome bug reports, performance enhancements, documentation improvements, and category scanner contributions.

---

## Code of Conduct & Core Philosophy

Sift is built on four core architectural principles:
1. **Zero Background Daemons**: Sift must never run background services, daemon helpers, or telemetry collectors. It consumes 0 MB when inactive.
2. **Safety First**: Sift strictly refuses direct root/sudo execution (`IsRunningAsSudo()` guard). File deletion uses macOS Trash-first semantics and TOCTOU inode-binding guards.
3. **High Performance**: Native Go, concurrent worker pools, zero external runtime dependencies.
4. **macOS Native**: Respects Apple Silicon, macOS Sequoia/Sonoma standards, SIP paths, and PAM Touch ID authentication.

---

## Development Setup

### Prerequisites

- macOS 13 (Ventura) or newer (macOS 15 Sequoia recommended)
- **Go 1.23+** installed (`brew install go`)
- `git`, `make`

### Building from Source

```bash
git clone https://github.com/SadeepaNHerath/Sift.git
cd Sift

# Build the sf binary into ./bin/sf
make build

# Run unit tests with the Go race detector
make test

# Format code according to strict Go standards
make fmt

# Run static analysis
make lint
```

---

## Submitting Pull Requests

1. Fork the repository on GitHub.
2. Create a feature branch off `dev`: `git checkout -b feature/my-enhancement dev`.
3. Keep commits focused and atomic with descriptive commit messages following the Conventional Commits format (`feat:`, `fix:`, `perf:`, `docs:`).
4. Verify tests and formatting before pushing:
   ```bash
   diff -u <(echo -n) <(gofmt -d .)
   go vet ./...
   go test -race ./...
   ```
5. Submit your Pull Request targeting the **`dev`** branch.
