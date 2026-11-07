# Contributing

Thanks for taking the time to contribute.

## Ways to contribute

- Report bugs and suggest features via GitHub Issues
- Improve documentation or translations
- Submit pull requests for bug fixes and small features
- Share feedback on usability and performance

For larger changes, please open an issue first so we can agree on the approach before you invest time in a PR.

## Development setup

Requirements:

- Windows 10/11
- Go 1.27+
- Claude Code installed, with sessions under `~/.claude/projects`

Clone and run:

```bash
git clone https://github.com/zhangweiweikol/claude-session.git
cd claude-session
go run ./cmd/server
```

The server listens on <http://127.0.0.1:7788> by default.

## Build

```bash
# Debug build (console output)
go build -o bin/session-deck-debug.exe ./cmd/server

# Release build (GUI subsystem, no console window)
go build -trimpath -ldflags "-H windowsgui -s -w" -o bin/session-deck.exe ./cmd/server
```

Or use the packaging script:

```powershell
./scripts/build-release.ps1 -Version v0.1.0 -AppName session-deck
```

## Checks

Run these before opening a pull request:

```bash
gofmt -l .
go vet ./...
go test ./...
```

`gofmt -l .` should print nothing.

## Pull requests

1. Fork the repository and create a branch from `main`
2. Keep changes focused — one topic per PR
3. Update `README.md` / `README.en.md` if behavior or configuration changes
4. Add an entry to `CHANGELOG.md` under `Unreleased`
5. Make sure `gofmt`, `go vet`, and `go test` pass
6. Describe what changed and why in the PR description

## Commit messages

Follow the existing style:

```
feat: add session lineage view
fix: handle missing jsonl files
docs: update installation steps
refactor: extract jsonl parser
```

Use the imperative mood and keep the subject under ~72 characters.

## Code style

- Follow standard Go formatting (`gofmt`)
- Keep functions small and names descriptive
- Prefer the standard library over new dependencies
- Do not add a dependency without a clear justification in the PR

## License

By contributing, you agree that your contributions will be licensed under the
[Apache License 2.0](LICENSE).
