// Files dropped on a terminal (Finder, screenshots…).

/** Escapes a path the way Terminal.app does when a file is dropped on it:
 * a backslash before every character a shell would treat specially. Agents
 * read it back as a path (Claude Code attaches dropped images). */
export function quotePath(p: string): string {
  return p.replace(/[^\w\-./,:@+=%^\u0080-￿]/g, '\\$&')
}

/** The text a drop types: the paths, escaped and separated by spaces, with a
 * trailing space so the next word does not stick to the last one. Empty when
 * nothing usable was dropped (Wails also reports drags that carry no files,
 * like reordering the sidebar, as a drop with no paths). */
export function dropText(paths: readonly string[]): string {
  const ok = paths.filter((p) => p.trim() !== '')
  return ok.length ? ok.map(quotePath).join(' ') + ' ' : ''
}
