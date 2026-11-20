import { describe, expect, it } from 'vitest'
import { canDeliver, holdKey, isShellSession, messageRows, pendingFor, showMessagesTab, USER, type Message } from './messages'

const msg = (o: Partial<Message>): Message => ({ id: 1, projectId: 'p', from: 'a', to: 'b', body: 'hi', at: '2026-11-18T10:00:00Z', ...o }) as Message

describe('messageRows', () => {
  const sessions = [
    { id: 'a', name: 'alpha', profileId: 'claude' },
    { id: 'b', name: '', profileId: 'codex' },
  ]
  it('splits in and out, names the other end', () => {
    const rows = messageRows(
      [msg({ id: 1, from: 'a', to: 'b' }), msg({ id: 2, from: 'b', to: 'a', deliveredAt: 'x' }), msg({ id: 3, from: 'c', to: 'd' })],
      'a',
      sessions,
    )
    expect(rows.map((r) => [r.id, r.dir, r.peerName, r.pending])).toEqual([
      [1, 'out', 'codex', true],
      [2, 'in', 'codex', false],
    ])
  })
  it('keeps the saved name of a closed session and marks it gone', () => {
    const [r] = messageRows([msg({ from: 'z', fromName: 'old', to: 'a' })], 'a', sessions)
    expect(r).toMatchObject({ peerName: 'old', gone: true })
  })
  it('messages from the user have no peer name', () => {
    const [r] = messageRows([msg({ from: USER, to: 'a' })], 'a', sessions)
    expect(r).toMatchObject({ peer: USER, peerName: '', gone: false })
  })
})

describe('deliver', () => {
  it('counts pending messages for a session', () => {
    expect(pendingFor([msg({ to: 'b' }), msg({ to: 'b', deliveredAt: 'x' }), msg({ to: 'c' })], 'b')).toBe(1)
  })
  it('is offered to idle agents with pending messages, never shells or prompts', () => {
    const agent = { profileId: 'codex', cmd: '/usr/local/bin/codex', state: 'idle' }
    expect(canDeliver(agent, 1)).toBe(true)
    expect(canDeliver({ ...agent, state: 'waiting_input' }, 2)).toBe(false)
    expect(canDeliver({ ...agent, state: 'running' }, 1)).toBe(false)
    expect(canDeliver(agent, 0)).toBe(false)
    expect(canDeliver({ profileId: 'myshell', cmd: '/bin/zsh', state: 'idle' }, 1)).toBe(false)
    // Held messages can always be let through.
    expect(canDeliver({ ...agent, state: 'running' }, 1, true)).toBe(true)
    expect(canDeliver({ profileId: 'myshell', cmd: '/bin/zsh', state: 'idle' }, 1, true)).toBe(true)
    expect(isShellSession({ profileId: 'shell', cmd: '' })).toBe(true)
  })
})

describe('showMessagesTab', () => {
  const sessions = [
    { id: 'a', projectId: 'p', state: 'idle' },
    { id: 'b', projectId: 'p', state: 'exited' },
    { id: 'c', projectId: 'q', state: 'idle' },
  ]
  it('needs another open session in the project or messages', () => {
    expect(showMessagesTab({ id: 'a', projectId: 'p' }, sessions, 0)).toBe(false)
    expect(showMessagesTab({ id: 'a', projectId: 'p', msgPending: 1 }, sessions, 0)).toBe(true)
    expect(showMessagesTab({ id: 'a', projectId: 'p' }, sessions, 3)).toBe(true)
    expect(showMessagesTab({ id: 'a', projectId: 'p' }, [...sessions, { id: 'd', projectId: 'p', state: 'running' }], 0)).toBe(true)
  })
})

describe('holdKey', () => {
  it('names each reason', () => {
    expect(holdKey('unsent input', 'idle')).toBe('msg.hold.draft')
    expect(holdKey('nudged', 'idle')).toBe('msg.hold.nudged')
    expect(holdKey('held', 'running')).toBe('msg.hold.held')
    expect(holdKey('no turn end', 'idle')).toBe('msg.hold.noTurnEnd')
    expect(holdKey('off', 'idle')).toBe('msg.hold.off')
    expect(holdKey('chain limit', 'idle')).toBe('msg.hold.limit')
  })
  it('falls back to the state', () => {
    expect(holdKey('', 'running')).toBe('msg.autoHelp')
    expect(holdKey(undefined, 'idle')).toBe('msg.hold.soon')
    expect(holdKey('not idle', 'waiting_input')).toBe('msg.manualHelp')
  })
})

import { conversationRows, heldRecipients } from './messages'
describe('conversation', () => {
  const ms = [
    { id: 1, projectId: 'p', from: 'a', fromName: 'old-a', to: 'b', toName: 'fake-b', body: 'hi', at: '1', deliveredAt: '2' },
    { id: 2, projectId: 'p', from: 'b', to: 'a', body: 'merged?', at: '3', held: true },
    { id: 3, projectId: 'p', from: 'user', to: 'c', body: 'go', at: '4' },
  ] as unknown as Message[]
  const sessions = [{ id: 'a', name: 'fake-a', profileId: 'claude' }, { id: 'b', name: '', profileId: 'codex' }]
  it('names, status and filter', () => {
    const rows = conversationRows(ms, sessions)
    expect(rows.map((r) => r.status)).toEqual(['delivered', 'held', 'pending'])
    expect(rows[0].fromName).toBe('fake-a')
    expect(rows[1].fromName).toBe('codex')
    expect(rows[2].fromName).toBe('')
    expect(rows[2].toName).toBe('c')
    expect(conversationRows(ms, sessions, 'c').map((r) => r.id)).toEqual([3])
    expect(heldRecipients(rows)).toEqual(['a'])
  })
})
