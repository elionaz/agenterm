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
export const themeAliases: Record<string, string> = {
  'panal-dark': 'bright-lights',
  'panal-light': 'alphabetical',
  'solarized-dark': 'z',
  'solarized-light': 'hot-fuss',
  dracula: 'funeral',
  'gruvbox-dark': 'is-this-it',
  nord: 'silent-alarm',
}

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
  // Is This It (The Strokes, 2001): skin and amber tones over black.
  'is-this-it': {
    light: false,
    colors: {
      background: '#15110e',
      foreground: '#ecdccb',
      cursor: '#e0a96d',
      cursorAccent: '#15110e',
      selectionBackground: '#3a2c22',
      black: '#241c17',
      red: '#e0675a',
      green: '#a2bf7c',
      yellow: '#e5b46a',
      blue: '#7ea3cf',
      magenta: '#cf8fae',
      cyan: '#78b7a8',
      white: '#d3c3b2',
      brightBlack: '#76665a',
      brightRed: '#ee857a',
      brightGreen: '#b8d197',
      brightYellow: '#f0c88a',
      brightBlue: '#9bbbe0',
      brightMagenta: '#dfa9c3',
      brightCyan: '#95cbbd',
      brightWhite: '#fff6ec',
    },
  },
  // Funeral (Arcade Fire, 2004): night blue and wine.
  'funeral': {
    light: false,
    colors: {
      background: '#0f1420',
      foreground: '#d9dcea',
      cursor: '#d4596e',
      cursorAccent: '#0f1420',
      selectionBackground: '#2c2438',
      black: '#1a2030',
      red: '#d4596e',
      green: '#8fbf8a',
      yellow: '#dcb66e',
      blue: '#6f9be0',
      magenta: '#b886d6',
      cyan: '#6cb8c8',
      white: '#c3c7d6',
      brightBlack: '#626a80',
      brightRed: '#e57a8c',
      brightGreen: '#a9d2a3',
      brightYellow: '#e9ca8e',
      brightBlue: '#8fb2ec',
      brightMagenta: '#cca2e3',
      brightCyan: '#8dcbd8',
      brightWhite: '#f4f5fb',
    },
  },
  // Silent Alarm (Bloc Party, 2005): cold blues and cyan.
  'silent-alarm': {
    light: false,
    colors: {
      background: '#0d1a24',
      foreground: '#d6e6ee',
      cursor: '#4fc3e0',
      cursorAccent: '#0d1a24',
      selectionBackground: '#1f3a4c',
      black: '#16283a',
      red: '#e26d6d',
      green: '#7fc79a',
      yellow: '#e3c57a',
      blue: '#5aa6e6',
      magenta: '#a993e0',
      cyan: '#4fc3e0',
      white: '#bfd2dc',
      brightBlack: '#5b7a8e',
      brightRed: '#ef8c8c',
      brightGreen: '#9bd8b0',
      brightYellow: '#eed699',
      brightBlue: '#7fbcef',
      brightMagenta: '#c0adec',
      brightCyan: '#79d4ea',
      brightWhite: '#f2fbff',
    },
  },
  // Z (My Morning Jacket, 2005): moss green and ochre.
  'z': {
    light: false,
    colors: {
      background: '#141812',
      foreground: '#dcdcc8',
      cursor: '#d3ac4e',
      cursorAccent: '#141812',
      selectionBackground: '#313a28',
      black: '#1f251b',
      red: '#d46a4f',
      green: '#9cbf62',
      yellow: '#d3ac4e',
      blue: '#7ea6c4',
      magenta: '#c08aa8',
      cyan: '#7ab9a2',
      white: '#c4c4ae',
      brightBlack: '#6a735e',
      brightRed: '#e48a70',
      brightGreen: '#b4d37e',
      brightYellow: '#e2c16e',
      brightBlue: '#9cbdd5',
      brightMagenta: '#d3a5be',
      brightCyan: '#97ccb8',
      brightWhite: '#f7f6e8',
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
      white: '#6b5c44',
      brightBlack: '#54473a',
      brightRed: '#bf4a34',
      brightGreen: '#5c8a33',
      brightYellow: '#a07214',
      brightBlue: '#3f6fa0',
      brightMagenta: '#9a5084',
      brightCyan: '#33827d',
      brightWhite: '#8a7a5c',
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
      white: '#6f6876',
      brightBlack: '#56505c',
      brightRed: '#d64c62',
      brightGreen: '#4c975b',
      brightYellow: '#ad8210',
      brightBlue: '#4a78c8',
      brightMagenta: '#b858ab',
      brightCyan: '#2e8b9b',
      brightWhite: '#8c8478',
    },
  },
  // Hot Fuss (The Killers, 2004): cream with pinks and blues.
  'hot-fuss': {
    light: true,
    colors: {
      background: '#fbf4ec',
      foreground: '#2a2433',
      cursor: '#c8376e',
      cursorAccent: '#fbf4ec',
      selectionBackground: '#f0dbe2',
      black: '#2a2433',
      red: '#c22f4d',
      green: '#2f7d4a',
      yellow: '#8f6400',
      blue: '#2b5cb3',
      magenta: '#b0337f',
      cyan: '#18738a',
      white: '#6e6672',
      brightBlack: '#56505c',
      brightRed: '#cf3f60',
      brightGreen: '#358a55',
      brightYellow: '#9c6e00',
      brightBlue: '#3a6cc2',
      brightMagenta: '#bb3f8a',
      brightCyan: '#1f8096',
      brightWhite: '#8f8580',
    },
  },
}

export const themeNames = Object.keys(themes)

/** Display names: the album titles (proper nouns, not translated). */
export const themeLabels: Record<string, string> = {
  'bright-lights': 'Turn On the Bright Lights',
  'room-on-fire': 'Room on Fire',
  'is-this-it': 'Is This It',
  funeral: 'Funeral',
  'silent-alarm': 'Silent Alarm',
  z: 'Z',
  'it-still-moves': 'It Still Moves',
  alphabetical: 'Alphabetical',
  'hot-fuss': 'Hot Fuss',
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
  fontFamily: '"SF Mono", Menlo, Monaco, "DejaVu Sans Mono", "Ubuntu Mono", "Liberation Mono", monospace',
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
