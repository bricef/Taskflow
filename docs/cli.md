# TaskFlow CLI

The `taskflow` CLI is a thin HTTP client that calls the TaskFlow server. Commands are derived automatically from the domain model — every Resource and Operation in the model becomes a CLI command.

## Configuration

Configuration is resolved in precedence order (highest first):

1. **Command-line flags**: `--url`, `--api-key`
2. **Environment variables**: `TASKFLOW_URL`, `TASKFLOW_API_KEY`
3. **Config file**: `~/.config/taskflow/config.yaml` (or `./config.yaml`)
4. **Defaults**: `http://localhost:8374`

Example config file (`~/.config/taskflow/config.yaml`):

```yaml
url: http://localhost:8374
api_key: your-api-key-here
```

## Global Flags

| Flag | Description |
|------|-------------|
| `--url` | TaskFlow server URL |
| `--api-key` | API key for authentication |
| `--json` | Output raw JSON instead of formatted tables |
| `--help` | Show help for any command |

## Commands

### actor

```
taskflow actor create      --name <name> --display_name <name> --type <human|ai_agent> --role <admin|member|read_only>
taskflow actor list
taskflow actor get         <name>
taskflow actor update      <name> [--display_name <name>] [--role <role>] [--active <bool>]
taskflow actor rotate_key  <name>                         # generate new API key (shown once)
```

Creating an actor returns the API key in the response (shown once — save it). Use `rotate_key` to generate a new key if compromised. Use `update --active false` to deactivate an actor (revokes access, preserves audit history).

### admin

```
taskflow admin stats                                    # system-wide statistics (admin only)
```

### board

```
taskflow board create   --slug <slug> --name <name> [--workflow <json>]
taskflow board list     [--include_deleted]
taskflow board get      <slug>
taskflow board update   <slug> [--name <name>] [--description <desc>]
taskflow board delete   <slug>                          # soft-delete (admin only)
taskflow board reassign <slug> --target_board <slug> [--states <state1,state2>]
taskflow board detail   <slug>                          # full board dump
taskflow board overview <slug>                          # task counts by state
taskflow board audit    <slug>                          # board audit log
```

### task

```
taskflow task create     <slug> --title <title> [--description <desc>] [--priority <priority>] [--tags <t1,t2>] [--assignee <name>]
taskflow task list       <slug> [--state <state>] [--assignee <name>] [--priority <p>] [--tag <tag>] [--q <search>]
                                [--sort <field>] [--order <asc|desc>] [--include_closed] [--include_deleted]
taskflow task get        <slug> <num>
taskflow task detail     <slug> <num>                   # task with comments, deps, attachments, audit
taskflow task update     <slug> <num> [--title <title>] [--description <desc>] [--priority <p>] [--tags <t1,t2>] [--assignee <name>]
taskflow task transition <slug> <num> --transition <name> [--comment <text>]
taskflow task delete     <slug> <num>
taskflow task search     [--q <query>] [--state <state>] [--assignee <name>] [--priority <p>] [--include_closed]
taskflow task audit      <slug> <num>                   # task audit log
```

### workflow

```
taskflow workflow get    <slug>
taskflow workflow set    <slug>                          # reads workflow JSON from --workflow flag
taskflow workflow health <slug>
```

### comment

```
taskflow comment create  <slug> <num> --body <text>
taskflow comment list    <slug> <num>
taskflow comment update  <id> --body <text>
```

### dependency

```
taskflow dependency create <slug> <num> --depends_on_board <slug> --depends_on_num <num> --dep_type <depends_on|relates_to>
taskflow dependency list   <slug> <num>
taskflow dependency delete <id>
```

### attachment

```
taskflow attachment create <slug> <num> --ref_type <url|file|git_commit|git_branch|git_pr> --reference <value> --label <label>
taskflow attachment list   <slug> <num>
taskflow attachment delete <id>
```

### webhook

```
taskflow webhook create  --url <url> --events <e1,e2> --secret <secret> [--board_slug <slug>]
taskflow webhook list
taskflow webhook get     <id>
taskflow webhook update  <id> [--url <url>] [--events <e1,e2>] [--active <bool>]
taskflow webhook delete  <id>
```

### delivery

```
taskflow delivery list   <id>                           # webhook delivery attempts
```

### tag

```
taskflow tag list        <slug>                         # tags in use on a board
```

## Special Values

- **`@me`** — use as `--assignee @me` to assign tasks to yourself or filter by your own tasks
- **Priority values** — `critical`, `high`, `medium`, `low`, or omit for none

## Output Formats

By default, list commands produce a table and single-item commands produce key-value pairs:

```
$ taskflow task list my-board
BOARD_SLUG  CREATED_BY  NUM  PRIORITY  STATE    TITLE
my-board    admin       1    high      backlog  Fix auth bug
my-board    admin       2    none      backlog  Update docs

$ taskflow task get my-board 1
board_slug:  my-board
created_by:  admin
num:         1
priority:    high
state:       backlog
title:       Fix auth bug
```

Use `--json` for raw JSON output (useful for scripting and piping):

```
$ taskflow task get my-board 1 --json
{"board_slug":"my-board","num":1,"title":"Fix auth bug","state":"backlog","priority":"high",...}
```
