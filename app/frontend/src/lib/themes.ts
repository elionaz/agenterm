// Built-in terminal palettes (settings terminal.theme). Keep the names in
// sync with internal/settings Themes.
import type { ITheme } from '@xterm/xterm'

export interface TermTheme {
  /** Light background: search highlights and the preview adapt. */
  light: boolean
  colors: ITheme
}

export const DEFAULT_THEME = 'agenterm-dark'

export const themes: Record<string, TermTheme> = {
  'agenterm-dark': {
    light: false,
    colors: {
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
    },
  },
  // One Light-based.
  'agenterm-light': {
    light: true,
    colors: {
      background: '#fafafa',
      foreground: '#383a42',
      cursor: '#526eff',
      cursorAccent: '#fafafa',
      selectionBackground: '#d4dbe8',
      black: '#383a42',
      red: '#e45649',
      green: '#50a14f',
      yellow: '#c18401',
      blue: '#4078f2',
      magenta: '#a626a4',
      cyan: '#0184bc',
      white: '#a0a1a7',
      brightBlack: '#696c77',
      brightRed: '#ca1243',
      brightGreen: '#3f8f3e',
      brightYellow: '#986801',
      brightBlue: '#2f63d6',
      brightMagenta: '#8a1f88',
      brightCyan: '#0f6f99',
      brightWhite: '#ffffff',
    },
  },
  // Ethan Schoonover's palette; bright black is base01 (base03 would be
  // invisible on the background).
  'solarized-dark': {
    light: false,
    colors: {
      background: '#002b36',
      foreground: '#839496',
      cursor: '#93a1a1',
      cursorAccent: '#002b36',
      selectionBackground: '#073642',
      black: '#073642',
      red: '#dc322f',
      green: '#859900',
      yellow: '#b58900',
      blue: '#268bd2',
      magenta: '#d33682',
      cyan: '#2aa198',
      white: '#eee8d5',
      brightBlack: '#586e75',
      brightRed: '#cb4b16',
      brightGreen: '#586e75',
      brightYellow: '#657b83',
      brightBlue: '#839496',
      brightMagenta: '#6c71c4',
      brightCyan: '#93a1a1',
      brightWhite: '#fdf6e3',
    },
  },
  'solarized-light': {
    light: true,
    colors: {
      background: '#fdf6e3',
      foreground: '#657b83',
      cursor: '#586e75',
      cursorAccent: '#fdf6e3',
      selectionBackground: '#eee8d5',
      black: '#073642',
      red: '#dc322f',
      green: '#859900',
      yellow: '#b58900',
      blue: '#268bd2',
      magenta: '#d33682',
      cyan: '#2aa198',
      white: '#eee8d5',
      brightBlack: '#586e75',
      brightRed: '#cb4b16',
      brightGreen: '#586e75',
      brightYellow: '#657b83',
      brightBlue: '#839496',
      brightMagenta: '#6c71c4',
      brightCyan: '#93a1a1',
      brightWhite: '#fdf6e3',
    },
  },
  // draculatheme.com terminal spec.
  dracula: {
    light: false,
    colors: {
      background: '#282a36',
      foreground: '#f8f8f2',
      cursor: '#f8f8f2',
      cursorAccent: '#282a36',
      selectionBackground: '#44475a',
      black: '#21222c',
      red: '#ff5555',
      green: '#50fa7b',
      yellow: '#f1fa8c',
      blue: '#bd93f9',
      magenta: '#ff79c6',
      cyan: '#8be9fd',
      white: '#f8f8f2',
      brightBlack: '#6272a4',
      brightRed: '#ff6e6e',
      brightGreen: '#69ff94',
      brightYellow: '#ffffa5',
      brightBlue: '#d6acff',
      brightMagenta: '#ff92df',
      brightCyan: '#a4ffff',
      brightWhite: '#ffffff',
    },
  },
  // gruvbox dark, medium contrast.
  'gruvbox-dark': {
    light: false,
    colors: {
      background: '#282828',
      foreground: '#ebdbb2',
      cursor: '#ebdbb2',
      cursorAccent: '#282828',
      selectionBackground: '#504945',
      black: '#282828',
      red: '#cc241d',
      green: '#98971a',
      yellow: '#d79921',
      blue: '#458588',
      magenta: '#b16286',
      cyan: '#689d6a',
      white: '#a89984',
      brightBlack: '#928374',
      brightRed: '#fb4934',
      brightGreen: '#b8bb26',
      brightYellow: '#fabd2f',
      brightBlue: '#83a598',
      brightMagenta: '#d3869b',
      brightCyan: '#8ec07c',
      brightWhite: '#ebdbb2',
    },
  },
  // nordtheme.com.
  nord: {
    light: false,
    colors: {
      background: '#2e3440',
      foreground: '#d8dee9',
      cursor: '#d8dee9',
      cursorAccent: '#2e3440',
      selectionBackground: '#434c5e',
      black: '#3b4252',
      red: '#bf616a',
      green: '#a3be8c',
      yellow: '#ebcb8b',
      blue: '#81a1c1',
      magenta: '#b48ead',
      cyan: '#88c0d0',
      white: '#e5e9f0',
      brightBlack: '#4c566a',
      brightRed: '#bf616a',
      brightGreen: '#a3be8c',
      brightYellow: '#ebcb8b',
      brightBlue: '#81a1c1',
      brightMagenta: '#b48ead',
      brightCyan: '#8fbcbb',
      brightWhite: '#eceff4',
    },
  },
}

