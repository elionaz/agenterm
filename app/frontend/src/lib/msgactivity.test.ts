import { describe, expect, it } from 'vitest'
import { ACTIVITY_MS, currentActivity, pruneActivity, recordSent, sessionColor } from './msgactivity'

describe('sessionColor', () => {
  it('is stable and a badge color', () => {
    expect(sessionColor('abcd1234')).toBe(sessionColor('abcd1234'))
    expect(sessionColor('user')).toBe('')
    const seen = new Set(['a1', 'b2', 'c3', 'd4', 'e5', 'f6', 'g7', 'h8'].map(sessionColor))
    expect(seen.size).toBeGreaterThan(2)
  })
})

describe('message activity', () => {
  const ev = { msgFrom: 'a', msgFromName: 'fake-a', msgTo: 'b', msgToName: 'fake-b', msgExcerpt: 'merged?', msgHeld: true }
  it('records both sides', () => {
    const act = recordSent({}, ev, 1000)
    expect(act.a).toMatchObject({ dir: 'out', peer: 'b', peerName: 'fake-b', text: 'merged?', held: true })
    expect(act.b).toMatchObject({ dir: 'in', peer: 'a', peerName: 'fake-a' })
  })
  it('the user only shows on the recipient', () => {
    const act = recordSent({}, { msgFrom: 'user', msgTo: 'b', msgExcerpt: 'hi' }, 0)
    expect(Object.keys(act)).toEqual(['b'])
    expect(act.b.peer).toBe('user')
  })
  it('expires', () => {
    const act = recordSent({}, ev, 1000)
    expect(currentActivity(act, 'a', 1000 + ACTIVITY_MS - 1)).not.toBeNull()
    expect(currentActivity(act, 'a', 1000 + ACTIVITY_MS)).toBeNull()
    expect(pruneActivity(act, 1000 + ACTIVITY_MS)).toEqual({})
    expect(pruneActivity(act, 1001)).toBe(act)
  })
})

import { messagesAuto } from './msgactivity'
describe('messagesAuto', () => {
  it('project choice, else the default', () => {
    expect(messagesAuto({ messages: 'manual' }, true)).toBe(false)
    expect(messagesAuto({ messages: 'auto' }, false)).toBe(true)
    expect(messagesAuto({}, true)).toBe(true)
    expect(messagesAuto({ messages: '' }, false)).toBe(false)
  })
})
