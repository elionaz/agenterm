# panal tools for agents (MCP)

Agent sessions get **panal's own MCP server**, named `panal`. Its tools let an agent coordinate
with the other sessions of its project and read panal's state, without running `panal` commands
in its shell and without knowing their syntax.

| tool | what it does |
|---|---|
| `peers` | the other sessions of the project: id, name, agent, state, what they're doing, branch or folder |
| `msg_send` | send a message to a session (`to`: its id, its name or `all`); it says when the message waits for you (manual projects) |
| `msg_read` | your new messages, marked delivered (and how many wait for your user) |
| `wait` | block until a session finishes its turn, needs its user or exits (`timeoutSec`, 300 by default, at most 600) |
| `session_status` | a session's state and changes: branch, files, commits ahead, uncommitted, merged; yours by default |
| `history_search` | search the sessions that ended (this project unless `allProjects`) |
| `history_get` | read an ended session: its prompts and the end of its terminal text |

It only reads and messages: it never starts, stops or changes sessions. The messages follow the
same rules as `panal msg` (see [messages.md](messages.md)): automatic or manual per project, and
every one shows up in the app.

## Which agents get it

panal adds the server **for that session only**. Your own agent configuration is left alone:

| agent | how |
|---|---|
| Claude Code | `--mcp-config ~/.panal/hooks/mcp.json`, next to the browser server |
| codex | `-c mcp_servers.panal.command=…` / `-c mcp_servers.panal.args=…` |
| Gemini CLI | `mcpServers.panal` in the system settings panal hands it (servers in your real system settings stay) |
| opencode | `mcp.panal` in its inline config (`OPENCODE_CONFIG_CONTENT`) |
| Cursor CLI | `mcp.json` in the plugin panal loads with `--plugin-dir` (Cursor may ask you to approve the server the first time) |

Other agents, and anything you run yourself, can start it with `panal mcp`: MCP over stdio, bound
to the daemon of the session it runs in. The daemon knows which session calls from the process
tree (the server is a child of the agent), as with `panal msg`.

The agents' first message (the one that introduces the other sessions) points at these tools
when the session has them, and at `panal msg` otherwise.

## Turning it off

**Settings → General → Give agents panal tools (MCP)**, or in `settings.yaml`:

```yaml
mcp:
  enabled: false
```

It applies to the sessions started after the change.
