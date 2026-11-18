// The daemon after an update (app/daemonversion.go).

/** Whether the app restarts an older daemon on its own: only once per run
 * (a restart that leaves it old must not loop), never a newer daemon (that
 * would downgrade it) and never while an agent is in the middle of a turn
 * (the restart would cut it off; the banner asks instead). Agents waiting
 * for input or idle continue their conversation after the restart. */
export function shouldAutoRestart(s: { mismatch: boolean; newer: boolean }, busy: number, tried: boolean): boolean {
  return s.mismatch && !s.newer && busy === 0 && !tried
}
