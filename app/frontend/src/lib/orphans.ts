import type { WorktreeInfo } from './api'

type Orphan = Pick<WorktreeInfo, 'orphan' | 'createdAt'> & { git?: { merged?: boolean; uncommitted: number } | null }

/** Merged into its base with nothing uncommitted: removing it (and its branch) loses nothing. */
export function removable(w: Orphan): boolean {
  return !!w.git?.merged && w.git.uncommitted === 0
}

/** A project's orphaned worktrees, the removable ones first (otherwise the daemon's order). */
export function orphanList<T extends Orphan>(list: T[]): T[] {
  const orphans = list.filter((w) => w.orphan)
  return [...orphans.filter(removable), ...orphans.filter((w) => !removable(w))]
}
