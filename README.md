# jiractl

[![CI](https://github.com/thomas-sievering/jiractl/actions/workflows/ci.yml/badge.svg)](https://github.com/thomas-sievering/jiractl/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/go-1.23%2B-00ADD8?logo=go)](https://go.dev/)
[![Release](https://img.shields.io/github/v/release/thomas-sievering/jiractl?display_name=tag)](https://github.com/thomas-sievering/jiractl/releases)
[![Platforms](https://img.shields.io/badge/platforms-windows%20%7C%20linux%20%7C%20macOS-6f42c1)](#install)
[![License](https://img.shields.io/badge/license-MIT-green)](./LICENSE)

Jira Cloud CLI for agents.

Fetch assigned issues, search with JQL, and get compact JSON - designed for AI agents and automation, not TUIs.

## Why jiractl?

The existing [jira-cli](https://github.com/ankitpokhrel/jira-cli) is built for humans (interactive TUI, incomplete JSON output, heavy dependencies). `jiractl` is built for agents:

- **Compact JSON output** - 9 essential fields per issue, not Jira's 200+ field API dump
- **Zero dependencies** - single static binary, stdlib only
- **Simple auth** - email + API token, no OAuth dance
- **Predictable** - every data command supports `--json`, errors go to stderr

## Quick Start

```powershell
# 1) Generate an API token at:
#    https://id.atlassian.com/manage-profile/security/api-tokens

# 2) Login
jiractl auth login --server https://company.atlassian.net --email you@company.com

# 3) View your assigned issues
jiractl issues mine

# 4) View a specific issue
jiractl issues view PROJ-123

# 5) Search with JQL
jiractl issues search --jql "project = PROJ AND status = 'In Progress'"

# 6) Get JSON for agent consumption
jiractl issues mine --json
```

## Install

### Option A: Download binary

Grab the latest release for your platform from [Releases](https://github.com/thomas-sievering/jiractl/releases) and add it to your PATH.

### Option B: Build from source

```powershell
go build -o jiractl.exe .
```

Set a release version at build time:

```powershell
go build -ldflags "-X main.version=v0.1.0" -o jiractl.exe .
```

End users do **not** need Go installed if you distribute the binary.

## Commands

### Auth

```
jiractl auth login   --server URL --email EMAIL [--token TOKEN]
jiractl auth status  [--json]
jiractl auth logout
```

`auth login` verifies credentials against the Jira API before saving. If `--token` is omitted, it prompts interactively (or reads `JIRACTL_API_TOKEN`).

### Issues

```
jiractl issues mine    [--limit N] [--status STATUS] [--json]
jiractl issues view    ISSUE-KEY [--comment-limit N] [--json]
jiractl issues search  --jql "..." [--limit N] [--json]
```

| Command | Description | Default limit |
|---------|-------------|---------------|
| `mine` | Issues assigned to you, ordered by last updated | 50 |
| `view` | Single issue detail by key (e.g. `PROJ-123`) | comments: 20 |
| `search` | Custom JQL query | 50 |

### Other

```
jiractl version
jiractl help
```

## JSON Output

Add `--json` to any data command. Output is **compact by default** (single line, minimal tokens).

```powershell
jiractl issues mine --json
```

```json
{"server":"https://company.atlassian.net","count":2,"total":24,"has_more":true,"issues":[{"key":"PROJ-123","summary":"Fix login bug","status":"In Progress","type":"Bug","priority":"High","assignee":"you@company.com","created":"2026-02-10","updated":"2026-02-15","url":"https://company.atlassian.net/browse/PROJ-123"},{"key":"PROJ-456","summary":"Add dark mode","status":"To Do","type":"Story","priority":"Medium","assignee":"you@company.com","created":"2026-02-12","updated":"2026-02-14","url":"https://company.atlassian.net/browse/PROJ-456"}]}
```

Set `JIRACTL_JSON_PRETTY=1` for indented output:

```powershell
$env:JIRACTL_JSON_PRETTY = "1"   # PowerShell
export JIRACTL_JSON_PRETTY=1      # bash/zsh
```

```json
{
  "server": "https://company.atlassian.net",
  "count": 1,
  "total": 1,
  "has_more": false,
  "issues": [
    {
      "key": "PROJ-123",
      "summary": "Fix login bug",
      "status": "In Progress",
      "type": "Bug",
      "priority": "High",
      "assignee": "you@company.com",
      "created": "2026-02-10",
      "updated": "2026-02-15",
      "url": "https://company.atlassian.net/browse/PROJ-123"
    }
  ]
}
```

Human-readable output (without `--json`):

```
Assigned issues (2):
- PROJ-123      [In Progress]  Fix login bug
- PROJ-456      [To Do]        Add dark mode
```

## Authentication

`jiractl` uses Jira Cloud basic auth (email + API token).

### Generate an API token

1. Go to https://id.atlassian.com/manage-profile/security/api-tokens
2. Click **Create API token**
3. Copy the token

### Login

```powershell
jiractl auth login --server https://company.atlassian.net --email you@company.com
# Prompts for API token interactively
```

Or pass the token directly (useful for scripts):

```powershell
jiractl auth login --server https://company.atlassian.net --email you@company.com --token YOUR_TOKEN
```

### Environment variables (CI / agent use)

For non-interactive environments, set these instead of using `auth login`:

| Variable | Description |
|----------|-------------|
| `JIRACTL_SERVER` | Jira Cloud URL (e.g. `https://company.atlassian.net`) |
| `JIRACTL_EMAIL` | Account email |
| `JIRACTL_API_TOKEN` | API token |

Resolution order: **flags > env vars > config file**.

## Files and Storage

Config is stored in your OS config directory with `0600` permissions:

| OS | Path |
|----|------|
| Windows | `%APPDATA%\jiractl\config.json` |
| Linux | `~/.config/jiractl/config.json` |
| macOS | `~/.config/jiractl/config.json` |

Config format:

```json
{
  "server": "https://company.atlassian.net",
  "email": "you@company.com",
  "api_token": "..."
}
```

## Agent Integration

`jiractl` ships with a [`SKILL.md`](./SKILL.md) for use as a Claude Code / Codex / Cursor skill.

Install it by adding this repo's `SKILL.md` to your project's skills directory. The skill auto-detects when users ask about Jira issues and runs the right commands.

## Troubleshooting

| Error | Fix |
|-------|-----|
| `not authenticated` | Run `jiractl auth login --server URL --email EMAIL` |
| `401 Unauthorized` | API token expired. Generate a new one and re-login |
| `404 Not Found` | Check the issue key or server URL is correct |
| `403 Forbidden` | Your account may lack permission for that project |
| Connection errors | Check the server URL and your network connection |

## Automated Releases

On tag push (`v*`), GitHub Actions builds binaries for 6 targets:

| OS | Architectures |
|----|---------------|
| Windows | amd64, arm64 |
| Linux | amd64, arm64 |
| macOS | amd64, arm64 |

Publish a release:

```powershell
git tag v0.1.0
git push origin v0.1.0
```
