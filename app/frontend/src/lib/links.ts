// Terminal links (plain-text URLs and OSC 8 hyperlinks).

/** Opens a link on ⌘-click (a plain click stays a click for the program and
 * for selecting text), only for http(s) URLs. Returns whether it opened. */
export function openLink(event: { metaKey: boolean }, uri: string, open: (url: string) => void): boolean {
  if (!event.metaKey) return false
  let url: URL
  try {
    url = new URL(uri)
  } catch {
    return false
  }
  if (url.protocol !== 'http:' && url.protocol !== 'https:') return false
  open(url.href)
  return true
}
