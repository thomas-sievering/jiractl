---
name: jiractl
version: "1.0"
description: Use this skill when user asks about Jira issues, tasks, tickets, sprint work, or "what's assigned to me".
user-invocable: true
argument-hint: "[mine|view|search] [options]"
allowed-tools: Read, Bash
---

# jiractl Skill

Agent workflow for using `jiractl` to fetch Jira issues.

## Arguments

Parse `$ARGUMENTS` into:
- `mode`: `mine`, `view`, `search`, or `auth` (first token if present)
- `target`: issue key (e.g. `PROJ-123`), JQL string, or empty
- `extra`: remaining flags

If mode is missing, infer from user request:
- "my issues", "my tasks", "assigned to me", "what should I work on" -> `mine`
- "show PROJ-123", "details on ticket" -> `view`
- "find issues", "search for", "JQL" -> `search`
- "login", "auth", "connect jira" -> `auth`

## Examples

- User says: "show me my Jira tasks"
  - Run: `jiractl issues mine --json`
- User says: "what's PROJ-123 about?"
  - Run: `jiractl issues view PROJ-123 --json`
- User says: "find open bugs in PROJ"
  - Run: `jiractl issues search --jql "project = PROJ AND type = Bug AND status != Done" --json`

## Steps

### 1) Validate auth first

Run:

```powershell
jiractl auth status --json
```

If not authenticated, tell the user to run:

```powershell
jiractl auth login --server https://company.atlassian.net --email you@company.com
```

### 2) Fetch issues (compact JSON for agent parsing)

My issues:

```powershell
jiractl issues mine --limit 20 --json
```

Single issue:

```powershell
jiractl issues view PROJ-123 --json
```

Custom search:

```powershell
jiractl issues search --jql "project = PROJ AND status = 'In Progress'" --limit 50 --json
```

### 3) Return structured results

Parse JSON output and summarize for the user:
- Issue key and summary
- Status and priority
- Assignee
- Link to issue

Use the `url` field from JSON to provide clickable links.

## Error Handling

- If command returns `not authenticated`, instruct user to run `jiractl auth login`.
- If command returns `401`, the API token may have expired. Instruct user to generate a new token.
- If command returns `404`, the issue key or server URL may be incorrect.
- If search returns no results, suggest broadening the JQL query.
