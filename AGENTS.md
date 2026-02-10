# PROJECT KNOWLEDGE BASE

**Generated:** 2026-02-10
**Type:** Monorepo (Go)

## OVERVIEW
Go-based monorepo containing a FastCGI process manager (`fcgi-spawner`) and a GitHub OAuth notification service (`github-notifications-oauth`). No root module orchestration (missing `go.work`).

## STRUCTURE
```
.
├── fcgi-spawner/               # FastCGI proxy & process manager
├── github-notifications-oauth/ # GitHub API integration service
└── AGENTS.md                   # This file
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Run Spawner | `fcgi-spawner/cmd/spawner` | Core service |
| Run OAuth | `github-notifications-oauth/cmd/server` | Auth service |
| Deploy Scripts | `fcgi-spawner/scripts/` | Systemd/Nginx setup (Caution: sudo) |

## CONVENTIONS
- **Go Modules**: Independent `go.mod` per directory. No unified build.
- **Versions**: Inconsistent Go versions (1.24.0 vs 1.25.1).
- **Scripts**: Deploy scripts rely on `sudo` and system paths (not CI-safe).

## COMMANDS
```bash
# Recommended: Initialize workspace
go work init ./fcgi-spawner ./github-notifications-oauth

# Run Spawner
cd fcgi-spawner && go run cmd/spawner/main.go

# Run OAuth Server
cd github-notifications-oauth && go run cmd/server/main.go
```

## NOTES
- **Missing CI**: No GitHub Actions or Makefile.
- **Root**: No root `go.mod`. Use `go work` or `cd` into modules.
