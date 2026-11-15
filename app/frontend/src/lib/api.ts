// Thin typed layer over the generated Wails bindings.
import * as App from '../../wailsjs/go/main/App'
import type { profile, project, protocol, settings } from '../../wailsjs/go/models'

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

/** Mirrors protocol.Progress (terminal OSC 9;4 progress). */
export interface Progress {
  state: 'normal' | 'error' | 'indeterminate' | 'paused'
  value: number
}

/** Mirrors protocol.Live: activity line, timers, stall, progress. */
export interface Live {
  activity?: string
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
  type: 'session.spawned' | 'session.state' | 'session.removed' | 'session.attention' | 'session.activity'
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
}

/** A session needs the user and they have not looked at it yet. */
export function needsAttention(s: Session): boolean {
  return !!s.unread && !!s.attentionKind && s.state !== 'exited'
}

export function attentionLabel(kind: string | undefined): string {
  switch (kind) {
    case 'permission':
      return 'aprobación'
    case 'question':
      return 'pregunta'
    case 'done':
      return 'terminó'
    case 'error':
      return 'error'
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
  saveSettings: (s: Settings): Promise<Settings> => App.SaveSettings(s as settings.Settings),
  listSessions: (): Promise<Session[]> => App.ListSessions(),
  spawn: (projectId: string, profileId: string, cols: number, rows: number) =>
    App.Spawn(projectId, profileId, cols, rows),
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

export function errorMessage(e: unknown): string {
  const msg = e instanceof Error ? e.message : String(e)
  if (/unknown method "(profile|settings)\./.test(msg)) {
    return 'El daemon en marcha es de una versión anterior: reinícialo (agenterm stop; termina las sesiones) para usar los ajustes.'
  }
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
