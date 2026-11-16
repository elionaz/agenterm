// English UI strings: the source of truth. Every key must also exist in
// es.ts (the Dict type enforces it; scripts/check-i18n.ts checks the
// placeholders too). {name} is interpolated; { one, other } entries are
// plurals chosen by the `n` parameter. Keys marked (html) are rendered with
// {@html}: markup lives here only and parameters are escaped.

/** A plural message, chosen by the `n` parameter. */
export interface Plural {
  one: string
  other: string
}

export type Msg = string | Plural

export const en = {
  // Session states and attention kinds.
  'state.running': 'working',
  'state.waiting_input': 'waiting',
  'state.idle': 'idle',
  'state.exited': 'exited',
  'state.exitCode': 'exit {code}',
  'attention.permission': 'approval',
  'attention.question': 'question',
  'attention.done': 'done',
  'attention.error': 'error',

  // Timers (activity.ts). {d} is a duration such as "4m 12s".
  'timer.stalled': 'stalled? no activity',
  'timer.stalledFor': 'stalled? no activity for {d}',
  'timer.stalledShort': 'stalled?',
  'timer.working': 'working {d}',
  'timer.lastOutput': ' · last output {d} ago',
  'timer.finishedIn': 'finished in {d}',
  'timer.ago': ' · {d} ago',
  'timer.waiting': 'waiting {d}',

  // Progress (OSC 9;4).
  'progress.title': 'Progress: {label}',
  'progress.indeterminate': 'in progress',
  'progress.error': 'error · {value}%',
  'progress.paused': 'paused · {value}%',

  // Hook activity (protocol activityKind); {target} is a file, command…
  'activity.thinking': 'Thinking…',
  'activity.edit': 'Editing {target}',
  'activity.edit.bare': 'Editing',
  'activity.read': 'Reading {target}',
  'activity.read.bare': 'Reading',
  'activity.bash': 'Bash: {target}',
  'activity.bash.bare': 'Bash',
  'activity.search': 'Searching {target}',
  'activity.search.bare': 'Searching',
  'activity.web': 'Web: {target}',
  'activity.web.bare': 'Web',
  'activity.subagent': 'Subagent: {target}',
  'activity.subagent.bare': 'Subagent',
  'activity.todo': 'Updating tasks',
  'activity.mcp': 'MCP: {target}',
  'activity.mcp.bare': 'MCP',

  // Main window (App.svelte).
  'view.terminal': 'Terminal',
  'view.changes': 'Changes',
  'empty.noProjects': 'No projects',
  'empty.addFolder': 'Add a folder to start launching agents in it.',
  'empty.addProject': '+ Add project',
  'empty.noSessions': 'No sessions',
  /** (html) */
  'empty.launchHint': 'Launch an agent with <b>+</b> next to a project.',
  /** (html) */
  'empty.launchHintKey': 'Launch an agent with <b>+</b> next to a project or press <kbd>⌘N</kbd> ({name}).',
  'empty.chooseAgents': 'Choose agents…',
  'banner.reconnecting': 'Reconnecting to the daemon…',

  // Terminal (terminals.ts).
  'term.exited': '[process exited · code {code}]',
  'term.disconnected': '[disconnected: {msg}]',

  // Sidebar.
  'sidebar.overview': 'Overview (⌘0)',
  'sidebar.overviewLabel': 'Overview',
  'sidebar.expand': 'Expand',
  'sidebar.collapse': 'Collapse',
  'sidebar.needAttention': { one: '{n} session needs your attention', other: '{n} sessions need your attention' },
  'sidebar.projectOverview': 'Project overview (⌘0)',
  'sidebar.removeProject': 'Remove project (does not delete files)',
  'sidebar.newSession': 'New session',
  'sidebar.noAgents': 'No agents available',
  'sidebar.chooseAgents': 'Choose agents…',
  'sidebar.back': 'Back',
  'sidebar.sameFolder': 'Same folder',
  'sidebar.newWorktree': 'New worktree',
  'sidebar.checkingRepo': 'Checking the repository…',
  'sidebar.checkingGit': 'checking git…',
  'sidebar.unavailable': 'Unavailable: {reason}',
  'sidebar.ownBranch': 'own branch, separate folder',
  'sidebar.branch': 'Branch',
  'sidebar.base': 'Base',
  'sidebar.current': ' (current)',
  'sidebar.launch': 'Launch ↵',
  'sidebar.filesChanged': { one: '{n} file changed', other: '{n} files changed' },
  'sidebar.removeFromList': 'Remove from list',
  'sidebar.stopSession': 'Stop session',
  'sidebar.noSessions': 'No sessions',
  'sidebar.orphansTitle': 'agenterm worktrees without a session: their folder and branch are still there',
  'sidebar.orphans': 'Orphaned worktrees',
  'sidebar.orphanTitle': '{path}\nbranch {branch} (from {base})',
  'sidebar.viewChanges': 'View changes',
  'sidebar.openShell': 'Open shell here',
  'sidebar.remove': 'Remove',
  'sidebar.addProject': '+ Project',
  'sidebar.settings': 'Settings (⌘,)',
  'sidebar.settingsLabel': 'Settings',
  'sidebar.worktreeOn': 'Worktree on branch {branch}',
  'sidebar.worktreeFrom': ' (from {base})',
  'git.uncommitted': '{n} uncommitted',
  'git.commitsAhead': { one: '{n} commit ahead', other: '{n} commits ahead' },
  'git.unavailable': 'git unavailable',
  'git.git_missing': 'git is not installed',
  'git.not_a_repo': 'not a git repository',
  'git.no_commits': 'the repository has no commits yet',

  // Grid overview.
  'grid.allSessions': 'All sessions',
  'grid.count': { one: '{n} session', other: '{n} sessions' },
  'grid.allProjects': 'all projects',
  'grid.close': 'Close (Esc / ⌘0)',
  'grid.empty': 'No sessions.',
  'grid.emptyProject': 'No sessions in this project.',

  // Changes view.
  'changes.copied': 'Copied: {branch}',
  'changes.merged': { one: '{branch} merged into {base} (1 commit)', other: '{branch} merged into {base} ({n} commits)' },
  'changes.mergedUncommitted': {
    one: ' · note: 1 uncommitted change was not included',
    other: ' · note: {n} uncommitted changes were not included',
  },
  'changes.files': { one: '1 file', other: '{n} files' },
  'changes.ahead': { one: '· 1 commit ahead', other: '· {n} commits ahead' },
  'changes.refresh': 'Refresh',
  'changes.sideBySide': 'Side by side',
  'changes.unified': 'Unified',
  'changes.openEditor': 'Open in editor',
  'changes.openFinder': 'Open in Finder',
  'changes.copyBranch': 'Copy branch',
  'changes.mergeInto': 'Merge into {base}',
  'changes.stopFirst': 'Stop the session before removing its worktree',
  'changes.removeWorktree': 'Remove worktree',
  'changes.close': 'Close (Esc)',
  'changes.closeLabel': 'Close',
  /** (html) */
  'changes.mergeInfo':
    '<code>--no-ff</code> merge of <b>{branch}</b> into <b>{base}</b>, in the project’s main folder. It only runs if that folder is on <b>{base}</b> with no uncommitted changes (no automatic stash); on conflicts it is aborted.',
  'changes.mergeUncommitted': {
    one: 'There is 1 uncommitted change in the worktree: it is not included.',
    other: 'There are {n} uncommitted changes in the worktree: they are not included.',
  },
  'changes.doMerge': { one: 'Merge (1 commit)', other: 'Merge ({n} commits)' },
  'changes.nothingToMerge': 'Nothing to merge',
  'changes.cancel': 'Cancel',
  'changes.removeInfo': 'The worktree folder is deleted. Your project folder is not touched.',
  'changes.removeInfoDir': 'The worktree folder ({dir}) is deleted. Your project folder is not touched.',
  'changes.deleteBranch': 'Also delete branch {branch}',
  'changes.checkingLoss': 'Checking what would be lost…',
  'changes.lossFiles': { one: '1 uncommitted file would be lost:', other: '{n} uncommitted files would be lost:' },
  'changes.andMore': '… and {n} more',
  'changes.lossCommits': { one: 'And 1 commit not merged into {base}:', other: 'And {n} commits not merged into {base}:' },
  'changes.removeAnyway': 'Remove anyway (cannot be undone)',
  'changes.nothingLost': 'Nothing is lost.',
  'changes.nothingLostKeep': 'Nothing is lost: the branch and its commits are kept.',
  'changes.remove': 'Remove',
  'changes.retry': 'Retry',
  'changes.loading': 'Loading changes…',
  'changes.truncated':
    'The diff is very large (over 2 MB): some files are shown without content. Open it in the editor to see all of it.',
  'changes.commits': 'Commits ({n})',
  'changes.fileList': 'Files ({n})',
  'changes.noChanges': 'No changes.',
  'changes.noChangesBase': 'No changes against {base}.',
  'changes.binary': '{path}: binary file (no content view).',
  'changes.noContentLimit': '{path}: no content (diff size limit).',
  'changes.noContentMode': '{path}: no content (mode change or rename only).',

  // Settings.
  'settings.title': 'Settings',
  'settings.close': 'Close (Esc)',
  'settings.tab.general': 'General',
  'settings.tab.agents': 'Agents',
  'settings.tab.notify': 'Notifications',
  'settings.tab.git': 'Git',
  'settings.save': 'Save',
  'settings.saved': 'Saved ✓',
  'settings.retry': 'Retry',
  'settings.cancel': 'Cancel',
  'settings.create': 'Create',

  'settings.language': 'Language',
  'settings.languageHelp': 'Language of the app and its notifications. Applies right away.',
  'settings.languageAuto': 'Automatic (system: {lang})',
  /** (html) */
  'settings.languageHint': 'Saved to <code>settings.yaml</code> (<code>ui.language</code>).',

  'settings.source.variant': 'variant of {id}',
  'settings.source.catalog': 'catalog',
  'settings.source.override': 'modified',
  'settings.source.custom': 'custom',
  'settings.variantName': '{name} (variant)',
  'settings.err.env': 'Environment: "{line}" is not of the form KEY=value',
  'settings.err.idle': 'Idle seconds: enter a positive number',
  'settings.err.exists': 'An agent with id "{id}" already exists',
  'settings.err.worktreeDir': 'Worktree folder: enter an absolute path (or one starting with ~/), or leave it empty',
  'settings.err.stall': 'Seconds without activity: enter a whole number (0 disables detection)',

  'settings.f.id': 'Id',
  'settings.f.idPlaceholder': 'my-agent',
  'settings.f.name': 'Name',
  'settings.f.namePlaceholder': 'My agent',
  'settings.f.variantOf': 'Variant of',
  'settings.f.cmd': 'Command',
  'settings.f.cmdPlaceholder': 'my-cli',
  'settings.f.args': 'Arguments',
  'settings.f.argsPlaceholder': "--model x 'with spaces'",
  'settings.f.env': 'Environment',
  'settings.f.envHelp': '(KEY=value, one per line; $VARS are expanded)',
  'settings.f.status': 'Status',
  'settings.f.statusHeuristic': 'heuristic (output / idle)',
  'settings.f.statusHooks': 'agent hooks',
  'settings.f.hooks': 'Hooks integration',
  'settings.f.hooksNone': 'none',
  'settings.f.idle': 'Idle seconds',
  'settings.f.idlePlaceholder': 'default (3)',
  'settings.f.patterns': 'Attention patterns',
  'settings.f.patternsHelp': '(regex over the output, one per line)',

  /** (html) */
  'settings.agentsHint':
    '{installed} of {total} found. The first in the list is the <kbd>⌘N</kbd> one; agents that are not installed or are disabled are not shown in the <b>+</b> menu.',
  'settings.scanning': 'Scanning…',
  'settings.rescan': 'Rescan',
  'settings.customAgent': '+ Custom agent',
  'settings.newAgent': 'New agent',
  'settings.drag': 'Drag to reorder',
  'settings.up': 'Move up',
  'settings.down': 'Move down',
  'settings.badgeError': 'error',
  'settings.installed': '✓ installed',
  'settings.notFound': 'not found',
  'settings.disable': 'Disable',
  'settings.enable': 'Enable',
  'settings.enabledTitle': 'Enabled: shown in the menu if installed',
  'settings.disabledTitle': 'Disabled: hidden from the menu',
  'settings.confirmReset': 'Reset to the catalog values?',
  'settings.confirmDelete': 'Delete this agent?',
  'settings.yes': 'Yes',
  'settings.no': 'No',
  'settings.edit': 'Edit',
  'settings.duplicate': 'Duplicate as variant',
  'settings.delete': 'Delete',
  'settings.reset': 'Reset',
  'settings.install': 'Install:',
  /** (html) */
  'settings.notInPath': '<code>{cmd}</code> not found in PATH.',
  'settings.web': 'web',
  'settings.newVariant': 'New variant of {name}',
  /** (html) */
  'settings.agentsFoot':
    'Saved to <code>profiles.yaml</code> (with a <code>.bak</code> copy); fields you do not change are inherited from the catalog.',

  'settings.n.enabled': 'Notifications',
  'settings.n.enabledHelp': 'Alert when an agent needs your approval, asks you something or fails.',
  'settings.n.sound': 'Sound',
  'settings.n.soundHelp': 'Only for approvals and questions.',
  'settings.n.done': 'Notify when done',
  'settings.n.doneHelp': 'Also when an agent finishes its turn.',
  'settings.n.stall': 'Notify if it looks stalled',
  'settings.n.stallHelp':
    'A quiet heads-up (no sound, not counted as pending) when a turn has gone a while without output or events.',
  'settings.n.stallSeconds': 'Seconds without activity before “stalled?”',
  'settings.n.stallSecondsHelp':
    'Only for agents with hooks (Claude Code) while a turn is open. 0 disables detection. Applies to new sessions.',
  /** (html) */
  'settings.n.hint': 'Saved to <code>settings.yaml</code>; the app and the daemon re-read it for every notification.',

  'settings.g.mode': 'Default launch mode',
  /** (html) */
  'settings.g.modeHelp':
    'What the <b>+</b> menu suggests when you have not chosen anything in that project yet (the last choice is remembered per project).',
  'settings.g.same': 'Same folder',
  'settings.g.worktree': 'New worktree',
  'settings.g.editor': 'Editor',
  /** (html) */
  'settings.g.editorHelp':
    'For “Open in editor”: a command (<code>code</code>, <code>cursor -n</code>, <code>zed</code>) or the name of a macOS app (<code>Visual Studio Code</code>). Empty: <code>code</code> if installed, otherwise Finder.',
  'settings.g.dir': 'Worktree folder',
  /** (html) */
  'settings.g.dirHelp':
    'Where worktrees are created (never inside the project). Empty: <code>~/.agenterm/worktrees</code>. Applies to new worktrees.',
  /** (html) */
  'settings.g.hint':
    'Saved to <code>settings.yaml</code> (<code>git.editor</code>, <code>git.worktree_dir</code>, <code>git.default_mode</code>).',

  // Daemon errors by code (protocol errorCode); unknown codes show the
  // daemon's English message.
  'err.oldDaemon':
    'The running daemon is an older version: restart it (agenterm stop; this ends the sessions) to use the settings.',
  'err.live_sessions': {
    one: '1 live session: {sessions}; use --force to stop it',
    other: '{n} live sessions: {sessions}; use --force to stop them',
  },
  'err.worktree_dirty': {
    one: 'Worktree not removed: 1 uncommitted file would be lost ({files}).',
    other: 'Worktree not removed: {n} uncommitted files would be lost ({files}).',
  },
  'err.worktree_unmerged': {
    one: 'Worktree not removed: 1 commit on {branch} is not merged into {base}.',
    other: 'Worktree not removed: {n} commits on {branch} are not merged into {base}.',
  },
  'err.worktree_live': 'Session {sessionId} is still running in this worktree: stop it before removing the worktree.',
  'err.worktree_not_found': 'Worktree not found: {id}',
  'err.merge_main_dirty': {
    one: 'Cannot merge: the main folder ({dir}) has 1 uncommitted change; commit or stash it first (agenterm does not stash automatically).',
    other:
      'Cannot merge: the main folder ({dir}) has {n} uncommitted changes; commit or stash them first (agenterm does not stash automatically).',
  },
  'err.merge_wrong_branch': 'Cannot merge: the main folder ({dir}) is on branch {current}, not {base}; switch it to {base} and try again.',
  'err.merge_wrong_branch_detached':
    'Cannot merge: the main folder ({dir}) has a detached HEAD, not {base}; switch it to {base} and try again.',
  'err.merge_nothing': 'Nothing to merge: {branch} has no commits that are not already in {base}.',
  'err.merge_conflict': 'Merging {branch} into {base} conflicts ({files}): the merge was aborted; resolve it by hand.',
  'err.branch_missing': 'Branch {branch} no longer exists.',
  'err.branch_exists': 'Branch {branch} already exists.',
  'err.branch_invalid': 'Invalid branch name: "{branch}".',
  'err.profile_not_installed': '{profileId}: command "{cmd}" not found in PATH.',
  'err.profile_not_installed_hint': '{profileId}: command "{cmd}" not found in PATH (install: {install}).',
  'err.not_a_repo': '{dir} is not a git repository: worktree mode is not available.',
  'err.git_missing': 'git is not installed.',
  'err.no_commits': 'The repository has no commits yet: make a first commit to use worktrees.',
  'err.base_missing': 'Base "{base}" does not exist in the repository.',
  'err.base_missing_none': 'Cannot find the base branch (detached HEAD and no main/master): choose a base.',
  'err.dir_missing': 'The folder no longer exists: {dir}',
} satisfies Record<string, Msg>

export type MsgKey = keyof typeof en

/** What a translation must provide: every key, plural where en is. */
export type Dict = { [K in MsgKey]: (typeof en)[K] extends string ? string : Plural }
