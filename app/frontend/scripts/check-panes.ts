// Unit checks for the split-pane tree (src/lib/panes.ts). Run by
// `npm run check` with Node's built-in TypeScript type stripping.
import assert from 'node:assert/strict'
import {
  MAX_PANES,
  assign,
  close,
  cycle,
  focusedLeaf,
  geometry,
  leaves,
  neighbor,
  parse,
  prune,
  sessionsOf,
  setRatio,
  single,
  split,
} from '../src/lib/panes.ts'

// a | b, then b split down into b / c:  [ a | b ]
//                                        [   | c ]
let l = single('a')
const la = l.focus
l = split(l, la, 'row', 'b')
const lb = l.focus
assert.notEqual(lb, la, 'split focuses the new pane')
l = split(l, lb, 'col', 'c')
const lc = l.focus
assert.deepEqual(sessionsOf(l), ['a', 'b', 'c'])

const { rects, dividers } = geometry(l.root)
assert.deepEqual(rects.get(la), { x: 0, y: 0, w: 0.5, h: 1 })
assert.deepEqual(rects.get(lb), { x: 0.5, y: 0, w: 0.5, h: 0.5 })
assert.deepEqual(rects.get(lc), { x: 0.5, y: 0.5, w: 0.5, h: 0.5 })
assert.equal(dividers.length, 2)

// Directional and cyclic focus moves.
assert.equal(neighbor(l, lc, 'left'), la)
assert.equal(neighbor(l, lc, 'up'), lb)
assert.equal(neighbor(l, lb, 'down'), lc)
assert.equal(neighbor(l, la, 'right') !== null, true)
assert.equal(neighbor(l, la, 'left'), null)
assert.equal(cycle(l, lc, 1), la)
assert.equal(cycle(l, la, -1), lc)

// Ratios are clamped.
const top = l.root
assert.equal(top.type, 'split')
const r = setRatio(l, top.id, 0.99)
assert.equal(r.root.type === 'split' && r.root.ratio, 0.9)
assert.equal(l.root.type === 'split' && l.root.ratio, 0.5, 'setRatio does not mutate')

// Closing the focused pane moves the focus to its neighbour; the last
// pane cannot be closed.
let c = close(l, lc)
assert.deepEqual(sessionsOf(c), ['a', 'b'])
assert.equal(c.focus, lb)
c = close(c, lb)
assert.deepEqual(sessionsOf(c), ['a'])
assert.equal(close(c, c.focus), c)

// Prune drops panes of gone sessions, keeping (emptied) the last one.
const p = prune(l, new Set(['a', 'c']))
assert.deepEqual(sessionsOf(p), ['a', 'c'])
assert.equal(prune(l, new Set(['a', 'b', 'c'])), l, 'nothing to prune: same layout')
const none = prune(l, new Set())
assert.equal(leaves(none.root).length, 1)
assert.equal(focusedLeaf(none).session, null)

// assign replaces a pane's session.
assert.deepEqual(sessionsOf(assign(l, la, 'z')), ['z', 'b', 'c'])

// Stored layouts round-trip; broken ones are rejected.
assert.deepEqual(parse(JSON.stringify(l)), l)
assert.equal(parse('{"root":{"type":"leaf"}}'), null)
assert.equal(parse('not json'), null)
assert.equal(parse(JSON.stringify({ root: { type: 'split', id: 's', dir: 'row', ratio: 0.5, a: { type: 'leaf', id: 'x', session: 'a' }, b: { type: 'leaf', id: 'x', session: 'b' } } })), null, 'duplicate ids')
let big = single('s0')
for (let i = 1; i <= MAX_PANES; i++) big = split(big, big.focus, 'row', `s${i}`)
assert.equal(parse(JSON.stringify(big)), null, 'too many panes')
const unfocused = parse(JSON.stringify({ ...l, focus: 'gone' }))
assert.equal(unfocused?.focus, la, 'an unknown focus falls back to the first pane')

console.log('panes: OK')
