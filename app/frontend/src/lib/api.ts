// Thin typed layer over the generated Wails bindings.
import * as App from '../../wailsjs/go/main/App'
import type { gitx, main, profile, project, protocol, settings } from '../../wailsjs/go/models'
import { hasKey, t, type Params } from './i18n.svelte'

// Plain data (the generated classes carry a convertValues helper we never use).
export type Project = Omit<project.Project, 'convertValues'>
export type Profile = profile.Profile
/** A profiles.yaml entry as sent to profile.save (desired values). */
export type ProfileEntry = profile.Entry
export type CatalogInfo = Omit<protocol.CatalogInfo, 'convertValues'>
export type Settings = Omit<settings.Settings, 'convertValues'>
export type Session = Omit<protocol.SessionInfo, 'convertValues'>
export type SessionState = 'running' | 'waiting_input' | 'idle' | 'exited'
export type AttentionKind = 'permission' | 'question' | 'done' | 'error'
export type SpawnMode = 'same' | 'worktree'
export type SpawnOptions = main.SpawnOptions
export type GitInfo = Omit<protocol.GitInfo, 'convertValues'>
export type DiffFile = gitx.DiffFile
export type Commit = Omit<gitx.Commit, 'convertValues'>
export type DiffResult = Omit<protocol.DiffResult, 'convertValues'>
export type WorktreeInfo = Omit<protocol.WorktreeInfo, 'convertValues'>
export type ProjectGit = protocol.ProjectGit
export type LossInfo = Omit<gitx.LossError, 'convertValues'>
export type MergeResult = gitx.MergeResult

/** Mirrors protocol.Progress (terminal OSC 9;4 progress). */
export interface Progress {
  state: 'normal' | 'error' | 'indeterminate' | 'paused'
  value: number
}

/** Mirrors protocol.Live: activity line, timers, stall, progress. */
export interface Live {
  /** English for hook activity (render activityKind/Target instead);
   * verbatim for output lines. */
  activity?: string
  /** thinking | edit | read | bash | search | web | subagent | todo | mcp | tool */
  activityKind?: string
  activityTarget?: string
  activityAt?: string
  activitySource?: 'hook' | 'output'
  turnStartedAt?: string
  turnEndedAt?: string
  lastOutputAt?: string
  lastEventAt?: string
  stalled?: boolean
  stalledAt?: string
  progress?: Progress
}

/** Mirrors protocol.Event (Go). */
export interface DaemonEvent extends Live {
  type:
    | 'session.spawned'
    | 'session.state'
    | 'session.removed'
    | 'session.attention'
    | 'session.activity'
    | 'session.git'
    | 'worktree.changed'
  sessionId: string
  state?: SessionState
  prevState?: string
  source?: string
  exitCode?: number
  at: string
  attentionKind?: AttentionKind
  attentionMessage?: string
  attentionAt?: string
  unread?: boolean
  /** A new attention was raised (the backend already decided on notifying). */
  raised?: boolean
  /** session.activity: the session just became stalled. */
  stallRaised?: boolean
  /** session.git: the new change summary (absent: no longer a git folder). */
  git?: GitInfo
  /** worktree.changed: whose worktree list changed. */
  projectId?: string
}

/** A session needs the user and they have not looked at it yet. */
export function needsAttention(s: Session): boolean {
  return !!s.unread && !!s.attentionKind && s.state !== 'exited'
}

export function attentionLabel(kind: string | undefined): string {
  switch (kind) {
    case 'permission':
    case 'question':
    case 'done':
    case 'error':
      return t(`attention.${kind}`)
    default:
      return ''
  }
}

