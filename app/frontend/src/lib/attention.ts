// When looking at a session acknowledges its attention.
import type { Session } from './api'

/** Whether showing session s to the user acknowledges its attention. A
 * finished turn or an error is seen once it is on screen; an approval or a
 * question the agent still waits for is not answered by looking at it, so it
 * stays flagged until the session moves on (or the user types into it). */
export function ackOnView(s: Pick<Session, 'unread' | 'attentionKind' | 'state'>): boolean {
  return !!s.unread && !!s.attentionKind && s.state !== 'exited' && s.state !== 'waiting_input'
}
