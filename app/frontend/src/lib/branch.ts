// Branch names in tight spots (the sidebar).

/** maxBranchRunes: longer names are cut in the middle. */
export const maxBranchRunes = 28

/** A branch as the sidebar shows it: without the agent/ prefix agenterm
 * gives worktree branches, and cut in the middle when long, so both its
 * start (feat/, fix/) and its distinctive end stay readable. */
export function shortBranch(branch: string, max = maxBranchRunes): string {
  const b = branch.replace(/^agent\//, '')
  const r = [...b]
  if (r.length <= max) return b
  const keep = max - 1
  const head = Math.ceil(keep / 2)
  return r.slice(0, head).join('') + '…' + r.slice(r.length - (keep - head)).join('')
}
