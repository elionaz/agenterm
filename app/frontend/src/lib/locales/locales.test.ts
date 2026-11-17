// Every locale must have exactly en's keys, the same shape (plain vs plural)
// and the same {placeholders}.
import { describe, expect, it } from 'vitest'
import { en } from './en'
import { es } from './es'

type Msg = string | { one: string; other: string }

const placeholders = (m: Msg): string =>
  [...new Set((typeof m === 'string' ? m : m.one + m.other).match(/\{\w+\}/g) ?? [])].sort().join(',')

const base: Record<string, Msg> = en
const locales: Record<string, Record<string, Msg>> = { es }

describe.each(Object.entries(locales))('locale %s', (_name, dict) => {
  it('has the same keys as en', () => {
    expect(Object.keys(dict).sort()).toEqual(Object.keys(base).sort())
  })

  it('keeps the shape and placeholders of each message', () => {
    const errors: string[] = []
    for (const key of Object.keys(base)) {
      if (!(key in dict)) continue
      const a = base[key]
      const b = dict[key]
      if (typeof a !== typeof b) {
        errors.push(`"${key}" must be ${typeof a === 'string' ? 'a string' : 'a plural'}`)
        continue
      }
      // Plural "one" forms may spell the number out, so only compare the
      // union of both forms against the source.
      const pa = placeholders(a)
      const pb = placeholders(b)
      if (pa.replace('{n}', '') !== pb.replace('{n}', '')) errors.push(`"${key}" placeholders ${pb} != ${pa}`)
    }
    expect(errors).toEqual([])
  })
})
