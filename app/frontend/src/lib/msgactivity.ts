// Who is talking to whom: the recent message activity of each session
// (the sidebar's "→ fake-b: …" / "← fake-a: …" line) and a stable color per
// session, so the same session reads the same everywhere.
import { PROJECT_COLORS, hash, type ProjectColor } from './badge'

/** How long a message shows on the rows of its sender and recipient. */
export const ACTIVITY_MS = 15_000

/** The user's messages get no session color. */
export const USER = 'user'

export interface MsgActivity {
  /** out: this session sent it; in: it got it. */
  dir: 'out' | 'in'
  /** The other side: a session id, or USER. */
  peer: string
  peerName: string
  text: string
  held: boolean
  at: number
}

/** A session's color (one of the badge colors), stable for its id. */
export function sessionColor(id: string): ProjectColor | '' {
  if (!id || id === USER) return ''
  return PROJECT_COLORS[hash('s:' + id) % PROJECT_COLORS.length]
}

export interface SentEvent {
  msgFrom?: string
  msgFromName?: string
  msgTo?: string
  msgToName?: string
  msgExcerpt?: string
  msgHeld?: boolean
}

/** activity with a message.sent event recorded on both rows (now in ms). */
export function recordSent(activity: Record<string, MsgActivity>, ev: SentEvent, now: number): Record<string, MsgActivity> {
  const from = ev.msgFrom ?? ''
  const to = ev.msgTo ?? ''
  if (!to) return activity
  const text = ev.msgExcerpt ?? ''
  const held = !!ev.msgHeld
  const next = { ...activity }
  next[to] = { dir: 'in', peer: from || USER, peerName: ev.msgFromName || '', text, held, at: now }
  if (from && from !== USER) next[from] = { dir: 'out', peer: to, peerName: ev.msgToName || '', text, held, at: now }
  return next
}

/** session id's activity while it is fresh, else null. */
export function currentActivity(activity: Record<string, MsgActivity>, id: string, now: number): MsgActivity | null {
  const a = activity[id]
  return a && now - a.at < ACTIVITY_MS ? a : null
}

/** activity without the entries that expired. */
export function pruneActivity(activity: Record<string, MsgActivity>, now: number): Record<string, MsgActivity> {
  const out: Record<string, MsgActivity> = {}
  let changed = false
  for (const [k, v] of Object.entries(activity)) {
    if (now - v.at < ACTIVITY_MS) out[k] = v
    else changed = true
  }
  return changed ? out : activity
}

/** Whether a project's messages travel by themselves: its own choice, else
 * the messages.auto_deliver setting (defaultAuto). */
export function messagesAuto(p: { messages?: string }, defaultAuto: boolean): boolean {
  if (p.messages === 'auto') return true
  if (p.messages === 'manual') return false
  return defaultAuto
}
