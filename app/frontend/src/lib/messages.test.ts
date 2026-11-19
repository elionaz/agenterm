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
    expect(holdKey('not a reply', 'idle')).toBe('msg.hold.notReply')
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
