// One xterm.js Terminal per attached session, kept alive when not shown so
// switching is instant and scrollback is preserved. Shown terminals live in
// the slot of a split pane (place); the others wait in a hidden parking
// element.
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { SearchAddon, type ISearchOptions } from '@xterm/addon-search'
import { WebglAddon } from '@xterm/addon-webgl'
import { Unicode11Addon } from '@xterm/addon-unicode11'
import { WebLinksAddon } from '@xterm/addon-web-links'
import '@xterm/xterm/css/xterm.css'
import { BrowserOpenURL, ClipboardSetText, EventsOn } from '../../wailsjs/runtime/runtime'
import { api, b64ToBytes, errorMessage } from './api'
import { t } from './i18n.svelte'
import { SHIFT_ENTER, isShiftEnter } from './keys'
import { openLink } from './links'
import { defaultPrefs, themeOf, type TermPrefs } from './themes'

const MAX_REATTACH = 3

interface Entry {
  id: string
  el: HTMLDivElement
  term: Terminal
  fit: FitAddon
  search: SearchAddon
  offs: Array<() => void>
  /** True until the daemon finished replaying scrollback. While replaying,
   * xterm's automatic replies to queries in old output (DA, DSR…) must not
   * reach the live program. */
  replaying: boolean
  exited: boolean
  failures: number
  disposed: boolean
  /** Input not yet sent. Wails runs bound calls concurrently, so keystrokes
   * are sent one call at a time to keep their order. */
  pending: number[]
  sending: boolean
  fitTimer: number | undefined
}

/** Decides whether a ⌘-keydown belongs to the app rather than the terminal. */
export type ShortcutFilter = (e: KeyboardEvent) => boolean

export interface SearchResult {
  index: number
  count: number
}

export class TerminalManager {
  private parking: HTMLElement | null = null
  private entries = new Map<string, Entry>()
  /** The terminal that should have the keyboard (focused once placed). */
  private wanted: string | null = null
  private resizeObs: ResizeObserver | null = null
  private prefs: TermPrefs = defaultPrefs

  constructor(
    private isAppShortcut: ShortcutFilter,
    private onError: (msg: string) => void,
    /** A terminal got the keyboard (e.g. clicked): its pane takes the focus. */
    private onFocus: (id: string) => void = () => {},
  ) {}

  mount(parking: HTMLElement) {
    this.parking = parking
    this.resizeObs = new ResizeObserver((items) => {
      for (const it of items) {
        const e = [...this.entries.values()].find((x) => x.el === it.target)
        if (e) this.fitSoon(e)
      }
    })
    for (const e of this.entries.values()) this.resizeObs.observe(e.el)
  }

  unmount() {
    this.resizeObs?.disconnect()
    for (const id of [...this.entries.keys()]) this.dispose(id)
    this.parking = null
  }

  has(id: string) {
    return this.entries.has(id)
  }

  /** Shows id's terminal in slot, creating and attaching it on first use. */
  place(id: string, slot: HTMLElement) {
    let e = this.entries.get(id)
    if (!e) {
      e = this.create(id, slot)
      void this.attach(e)
    } else if (e.el.parentElement !== slot) {
      slot.appendChild(e.el)
      this.fitSoon(e)
    }
    if (this.wanted === id) e.term.focus()
  }

  /** Parks id's terminal again, if slot still holds it. */
  unplace(id: string, slot: HTMLElement) {
    const e = this.entries.get(id)
    if (e && e.el.parentElement === slot && this.parking) this.parking.appendChild(e.el)
  }

  /** Gives id's terminal the keyboard (as soon as it is placed). */
  focus(id: string | null = this.wanted) {
    this.wanted = id
    const e = id ? this.entries.get(id) : undefined
    if (e && e.el.parentElement !== this.parking) e.term.focus()
  }

  dispose(id: string) {
    const e = this.entries.get(id)
    if (!e) return
    e.disposed = true
    this.entries.delete(id)
    clearTimeout(e.fitTimer)
    this.resizeObs?.unobserve(e.el)
    e.offs.forEach((off) => off())
    void api.detach(id).catch(() => {})
    e.term.dispose()
    e.el.remove()
  }

