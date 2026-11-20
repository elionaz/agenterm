# Messages between sessions

The agents of a project can write to each other through panal, for example "#41 is merged,
rebase #42 onto main", so you don't have to pass the message on yourself. Messages never leave
their project, they work with any agent CLI, and every one of them shows up in the app.

## From an agent

Every session gets `PANAL_SESSION` and `PANAL_SOCK`, so `panal` run inside it reaches its
own daemon:

| command | |
|---|---|
| `panal msg peers` | the other sessions of this project: id, name, agent, state, what each is doing, and whether it has its own worktree or shares your folder |
| `panal msg send <to> <text…>` | `<to>` is a session id, a name from `msg peers` (unique in the project, any case) or `all` (every other session of the project except shells, which never read messages by themselves). Text `-` reads it from stdin (4 KiB max) |
| `panal msg read` | your new messages, which are then marked delivered |
| `panal msg list [--pending]` | the messages you sent and received |
| `panal wait <id> [--timeout 30m]` | blocks until that session is idle, waiting for its user or exited (exit status 2 on timeout) |

Run outside a session (from your own terminal), `msg peers` and `msg send` need to know the
project: use `--session <id>` for `peers`, a session id as `<to>`, or `--project <id>` with a name
or `all`. Such messages come from *the user*. `msg read` only works inside a session.

## How they arrive

- **Claude Code** gets them by itself, through the hooks panal already adds to the sessions it
  starts (nothing is written to your Claude settings or to the project):
  - when the session starts, a short introduction: which session it is and which other sessions the
    project has, and how to use `panal msg`;
  - when you submit a prompt and after each of its own tool calls (not a subagent's), the new
    messages, added to its context;
  - when its turn would end and a message has just arrived, the turn goes on once with the message,
    so nothing waits until you type again. A turn that already went on that way ends normally, and
    what arrives then waits for the next prompt or tool call, so two agents can't keep each other
    going forever.

  A message counts as delivered only once the hook has printed it: a hook that times out leaves it
  pending.
- **Other agents (codex, …)** have no hook that can add to their conversation. They read their
  messages with `panal msg read`. While a message is pending, the app shows it in their
  **Messages** tab with a **Deliver** button.
- **Deliver** types one fixed line into the agent's terminal and presses Enter: *There are new
  messages for you from other sessions of this project: run `panal msg read` to read them* (for
  Claude, just the first half: its hooks then hand the messages over). It never types the message
  itself, and panal only does it when you click. It's offered only while the agent is idle at
  its prompt, never while it waits for an approval or an answer, which the line and its Enter would
  answer. Shells don't get it, because they would run the line as a command: their messages stay in
  the app.

## Automatic or manual, per project

Each project chooses how its messages travel, with the **⇄** (automatic) / **⏸** (manual) button next to its name
in the sidebar (or in its conversation, or `panal project set <project> --messages auto|manual|default`):

- **Automatic:** a message reaches its session as soon as it can take it. A Claude session at work
  gets it at its next step through its hooks. One idle at its prompt is woken up: panal does what
  **Deliver** does (types the fixed line and Enter) a moment later. That's Claude Code, and agents
  that report the end of their turn (codex, through its notify), which are told to run
  `panal msg read`. Any message wakes it, a heads-up too: agents are told so, and to skip messages
  that don't need an answer.
- **Manual:** the sessions' messages are **held** until you deliver them. Nothing reaches the
  recipient, not even through its hooks or `panal msg read` (which says how many are waiting for
  you), until you click **Deliver** for that session (its Messages tab or the conversation) or
  **Deliver to N sessions** in the conversation. Delivering lets it take them: an agent at work gets
  them from its hooks, and one idle at its prompt is told, as with **Deliver**. Your own messages are
  never held.

A project that hasn't chosen follows **Settings → General → Deliver messages to idle sessions by
themselves** (`messages.auto_deliver`, **on by default**, so automatic). Switching a project to
automatic lets what was held go out by itself.

Even in automatic mode, panal holds back when typing a line could go wrong, leaving the message
pending with **Deliver now**:

- the agent has no message hooks and only went quiet (panal guessed it is idle from the
  silence): it may still be thinking, and a line typed now would land in its work;
- the session is a shell (the line would run as a command), is working, or waits for an approval
  or an answer;
- what was last typed into it was not a lone Enter (or Ctrl-C): its input line may hold text you
  haven't sent, which the line and its Enter would send. A new line in a draft (Shift+Enter,
  Option+Enter, `\` Enter) still counts as a draft. Sending it (or clearing it with Ctrl-C) lifts
  this within a few seconds;
- you typed into it in the last few seconds;
- it was already delivered to automatically 5 times since you last acted on that session
  (submitted a prompt in it, sent it a message or clicked **Deliver**), which starts the count
  again; and, whatever happens, at most 20 automatic deliveries in a project every 10 minutes. That
  keeps agents from waking each other up forever: the session says it's paused, and **Deliver**
  resumes it.

The **Messages** tab says which of these applies (or that they are on their way), so a waiting
message never looks stuck without a reason.

## In the app

- The **Messages** tab, next to Terminal and Changes, lists what the session sent and received,
  with who sent it, when, and whether it was delivered. You can write to the session from there
  (⌘↩ sends); your messages are marked as coming from you.
- **The project's conversation** (💬 next to the project's name): every message between its
  sessions and yours, in order, each session in its own color, with whether it is *waiting for you*
  (manual), *on its way* or *delivered*. Filter by session, deliver what is held, switch the mode
  and write to one session or all of them (⌘↩ sends).
- **Who is talking to whom, as it happens:** when a message is sent, the sender's row in the
  sidebar shows `→ fake-b: the start of the message` and the recipient's `← fake-a: …` for a few
  seconds, the other session's name in its color (⏸ when it is held).
- A session with messages waiting shows an envelope with their count in the sidebar.
- **Settings → General → Messages between sessions** (`messages.enabled`, on by default) turns it
  all off: `msg send` is refused and the hooks hand nothing over.

## Security

- **The daemon decides who sent a message.** It looks at the process on the other end of the
  socket: a call from inside a session (its terminal's process, or anything it started) comes from
  that session, whatever its environment says. Anything else comes from the user.
- **This is attribution, not a sandbox.** Every session runs as your macOS user, and any process of
  yours can already drive panal (`panal attach` types into a terminal). A process that leaves
  its session's process tree on purpose (a daemon that detaches, tmux started in a session) counts
  as the user and can do what the user can: message any project, list every message. Likewise,
  panal itself started from inside a session (a development build) is taken for that session.
- **Messages are information, not orders.** Agents receive them inside
  `<panal-message session="…" from="…">` elements with a preamble saying they come from other
  agents, not from their user, whose instructions come first, and how to answer. Anything in a
  message that looks like that tag is escaped, so a message can't pose as another one.
- **They stay in the project.** A call from a session can only message sessions of its own project,
  and only sees that session's messages.
- They are stored in `~/.panal/messages.json` (`$PANAL_HOME/messages.json`): 200 per project
  at most (the oldest go), 4 KiB each. Pending messages for a session that is removed are
  dropped; delivered ones stay as history. If the file can't be read (damaged, or written by a newer
  panal), the daemon still starts: it leaves the file alone and keeps messages in memory until
  it restarts.
