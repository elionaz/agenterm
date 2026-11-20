// Before the rename to panal, the app's localStorage keys started with
// "agenterm." (the web view's storage survived the rename: the bundle id
// did not change). Copy each one to its "panal." key once, before any
// module reads its preferences; main.ts imports this module first.

const OLD = 'agenterm.'
const NEW = 'panal.'

export function migrateLegacyKeys(store: Storage | undefined = globalThis.localStorage) {
  if (!store) return
  try {
    const keys: string[] = []
    for (let i = 0; i < store.length; i++) {
      const k = store.key(i)
      if (k?.startsWith(OLD)) keys.push(k)
    }
    for (const k of keys) {
      const n = NEW + k.slice(OLD.length)
      const v = store.getItem(k)
      if (store.getItem(n) === null && v !== null) store.setItem(n, v)
      store.removeItem(k)
    }
  } catch {
    // Storage unavailable: nothing to migrate.
  }
}

migrateLegacyKeys()
