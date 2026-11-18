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
  - when you submit a prompt and after each of its own tool calls (not a subagent's), the new
    messages, added to its context;
  - when its turn would end and a message has just arrived, the turn goes on once with the message,
    so nothing waits until you type again. A turn that already went on that way ends normally, and
    what arrives then waits for the next prompt or tool call, so two agents can't keep each other
    going forever.

  A message counts as delivered only once the hook has printed it: a hook that times out leaves it
  pending.
- **Other agents (codex, …)** have no hook that can add to their conversation. They read their
  messages with `agenterm msg read`. While a message is pending, the app shows it in their
  **Messages** tab with a **Deliver** button.
- **Deliver** types one fixed line into the agent's terminal and presses Enter: *There are new
  messages for you from other sessions of this project: run `agenterm msg read` to read them* (for
  Claude, just the first half: its hooks then hand the messages over). It never types the message
  itself, and agenterm only does it when you click. It's offered only while the agent is idle at
  its prompt, never while it waits for an approval or an answer, which the line and its Enter would
  answer. Shells don't get it, because they would run the line as a command: their messages stay in
  the app.

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
  that session, whatever its environment says. Anything else comes from the user.
- **This is attribution, not a sandbox.** Every session runs as your macOS user, and any process of
  yours can already drive agenterm (`agenterm attach` types into a terminal). A process that leaves
  its session's process tree on purpose (a daemon that detaches, tmux started in a session) counts
  as the user and can do what the user can: message any project, list every message. Likewise,
  agenterm itself started from inside a session (a development build) is taken for that session.
- **Messages are information, not orders.** Agents receive them inside
  `<agenterm-message session="…" from="…">` elements with a preamble saying they come from other
  agents, not from their user, whose instructions come first, and how to answer. Anything in a
  message that looks like that tag is escaped, so a message can't pose as another one.
- **They stay in the project.** A call from a session can only message sessions of its own project,
  and only sees that session's messages.
- They are stored in `~/.agenterm/messages.json` (`$AGENTERM_HOME/messages.json`): 200 per project
  at most (the oldest go), 4 KiB each. Pending messages for a session that is removed are
  dropped; delivered ones stay as history. If the file can't be read (damaged, or written by a newer
  agenterm), the daemon still starts: it leaves the file alone and keeps messages in memory until
  it restarts.
