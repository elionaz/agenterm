// Activity line, timers and progress helpers shared by the sidebar, the
// title bar and the grid overview.
import type { DaemonEvent, Live, Progress, Session } from './api'

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
    return {
      ...s,
      ...timers,
      state: ev.state ?? s.state,
      activity: ev.activity ?? s.activity,
      activityAt: ev.activityAt ?? s.activityAt,
      activitySource: ev.activitySource ?? s.activitySource,
      progress: ev.progress as Session['progress'],
    }
  }
  if (ev.type === 'session.state') return { ...s, ...timers }
  return s
}

const ms = (t: string | undefined | null): number => (t ? Date.parse(t) : NaN)

/** 45s · 4m 12s · 1h 05m */
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

/** The timer line: "trabajando 4m 12s", "¿trabado? sin actividad hace 5m",
 * "terminó en 6m". Empty when there is nothing useful to say. */
export function timerInfo(s: Session, now: number, verbose = false): TimerInfo {
  if (s.state === 'exited') return { text: '', stalled: false }
  if (s.stalled && s.state === 'running') {
    const last = lastSignal(s)
    const quiet = isNaN(last) ? '' : ` hace ${fmtDuration(now - last, false)}`
    return { text: `¿trabado? sin actividad${quiet}`, stalled: true }
  }
  const start = ms(s.turnStartedAt)
  if (s.state === 'running' && !isNaN(start)) {
    let text = `trabajando ${fmtDuration(now - start)}`
    const out = ms(s.lastOutputAt)
    if (verbose && !isNaN(out) && now - out >= 10_000) text += ` · última salida hace ${fmtDuration(now - out, false)}`
    return { text, stalled: false }
  }
  const end = ms(s.turnEndedAt)
  if (s.state === 'idle' && !isNaN(start) && !isNaN(end) && end >= start) {
    let text = `terminó en ${fmtDuration(end - start, false)}`
    if (verbose) text += ` · hace ${fmtDuration(now - end, false)}`
    return { text, stalled: false }
  }
  if (s.state === 'waiting_input' && s.attentionAt) {
    return { text: `esperando ${fmtDuration(now - ms(s.attentionAt), false)}`, stalled: false }
  }
  return { text: '', stalled: false }
}

/** Short compact timer for the sidebar row. */
export function shortTimer(s: Session, now: number): TimerInfo {
  const t = timerInfo(s, now)
  if (t.stalled) return { text: '¿trabado?', stalled: true }
  return { text: t.text.replace(/^trabajando /, ''), stalled: false }
}

export function progressOf(s: Session): Progress | undefined {
  return (s.progress as Progress | undefined) ?? undefined
}

export function progressLabel(p: Progress): string {
  switch (p.state) {
    case 'indeterminate':
      return 'en progreso'
    case 'error':
      return `error · ${p.value}%`
    case 'paused':
      return `en pausa · ${p.value}%`
    default:
      return `${p.value}%`
  }
}
