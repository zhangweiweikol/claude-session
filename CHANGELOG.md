# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-11-06

### Added

- Scan `~/.claude/projects`, index JSONL sessions, and group them into projects by `cwd`
- Session list with title, last prompt, timestamp, project path, message count, token usage, and git branch
- Session detail with message timeline, collapsible tool calls, and export to JSONL or Markdown
- Full-text search over session content using SQLite FTS5 with trigram tokenizer
- Global search by title, last prompt, or session ID
- Session lineage: parse `forkedFrom`, show parent/child relationships, and render a family tree
- Logical groups with nested folders, rename, move, and reorder
- Project pinning, aliases, recent projects, and project search
- Rename sessions with a `custom-title` line for Claude Code compatibility
- Create sessions via CLI or as draft JSONL files
- Branch and fork sessions with generated names and `forkedFrom` tracking
- Context compaction via `claude --resume <id> -p /compact`
- Trash with preview, restore, and permanent delete
- Resume sessions in Windows Terminal + PowerShell, with fallback order
- Real-time refresh through Claude Code hooks, SSE, and a 30-second fallback scan
- Light and dark themes, collapsible panels, and resizable layout
- Chinese and English READMEs, screenshots, and demo GIF
- Release packaging script and GitHub Actions workflow for tagged releases

[Unreleased]: https://github.com/zhangweiweikol/claude-session/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/zhangweiweikol/claude-session/releases/tag/v0.1.0
