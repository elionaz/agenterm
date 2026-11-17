// Keys the terminal translates itself instead of leaving them to xterm.js.

type KeyLike = Pick<KeyboardEvent, 'key' | 'shiftKey' | 'metaKey' | 'ctrlKey' | 'altKey' | 'isComposing'>

/** Shift+Enter inserts a new line instead of submitting. xterm.js sends a
 * plain carriage return for it, the same as Enter, so agents like Claude Code
 * or Codex would submit the prompt. A line feed (Ctrl+J) is the newline key
 * both understand; in a shell it still runs the command, like Enter. */
export const SHIFT_ENTER = '\n'

export function isShiftEnter(ev: KeyLike): boolean {
  return ev.key === 'Enter' && ev.shiftKey && !ev.metaKey && !ev.ctrlKey && !ev.altKey && !ev.isComposing
}
