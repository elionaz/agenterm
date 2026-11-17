// Unit checks for terminal links (src/lib/links.ts). Run by `npm run check`.
import assert from 'node:assert/strict'
import { openLink } from '../src/lib/links.ts'

const opened: string[] = []
const open = (u: string) => { opened.push(u) }
const cmd = { metaKey: true }

assert.equal(openLink({ metaKey: false }, 'https://example.com', open), false, 'plain click does not open')
assert.equal(openLink(cmd, 'https://example.com/a?b=1', open), true)
assert.equal(openLink(cmd, 'http://localhost:5173/', open), true)
for (const bad of ['javascript:alert(1)', 'file:///etc/passwd', 'agenterm://session/x', 'not a url', ''])
  assert.equal(openLink(cmd, bad, open), false, bad)
assert.deepEqual(opened, ['https://example.com/a?b=1', 'http://localhost:5173/'])
console.log('links: OK')
