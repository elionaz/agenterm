// Drag & drop reordering helpers (sidebar projects and sessions).

/** The slot a drag over item i points at: before it (i) or after it (i + 1),
 * by which half of the item the pointer is in. */
export function dropSlot(clientY: number, rect: { top: number; height: number }, i: number): number {
  return clientY < rect.top + rect.height / 2 ? i : i + 1
}

/** ids with id moved to slot at (a slot is a gap: 0 is before the first item,
 * ids.length after the last), or null when that leaves the order unchanged. */
export function moveTo(ids: string[], id: string, at: number): string[] | null {
  const from = ids.indexOf(id)
  if (from < 0 || at < 0 || at > ids.length) return null
  const to = at > from ? at - 1 : at
  if (to === from) return null
  const out = ids.slice()
  out.splice(from, 1)
  out.splice(to, 0, id)
  return out
}

/** ids with id moved one place up (-1) or down (+1), or null at an edge. */
export function moveBy(ids: string[], id: string, delta: -1 | 1): string[] | null {
  const from = ids.indexOf(id)
  if (from < 0) return null
  return moveTo(ids, id, delta < 0 ? from - 1 : from + 2)
}

/** items with the members of a group put in the order of ids (members not
 * listed follow, in their order); everything else keeps its position. The
 * daemon applies the same rule (internal/order.Within). */
export function arrangeWithin<T extends { id: string }>(items: T[], member: (x: T) => boolean, ids: string[]): T[] {
  const group = items.filter(member)
  const pos = new Map(ids.map((id, i) => [id, i]))
  const sorted = group.slice().sort((a, b) => (pos.get(a.id) ?? ids.length) - (pos.get(b.id) ?? ids.length))
  let k = 0
  return items.map((x) => (member(x) ? sorted[k++] : x))
}
