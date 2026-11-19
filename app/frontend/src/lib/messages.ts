// Messages between the sessions of a project (agenterm msg): what the
// Messages tab shows.
import type { mailbox } from '../../wailsjs/go/models'
import type { Session } from './api'

export type Message = Omit<mailbox.Message, 'convertValues'>

/** The sender of messages that do not come from a session. */
export const USER = 'user'

/** Mirrors SHELL_PROFILE / isShell in the daemon: a nudge would run as a command. */
const SHELLS = new Set(['sh', 'bash', 'zsh', 'fish', 'dash', 'ksh', 'tcsh', 'csh', 'nu'])

export function isShellSession(s: Pick<Session, 'profileId' | 'cmd'>): boolean {
  const base = (s.cmd ?? '').split('/').pop() ?? ''
  return s.profileId === 'shell' || SHELLS.has(base)
}

export interface MessageRow {
  id: number
  /** in: sent to this session; out: sent by it. */
  dir: 'in' | 'out'
  /** The other end: a session id, or USER. */
  peer: string
  /** Its display name (current one when it is still open). */
  peerName: string
  /** The peer is not open any more. */
  gone: boolean
  body: string
  at: string
  pending: boolean
}

/** rows are the messages of session self, oldest first, as the tab lists them. */
export function messageRows(ms: Message[], self: string, sessions: Pick<Session, 'id' | 'name' | 'profileId'>[]): MessageRow[] {
  const byId = new Map(sessions.map((s) => [s.id, s]))
  return ms
    .filter((m) => m.from === self || m.to === self)
    .map((m) => {
      const dir = m.to === self ? 'in' : 'out'
      const peer = dir === 'in' ? m.from : m.to
      const live = byId.get(peer)
      const saved = (dir === 'in' ? m.fromName : m.toName) || peer
      return {
        id: m.id,
        dir,
        peer,
        peerName: peer === USER ? '' : live ? live.name || live.profileId : saved,
        gone: peer !== USER && !live,
        body: m.body,
        at: String(m.at ?? ''),
        pending: !m.deliveredAt,
      } satisfies MessageRow
    })
}

/** How many messages wait for self. */
export function pendingFor(ms: Message[], self: string): number {
  return ms.filter((m) => m.to === self && !m.deliveredAt).length
}

/** The Deliver button: something waits, the session can be told (not a shell),
 * and it is at its prompt. Not mid-turn (hooks hand messages over by themselves
 * then) and never while it waits for an answer: a permission dialog or a
 * question would take the typed line and its Enter as the answer. */
export function canDeliver(s: Pick<Session, 'profileId' | 'cmd' | 'state'>, pending: number): boolean {
  return pending > 0 && !isShellSession(s) && s.state === 'idle'
}

/** The Messages tab is offered when the project has another open session or
 * this one has messages. */
export function showMessagesTab(s: Pick<Session, 'id' | 'projectId' | 'msgPending'>, sessions: Pick<Session, 'id' | 'projectId' | 'state'>[], known: number): boolean {
  if ((s.msgPending ?? 0) > 0 || known > 0) return true
  return sessions.some((o) => o.id !== s.id && o.projectId === s.projectId && o.state !== 'exited')
}

/** What the Messages tab says about waiting messages, from the daemon's
 * reason (SessionInfo.msgHold) and the session's state. */
export function holdKey(
  hold: string | undefined,
  state: string,
):
  | 'msg.autoHelp'
  | 'msg.manualHelp'
  | 'msg.hold.soon'
  | 'msg.hold.nudged'
  | 'msg.hold.draft'
  | 'msg.hold.typing'
  | 'msg.hold.notReply'
  | 'msg.hold.noTurnEnd'
  | 'msg.hold.off'
  | 'msg.hold.limit' {
  switch (hold ?? '') {
    case 'nudged':
      return 'msg.hold.nudged'
    case 'unsent input':
      return 'msg.hold.draft'
    case 'typed just now':
      return 'msg.hold.typing'
    case 'not a reply':
      return 'msg.hold.notReply'
    case 'no turn end':
      return 'msg.hold.noTurnEnd'
    case 'off':
      return 'msg.hold.off'
    case 'chain limit':
    case 'project rate limit':
      return 'msg.hold.limit'
  }
  if (state === 'running') return 'msg.autoHelp'
  if (state === 'idle') return 'msg.hold.soon'
  return 'msg.manualHelp' // waiting for an answer: not over that
}
