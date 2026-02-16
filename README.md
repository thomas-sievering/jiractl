# jiractl

[![CI](https://github.com/thomas-sievering/jiractl/actions/workflows/ci.yml/badge.svg)](https://github.com/thomas-sievering/jiractl/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/go-1.23%2B-00ADD8?logo=go)](https://go.dev/)
[![Release](https://img.shields.io/badge/release-not%20published-lightgrey)](https://github.com/thomas-sievering/jiractl/releases)
[![Platforms](https://img.shields.io/badge/platforms-windows%20%7C%20linux%20%7C%20macOS-6f42c1)](#install)
[![License](https://img.shields.io/badge/license-MIT-green)](./LICENSE)

Jira Cloud CLI for agents.

View assigned issues, search with JQL, and get compact JSON output.

## Quick Start

```powershell
# 1) Login with your Jira Cloud credentials
jiractl auth login --server https://company.atlassian.net --email you@company.com

# 2) View your assigned issues
jiractl issues mine --limit 20

# 3) View a specific issue
jiractl issues view PROJ-123

# 4) Search with JQL
jiractl issues search --jql "project = PROJ AND status = 'In Progress'" --limit 50

# 5) Get JSON output for agent consumption
jiractl issues mine --json
```

## Install

### Option A: Download Binary (recommended for users)

Use the GitHub Release asset for your OS and run `jiractl` directly.

### Option B: Build from source (dev)

```powershell
go build -o jiractl.exe .
```

End users do **not** need Go if you ship the binary.

## Commands

### Auth

```powershell
jiractl auth login --server https://company.atlassian.net --email you@company.com
jiractl auth status --json
jiractl auth logout
```

### Issues

```powershell
# My assigned issues
jiractl issues mine --limit 20 --status "In Progress"

# View single issue
jiractl issues view PROJ-123 --json

# Custom JQL search
jiractl issues search --jql "project = PROJ" --limit 50 --json
```

## Auth Configuration

Auth uses Jira Cloud basic auth (email + API token).

Generate an API token at: https://id.atlassian.com/manage-profile/security/api-tokens

Config priority: flags > env vars > config file.

Environment variables for CI/agent use:
- `JIRACTL_SERVER` - Jira Cloud server URL
- `JIRACTL_EMAIL` - account email
- `JIRACTL_API_TOKEN` - API token

## JSON Output

- `--json` outputs compact JSON by default (agent-friendly).
- Set `JIRACTL_JSON_PRETTY=1` to switch to pretty JSON for debugging.

```powershell
# compact JSON
jiractl issues mine --json

# pretty JSON
$env:JIRACTL_JSON_PRETTY = "1"
jiractl issues mine --json
```

## Files and Storage

`jiractl` stores config in your user config dir:

- Windows: `%APPDATA%\jiractl\config.json`
- Linux/macOS: `~/.config/jiractl/config.json`

## Troubleshooting

- `not authenticated`:
  Run `jiractl auth login --server https://company.atlassian.net --email you@company.com`
- `401 Unauthorized`:
  Your API token may have expired. Generate a new one and re-login.
- `404 Not Found`:
  Check that the issue key or server URL is correct.

## Automated Releases

This repo includes `.github/workflows/release.yml`.

On tag push (`v*`), GitHub Actions will:

- Build binaries for Windows/Linux/macOS (amd64 + arm64)
- Package assets (`.zip` for Windows, `.tar.gz` for Linux/macOS)
- Publish them to the GitHub Release for that tag

Publish a release:

```powershell
git tag v0.1.0
git push origin v0.1.0
```
