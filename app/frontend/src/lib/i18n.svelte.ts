// Tiny typed i18n: the current language is reactive state, so templates
// that call t() re-render when it changes (no reload). Dictionaries live in
// ./locales (en is the source of truth).
import { keyLabel } from './platform'
import { en, type Msg, type MsgKey } from './locales/en'
import { es } from './locales/es'

export type Lang = 'en' | 'es'
/** settings.yaml ui.language. */
export type LangSetting = 'auto' | Lang
export type Params = Record<string, string | number>

const dicts: Record<Lang, Record<MsgKey, Msg>> = { en, es }
const STORAGE_KEY = 'agenterm.language'

/** First supported language in a preference list ("es-ES", "en"…). */
export function matchLang(tags: readonly string[]): Lang | null {
  for (const tag of tags) {
    const prim = tag.toLowerCase().split(/[-_.]/)[0]
    if (prim === 'en' || prim === 'es') return prim
  }
  return null
}

export function normalizeSetting(s: string | undefined | null): LangSetting {
  return s === 'en' || s === 'es' ? s : 'auto'
}

function browserLang(): Lang {
  try {
    return matchLang(navigator.languages?.length ? navigator.languages : [navigator.language]) ?? 'en'
  } catch {
    return 'en'
  }
}

function storedSetting(): LangSetting {
  try {
    return normalizeSetting(localStorage.getItem(STORAGE_KEY))
  } catch {
    return 'auto'
  }
}

const state = $state({ setting: storedSetting(), system: browserLang() })

/** The language strings are rendered in. */
export function lang(): Lang {
  return state.setting === 'auto' ? state.system : state.setting
}

/** The ui.language setting in effect. */
export function langSetting(): LangSetting {
  return state.setting
}

/** What "auto" resolves to. */
export function systemLang(): Lang {
  return state.system
}

/** BCP 47 tag for Intl / toLocaleString. */
export function locale(): string {
  return lang() === 'es' ? 'es-ES' : 'en-US'
}

function applyDocumentLang() {
  try {
    document.documentElement.lang = lang()
  } catch {
    /* no document (tests) */
  }
}

/** Applies a ui.language value (from settings.get/save) right away. The
 * last one is cached locally so the next start does not flash English. */
export function setLangSetting(s: string | undefined | null) {
  state.setting = normalizeSetting(s)
  try {
    localStorage.setItem(STORAGE_KEY, state.setting)
  } catch {
    /* storage unavailable */
  }
  applyDocumentLang()
}

/** The OS preferred language as the backend sees it (macOS AppleLanguages);
 * the webview's navigator.languages may only list the app's own locales. */
export function setSystemLang(l: string | undefined | null) {
  const m = l ? matchLang([l]) : null
  if (m) state.system = m
  applyDocumentLang()
}

export function hasKey(key: string): key is MsgKey {
  return key in en
}

const pluralRules: Partial<Record<Lang, Intl.PluralRules>> = {}

function pick(m: Msg, l: Lang, params?: Params): string {
  if (typeof m === 'string') return m
  const n = Number(params?.n ?? 0)
  const rules = (pluralRules[l] ??= new Intl.PluralRules(l))
  return rules.select(n) === 'one' ? m.one : m.other
}

function interpolate(s: string, params: Params | undefined, esc: (v: string) => string): string {
  if (!params) return s
  return s.replace(/\{(\w+)\}/g, (whole, name: string) => (name in params ? esc(String(params[name])) : whole))
}

/** Translates key in the current language; {name} takes params.name and
 * plural entries pick their form by params.n. */
export function t(key: MsgKey, params?: Params): string {
  const l = lang()
  const m = dicts[l][key] ?? en[key]
  return keyLabel(interpolate(pick(m, l, params), params, (v) => v))
}

export function escapeHtml(s: string): string {
  return s.replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]!)
}

/** t() for (html) entries: the markup comes from the dictionary only;
 * parameters are escaped. Use with {@html}. */
export function th(key: MsgKey, params?: Params): string {
  const l = lang()
  const m = dicts[l][key] ?? en[key]
  return keyLabel(interpolate(pick(m, l, params), params, escapeHtml))
}

applyDocumentLang()