  /** Disposes terminals whose session no longer exists. */
  retain(ids: Set<string>) {
    for (const id of [...this.entries.keys()]) if (!ids.has(id)) this.dispose(id)
  }

  /** The current size of id's terminal, if it has one. */
  size(id: string): { cols: number; rows: number } | null {
    const e = this.entries.get(id)
    return e ? { cols: e.term.cols, rows: e.term.rows } : null
  }

  /** Best-guess size for a terminal shown in an area of w×h pixels. */
  proposeSize(w: number, h: number): { cols: number; rows: number } {
    const cols = Math.floor((w - 16) / (this.prefs.fontSize * 0.602))
    const rows = Math.floor((h - 8) / Math.ceil(this.prefs.fontSize * this.prefs.lineHeight))
    if (cols > 10 && rows > 3) return { cols, rows }
    return { cols: 120, rows: 32 }
  }

  /** Applies font/cursor/theme settings to every terminal and refits. */
  setPrefs(p: TermPrefs) {
    this.prefs = p
    for (const e of this.entries.values()) {
      e.term.options.fontFamily = p.fontFamily
      e.term.options.fontSize = p.fontSize
      e.term.options.lineHeight = p.lineHeight
      e.term.options.cursorStyle = p.cursorStyle
      e.term.options.theme = themeOf(p.theme).colors
      this.fitSoon(e)
    }
  }

  /** Finds the next (or previous) match in id's buffer. */
  find(id: string, query: string, opts: { caseSensitive: boolean; backwards?: boolean; incremental?: boolean }): boolean {
    const e = this.entries.get(id)
    if (!e || !query) return false
    const light = themeOf(this.prefs.theme).light
    const o: ISearchOptions = {
      caseSensitive: opts.caseSensitive,
      incremental: opts.incremental,
      decorations: {
        matchBackground: light ? '#f3e08a' : '#5a4a1c',
        matchOverviewRuler: '#d7a83b',
        activeMatchBackground: light ? '#f0a35a' : '#a8632a',
        activeMatchColorOverviewRuler: '#f0a35a',
      },
    }
    return opts.backwards ? e.search.findPrevious(query, o) : e.search.findNext(query, o)
  }

  /** Removes search highlights from id's terminal. */
  clearSearch(id: string) {
    const e = this.entries.get(id)
    e?.search.clearDecorations()
    e?.term.clearSelection()
  }

  /** Calls cb with id's match count/position whenever they change. */
  onSearchResults(id: string, cb: (r: SearchResult) => void): () => void {
    const e = this.entries.get(id)
    if (!e) return () => {}
    const d = e.search.onDidChangeResults((r) => cb({ index: r.resultIndex, count: r.resultCount }))
    return () => d.dispose()
  }

  private fitSoon(e: Entry) {
    clearTimeout(e.fitTimer)
    e.fitTimer = window.setTimeout(() => this.fit(e), 30)
  }

  private fit(e: Entry) {
    // Parked (or not laid out yet): keep the last size.
    if (e.disposed || !e.el.offsetWidth || !e.el.offsetHeight) return
    try {
      e.fit.fit() // onResize forwards the new size to the PTY
    } catch {
      /* not laid out yet */
    }
  }

