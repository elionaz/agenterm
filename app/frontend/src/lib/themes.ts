// Built-in terminal palettes (settings terminal.theme). Keep the names in
// sync with internal/settings Themes.
import type { ITheme } from '@xterm/xterm'

export interface TermTheme {
  /** Light background: search highlights and the preview adapt. */
  light: boolean
  colors: ITheme
}

export const DEFAULT_THEME = 'bright-lights'

/** Retired names and their replacements (mirrors internal/settings ThemeAliases). */
export const themeAliases: Record<string, string> = { 'agenterm-dark': 'bright-lights', 'agenterm-light': 'alphabetical' }

export const themes: Record<string, TermTheme> = {
  // Turn On the Bright Lights (Interpol, 2002): near black with a deep red accent. The default.
  'bright-lights': {
    light: false,
    colors: {
      background: '#111214',
      foreground: '#d8d9dc',
      cursor: '#e24a3f',
      cursorAccent: '#111214',
      selectionBackground: '#3b2a2c',
      black: '#1c1d20',
      red: '#e24a3f',
      green: '#8fc07f',
      yellow: '#e1b962',
      blue: '#71a0d9',
      magenta: '#c381ca',
      cyan: '#61b4b9',
      white: '#c5c6ca',
      brightBlack: '#62646b',
      brightRed: '#f06c61',
      brightGreen: '#a7d196',
      brightYellow: '#edcb82',
      brightBlue: '#8eb7e7',
      brightMagenta: '#d59bda',
      brightCyan: '#7ec8cc',
      brightWhite: '#ffffff',
    },
  },
  // Room on Fire (The Strokes, 2003): reds, pinks and oranges over dark wine.
  'room-on-fire': {
    light: false,
    colors: {
      background: '#1a1115',
      foreground: '#eddcd5',
      cursor: '#ff5c70',
      cursorAccent: '#1a1115',
      selectionBackground: '#4b2632',
      black: '#2a1b21',
      red: '#ff5c70',
      green: '#a3c98d',
      yellow: '#f5b468',
      blue: '#86a8e8',
      magenta: '#ea7cbc',
      cyan: '#72c4c0',
      white: '#d9c7c1',
      brightBlack: '#7a5f69',
      brightRed: '#ff7e8e',
      brightGreen: '#b8d9a3',
      brightYellow: '#f9c98c',
      brightBlue: '#a1bcf2',
      brightMagenta: '#f39dcf',
      brightCyan: '#93d6d3',
      brightWhite: '#fff4f0',
    },
  },
  // It Still Moves (My Morning Jacket, 2003): sepia paper, ochres and browns.
  'it-still-moves': {
    light: true,
    colors: {
      background: '#f3e9d6',
      foreground: '#3a2e21',
      cursor: '#9a5a1c',
      cursorAccent: '#f3e9d6',
      selectionBackground: '#e0cdaa',
      black: '#3a2e21',
      red: '#a8321f',
      green: '#4a7426',
      yellow: '#8a5e00',
      blue: '#2c5b8a',
      magenta: '#83386c',
      cyan: '#236d69',
      white: '#a3916f',
      brightBlack: '#7d6d56',
      brightRed: '#bf4a34',
      brightGreen: '#5c8a33',
      brightYellow: '#a07214',
      brightBlue: '#3f6fa0',
      brightMagenta: '#9a5084',
      brightCyan: '#33827d',
      brightWhite: '#fffaf0',
    },
  },
  // Alphabetical (Phoenix, 2004): warm cream with soft, bright colours.
  'alphabetical': {
    light: true,
    colors: {
      background: '#fbf7ef',
      foreground: '#2d2933',
      cursor: '#cf4679',
      cursorAccent: '#fbf7ef',
      selectionBackground: '#e8ddcd',
      black: '#2d2933',
      red: '#c3334b',
      green: '#3a8449',
      yellow: '#946b00',
      blue: '#305fb0',
      magenta: '#a04195',
      cyan: '#1c7686',
      white: '#a69e93',
      brightBlack: '#857e8a',
      brightRed: '#d64c62',
      brightGreen: '#4c975b',
      brightYellow: '#ad8210',
      brightBlue: '#4a78c8',
      brightMagenta: '#b858ab',
      brightCyan: '#2e8b9b',
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
  'bright-lights': 'Turn On the Bright Lights',
  'room-on-fire': 'Room on Fire',
  'it-still-moves': 'It Still Moves',
  alphabetical: 'Alphabetical',
  'solarized-dark': 'Solarized Dark',
  'solarized-light': 'Solarized Light',
  dracula: 'Dracula',
  'gruvbox-dark': 'Gruvbox Dark',
  nord: 'Nord',
}

/** Who made the album a theme is named after (shown under its name). */
export const themeCredits: Record<string, string> = {
  'bright-lights': 'Interpol · 2002',
  'room-on-fire': 'The Strokes · 2003',
  'it-still-moves': 'My Morning Jacket · 2003',
  alphabetical: 'Phoenix · 2004',
}

/** The built-in name for a setting value: retired names map to their
 * replacement, unknown ones to the default. */
export function themeName(name: string | undefined): string {
  const n = (name ?? '').trim().toLowerCase()
  const real = themeAliases[n] ?? n
  return themes[real] ? real : DEFAULT_THEME
}

export function themeOf(name: string | undefined): TermTheme {
  return themes[themeName(name)]
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
    theme: themeName(p?.theme),
  }
}
