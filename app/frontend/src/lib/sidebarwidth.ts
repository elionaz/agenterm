// The sidebar's width: dragged by its right edge, remembered per machine.

export const SIDEBAR_DEFAULT = 248
export const SIDEBAR_MIN = 220
export const SIDEBAR_MAX = 420
const KEY = 'agenterm.sidebarWidth'

/** w rounded and kept within [SIDEBAR_MIN, SIDEBAR_MAX]; the default for
 * anything that is not a finite number. */
export function clampSidebar(w: number): number {
  if (!Number.isFinite(w)) return SIDEBAR_DEFAULT
  return Math.min(SIDEBAR_MAX, Math.max(SIDEBAR_MIN, Math.round(w)))
}

/** The remembered width (the default if none, or storage is unavailable). */
export function loadSidebarWidth(store: Pick<Storage, 'getItem'> | undefined = globalThis.localStorage): number {
  try {
    const v = store?.getItem(KEY)
    return v ? clampSidebar(Number(v)) : SIDEBAR_DEFAULT
  } catch {
    return SIDEBAR_DEFAULT
  }
}

/** Remembers w; the default width is stored as "no preference". */
export function saveSidebarWidth(w: number, store: Pick<Storage, 'setItem' | 'removeItem'> | undefined = globalThis.localStorage) {
  try {
    if (w === SIDEBAR_DEFAULT) store?.removeItem(KEY)
    else store?.setItem(KEY, String(clampSidebar(w)))
  } catch {
    // private window, blocked storage: the width just isn't remembered
  }
}