  private create(id: string, slot: HTMLElement): Entry {
    const el = document.createElement('div')
    el.className = 'term-instance'
    slot.appendChild(el)

    const p = this.prefs
    const term = new Terminal({
      fontFamily: p.fontFamily,
      fontSize: p.fontSize,
      lineHeight: p.lineHeight,
      cursorStyle: p.cursorStyle,
      theme: themeOf(p.theme).colors,
      cursorBlink: true,
      allowProposedApi: true, // unicode11, search decorations
      scrollback: 10000,
      macOptionIsMeta: true,
      macOptionClickForcesSelection: true,
      drawBoldTextInBrightColors: false,
      // OSC 8 hyperlinks (printed by gh, ls --hyperlink, …).
      linkHandler: { activate: (ev, text) => { openLink(ev, text, BrowserOpenURL) } },
    })
    const fit = new FitAddon()
    const search = new SearchAddon()
    term.loadAddon(fit)
    term.loadAddon(search)
    term.loadAddon(new Unicode11Addon())
    term.loadAddon(new WebLinksAddon((ev, uri) => { openLink(ev, uri, BrowserOpenURL) })) // plain-text URLs
    term.unicode.activeVersion = '11'
    term.open(el)
    try {
      const gl = new WebglAddon()
      gl.onContextLoss(() => gl.dispose()) // falls back to the DOM renderer
      term.loadAddon(gl)
    } catch {
      /* no WebGL2: the default DOM renderer is used */
    }

    const e: Entry = {
      id, el, term, fit, search, offs: [], replaying: true, exited: false, failures: 0, disposed: false,
      pending: [], sending: false, fitTimer: undefined,
    }
    this.entries.set(id, e)
    this.resizeObs?.observe(el)

    const encoder = new TextEncoder()
    term.attachCustomKeyEventHandler((ev) => {
      if (isShiftEnter(ev)) {
        // Handled on keydown; the keypress that follows must not send "\r".
        if (ev.type === 'keydown') {
          ev.preventDefault()
          this.send(e, encoder.encode(SHIFT_ENTER))
        }
        return false
      }
      if (ev.type !== 'keydown' || !ev.metaKey) return true
      if (ev.key === 'c' && term.hasSelection()) {
        void ClipboardSetText(term.getSelection())
        return false
      }
      // App shortcuts bubble to the window handler; xterm must ignore them.
      // Everything else (⌘V paste, ⌘A…) keeps its native behaviour.
      return !this.isAppShortcut(ev)
    })
    term.onData((d) => this.send(e, encoder.encode(d)))
    term.onBinary((d) => this.send(e, Uint8Array.from(d, (c) => c.charCodeAt(0) & 0xff)))
    term.onResize(({ cols, rows }) => {
      if (!e.exited) void api.resize(id, cols, rows).catch(() => {})
    })
    const onFocusIn = () => {
      this.wanted = id
      this.onFocus(id)
    }
    el.addEventListener('focusin', onFocusIn)

    e.offs.push(
      () => el.removeEventListener('focusin', onFocusIn),
      EventsOn(`pty:${id}`, (b64: string) => term.write(b64ToBytes(b64))),
      EventsOn(`pty-replay-done:${id}`, () => {
        e.replaying = false
        e.failures = 0
      }),
      EventsOn(`pty-exit:${id}`, (code: number) => {
        e.exited = true
        e.replaying = false
        term.write(`\r\n\x1b[2m${t('term.exited', { code })}\x1b[0m\r\n`)
      }),
      EventsOn(`pty-error:${id}`, (msg: string) => this.recover(e, msg)),
    )
    this.fit(e)
    return e
  }

  private send(e: Entry, bytes: Uint8Array) {
    if (e.replaying || e.exited || e.disposed) return
    for (const b of bytes) e.pending.push(b)
    if (!e.sending) void this.drain(e)
  }

  private async drain(e: Entry) {
    e.sending = true
    while (e.pending.length && !e.disposed) {
      const chunk = e.pending.splice(0, e.pending.length)
      try {
        await api.inputBytes(e.id, Uint8Array.from(chunk))
      } catch {
        e.pending.length = 0 // detached / exited: drop
      }
    }
    e.sending = false
  }

  private async attach(e: Entry) {
    this.fit(e)
    try {
      await api.attach(e.id, e.term.cols, e.term.rows)
    } catch (err) {
      if (!e.disposed) this.onError(`attach ${e.id}: ${errorMessage(err)}`)
    }
  }

  /** The stream dropped (slow client, daemon restart): reset and replay. */
  private recover(e: Entry, msg: string) {
    if (e.disposed || e.exited) return
    e.failures++
    if (e.failures > MAX_REATTACH) {
      e.term.write(`\r\n\x1b[31m${t('term.disconnected', { msg })}\x1b[0m\r\n`)
      return
    }
    setTimeout(() => {
      if (e.disposed) return
      e.term.reset()
      e.replaying = true
      void this.attach(e)
    }, 300 * e.failures)
  }
}
