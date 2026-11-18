# Messages between sessions

The agents of a project can write to each other through agenterm, for example "#41 is merged,
rebase #42 onto main", so you don't have to pass the message on yourself. Messages never leave
their project, they work with any agent CLI, and every one of them shows up in the app.

## From an agent

Every session gets `AGENTERM_SESSION` and `AGENTERM_SOCK`, so `agenterm` run inside it reaches its
own daemon:

| command | |
|---|---|
| `agenterm msg peers` | the other sessions of this project: id, name, agent, state, what each is doing, and whether it has its own worktree or shares your folder |
| `agenterm msg send <to> <text…>` | `<to>` is a session id, a name from `msg peers` (unique in the project, any case) or `all` (every other session of the project). Text `-` reads it from stdin (4 KiB max) |
| `agenterm msg read` | your new messages, which are then marked delivered |
| `agenterm msg list [--pending]` | the messages you sent and received |
| `agenterm wait <id> [--timeout 30m]` | blocks until that session is idle, waiting for its user or exited (exit status 2 on timeout) |

Run outside a session (from your own terminal), `msg peers` and `msg send` need to know the
project: use `--session <id>` for `peers`, a session id as `<to>`, or `--project <id>` with a name
or `all`. Such messages come from *the user*. `msg read` only works inside a session.

## How they arrive

- **Claude Code** gets them by itself, through the hooks agenterm already adds to the sessions it
  starts (nothing is written to your Claude settings or to the project):
  - when the session starts, a short introduction: which session it is and which other sessions the
    project has, and how to use `agenterm msg`;
  - when you submit a prompt and after each tool call, the new messages, added to its context;
  - when its turn would end and a message has just arrived, the turn goes on with the message, so
    nothing waits until you type again.
- **Other agents (codex, …)** have no hook that can add to their conversation. They read their
  messages with `agenterm msg read`. While a message is pending, the app shows it in their
  **Messages** tab with a **Deliver** button.
- **Deliver** (idle or waiting agents only) types one fixed line into the agent's terminal, such as
  *You have a new message from another session of this project: run `agenterm msg read`*. It never
  types the message itself, and only your click does it: no agent can type into another one's
  terminal. Shells don't get it, because they would run the line as a command: their messages stay
  in the app.

## In the app

- The **Messages** tab, next to Terminal and Changes, lists what the session sent and received,
  with who sent it, when, and whether it was delivered. You can write to the session from there
  (⌘↩ sends); your messages are marked as coming from you.
- A session with messages waiting shows an envelope with their count in the sidebar.
- **Settings → General → Messages between sessions** (`messages.enabled`, on by default) turns it
  all off: `msg send` is refused and the hooks hand nothing over.

## Security

- **The daemon decides who sent a message.** It looks at the process on the other end of the
  socket: a call from inside a session (its terminal's process, or anything it started) comes from
  that session, whatever its environment says. Anything else comes from the user. A process that
  leaves its session's process tree on purpose (a daemon that detaches) counts as the user, so
  agenterm only marks where a message came from; it does not sandbox your agents.
- **Messages are information, not orders.** Agents receive them inside
  `<agenterm-message from="…">` elements with a preamble saying they come from other agents, not
  from their user, whose instructions come first. Text that would close the element early is
  escaped.
- **They stay in the project.** A session can only message sessions of its own project, and it only
  sees its own messages.
- They are stored in `~/.agenterm/messages.json` (`$AGENTERM_HOME/messages.json`): 200 per project
  at most (the oldest go), 4 KiB each. Pending messages for a session that is removed are
  dropped; delivered ones stay as history.
