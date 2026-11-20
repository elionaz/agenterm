import type { InstructionTopic, Profile } from './api'

/** setup_hooks values whose integration always hands over the instructions. */
const HOOKED = ['claude', 'gemini', 'opencode', 'cursor']

/** The agents that get the standing instructions and the ones that don't. */
export interface Receivers {
  yes: string[]
  no: string[]
}

/**
 * Splits the usable agents (installed, enabled, valid; not the plain
 * shell) by whether panal hands them the instructions: Claude Code,
 * Gemini CLI and Cursor in their session-start hook, opencode as an
 * instructions file, codex as developer_instructions unless the user's
 * config.toml sets its own (codexOwn). Names, without duplicates.
 */
export function receivers(profiles: Profile[], codexOwn: boolean): Receivers {
  const yes: string[] = []
  const no: string[] = []
  for (const p of profiles) {
    if (!p.installed || !p.enabled || p.error || p.id === 'shell') continue
    const hooks = p.setupHooks ?? ''
    const gets = HOOKED.includes(hooks) || (hooks === 'codex' && !codexOwn)
    const list = gets ? yes : no
    const name = p.name || p.id
    if (!list.includes(name)) list.push(name)
  }
  return { yes, no }
}

/** UTF-8 size of s, what the daemon's limits count. */
export function byteLength(s: string): number {
  return new TextEncoder().encode(s).length
}

/** The slug the daemon gives a new topic titled title (instructions.Slugify). */
export function slugify(title: string): string {
  let out = ''
  let dash = false
  for (const ch of title.toLowerCase().normalize('NFD')) {
    if (/\p{Mn}/u.test(ch)) continue
    if (/[a-z0-9]/.test(ch)) {
      if (dash && out) out += '-'
      dash = false
      out += ch
    } else dash = true
  }
  out = out.slice(0, 48).replace(/-+$/, '')
  return out || 'topic'
}

export type Load = 'always' | 'on_demand'

/** A suggested topic: created empty, with its own placeholder. */
export interface Suggestion {
  key: 'security' | 'architecture' | 'data' | 'git'
  load: Load
}

export const SUGGESTIONS: Suggestion[] = [
  { key: 'security', load: 'always' },
  { key: 'architecture', load: 'on_demand' },
  { key: 'data', load: 'on_demand' },
  { key: 'git', load: 'on_demand' },
]

/** The slugs a suggestion takes in each app language (and its short forms). */
const ALIASES: Record<Suggestion['key'], string[]> = {
  security: ['security', 'seguridad'],
  architecture: ['architecture', 'arquitectura'],
  data: ['data', 'data-management', 'datos', 'manejo-de-datos'],
  git: ['git'],
}

/** Suggestions not created yet: no topic has one of their slugs (in any
 * language) or the slug of their title in the current one. */
export function openSuggestions(topics: InstructionTopic[], title: (s: Suggestion) => string): Suggestion[] {
  const taken = new Set(topics.flatMap((t) => [t.slug, slugify(t.title)]))
  return SUGGESTIONS.filter((s) => ![...ALIASES[s.key], slugify(title(s))].some((x) => taken.has(x)))
}

/** An empty topic, ready to edit. */
export function blankTopic(title = '', load: Load = 'always', description = ''): InstructionTopic {
  return { slug: '', title, description, load, projects: [], enabled: true, order: 0, body: '' }
}

/** The ids of projects still registered (a topic may name removed ones). */
export function liveProjects(t: InstructionTopic, projectIds: string[]): string[] {
  return (t.projects ?? []).filter((id) => projectIds.includes(id))
}
