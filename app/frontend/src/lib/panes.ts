// Split panes: a binary tree per project. Leaves show one session each;
// splits place their two children side by side (row) or stacked (col).
// Every function returns a new tree (the old one is left untouched).

export type Dir = 'row' | 'col'

export interface Leaf {
  type: 'leaf'
  id: string
  session: string | null
}

export interface Split {
  type: 'split'
  id: string
  dir: Dir
  /** Share of the first child (0..1). */
  ratio: number
  a: PaneNode
  b: PaneNode
}

export type PaneNode = Leaf | Split

export interface Layout {
  root: PaneNode
  /** The focused leaf's id. */
  focus: string
}

/** Fractions of the pane area. */
export interface Rect {
  x: number
  y: number
  w: number
  h: number
}

export interface Divider {
  /** The split it resizes. */
  id: string
  dir: Dir
  /** The split's own rect (the divider sits at ratio inside it). */
  rect: Rect
  ratio: number
}

export const MAX_PANES = 6
export const MIN_RATIO = 0.1

let seq = 0
export function newId(): string {
  return `p${Date.now().toString(36)}${(seq++).toString(36)}`
}

export function single(session: string | null): Layout {
  const leaf: Leaf = { type: 'leaf', id: newId(), session }
  return { root: leaf, focus: leaf.id }
}

export function leaves(n: PaneNode): Leaf[] {
  return n.type === 'leaf' ? [n] : [...leaves(n.a), ...leaves(n.b)]
}

export function focusedLeaf(l: Layout): Leaf {
  const all = leaves(l.root)
  return all.find((x) => x.id === l.focus) ?? all[0]
}

export function sessionsOf(l: Layout): string[] {
  return leaves(l.root)
    .map((x) => x.session)
    .filter((s): s is string => !!s)
}

/** Each leaf's rect and each split's divider. */
export function geometry(root: PaneNode): { rects: Map<string, Rect>; dividers: Divider[] } {
  const rects = new Map<string, Rect>()
  const dividers: Divider[] = []
  const walk = (n: PaneNode, r: Rect) => {
    if (n.type === 'leaf') {
      rects.set(n.id, r)
      return
    }
    dividers.push({ id: n.id, dir: n.dir, rect: r, ratio: n.ratio })
    if (n.dir === 'row') {
      const w = r.w * n.ratio
      walk(n.a, { x: r.x, y: r.y, w, h: r.h })
      walk(n.b, { x: r.x + w, y: r.y, w: r.w - w, h: r.h })
    } else {
      const h = r.h * n.ratio
      walk(n.a, { x: r.x, y: r.y, w: r.w, h })
      walk(n.b, { x: r.x, y: r.y + h, w: r.w, h: r.h - h })
    }
  }
  walk(root, { x: 0, y: 0, w: 1, h: 1 })
  return { rects, dividers }
}

function map(n: PaneNode, fn: (n: PaneNode) => PaneNode | null): PaneNode | null {
  const out = fn(n)
  if (out !== n || out === null || out.type === 'leaf') return out
  const a = map(out.a, fn)
  const b = map(out.b, fn)
  if (!a) return b
  if (!b) return a
  return a === out.a && b === out.b ? out : { ...out, a, b }
}

/** Splits leaf id: the old leaf keeps the first half, session gets the
 * second, which takes the focus. */
export function split(l: Layout, id: string, dir: Dir, session: string | null): Layout {
  const leaf: Leaf = { type: 'leaf', id: newId(), session }
  const root = map(l.root, (n) =>
    n.type === 'leaf' && n.id === id ? { type: 'split', id: newId(), dir, ratio: 0.5, a: n, b: leaf } : n,
  )!
  return { root, focus: root === l.root ? l.focus : leaf.id }
}

/** Removes leaf id (its sibling takes the split's place). The last leaf
 * cannot be closed: it is returned unchanged. */
export function close(l: Layout, id: string): Layout {
  const all = leaves(l.root)
  if (all.length <= 1) return l
  const idx = all.findIndex((x) => x.id === id)
  if (idx < 0) return l
  const root = map(l.root, (n) => (n.type === 'leaf' && n.id === id ? null : n))!
  // A closed focused pane hands the focus to its neighbour in reading
  // order (the previous one, else the next).
  const focus = l.focus === id ? (all[idx - 1] ?? all[idx + 1]).id : l.focus
  return { root, focus }
}

