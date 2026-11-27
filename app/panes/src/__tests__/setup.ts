import '@testing-library/jest-dom'
import 'fake-indexeddb/auto'
import { enableMapSet } from 'immer'
import { afterEach, beforeEach, vi } from 'vitest'
import { IdbViewedOwner } from '@/features/git/lib/idb-viewed-owner'
import { installViewedOwner } from '@/features/git/lib/viewed-owner'
import { installAppEditor } from '@/features/editor/host/app-editor'

enableMapSet()

installAppEditor()
beforeEach(() => {
  installViewedOwner(new IdbViewedOwner())
  installAppEditor()
})

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

delete (globalThis as { Worker?: unknown }).Worker

window.HTMLElement.prototype.scrollIntoView = () => {}
Range.prototype.getBoundingClientRect = () => DOMRect.fromRect()
Range.prototype.getClientRects = () => [] as unknown as DOMRectList

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

if (typeof globalThis.DataTransfer === 'undefined') {
  class DataTransferStub {
    dropEffect = 'none'
    effectAllowed = 'uninitialized'
    files = [] as unknown as FileList
    items = [] as unknown as DataTransferItemList
    #data = new Map<string, string>()
    get types(): readonly string[] {
      return Array.from(this.#data.keys())
    }
    clearData(format?: string) {
      if (format) this.#data.delete(format)
      else this.#data.clear()
    }
    getData(format: string) {
      return this.#data.get(format) ?? ''
    }
    setData(format: string, value: string) {
      this.#data.set(format, value)
    }
    setDragImage() {}
  }
  Object.defineProperty(globalThis, 'DataTransfer', {
    value: DataTransferStub,
    writable: true,
    configurable: true,
  })
}

window.Element.prototype.getAnimations = () => []

HTMLCanvasElement.prototype.getContext = function () {
  return {
    font: '',
    fillStyle: '',
    textBaseline: 'alphabetic',
    measureText: (text: string) => ({ width: text.length * 8 }),
    fillText: () => {},
    clearRect: () => {},
    fillRect: () => {},
    setTransform: () => {},
  } as unknown as CanvasRenderingContext2D
} as unknown as typeof HTMLCanvasElement.prototype.getContext

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

if (typeof CSSStyleSheet !== 'undefined') {
  const proto = CSSStyleSheet.prototype as CSSStyleSheet & {
    replaceSync?: (text: string) => void
    replace?: (text: string) => Promise<CSSStyleSheet>
    __cssText?: string
  }
  if (typeof proto.replaceSync !== 'function') {
    proto.replaceSync = function replaceSync(this: typeof proto, text: string) {
      this.__cssText = text
    }
  }
  if (typeof proto.replace !== 'function') {
    proto.replace = function replace(this: typeof proto, text: string) {
      this.__cssText = text
      return Promise.resolve(this as unknown as CSSStyleSheet)
    }
  }
}
if (typeof Document !== 'undefined' && !('adoptedStyleSheets' in Document.prototype)) {
  Object.defineProperty(Document.prototype, 'adoptedStyleSheets', {
    get() {
      return this.__adopted ?? []
    },
    set(v) {
      this.__adopted = v
    },
    configurable: true,
  })
}
if (typeof ShadowRoot !== 'undefined' && !('adoptedStyleSheets' in ShadowRoot.prototype)) {
  Object.defineProperty(ShadowRoot.prototype, 'adoptedStyleSheets', {
    get() {
      return this.__adopted ?? []
    },
    set(v) {
      this.__adopted = v
    },
    configurable: true,
  })
}

if (typeof globalThis.ClipboardItem === 'undefined') {
  class ClipboardItemStub {
    readonly #data: Record<string, Promise<unknown>>
    constructor(data: Record<string, unknown>) {
      this.#data = {}
      for (const [type, value] of Object.entries(data)) {
        const settled = Promise.resolve(value)
        settled.then(undefined, () => undefined)
        this.#data[type] = settled
      }
    }
    get types(): string[] {
      return Object.keys(this.#data)
    }
    async getType(type: string): Promise<Blob> {
      const value = await this.#data[type]
      return value instanceof Blob ? value : new Blob([String(value)], { type })
    }
  }
  globalThis.ClipboardItem = ClipboardItemStub as unknown as typeof ClipboardItem
}
if (typeof navigator !== 'undefined' && !navigator.clipboard) {
  let items: ClipboardItem[] = []
  const clipboard = {
    write: async (next: ClipboardItem[]) => void (items = next),
    read: async () => items,
    writeText: async (text: string) => void (items = [new ClipboardItem({ 'text/plain': text })]),
    readText: async () => (items[0] ? (await items[0].getType('text/plain')).text() : ''),
  }
  Object.defineProperty(navigator, 'clipboard', {
    value: clipboard,
    configurable: true,
    writable: true,
  })
}

const setupManager = vi.hoisted(() => ({ current: null as unknown }))
vi.mock('@/core/stream/ws-manager', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/core/stream/ws-manager')>()
  const { streamDaemonFor } = await import('@/__tests__/__fixtures__/stream-daemon')
  const { splitHostPath } = await import('@/lib/host-path')
  const real = actual.wsManager
  const daemonOf = (endpoint: string) => {
    const { hostId, path } = splitHostPath(endpoint)
    return path === '/v0/stream' ? streamDaemonFor(hostId) : null
  }
  const wsManager: typeof real = {
    subscribe: (endpoint, cb) => {
      const daemon = daemonOf(endpoint)
      if (!daemon) return real.subscribe(endpoint, cb)
      daemon.deliver = cb
      queueMicrotask(() => cb({ t: 'ready', epoch: 'test' }))
      return () => {
        if (daemon.deliver === cb) daemon.deliver = () => {}
      }
    },
    send: (endpoint, data) => {
      const daemon = daemonOf(endpoint)
      if (!daemon) return real.send(endpoint, data)
      const req = data as { op: string; scope: string }
      daemon.sent.push(req)
      const frames = daemon.answer(req)
      queueMicrotask(() => frames.forEach((f) => daemon.deliver(f)))
    },
    opened: (endpoint) => (daemonOf(endpoint) ? Promise.resolve() : real.opened(endpoint)),
    onDrop: (endpoint, listener) => {
      const daemon = daemonOf(endpoint)
      if (!daemon) return real.onDrop(endpoint, listener)
      daemon.drop = listener
      return () => {}
    },
  }
  setupManager.current = wsManager
  return { ...actual, wsManager }
})

vi.mock('@/core/stream/stream', async (importOriginal) => {
  const stream = await importOriginal<typeof import('@/core/stream/stream')>()
  const { wsManager } = await import('@/core/stream/ws-manager')
  if (wsManager === setupManager.current) stream.startStream()
  return stream
})

afterEach(async () => {
  const { resetStreamDaemon } = await import('@/__tests__/__fixtures__/stream-daemon')
  resetStreamDaemon()
})
