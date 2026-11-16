// Checks that every locale has exactly en's keys, the same shape (plain vs
// plural) and the same {placeholders}. Run by `npm run check` with Node's
// built-in TypeScript type stripping (Node >= 22.18).
import { en } from '../src/lib/locales/en.ts'
import { es } from '../src/lib/locales/es.ts'

type Msg = string | { one: string; other: string }

const placeholders = (m: Msg): string =>
  [...new Set((typeof m === 'string' ? m : m.one + m.other).match(/\{\w+\}/g) ?? [])].sort().join(',')

const locales: Record<string, Record<string, Msg>> = { es }
const base: Record<string, Msg> = en
let errors = 0
const fail = (msg: string) => {
  console.error(`i18n: ${msg}`)
  errors++
}

for (const [name, dict] of Object.entries(locales)) {
  for (const key of Object.keys(base)) {
    if (!(key in dict)) {
      fail(`${name}: missing "${key}"`)
      continue
    }
    const a = base[key]
    const b = dict[key]
    if (typeof a !== typeof b) {
      fail(`${name}: "${key}" must be ${typeof a === 'string' ? 'a string' : 'a plural'}`)
      continue
    }
    // Plural "one" forms may spell the number out, so only compare the
    // union of both forms against the source.
    const pa = placeholders(a)
    const pb = placeholders(b)
    if (pa.replace('{n}', '') !== pb.replace('{n}', '')) fail(`${name}: "${key}" placeholders ${pb} != ${pa}`)
  }
  for (const key of Object.keys(dict)) if (!(key in base)) fail(`${name}: extra key "${key}"`)
}

if (errors) process.exit(1)
console.log(`i18n: ${Object.keys(base).length} keys, ${Object.keys(locales).length + 1} locales OK`)
