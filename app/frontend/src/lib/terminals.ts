// One xterm.js Terminal per attached session, kept alive (hidden) when not
// selected so switching is instant and scrollback is preserved.
import { Terminal, type ITheme } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebglAddon } from '@xterm/addon-webgl'
import { Unicode11Addon } from '@xterm/addon-unicode11'
import '@xterm/xterm/css/xterm.css'
import { ClipboardSetText, EventsOn } from '../../wailsjs/runtime/runtime'
import { api, b64ToBytes, errorMessage } from './api'

export const theme: ITheme = {
  background: '#141518',
  foreground: '#d6d7dc',
  cursor: '#d6d7dc',
  cursorAccent: '#141518',
  selectionBackground: '#3a3d4a',
  black: '#1d1f24',
  red: '#e06c75',
  green: '#98c379',
  yellow: '#e5c07b',
  blue: '#61afef',
  magenta: '#c678dd',
  cyan: '#56b6c2',
  white: '#c8ccd4',
  brightBlack: '#5c6370',
  brightRed: '#ef7b84',
  brightGreen: '#a8d38a',
  brightYellow: '#f0cd8b',
  brightBlue: '#74bdf7',
  brightMagenta: '#d58bea',
  brightCyan: '#69c7d3',
  brightWhite: '#ffffff',
}

const FONT_FAMILY = '"SF Mono", Menlo, Monaco, "JetBrains Mono", monospace'
const FONT_SIZE = 13
const MAX_REATTACH = 3

interface Entry {
  id: string
  el: HTMLDivElement
  term: Terminal
  fit: FitAddon
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
}

/** Decides whether a ⌘-keydown belongs to the app rather than the terminal. */
export type ShortcutFilter = (e: KeyboardEvent) => boolean

export class TerminalManager {
  private host: HTMLElement | null = null
  private entries = new Map<string, Entry>()
  private active: string | null = null
  private resizeObs: ResizeObserver | null = null
  private fitTimer: number | undefined

  constructor(
    private isAppShortcut: ShortcutFilter,
    private onError: (msg: string) => void,
  ) {}

  mount(host: HTMLElement) {
    this.host = host
    this.resizeObs = new ResizeObserver(() => {
      clearTimeout(this.fitTimer)
      this.fitTimer = window.setTimeout(() => this.fitActive(), 30)
    })
    this.resizeObs.observe(host)
  }

  unmount() {
    this.resizeObs?.disconnect()
    for (const id of [...this.entries.keys()]) this.dispose(id)
    this.host = null
  }

  has(id: string) {
    return this.entries.has(id)
  }

  /** Shows the terminal for id, creating and attaching it on first use. */
  show(id: string | null) {
    this.active = id
    for (const e of this.entries.values()) e.el.style.display = e.id === id ? '' : 'none'
    if (!id || !this.host) return
    let e = this.entries.get(id)
    if (!e) {
      e = this.create(id)
      void this.attach(e)
    } else {
      this.fit(e)
    }
    e.term.focus()
  }

  focus() {
    if (this.active) this.entries.get(this.active)?.term.focus()
  }

  dispose(id: string) {
    const e = this.entries.get(id)
    if (!e) return
    e.disposed = true
    this.entries.delete(id)
    e.offs.forEach((off) => off())
    void api.detach(id).catch(() => {})
    e.term.dispose()
    e.el.remove()
    if (this.active === id) this.active = null
  }

  /** Disposes terminals whose session no longer exists. */
  retain(ids: Set<string>) {
    for (const id of [...this.entries.keys()]) if (!ids.has(id)) this.dispose(id)
  }

  /** Best-guess size for a session that does not exist yet. */
  proposeSize(): { cols: number; rows: number } {
    for (const e of this.entries.values()) {
      if (e.el.style.display !== 'none') {
        const d = e.fit.proposeDimensions()
        if (d && d.cols > 0 && d.rows > 0) return { cols: d.cols, rows: d.rows }
      }
    }
    if (this.host) {
      const r = this.host.getBoundingClientRect()
      const cols = Math.floor((r.width - 16) / (FONT_SIZE * 0.602))
      const rows = Math.floor((r.height - 8) / Math.ceil(FONT_SIZE * 1.2))
      if (cols > 10 && rows > 3) return { cols, rows }
    }
    return { cols: 120, rows: 32 }
  }

  fitActive() {
    const e = this.active ? this.entries.get(this.active) : undefined
    if (e) this.fit(e)
  }

  private fit(e: Entry) {
    try {
      e.fit.fit() // onResize forwards the new size to the PTY
    } catch {
      /* not laid out yet */
    }
  }

  private create(id: string): Entry {
    const el = document.createElement('div')
    el.className = 'term-instance'
    this.host!.appendChild(el)

    const term = new Terminal({
      fontFamily: FONT_FAMILY,
      fontSize: FONT_SIZE,
      lineHeight: 1.15,
      theme,
      cursorBlink: true,
      allowProposedApi: true, // unicode11
      scrollback: 10000,
      macOptionIsMeta: true,
      macOptionClickForcesSelection: true,
      drawBoldTextInBrightColors: false,
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.loadAddon(new Unicode11Addon())
    term.unicode.activeVersion = '11'
    term.open(el)
    try {
      const gl = new WebglAddon()
      gl.onContextLoss(() => gl.dispose()) // falls back to the DOM renderer
      term.loadAddon(gl)
    } catch {
      /* no WebGL2: the default DOM renderer is used */
    }

    const e: Entry = { id, el, term, fit, offs: [], replaying: true, exited: false, failures: 0, disposed: false, pending: [], sending: false }
    this.entries.set(id, e)

    term.attachCustomKeyEventHandler((ev) => {
      if (ev.type !== 'keydown' || !ev.metaKey) return true
      if (ev.key === 'c' && term.hasSelection()) {
        void ClipboardSetText(term.getSelection())
        return false
      }
      // App shortcuts bubble to the window handler; xterm must ignore them.
      // Everything else (⌘V paste, ⌘A…) keeps its native behaviour.
      return !this.isAppShortcut(ev)
    })
    const encoder = new TextEncoder()
    term.onData((d) => this.send(e, encoder.encode(d)))
    term.onBinary((d) => this.send(e, Uint8Array.from(d, (c) => c.charCodeAt(0) & 0xff)))
    term.onResize(({ cols, rows }) => {
      if (!e.exited) void api.resize(id, cols, rows).catch(() => {})
    })

    e.offs.push(
      EventsOn(`pty:${id}`, (b64: string) => term.write(b64ToBytes(b64))),
      EventsOn(`pty-replay-done:${id}`, () => {
        e.replaying = false
        e.failures = 0
      }),
      EventsOn(`pty-exit:${id}`, (code: number) => {
        e.exited = true
        e.replaying = false
        term.write(`\r\n\x1b[2m[proceso terminado · código ${code}]\x1b[0m\r\n`)
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
      e.term.write(`\r\n\x1b[31m[desconectado: ${msg}]\x1b[0m\r\n`)
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