export const api = {
  listProjects: (): Promise<Project[]> => App.ListProjects(),
  /** Opens the native folder picker; resolves null if cancelled. */
  addProject: async (): Promise<Project | null> => (await App.AddProject()) ?? null,
  removeProject: (id: string) => App.RemoveProject(id),
  /** Every effective profile (hidden ones too), in menu order. */
  listProfiles: (): Promise<Profile[]> => App.ListProfiles(),
  profileCatalog: (): Promise<CatalogInfo[]> => App.ProfileCatalog(),
  saveProfile: (e: ProfileEntry): Promise<Profile[]> => App.SaveProfile(e),
  deleteProfile: (id: string): Promise<Profile[]> => App.DeleteProfile(id),
  reorderProfiles: (ids: string[]): Promise<Profile[]> => App.ReorderProfiles(ids),
  rescanProfiles: (): Promise<Profile[]> => App.RescanProfiles(),
  getSettings: (): Promise<Settings> => App.GetSettings(),
  /** Fields left out keep their saved values. */
  saveSettings: (s: Partial<Settings>): Promise<Settings> => App.SaveSettings(s as main.SettingsPatch),
  listSessions: (): Promise<Session[]> => App.ListSessions(),
  spawn: (projectId: string, profileId: string, cols: number, rows: number, opts?: Partial<SpawnOptions>) =>
    App.SpawnIn(projectId, profileId, cols, rows, {
      mode: opts?.mode ?? '',
      branch: opts?.branch ?? '',
      base: opts?.base ?? '',
      worktreeId: opts?.worktreeId ?? '',
    }),
  /** Whether a project can use worktrees, its branches and default base. */
  projectGit: (projectId: string): Promise<ProjectGit> => App.ProjectGit(projectId),
  sessionDiff: (id: string): Promise<DiffResult> => App.SessionDiff(id),
  worktreeDiff: (id: string): Promise<DiffResult> => App.WorktreeDiff(id),
  listWorktrees: (projectId: string): Promise<WorktreeInfo[]> => App.ListWorktrees(projectId),
  /** dryRun: only report what would be lost (result.loss). */
  removeWorktree: (id: string, o: { force?: boolean; deleteBranch?: boolean; dryRun?: boolean }) =>
    App.RemoveWorktree(id, !!o.force, !!o.deleteBranch, !!o.dryRun),
  mergeWorktree: (id: string): Promise<MergeResult> => App.MergeWorktree(id),
  /** Opens a worktree (id) or a session's folder in the editor or Finder. */
  openFolder: (target: 'editor' | 'finder', ref: { worktreeId?: string; sessionId?: string }) =>
    App.OpenWorktree(ref.worktreeId ?? '', ref.sessionId ?? '', target),
  kill: (id: string) => App.Kill(id),
  attach: (id: string, cols: number, rows: number) => App.Attach(id, cols, rows),
  detach: (id: string) => App.Detach(id),
  /** Raw bytes to the PTY (keystrokes are UTF-8 encoded by the caller). */
  inputBytes: (id: string, data: Uint8Array) => App.InputBase64(id, bytesToB64(data)),
  resize: (id: string, cols: number, rows: number) => App.Resize(id, cols, rows),
  /** Last lines of output, ANSI-stripped (grid cards). */
  tail: (id: string, lines: number): Promise<string[]> => App.Tail(id, lines),
  /** Marks a session's attention as seen. */
  ack: (id: string) => App.Ack(id),
  /** Which session the window shows ('' for none): no notifications for it. */
  setVisibleSession: (id: string) => App.SetVisibleSession(id),
  /** Session a notification click asked for before the UI was up ('' if none). */
  takePendingFocus: (): Promise<string> => App.TakePendingFocus(),
  /** What ui.language "auto" resolves to on the backend ("en" | "es"). */
  systemLanguage: (): Promise<string> => App.SystemLanguage(),
}

/** "±3" for a session's changed files ('' when unknown). */
export function changesLabel(g: GitInfo | undefined | null): string {
  return g ? `±${g.files}` : ''
}

/** Last spawn mode chosen for a project (localStorage), else the default. */
export function rememberedMode(projectId: string, fallback: SpawnMode): SpawnMode {
  try {
    const v = localStorage.getItem(`agenterm.spawnMode.${projectId}`)
    if (v === 'same' || v === 'worktree') return v
  } catch {
    /* storage unavailable */
  }
  return fallback
}

export function rememberMode(projectId: string, mode: SpawnMode) {
  try {
    localStorage.setItem(`agenterm.spawnMode.${projectId}`, mode)
  } catch {
    /* storage unavailable */
  }
}

/** Profiles that belong in a spawn menu (enabled, installed, valid). */
export function launchable(ps: Profile[]): Profile[] {
  // A daemon older than the catalog sends neither flag: offer everything.
  return ps.filter((p) => p.enabled !== false && p.installed !== false && !p.error)
}

