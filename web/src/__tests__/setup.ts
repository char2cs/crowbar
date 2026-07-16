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

// jsdom does not implement ResizeObserver. Inert stub: jsdom has no layout engine, so
// it could never report a real size change anyway — components that observe elements
// just need construction not to throw.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
Object.defineProperty(globalThis, 'ResizeObserver', {
  value: ResizeObserverStub,
  writable: true,
  configurable: true,
})

// jsdom does not implement the Web Animations API, and this stub is load-bearing
// BECAUSE of the ResizeObserver one directly above.
//
// Base UI's ScrollAreaViewport opens with `if (typeof ResizeObserver === 'undefined')
// return`. While jsdom had no ResizeObserver, that guard short-circuited the whole
// effect. Stubbing ResizeObserver satisfied the guard and un-gated the rest of it —
// which schedules a 0ms timer that calls `viewport.getAnimations({subtree: true})`.
//
// That call throws AFTER the test that rendered the ScrollArea has already finished, so
// it surfaces as an unhandled error with no owning test and fails the ENTIRE run while
// every assertion still reports green. Stubbing one hole in jsdom exposed the next one.
//
// An empty list is the honest answer, not a lie to keep quiet: jsdom runs no animations,
// so there are none to wait for, and Base UI early-returns on `animations.length === 0`.
window.Element.prototype.getAnimations = () => []

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

// jsdom does not implement matchMedia. Monaco's StandaloneThemeService reads it
// (for the OS high-contrast-scheme listener) as soon as the standalone editor
// services / any basic-languages contribution's shared editor/contrib chain is
// evaluated — for some import paths that happens at MODULE IMPORT time, before
// any per-test shim assigned after an `import` statement would run (ES module
// imports are hoisted ahead of sibling top-level statements in the same file,
// so a same-file shim can lose this race). A global stub here runs before every
// test file's module graph loads. Inert: never matches, no listeners fire.
if (typeof window !== 'undefined' && typeof window.matchMedia !== 'function') {
  window.matchMedia = ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  })) as typeof window.matchMedia
}
