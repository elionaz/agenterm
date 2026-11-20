// A catalog agent's install hint ("npm i -g x   (or brew install y)") split
// into the command panal runs and the alternatives shown as text
// (mirrors profile.InstallCommand).
export function installCommand(hint: string | undefined): { cmd: string; alt: string } {
  const h = (hint ?? '').trim()
  const i = h.indexOf('(or ')
  if (i < 0) return { cmd: h, alt: '' }
  return { cmd: h.slice(0, i).trim(), alt: h.slice(i + 4).trim().replace(/\)$/, '').trim() }
}