export const themeNames = Object.keys(themes)

/** Display names (proper nouns: not translated). */
export const themeLabels: Record<string, string> = {
  'agenterm-dark': 'agenterm Dark',
  'agenterm-light': 'agenterm Light',
  'solarized-dark': 'Solarized Dark',
  'solarized-light': 'Solarized Light',
  dracula: 'Dracula',
  'gruvbox-dark': 'Gruvbox Dark',
  nord: 'Nord',
}

export function themeOf(name: string | undefined): TermTheme {
  return themes[name ?? ''] ?? themes[DEFAULT_THEME]
}

/** settings.terminal as the app uses it (defaults mirror internal/settings). */
export interface TermPrefs {
  fontFamily: string
  fontSize: number
  lineHeight: number
  cursorStyle: 'block' | 'bar' | 'underline'
  theme: string
}

export const DEFAULT_FONT_SIZE = 13
export const MIN_FONT_SIZE = 8
export const MAX_FONT_SIZE = 32

export const defaultPrefs: TermPrefs = {
  fontFamily: '"SF Mono", Menlo, Monaco, monospace',
  fontSize: DEFAULT_FONT_SIZE,
  lineHeight: 1.15,
  cursorStyle: 'block',
  theme: DEFAULT_THEME,
}

/** settings.terminal as received (any string for the cursor style). */
export type TermPrefsInput = Partial<Omit<TermPrefs, 'cursorStyle'> & { cursorStyle: string }>

/** Fills in and clamps what a (possibly older) daemon sent. */
export function normPrefs(p: TermPrefsInput | undefined | null): TermPrefs {
  const size = Math.round(Number(p?.fontSize) || DEFAULT_FONT_SIZE)
  const lh = Number(p?.lineHeight) || defaultPrefs.lineHeight
  const cursor = p?.cursorStyle === 'bar' || p?.cursorStyle === 'underline' ? p.cursorStyle : 'block'
  return {
    fontFamily: p?.fontFamily?.trim() || defaultPrefs.fontFamily,
    fontSize: Math.min(Math.max(size, MIN_FONT_SIZE), MAX_FONT_SIZE),
    lineHeight: Math.min(Math.max(lh, 1), 2),
    cursorStyle: cursor,
    theme: themes[p?.theme ?? ''] ? p!.theme! : DEFAULT_THEME,
  }
}
