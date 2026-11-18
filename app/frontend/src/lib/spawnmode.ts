// The mode a new session is proposed (pure: no bindings).
export type SpawnMode = 'same' | 'worktree'

/** What the spawn menus, ⌘N and split panes propose: a shell starts in the
 * project folder (it is for looking around, not an agent with a branch); an
 * agent gets a worktree when another one already works in the folder, else
 * the project's remembered choice, else the setting (git.default_mode). */
export function proposedMode(o: { shell: boolean; busy: boolean; remembered: SpawnMode | null; fallback: SpawnMode }): SpawnMode {
  if (o.shell) return 'same'
  if (o.busy) return 'worktree'
  return o.remembered ?? o.fallback
}
