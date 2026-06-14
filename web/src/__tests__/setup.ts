import '@testing-library/jest-dom'
import 'fake-indexeddb/auto'

// Node 25 exposes a native localStorage that lacks .clear()/.removeItem() etc.
// Replace it with a proper in-memory implementation so tests can use the full Web Storage API.
function makeLocalStorage() {
  let store: Record<string, string> = {}
  return {
    getItem: (key: string) => store[key] ?? null,
    setItem: (key: string, value: string) => {
      store[key] = String(value)
    },
    removeItem: (key: string) => {
      delete store[key]
    },
    clear: () => {
      store = {}
    },
    key: (index: number) => Object.keys(store)[index] ?? null,
    get length() {
      return Object.keys(store).length
    },
  }
}

Object.defineProperty(globalThis, 'localStorage', {
  value: makeLocalStorage(),
  writable: true,
  configurable: true,
})

// jsdom does not implement scrollIntoView
window.HTMLElement.prototype.scrollIntoView = () => {}

// jsdom does not implement canvas getContext("2d") — provide a minimal stub so
// tests that indirectly call canvas text-measurement code don't crash.
HTMLCanvasElement.prototype.getContext = function () {
  return {
    font: '',
    measureText: (text: string) => ({ width: text.length * 8 }),
    fillText: () => {},
    clearRect: () => {},
    fillRect: () => {},
  } as unknown as CanvasRenderingContext2D
} as unknown as typeof HTMLCanvasElement.prototype.getContext

// jsdom does not implement the (deprecated) document.queryCommand* / execCommand
// APIs. Monaco's clipboard contribution calls document.queryCommandSupported at
// import time; the resulting throw prevents the ENTIRE monaco module — and any
// test file that imports an editor module — from loading under jsdom. Stubbing
// them lets those suites load and run (e.g. editor-api, pane-*, workspace-store).
if (typeof document !== 'undefined') {
  const doc = document as Document & {
    queryCommandSupported?: (commandId: string) => boolean
    queryCommandValue?: (commandId: string) => string
    execCommand?: (commandId: string, showUI?: boolean, value?: string) => boolean
  }
  if (typeof doc.queryCommandSupported !== 'function') doc.queryCommandSupported = () => false
  if (typeof doc.queryCommandValue !== 'function') doc.queryCommandValue = () => ''
  if (typeof doc.execCommand !== 'function') doc.execCommand = () => false
}
