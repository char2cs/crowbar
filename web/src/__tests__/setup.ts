import '@testing-library/jest-dom'

// Node 25 exposes a native localStorage that lacks .clear()/.removeItem() etc.
// Replace it with a proper in-memory implementation so tests can use the full Web Storage API.
function makeLocalStorage() {
  let store: Record<string, string> = {}
  return {
    getItem: (key: string) => store[key] ?? null,
    setItem: (key: string, value: string) => { store[key] = String(value) },
    removeItem: (key: string) => { delete store[key] },
    clear: () => { store = {} },
    key: (index: number) => Object.keys(store)[index] ?? null,
    get length() { return Object.keys(store).length },
  }
}

Object.defineProperty(globalThis, 'localStorage', {
  value: makeLocalStorage(),
  writable: true,
  configurable: true,
})
