# GITHUB NOTIFICATIONS OAUTH KNOWLEDGE BASE

**Generated:** 2026-02-10
**Domain:** OAuth2 Service & GitHub API Integration

## OVERVIEW
Web service handling GitHub OAuth flow and notification retrieval. Uses standard Clean Architecture-ish layout (internal/handlers, internal/services).

## STRUCTURE
```
github-notifications-oauth/
├── cmd/
│   └── server/         # Entry point (main.go)
├── internal/
│   ├── config/         # Env loading & config struct
│   ├── handlers/       # HTTP handlers (auth, callback, notifications)
│   └── services/       # Business logic (GitHub API client)
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Entry Point | `cmd/server/main.go` | Route registration, server start |
| Auth Logic | `internal/handlers/http.go` | OAuth callback & session handling |
| GitHub API | `internal/services/github.go` | API interactions |
| Config | `internal/config/config.go` | Environment variable parsing |

## CONVENTIONS
- **Module Name**: `github-notifications-oauth` (Non-canonical, lacks domain).
- **Structure**: Uses `internal/` pattern correctly (unlike spawner).
- **Env Vars**: Required: `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`, `OAUTH_STATE_STRING`.

## COMMANDS
```bash
# Run Server
export GITHUB_CLIENT_ID=...
export GITHUB_CLIENT_SECRET=...
export OAUTH_STATE_STRING=...
go run cmd/server/main.go -listenAddr :8080
```
