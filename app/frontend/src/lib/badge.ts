// Project badges: a colored square with the project's initials, or the
// emoji the user picked, so projects tell apart at a glance.
import type { Project } from './api'

/** The badge colors, as in project.Colors (Go); each is a style.css token. */
export const PROJECT_COLORS = ['blue', 'teal', 'green', 'amber', 'orange', 'red', 'pink', 'purple'] as const
export type ProjectColor = (typeof PROJECT_COLORS)[number]

/** A change to a project (project.update): fields left out stay; an empty
 * color or icon goes back to automatic. */
export interface ProjectPatch {
  name?: string
  color?: string
  icon?: string
}

/** FNV-1a with a final mix (its low bits alone spread poorly): a small,
 * stable hash, so an id gets the same color everywhere. */
function hash(s: string): number {
  let h = 0x811c9dc5
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 0x01000193)
  }
  h ^= h >>> 16
  h = Math.imul(h, 0x85ebca6b)
  h ^= h >>> 13
  h = Math.imul(h, 0xc2b2ae35)
  h ^= h >>> 16
  return h >>> 0
}

/** The project's color: the one picked, else one derived from its id. */
export function projectColor(p: Pick<Project, 'id' | 'color'>): ProjectColor {
  const c = p.color as ProjectColor | undefined
  if (c && PROJECT_COLORS.includes(c)) return c
  return PROJECT_COLORS[hash(p.id) % PROJECT_COLORS.length]
}

const segmenter = new Intl.Segmenter(undefined, { granularity: 'grapheme' })
function graphemes(s: string): string[] {
  return [...segmenter.segment(s)].map((x) => x.segment)
}

/** Two letters for a name: "web-shop" → "WS", "agenterm" → "AG", "my api" → "MA". */
export function initials(name: string): string {
  const words = name.split(/[^\p{L}\p{N}]+/u).filter(Boolean)
  if (!words.length) return graphemes(name.trim())[0] ?? '?'
  const g =
    words.length > 1 ? [graphemes(words[0])[0], graphemes(words[1])[0]] : graphemes(words[0]).slice(0, 2)
  return g.join('').toLocaleUpperCase()
}

/** What the badge shows: the icon (emoji: true when it is a single
 * non-letter symbol, drawn larger on a soft background) or the initials. */
export function badgeContent(p: Pick<Project, 'name' | 'icon'>): { text: string; emoji: boolean } {
  const icon = p.icon?.trim()
  if (!icon) return { text: initials(p.name), emoji: false }
  const g = graphemes(icon)
  return { text: g.slice(0, 2).join(''), emoji: g.length === 1 && !/^[\p{L}\p{N}]$/u.test(g[0]) }
}