/** Shows session in leaf id. */
export function assign(l: Layout, id: string, session: string | null): Layout {
  const root = map(l.root, (n) => (n.type === 'leaf' && n.id === id ? { ...n, session } : n))!
  return { ...l, root }
}

export function setRatio(l: Layout, splitId: string, ratio: number): Layout {
  const r = Math.min(Math.max(ratio, MIN_RATIO), 1 - MIN_RATIO)
  const root = map(l.root, (n) => (n.type === 'split' && n.id === splitId ? { ...n, ratio: r } : n))!
  return { ...l, root }
}

/** Drops the panes of sessions not in alive (the last pane is kept,
 * emptied). Returns l itself when nothing changed. */
export function prune(l: Layout, alive: Set<string>): Layout {
  let out = l
  for (const leaf of leaves(l.root)) {
    if (!leaf.session || alive.has(leaf.session)) continue
    const closed = close(out, leaf.id)
    out = closed === out ? assign(out, leaf.id, null) : closed
  }
  return out
}

/** The leaf after (delta 1) or before (-1) id, wrapping around. */
export function cycle(l: Layout, id: string, delta: number): string {
  const all = leaves(l.root)
  const i = all.findIndex((x) => x.id === id)
  return all[(((i < 0 ? 0 : i) + delta) % all.length + all.length) % all.length].id
}

/** The nearest leaf in a direction from id (null at the edge). */
export function neighbor(l: Layout, id: string, toward: 'left' | 'right' | 'up' | 'down'): string | null {
  const { rects } = geometry(l.root)
  const from = rects.get(id)
  if (!from) return null
  const eps = 1e-6
  let best: string | null = null
  let bestScore = Infinity
  for (const [lid, r] of rects) {
    if (lid === id) continue
    let gap: number
    let overlap: number
    switch (toward) {
      case 'left':
        gap = from.x - (r.x + r.w)
        overlap = Math.min(from.y + from.h, r.y + r.h) - Math.max(from.y, r.y)
        break
      case 'right':
        gap = r.x - (from.x + from.w)
        overlap = Math.min(from.y + from.h, r.y + r.h) - Math.max(from.y, r.y)
        break
      case 'up':
        gap = from.y - (r.y + r.h)
        overlap = Math.min(from.x + from.w, r.x + r.w) - Math.max(from.x, r.x)
        break
      default:
        gap = r.y - (from.y + from.h)
        overlap = Math.min(from.x + from.w, r.x + r.w) - Math.max(from.x, r.x)
    }
    if (gap < -eps || overlap <= eps) continue
    // Closest first, then the one sharing the most edge.
    const score = gap * 10 - overlap
    if (score < bestScore) {
      bestScore = score
      best = lid
    }
  }
  return best
}

/** Parses a stored layout; null when it is not a valid one. */
export function parse(json: string | null): Layout | null {
  if (!json) return null
  try {
    const v = JSON.parse(json) as { root?: unknown; focus?: unknown }
    const ids = new Set<string>()
    const check = (n: unknown, depth: number): n is PaneNode => {
      if (!n || typeof n !== 'object' || depth > 8) return false
      const x = n as Record<string, unknown>
      if (typeof x.id !== 'string' || ids.has(x.id)) return false
      ids.add(x.id)
      if (x.type === 'leaf') return x.session === null || typeof x.session === 'string'
      return (
        x.type === 'split' &&
        (x.dir === 'row' || x.dir === 'col') &&
        typeof x.ratio === 'number' &&
        x.ratio >= MIN_RATIO &&
        x.ratio <= 1 - MIN_RATIO &&
        check(x.a, depth + 1) &&
        check(x.b, depth + 1)
      )
    }
    if (!check(v.root, 0) || leaves(v.root).length > MAX_PANES) return null
    const root = v.root
    const focus = typeof v.focus === 'string' && leaves(root).some((x) => x.id === v.focus) ? v.focus : leaves(root)[0].id
    return { root, focus }
  } catch {
    return null
  }
}

const key = (projectId: string) => `agenterm.panes.${projectId}`

export function loadLayout(projectId: string): Layout | null {
  try {
    return parse(localStorage.getItem(key(projectId)))
  } catch {
    return null
  }
}

export function saveLayout(projectId: string, l: Layout) {
  try {
    localStorage.setItem(key(projectId), JSON.stringify(l))
  } catch {
    /* storage unavailable */
  }
}
