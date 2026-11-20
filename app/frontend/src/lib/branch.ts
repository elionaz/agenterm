// Branch names in tight spots (the sidebar).

/** tailRunes: how much of a branch's end always stays visible. */
export const tailRunes = 12

/** A branch as the sidebar shows it: without the agent/ prefix panal
 * gives worktree branches. */
export function shortBranch(branch: string): string {
  return branch.replace(/^agent\//, '')
}

/** shortBranch split for a middle ellipsis: head shrinks (CSS ellipsis)
 * and tail, the distinctive end, stays whole. The cut falls on a - / _
 * separator near the end when there is one. Short names are all head. */
export function branchParts(branch: string, tail = tailRunes): { head: string; tail: string } {
  const r = [...shortBranch(branch)]
  if (r.length <= tail + 4) return { head: r.join(''), tail: '' }
  let cut = r.length - tail
  for (let i = cut; i < r.length - 3; i++) {
    if ('-/_.'.includes(r[i])) {
      cut = i
      break
    }
  }
  return { head: r.slice(0, cut).join(''), tail: r.slice(cut).join('') }
}
