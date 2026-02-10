# FCGI SPAWNER KNOWLEDGE BASE

**Generated:** 2026-02-10
**Domain:** FastCGI Process Manager & Proxy

## OVERVIEW
FastCGI process manager that spawns/monitors child processes (via socket or stdio) and proxies HTTP requests to them. Acts as a reverse proxy with process lifecycle management.

## STRUCTURE
```
fcgi-spawner/
├── cmd/
│   ├── spawner/    # CORE LOGIC: Main service + tests
│   └── */          # EXAMPLE APPS: hello, auth, sse, etc.
├── internal/       # (Empty/Missing) - Logic is currently in cmd/spawner
├── scripts/        # Deploy scripts (systemd, nginx)
└── web/            # Default web root for .fcgi binaries
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Core Logic | `cmd/spawner/main.go` | Monolithic main (Config, Spawner, Proxy) |
| Tests | `cmd/spawner/main_test.go` | Table-driven, mocks OS calls |
| Examples | `cmd/*/main.go` | Demo apps (stdio & socket modes) |
| Deploy | `scripts/deploy.sh` | **WARNING**: Uses sudo/system modification |

## CODE MAP
| Symbol | Type | Location | Role |
|--------|------|----------|------|
| `Spawner` | struct | `cmd/spawner/main.go` | Central manager for child processes |
| `childProcess` | struct | `cmd/spawner/main.go` | Wraps cmd execution & socket monitoring |
| `proxyRequest` | func | `cmd/spawner/main.go` | Reverse proxy logic using `fcgi_client` |
| `loadConfig` | func | `cmd/spawner/main.go` | Flag parsing (defaults to `/web`) |

## CONVENTIONS
- **Flags**: Defaults use absolute paths (`/web`, `/var/run`). Override via CLI.
- **Tests**: Mock OS calls via package-level variables (`osRemove`, `osStat`).
- **Modes**: Apps support socket mode (arg 1) or stdio mode (default).

## ANTI-PATTERNS
- **Monolithic Main**: Core logic is in `cmd/spawner/main.go` (~600 lines) instead of `internal/`.
- **Sudo in Scripts**: `deploy.sh` modifies system directly. Unsafe for CI.
- **Test in Cmd**: `cmd/spawner` contains `main_test.go` with `package main`.
- **Absolute Defaults**: Default `WebRoot` is `/web` (root), likely to fail.

## COMMANDS
```bash
# Run Spawner (Local)
go run cmd/spawner/main.go -webRoot ./web -listenAddr :8080

# Build Example
go build -o web/hello.fcgi cmd/hello/main.go

# Test (Core)
cd cmd/spawner && go test -v
```
