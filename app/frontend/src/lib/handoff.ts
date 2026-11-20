// Sessions continued by another (session.handoff): the earlier one, once
// closed, shows under the session that continues its work instead of as a
// session of its own.
import type { Session } from './api'

type Info = Pick<Session, 'id' | 'state' | 'continuedFrom'>

/** A row of a project's session list. */
export interface Row<S> {
  s: S
  /** An earlier session shown under the one that continues it. */
  child: boolean
  /** For a session continued from others: how many earlier ones it has. */
  earlier: number
  /** Position among the top-level rows (⌘1–9), from 0; -1 for children. */
  n: number
}

/**
 * The rows of a project's list, in its order: every session except those
 * folded under the one that continues them (closed, and continued by a
 * session in the list), which follow it, chained (A → B → C: under C, B
 * then A) and only when it is expanded.
 */
export function arrange<S extends Info>(list: S[], expanded: Record<string, boolean>): Row<S>[] {
  const byId = new Map(list.map((s) => [s.id, s]))
  const earlierOf = (s: S): S[] => {
    const out: S[] = []
    const seen = new Set([s.id])
    for (let p = s.continuedFrom ? byId.get(s.continuedFrom) : undefined; p && p.state === 'exited' && !seen.has(p.id); ) {
      out.push(p)
      seen.add(p.id)
      p = p.continuedFrom ? byId.get(p.continuedFrom) : undefined
    }
    return out
  }
  const folded = new Set<string>()
  for (const s of list) {
    if (s.state !== 'exited' || !folded.has(s.id)) for (const e of earlierOf(s)) folded.add(e.id)
  }
  const rows: Row<S>[] = []
  let n = 0
  for (const s of list) {
    if (folded.has(s.id)) continue
    const earlier = earlierOf(s)
    rows.push({ s, child: false, earlier: earlier.length, n: n++ })
    if (expanded[s.id]) for (const e of earlier) rows.push({ s: e, child: true, earlier: 0, n: -1 })
  }
  return rows
}

/** The sessions ⌘1–9 pick: the top-level rows. */
export function topLevel<S extends Info>(list: S[]): S[] {
  return arrange(list, {}).map((r) => r.s)
}