/** Splits a command line into arguments (quotes and backslashes like sh). */
export function splitArgs(s: string): string[] {
  const out: string[] = []
  let cur = ''
  let have = false
  let quote: '"' | "'" | null = null
  for (let i = 0; i < s.length; i++) {
    const c = s[i]
    if (quote) {
      if (c === quote) quote = null
      else if (c === '\\' && quote === '"' && i + 1 < s.length) cur += s[++i]
      else cur += c
    } else if (c === '"' || c === "'") {
      quote = c
      have = true
    } else if (c === '\\' && i + 1 < s.length) {
      cur += s[++i]
      have = true
    } else if (/\s/.test(c)) {
      if (have) out.push(cur)
      cur = ''
      have = false
    } else {
      cur += c
      have = true
    }
  }
  if (have) out.push(cur)
  return out
}

/** Inverse of splitArgs. */
export function joinArgs(args: string[] | undefined): string {
  return (args ?? []).map((a) => (a === '' || /[\s'"\\]/.test(a) ? `'${a.replace(/'/g, `'\\''`)}'` : a)).join(' ')
}

/** A daemon refusal with a stable code (see internal/apierr). The app
 * passes it as JSON in the error message (app/errors.go). */
export interface CodedError {
  message: string
  code: string
  data?: Record<string, unknown>
}

export function parseCodedError(msg: string): CodedError | null {
  if (!msg.startsWith('{')) return null
  try {
    const v = JSON.parse(msg) as Partial<CodedError>
    if (typeof v.code === 'string' && typeof v.message === 'string') return v as CodedError
  } catch {
    /* not JSON */
  }
  return null
}

function preview(list: unknown, n: number): string {
  const xs = Array.isArray(list) ? list.map(String) : []
  return xs.length <= n ? xs.join(', ') : `${xs.slice(0, n).join(', ')}, … +${xs.length - n}`
}

/** Localized text for a coded error, or null for codes we do not know. */
export function codedMessage(ce: CodedError): string | null {
  const d = ce.data ?? {}
  const p: Params = {}
  for (const [k, v] of Object.entries(d)) if (typeof v === 'string' || typeof v === 'number') p[k] = v
  if (typeof d.count === 'number') p.n = d.count
  let key = `err.${ce.code}`
  switch (ce.code) {
    case 'live_sessions': {
      const list = Array.isArray(d.sessions) ? (d.sessions as Array<{ id?: string; profileId?: string }>) : []
      p.sessions = list.map((x) => `${x.id} (${x.profileId})`).join(', ')
      break
    }
    case 'worktree_dirty':
      p.n = Array.isArray(d.uncommitted) ? d.uncommitted.length : 0
      p.files = preview(d.uncommitted, 5)
      break
    case 'worktree_unmerged':
      p.n = Array.isArray(d.unmerged) ? d.unmerged.length : 0
      break
    case 'merge_wrong_branch':
      if (!d.current) key = 'err.merge_wrong_branch_detached'
      break
    case 'merge_conflict':
      p.files = preview(d.files, 5)
      break
    case 'profile_not_installed':
      if (d.install) key = 'err.profile_not_installed_hint'
      break
    case 'base_missing':
      if (!d.base) key = 'err.base_missing_none'
      break
  }
  return hasKey(key) ? t(key, p) : null
}

/** User-facing text for a failed call: coded daemon refusals in the UI
 * language, anything else as reported (English from the daemon). */
export function errorMessage(e: unknown): string {
  const msg = e instanceof Error ? e.message : String(e)
  const ce = parseCodedError(msg)
  if (ce) return codedMessage(ce) ?? ce.message
  if (/unknown method "(profile|settings)\./.test(msg)) return t('err.oldDaemon')
  return msg
}

/** Decodes a base64 PTY chunk into raw bytes (xterm decodes UTF-8 itself,
 * statefully, so multi-byte sequences split across chunks stay intact). */
export function b64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64)
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}

export function bytesToB64(bytes: Uint8Array): string {
  let bin = ''
  for (let i = 0; i < bytes.length; i += 0x8000) {
    bin += String.fromCharCode(...bytes.subarray(i, i + 0x8000))
  }
  return btoa(bin)
}
