// Helpers of the History view (pure, tested).

export interface Part {
  text: string
  hit: boolean
}

/** line split at the byte ranges the daemon matched (Go byte offsets). */
export function highlight(line: string, matches: number[][] | null | undefined): Part[] {
  if (!matches?.length) return [{ text: line, hit: false }]
  const bytes = new TextEncoder().encode(line)
  const dec = new TextDecoder()
  const ranges = [...matches].sort((a, b) => a[0] - b[0])
  const parts: Part[] = []
  let at = 0
  for (const [s, e] of ranges) {
    if (s < at || e > bytes.length || s >= e) continue
    if (s > at) parts.push({ text: dec.decode(bytes.slice(at, s)), hit: false })
    parts.push({ text: dec.decode(bytes.slice(s, e)), hit: true })
    at = e
  }
  if (at < bytes.length) parts.push({ text: dec.decode(bytes.slice(at)), hit: false })
  return parts
}

/** Lines of text that contain every word (case-insensitive), as indexes. */
export function matchingLines(text: string, query: string): number[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean)
  if (!words.length) return []
  const out: number[] = []
  text.split('\n').forEach((l, i) => {
    const low = l.toLowerCase()
    if (words.some((w) => low.includes(w))) out.push(i)
  })
  return out
}

/** "45 min", "2 h 5 min", "3 d": how long a session lasted. */
export function duration(from: string, to: string): string {
  const ms = new Date(to).getTime() - new Date(from).getTime()
  if (!(ms >= 0)) return ''
  const min = Math.round(ms / 60000)
  if (min < 60) return `${min} min`
  const h = Math.floor(min / 60)
  if (h < 48) return min % 60 ? `${h} h ${min % 60} min` : `${h} h`
  return `${Math.round(h / 24)} d`
}
