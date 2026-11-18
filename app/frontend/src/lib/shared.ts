// Sessions that share their project's folder: without a worktree, every
// session of a project works on the same branch and the same changes.
import type { Session } from './api'

type Info = Pick<Session, 'id' | 'projectId' | 'profileId' | 'mode' | 'worktreeId' | 'state'>

/** The plain shell: a terminal next to the agents, not one of them. */
export const SHELL_PROFILE = 'shell'

/** The session runs in its project's folder, not in a worktree. */
export function inProjectFolder(s: Pick<Session, 'mode' | 'worktreeId'>): boolean {
  return s.mode !== 'worktree' && !s.worktreeId
}

/** The other live agents working in a project's folder. Exited and suspended
 * sessions don't count (nothing runs there), nor do plain shells. */
export function folderSharers<S extends Info>(sessions: S[], projectId: string, exceptId?: string): S[] {
  return sessions.filter(
    (s) =>
      s.projectId === projectId &&
      s.id !== exceptId &&
      s.profileId !== SHELL_PROFILE &&
      inProjectFolder(s) &&
      s.state !== 'exited' &&
      s.state !== 'suspended',
  )
}

/** "claude ×2, codex": repeated names (sessions not renamed) counted once. */
export function joinNames(names: string[]): string {
  const counts = new Map<string, number>()
  for (const n of names) counts.set(n, (counts.get(n) ?? 0) + 1)
  return [...counts].map(([n, c]) => (c > 1 ? `${n} ×${c}` : n)).join(', ')
}
