# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 2.2.x   | :white_check_mark: |
| < 2.2   | :x:                |

---

## Reporting a Security Concern

The security of system maintenance tools is paramount. Sift incorporates defense-in-depth security invariants:
- Direct root/sudo execution lockout to prevent permission elevation attacks.
- Strict System Integrity Protection (SIP) and protected path denylists.
- Inode and device binding to defend against Time-of-Check to Time-of-Use (TOCTOU) symlink race attacks.
- Atomic file writes with `0644`/`0600` permission clamping.

If you discover a security vulnerability or bypass in any safety engine layer, please **do not report it via a public GitHub issue**.

Instead, please report security vulnerabilities directly to the maintainer via GitHub Private Vulnerability Reporting:
- Navigate to the **Security** tab of [github.com/SadeepaNHerath/Sift](https://github.com/SadeepaNHerath/Sift)
- Click **"Report a vulnerability"**

We will acknowledge receipt within 48 hours and work with you on a coordinated disclosure plan.
