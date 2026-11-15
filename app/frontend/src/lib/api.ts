// Thin typed layer over the generated Wails bindings.
import * as App from '../../wailsjs/go/main/App'
import type { profile, project, protocol } from '../../wailsjs/go/models'

// Plain data (the generated classes carry a convertValues helper we never use).
export type Project = Omit<project.Project, 'convertValues'>
export type Profile = profile.Profile
export type Session = Omit<protocol.SessionInfo, 'convertValues'>
export type SessionState = 'running' | 'waiting_input' | 'idle' | 'exited'

/** Mirrors protocol.Event (Go). */
export interface DaemonEvent {
  type: 'session.spawned' | 'session.state' | 'session.removed'
  sessionId: string
  state?: SessionState
  prevState?: string
  source?: string
  exitCode?: number
  at: string
}

export const api = {
  listProjects: (): Promise<Project[]> => App.ListProjects(),
  /** Opens the native folder picker; resolves null if cancelled. */
  addProject: async (): Promise<Project | null> => (await App.AddProject()) ?? null,
  removeProject: (id: string) => App.RemoveProject(id),
  listProfiles: (): Promise<Profile[]> => App.ListProfiles(),
  listSessions: (): Promise<Session[]> => App.ListSessions(),
  spawn: (projectId: string, profileId: string, cols: number, rows: number) =>
    App.Spawn(projectId, profileId, cols, rows),
  kill: (id: string) => App.Kill(id),
  attach: (id: string, cols: number, rows: number) => App.Attach(id, cols, rows),
  detach: (id: string) => App.Detach(id),
  /** Raw bytes to the PTY (keystrokes are UTF-8 encoded by the caller). */
  inputBytes: (id: string, data: Uint8Array) => App.InputBase64(id, bytesToB64(data)),
  resize: (id: string, cols: number, rows: number) => App.Resize(id, cols, rows),
}

export function errorMessage(e: unknown): string {
  if (e instanceof Error) return e.message
  return String(e)
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
