// Activity line, timers and progress helpers shared by the sidebar, the
// title bar and the grid overview.
import { attentionLabel, needsAttention, type DaemonEvent, type Live, type Progress, type Session } from './api'
import { hasKey, t } from './i18n.svelte'

/** Merges the live fields of an event into a session. session.activity
 * carries all of them; session.state only the timers (activity text and
 * progress are left alone). Optional fields are omitted when cleared, so
 * those that each event type owns are copied as-is. */
export function mergeLive(s: Session, ev: DaemonEvent): Session {
  const timers = {
    turnStartedAt: ev.turnStartedAt ?? s.turnStartedAt,
    turnEndedAt: ev.turnEndedAt,
    lastOutputAt: ev.lastOutputAt ?? s.lastOutputAt,
    lastEventAt: ev.lastEventAt ?? s.lastEventAt,
    stalled: ev.stalled,
    stalledAt: ev.stalledAt,
  }
  if (ev.type === 'session.activity') {
    // The structured form goes with the text it describes.
    const fresh = ev.activity !== undefined
    return {
      ...s,
      ...timers,
      state: ev.state ?? s.state,
      activity: ev.activity ?? s.activity,
      activityKind: fresh ? ev.activityKind : s.activityKind,
      activityTarget: fresh ? ev.activityTarget : s.activityTarget,
      activityAt: ev.activityAt ?? s.activityAt,
      activitySource: ev.activitySource ?? s.activitySource,
      progress: ev.progress as Session['progress'],
    }
  }
  if (ev.type === 'session.state') return { ...s, ...timers }
  return s
}

/** A session's activity line in the UI language: hook activity is
 * rendered from its kind and target; output lines stay verbatim. */
export function activityText(s: Pick<Live, 'activity' | 'activityKind' | 'activityTarget'>): string {
  const kind = s.activityKind
  if (kind) {
    const key = `activity.${kind}`
    if (hasKey(key)) {
      if (s.activityTarget) return t(key, { target: s.activityTarget })
      const bare = `${key}.bare`
      return hasKey(bare) ? t(bare) : t(key, { target: '' })
    }
    if (kind === 'tool' && s.activityTarget) return s.activityTarget
  }
  return s.activity ?? ''
}

/** Status word for badges: attention first, then the state. */
export function stateLabel(s: Session): string {
  if (needsAttention(s)) return attentionLabel(s.attentionKind)
  switch (s.state) {
    case 'running':
    case 'waiting_input':
    case 'idle':
      return t(`state.${s.state}`)
    case 'exited':
      return s.exitCode != null ? t('state.exitCode', { code: s.exitCode }) : t('state.exited')
    case 'suspended':
      return t('state.suspended')
  }
  return s.state
}

const ms = (t: string | undefined | null): number => (t ? Date.parse(t) : NaN)

/** 45s · 4m 12s · 1h 05m (the same in every language). */
export function fmtDuration(millis: number, seconds = true): string {
  const t = Math.max(0, Math.floor(millis / 1000))
  const h = Math.floor(t / 3600)
  const m = Math.floor((t % 3600) / 60)
  const sec = t % 60
  if (h) return `${h}h ${String(m).padStart(2, '0')}m`
  if (m) return seconds ? `${m}m ${String(sec).padStart(2, '0')}s` : `${m}m`
  return `${sec}s`
}

/** Last output or hook event, whichever is newer (ms epoch, NaN if none). */
function lastSignal(s: Pick<Live, 'lastOutputAt' | 'lastEventAt'>): number {
  const a = ms(s.lastOutputAt)
  const b = ms(s.lastEventAt)
  if (isNaN(a)) return b
  if (isNaN(b)) return a
  return Math.max(a, b)
}

export interface TimerInfo {
  text: string
  /** stalled: amber warning */
  stalled: boolean
}

/** The timer line: "working 4m 12s", "stalled? no activity for 5m",
 * "finished in 6m". Empty when there is nothing useful to say. */
export function timerInfo(s: Session, now: number, verbose = false): TimerInfo {
  if (s.state === 'exited') return { text: '', stalled: false }
  if (s.stalled && s.state === 'running') {
    const last = lastSignal(s)
    const text = isNaN(last) ? t('timer.stalled') : t('timer.stalledFor', { d: fmtDuration(now - last, false) })
    return { text, stalled: true }
  }
  const start = ms(s.turnStartedAt)
  if (s.state === 'running' && !isNaN(start)) {
    let text = t('timer.working', { d: fmtDuration(now - start) })
    const out = ms(s.lastOutputAt)
    if (verbose && !isNaN(out) && now - out >= 10_000) text += t('timer.lastOutput', { d: fmtDuration(now - out, false) })
    return { text, stalled: false }
  }
  const end = ms(s.turnEndedAt)
  if (s.state === 'idle' && !isNaN(start) && !isNaN(end) && end >= start) {
    let text = t('timer.finishedIn', { d: fmtDuration(end - start, false) })
    if (verbose) text += t('timer.ago', { d: fmtDuration(now - end, false) })
    return { text, stalled: false }
  }
  if (s.state === 'waiting_input' && s.attentionAt) {
    return { text: t('timer.waiting', { d: fmtDuration(now - ms(s.attentionAt), false) }), stalled: false }
  }
  return { text: '', stalled: false }
}

/** Short compact timer for the sidebar row (a running turn: just the
 * elapsed time). */
export function shortTimer(s: Session, now: number): TimerInfo {
  const info = timerInfo(s, now)
  if (info.stalled) return { text: t('timer.stalledShort'), stalled: true }
  const start = ms(s.turnStartedAt)
  if (s.state === 'running' && !isNaN(start)) return { text: fmtDuration(now - start), stalled: false }
  return info
}

export function progressOf(s: Session): Progress | undefined {
  return (s.progress as Progress | undefined) ?? undefined
}

export function progressLabel(p: Progress): string {
  switch (p.state) {
    case 'indeterminate':
      return t('progress.indeterminate')
    case 'error':
      return t('progress.error', { value: p.value })
    case 'paused':
      return t('progress.paused', { value: p.value })
    default:
      return `${p.value}%`
  }
}
