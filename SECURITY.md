# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| 0.1.x   | ✅        |

## Reporting a Vulnerability

**Please do not open a public issue for security problems.**

Use GitHub's private vulnerability reporting:

1. Open the repository's **Security** tab
2. Click **Report a vulnerability**
3. Describe the issue, including reproduction steps and affected version

If private reporting is unavailable, contact the maintainer through the GitHub profile
[@zhangweiweikol](https://github.com/zhangweiweikol) and ask for a private channel first.

## What to include

- Affected version or commit
- Steps to reproduce
- Impact (data exposure, command execution, denial of service, etc.)
- Any suggested fix

## Response

This is a personal project maintained on a best-effort basis. You can expect:

- An initial acknowledgement within 7 days
- A fix or mitigation plan for confirmed issues as soon as practical
- Credit in the release notes if you want it

## Scope

The application is designed to listen on `127.0.0.1` only and validates the `Origin`
header. Security-relevant areas include:

- Session ID and project path validation before shell invocation
- JSONL parsing of untrusted session files
- HTTP handlers and SSE endpoints
- Export and trash file operations

## Out of scope

- Vulnerabilities requiring the attacker to already have local code execution
- Issues caused by running the server on a public interface against the documented design
- Vulnerabilities in Claude Code itself (report those to Anthropic)
