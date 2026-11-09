# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Desk tool ("CC会话管理" / session-deck) that browses, searches, organizes, and resumes local Claude Code sessions stored in `~/.claude/projects`. Go + SQLite + embedded vanilla-JS frontend, shipped as a single Windows binary that listens on `127.0.0.1:7788`.

Runs on Windows only: the resume path drives Windows Terminal / PowerShell, and release CI is `windows-latest`. Go 1.27+.

## Commands

```bash
# Run for development (console output + default data/sessions.db)
go run ./cmd/server

# Run against a scratch sessions dir / DB (useful when testing scanner changes)
go run ./cmd/server -projects /tmp/fake-projects -db /tmp/test.db

# Debug build
go build -o bin/session-deck-debug.exe ./cmd/server

# Release build (GUI subsystem: no console window, logs go to data/session-deck.log)
go build -trimpath -ldflags "-H windowsgui -s -w" -o bin/session-deck.exe ./cmd/server

# Checks required before a PR (gofmt -l . must print nothing)
gofmt -l .
go vet ./...
go test ./...

# Single test
go test ./internal/store -run TestFTSSearchAndTrashVisibility -v

# Package release zip + SHA256SUMS into dist/
pwsh ./scripts/build-release.ps1 -Version v0.1.0 -AppName session-deck
```

Tagging `v*` triggers `.github/workflows/release.yml`: `go test ./...` → `build-release.ps1` → GitHub Release with the zip and checksum.

## Architecture

### Data flow

```
~/.claude/projects/<encoded-cwd>/<uuid>.jsonl   (source of truth, never restructured)
        │
        ▼
internal/scanner   parse metadata, messages, forkedFrom
        │
        ▼
internal/store     SQLite index: projects / folders / sessions / sessions_fts
        │
        ▼
internal/api       JSON + SSE over net/http
        │
        ▼
web/               embedded SPA (go:embed, no build step)
```

`cmd/server/main.go` wires everything: open store, construct scanner/opener/creator/trash, `api.New(...)`, run an initial scan, then start a 30s scan goroutine and the HTTP server. A first start with an empty FTS table resets `file_mtime` so existing rows are re-parsed and indexed.

### Two "project" dimensions per session

- `project_id` — the project derived from the jsonl `cwd`, set by the scanner.
- `display_project_id` — where the session is shown. When the latest `cwd` in the file differs, the scanner auto-attributes it to that project (`display_project_source` empty); a manual user move sets `display_project_source = 'manual'` and the auto-attribution stops. Lists filter on `COALESCE(display_project_id, project_id)`.
- `folders` are logical only: nested groups exist solely in SQLite and never touch `~/.claude/projects`.

### Incremental scanning contract

The scanner skips a session when `size` and `mtime` both match the indexed row (and state is `ok`). Any code path that changes how a jsonl is parsed must force a re-parse by setting `file_mtime = -1` (or calling `InvalidateSession`). Migrations that add parsing-relevant columns do this with `UPDATE sessions SET file_mtime = -1`. The same session ID may appear in several project directories (via `/cd`); the newest mtime wins.

### Session lifecycle

- Delete moves the jsonl and its same-named attachment directory to `~/.claude-session/trash/<session-id>/` and marks the row `deleted`; restore/purge are the only reverse paths.
- Missing files become state `missing`; local metadata (folders, lineage) is retained.
- Rename appends a `custom-title` line to the jsonl so Claude Code picks it up; the DB row is updated too.
- `branch` / `fork` copy the source file line by line, replace `sessionId`, and write `forkedFrom` in the first line — matching Claude Code's on-disk format. Titles become `<title>-branch-NNN` / `<title>-fork-NNN` when no name is given.
- `forkedFrom` is read back by the scanner into `parent_id` / `fork_point_uuid`; derived sessions inherit the parent's folder when the new session has none.

### Write protection and resume

Before renaming/deleting/opening, the API checks `~/.claude/sessions/*.json` for an active session and returns 409. Resuming executes `wt -w 0 nt -d <cwd> pwsh -NoExit -EncodedCommand <base64>`, sanitizing Claude Code environment variables first; fallbacks are plain PowerShell, then `cmd /c start`.

### Refresh

Three paths trigger a scan: the 30s ticker, `POST /api/hooks/refresh` (debounced 500ms, called from Claude Code hooks), and `POST /api/sync`. A completed scan is broadcast over `/api/events` (SSE), which the frontend uses to reload.

## SQLite notes

- Pure-Go driver `modernc.org/sqlite` (no CGO). `MaxOpenConns(1)`, WAL, `busy_timeout=5000`, foreign keys on.
- Schema lives entirely in `internal/store/store.go:migrate`. Migrations are additive: `CREATE TABLE IF NOT EXISTS` plus `hasColumn`-guarded `ALTER TABLE`. Follow that pattern for new columns.
- `sessions_fts` uses the FTS5 `trigram` tokenizer (falling back to `unicode61` if the build lacks it). Queries shorter than 3 runes bypass FTS and use `LIKE`. FTS content is capped at the first 200KB per session.

## Frontend

`web/embed.go` embeds `index.html`, `style.css`, `app.js`, `favicon.svg` into the binary, so any web change needs a rebuild/restart — there is no dev server, bundler, or npm step.

`app.js` is a single strict-mode IIFE: a central `state` object plus `render*` functions that re-render from state; API calls go to the same origin. `style.css` defines a dark default palette and a `:root[data-theme="light"]` override via CSS variables. UI text, comments, and error messages are Simplified Chinese.

## Conventions

- Comments, log lines, API error messages, and UI copy are Simplified Chinese. Go source files start with `// Author: zhangweiwei`.
- Commits follow `feat:` / `fix:` / `docs:` / `refactor:` with an imperative subject.
- Behavior or configuration changes require updating both `README.md` and `README.en.md`, plus a `CHANGELOG.md` entry under `Unreleased`.
- Prefer the standard library; the project deliberately keeps a nearly empty dependency graph. Justify any new dependency.
- Treat `~/.claude/projects` as read-only except for the explicit operations above (rename append, branch/fork copies, trash moves). Grouping and project metadata belong in SQLite.

## Security boundaries

- Bind loopback only. Non-GET/HEAD requests must pass `sameOrigin` (`internal/api/api.go`).
- Session IDs are validated as UUIDs, and project paths must already exist in the index.
- External commands are launched with argument slices, never concatenated shell strings.
- `README.md` contains the full API route list; keep it in sync when adding endpoints.
