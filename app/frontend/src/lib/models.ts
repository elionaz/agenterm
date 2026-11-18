// Model ids as the sidebar shows them.

const FAMILIES: Record<string, string> = { opus: 'Opus', sonnet: 'Sonnet', haiku: 'Haiku', fable: 'Fable' }

/** prettyModel turns a model id into a short label: claude-opus-5-5 → "Opus 5.5",
 * claude-sonnet-5-5-20261001 → "Sonnet 5.5"; other ids are shown as they are
 * (a date suffix dropped, at most 24 characters). */
export function prettyModel(id: string | undefined | null): string {
  if (!id) return ''
  let s = id.trim().replace(/\[.*\]$/, '')
  s = s.replace(/[-@]\d{8}$/, '')
  const m = /^(?:claude-)?(opus|sonnet|haiku|fable)-(\d+(?:[-.]\d+)*)$/i.exec(s) ?? /^(?:claude-)?(\d+(?:[-.]\d+)*)-(opus|sonnet|haiku|fable)$/i.exec(s)
  if (m) {
    const fam = /\d/.test(m[1]) ? m[2] : m[1]
    const ver = /\d/.test(m[1]) ? m[1] : m[2]
    return `${FAMILIES[fam.toLowerCase()]} ${ver.replace(/-/g, '.')}`
  }
  return s.length > 24 ? s.slice(0, 23) + '…' : s
}
