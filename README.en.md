# Claude Session

English | [简体中文](README.md)

![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)
![License](https://img.shields.io/badge/license-Apache--2.0-blue)

A local web UI for browsing, organizing, and resuming Claude Code sessions, grouped by project.

Built with Go, SQLite, and vanilla front-end code. Ships as a single process and a single binary, listening on `127.0.0.1` only.

## Screenshots

![Interface](docs/screenshot.png)

![Demo](docs/demo.gif)

> **Note**: This is a practice project for learning Go — feel free to learn from it.

## Why

Claude Code stores every conversation as a jsonl file under `~/.claude/projects`, named by UUID. Resuming one means remembering `claude --resume <id>`. As the number of sessions grows, it becomes hard to find, recall, and manage them.

| Problem | What this project does |
|---|---|
| Sessions are scattered across directories named after encoded paths, with no indication of which project they belong to | Scans and indexes every session, groups them into projects using the `cwd` from each jsonl, and shows title, last prompt, timestamp, message count, token usage, and git branch in the list |
| Finding an old session means scrolling terminal history or guessing a UUID | Full-text search over session content (FTS5 trigram, Chinese substring matching), search by title / last prompt / session ID, and a `Ctrl+K` search box |
| Claude Code has no concept of folders, so sessions cannot be organized | Logical grouping: nested folders, moving sessions, renaming groups. Writes to SQLite only — the directory layout under `~/.claude/projects` is never touched |
| Sessions created with `branch` / `fork` look unrelated to their originals | Parses `forkedFrom` from the first line of each jsonl, shows "forked from *parent title*" and a fork count, renders a family tree in the lineage view, and shows the full ancestry chain with the fork point on the detail page |
| Long sessions keep growing and cost more tokens | One-click `claude --resume <id> -p /compact`, using Claude Code's official compaction flow |
| Starting a new line of thought from a historical point without damaging the original | `branch` copies the history into a new session and switches to it; `fork` creates an independent copy and stays on the current session. Both record `forkedFrom`; the original is untouched |
| Frequently used projects get lost when switching between many of them | Pinned favorites, project aliases, a "recent" group, and `Ctrl+P` project search |
| Accidentally deleted sessions cannot be recovered | Deletes go to a trash folder (jsonl plus its attachment directory), with preview, restore, and permanent delete |
| Claude Code's 30-day cleanup deletes old transcripts, leaving dangling index entries | Missing files are marked `missing`, while local grouping and ownership records are kept |
| Renaming or deleting a running session causes state conflicts | Checks `~/.claude/sessions/*.json` before writing and returns 409 if the session is running |
| The page always needs a manual refresh | Claude Code hooks trigger incremental scans, SSE pushes updates to the page, and a 30-second scan acts as a fallback |

## Features

### Browsing and search

- Scans `~/.claude/projects`, reads jsonl metadata, and groups sessions into projects by `cwd`
- Session list: title, last prompt, timestamp, project path, message count, token usage, git branch
- Session detail: message timeline, collapsible tool calls, truncation notice, one-click copy of the full session ID, breadcrumb-style group labels, and export to raw JSON (`.jsonl`) or Markdown (`.md`)
- Lineage on the detail page: a "root › parent › current" chain with per-level navigation; forked sessions show the exact fork point ("forked at message N")
- Global search by title / last prompt / session ID
- FTS5 full-text search (trigram tokenizer, supports Chinese substrings)
- Shortcuts: `⌘K` / `Ctrl+K` focuses search, `⌘P` / `Ctrl+P` searches projects
- Fork relationships: the list's meta row shows "forked from *parent title*" and an "N forks" count; parents that are in the trash or already cleaned up are marked as such, and the label links to the parent
- Lineage view: switch between "Time" and "Lineage" in the list header. Parents appear above their forks, indented and collapsible; families follow the current sort order while children stay in fork order, so the tree does not jump when sorting changes
- Sorting: "recent activity" groups by date; message-count and title sorting render a flat list without date headers

### Session management

- Row actions on hover: resume, rename, compact, branch, fork, assign to project, add to group, delete; multi-select for bulk grouping
- Multi-select delete, moving to trash or permanently deleting in bulk
- Rename: also appends a `custom-title` line so Claude Code can resume with the new title
- New session: create via CLI (invokes Claude Code) or write a draft file only
- Delete to trash, restore, and permanently delete (trashed sessions remain previewable)
- Empty trash: purge the current project's trash in one click, with an empty state when there is nothing to purge
- Context compaction: `claude --resume <id> -p /compact` runs Claude Code's compaction flow
- Branch / fork: both generate a new session ID and write `forkedFrom`. `branch` copies the history and switches to the new session; `fork` creates an independent copy and keeps you on the current one. After clicking, enter a name; the new title becomes `original-branch-name` / `original-fork-name` (leave blank for `001`, `002`, … such as `original-branch-001`). A checkbox in the dialog opens the session in a terminal after creation (selected by default)
- Resume sessions in Windows Terminal + PowerShell

### Projects and groups

- Add project: register a working directory that has no sessions yet, either by typing a path or browsing local disks
- Pinned projects: starred projects are pinned to the top; state is stored in SQLite
- Project alias: set an alias via the pencil button next to the star; the alias is used consistently in the project list, session assignment dropdown, and new-session dialog. Clear it to restore the original name
- Project search: filter by name or path inside the project panel, with `Ctrl+P` to focus
- Recent: the five most recently opened projects appear in a "Recent" group
- Delete project: removes the project and its session index, moving session files to the OS trash; the project directory on disk is never modified
- Logical groups: an "Organized" section with nested folders (five or more levels deep), expand/collapse, create/rename/delete, and move sessions. SQLite only — session files are never moved

### UI and interaction

- Theme: the sun/moon icon in the top bar toggles light and dark themes; the choice is stored in localStorage and defaults to light
- Collapsible panels: the project and group panels can be collapsed independently, and the ☰ button collapses the entire sidebar; state is stored in localStorage
- Resizable layout: the three column widths and the project panel height are draggable and persisted in localStorage

### Data refresh and security

- Claude Code hooks trigger asynchronous scans; results are pushed to the page over SSE
- Automatic rescan every 30 seconds, plus a manual rescan button
- Listens on `127.0.0.1` only and validates the Origin header; session IDs must be UUIDs and project paths must belong to an indexed project

## Requirements

- Windows (the terminal integration targets Windows Terminal and PowerShell)
- Go 1.27+ to build from source
- Claude Code installed, with sessions under `~/.claude/projects`

## Quick start

### Prebuilt release

- Double-click `session-deck.exe` or `start.bat`. The app runs in the background without a console window
- It opens <http://127.0.0.1:7788> automatically
- Logs are written to `data/session-deck.log`
- To stop, double-click `stop.bat` or end `session-deck.exe` in Task Manager

### Run from source

```bash
go run ./cmd/server
```

### Configuration

| Flag | Default | Description |
|---|---|---|
| `-addr` | `127.0.0.1:7788` | Listen address |
| `-db` | `data/sessions.db` | SQLite database path |
| `-projects` | `~/.claude/projects` | Claude Code session directory |

### Build

```bash
# Release build: GUI subsystem, no console window
go build -trimpath -ldflags "-H windowsgui -s -w" -o bin/session-deck.exe ./cmd/server

# Debug build: keeps console output
go build -o bin/session-deck-debug.exe ./cmd/server
```

> `-H windowsgui` makes the binary run under the GUI subsystem so double-clicking does not open a console window. All logs go to `data/session-deck.log`.

## API

```
GET    /api/health
GET    /api/projects
POST   /api/projects                    { path }
PATCH  /api/projects/{id}               { pinned?, alias? }
POST   /api/projects/{id}/move          { direction: up|down }  reorder pinned projects
POST   /api/projects/{id}/touch
DELETE /api/projects/{id}
GET    /api/projects/{id}/folders
POST   /api/projects/{id}/folders      { name, parent_id? }
PATCH  /api/projects/{id}/folders/{folder}   { name }
POST   /api/projects/{id}/folders/{folder}/move  { direction: up|down }  reorder groups
DELETE /api/projects/{id}/folders/{folder}
GET    /api/sessions?project_id=&folder_id=&q=&deleted=&limit=
POST   /api/sessions                   { project_id, folder_id, title, first_message, mode }
POST   /api/sessions/batch-delete      { ids, permanent }
GET    /api/sessions/{id}
GET    /api/sessions/{id}/ancestors    ancestry from root to parent
GET    /api/sessions/{id}/export       ?format=json|md
PATCH  /api/sessions/{id}              { folder_id?, title? }
DELETE /api/sessions/{id}              ?permanent=1  permanently delete
POST   /api/sessions/{id}/open
POST   /api/sessions/{id}/branch
POST   /api/sessions/{id}/fork
POST   /api/sessions/{id}/compact
POST   /api/projects/{id}/trash/purge
POST   /api/sessions/{id}/restore
GET    /api/fs/dirs?path=
POST   /api/sync
POST   /api/hooks/refresh
GET    /api/hooks/config
GET    /api/events                     SSE
```

`mode` is either `cli` (runs `claude --session-id`) or `draft` (writes the jsonl only). `deleted=1` queries the trash.

`folder_id` can be `unfiled` (not yet grouped) or a numeric group ID.

## Project structure

```
cmd/server/          entry point
internal/api/        HTTP routes, SSE, hooks
internal/creator/    create sessions via CLI or draft
internal/scanner/    jsonl scanning, metadata parsing, message reads, title writes
internal/store/      SQLite index, groups, full-text search
internal/trash/      trash file moves and restore
internal/opener/     terminal and PowerShell launch
internal/model/      data models
web/                 front end (embedded into the binary)
data/                SQLite database (created at runtime)
```

## Design principles

- **jsonl is the source of truth**: SQLite holds only indexes, groups, and local metadata. Deleting the database does not affect Claude Code.
- **Groups are logical**: folders exist only in SQLite and never change the layout under `~/.claude/projects`.
- **Lineage is read-only**: `parent_id` / `fork_point_uuid` are parsed from `forkedFrom` in the first jsonl line; session files are never modified. Relationships survive parent deletion and are shown as missing.
- **30-day cleanup**: Claude Code deletes expired transcripts by default. Sessions whose files disappear are marked `missing`; local grouping records are not deleted automatically.
- **Write protection**: `~/.claude/sessions/*.json` is checked before rename and delete; a running session returns 409.
- **Trash**: deletions move the jsonl and its attachment directory to `~/.claude-session/trash/<session-id>/`, where they can be restored or permanently deleted.
- **Full-text index**: `sessions_fts` is an SQLite FTS5 virtual table that indexes up to the first 200 KB of each session. Queries shorter than three characters fall back to `LIKE`.

## Opening a terminal

`POST /api/sessions/{id}/open` runs:

```
wt.exe -w 0 nt -d <project path> pwsh.exe -NoExit -EncodedCommand <PowerShell Base64>
```

`-w 0` (`last`) reuses the most recent Windows Terminal window: it opens a new tab there and brings the window to the foreground. If no Terminal window exists, `wt` creates one.

Fallback order:

1. `pwsh.exe` + Windows Terminal
2. `powershell.exe` + Windows Terminal
3. `cmd /c start` in a new window
4. Return an error; the front end keeps the resume command available for copying

Security constraints:

- Session IDs must be UUIDs
- Project paths must belong to an indexed project and must not contain quotes, newlines, or NUL
- Commands are passed as argument arrays, never concatenated into a shell string
- Listens on localhost only and validates the Origin header

## Live refresh with Claude Code hooks

The server exposes `POST /api/hooks/refresh`. Fetch a ready-to-paste snippet from `GET /api/hooks/config`:

```json
{
  "hooks": {
    "Stop": [
      { "hooks": [ { "type": "command", "command": "curl -s -X POST http://127.0.0.1:7788/api/hooks/refresh >/dev/null" } ] }
    ],
    "SessionStart": [
      { "hooks": [ { "type": "command", "command": "curl -s -X POST http://127.0.0.1:7788/api/hooks/refresh >/dev/null" } ] }
    ],
    "SessionEnd": [
      { "hooks": [ { "type": "command", "command": "curl -s -X POST http://127.0.0.1:7788/api/hooks/refresh >/dev/null" } ] }
    ]
  }
}
```

Merge these events into `~/.claude/settings.json`. Each completed reply, session start, or session end then triggers an incremental scan, and the page refreshes over SSE. Without hooks, the 30-second scan still runs as a fallback.

## Roadmap

- Editing session content
- Tags and bulk operations
- Multi-instance / remote access

## License

[Apache-2.0](LICENSE)
