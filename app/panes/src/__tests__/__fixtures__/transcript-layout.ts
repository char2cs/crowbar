import { vi } from 'vitest'

// jsdom measures every element 0×0, and a zero-height viewport windows down to
// no rows at all; a pane-sized rect makes the window the one the app produces.
export const rect = {
  top: 0,
  left: 0,
  right: 768,
  bottom: 800,
  width: 768,
  height: 800,
  x: 0,
  y: 0,
}

/** Hands the test every ResizeObserver callback by the element it watches. */
export function observeResizes() {
  const observed = new Map<Element, ResizeObserverCallback>()
  vi.stubGlobal(
    'ResizeObserver',
    class {
      constructor(private readonly callback: ResizeObserverCallback) {}
      observe(el: Element) {
        observed.set(el, this.callback)
      }
      unobserve() {}
      disconnect() {}
    },
  )
  return observed
}

/** jsdom lays nothing out: the list's metrics are the test's, and scrollTop clamps. */
export function layOut(el: HTMLElement, box: { scrollHeight: number; clientHeight: number }) {
  let top = 0
  Object.defineProperty(el, 'scrollTop', {
    configurable: true,
    get: () => top,
    set: (v: number) => {
      top = Math.max(0, Math.min(v, box.scrollHeight - box.clientHeight))
    },
  })
  Object.defineProperty(el, 'scrollHeight', { configurable: true, get: () => box.scrollHeight })
  Object.defineProperty(el, 'clientHeight', { configurable: true, get: () => box.clientHeight })
  // The virtualizer writes through scrollTo, which jsdom leaves out.
  el.scrollTo = ((options: ScrollToOptions) => {
    el.scrollTop = options.top ?? top
  }) as typeof el.scrollTo
}

/** Lays the list out from its rows: each row measures `row`, the list is the
 *  rows plus 200 of chrome, and a parked view measures 0×0. */
export function measureRows(opts: { row: number; viewport: number | (() => number) }) {
  const viewport = () => (typeof opts.viewport === 'number' ? opts.viewport : opts.viewport())
  const isList = (el: HTMLElement) => el.dataset.testid === 'agent-message-list'
  HTMLElement.prototype.getBoundingClientRect = function getBoundingClientRect() {
    const height = this.dataset.index ? opts.row : isList(this) ? viewport() : rect.height
    const r = { ...rect, height, bottom: height, width: height > 0 ? rect.width : 0 }
    return { ...r, toJSON: () => r } as DOMRect
  }
  const rowsHeight = (el: HTMLElement) =>
    parseFloat(el.querySelector<HTMLElement>('.virtual-rows')?.style.height ?? '0')
  const tops = new WeakMap<HTMLElement, number>()
  const metrics: PropertyDescriptorMap = {
    scrollHeight: {
      configurable: true,
      get(this: HTMLElement) {
        return isList(this) ? rowsHeight(this) + 200 : 0
      },
    },
    clientHeight: {
      configurable: true,
      get(this: HTMLElement) {
        return isList(this) ? viewport() : 0
      },
    },
    scrollTop: {
      configurable: true,
      get(this: HTMLElement) {
        return tops.get(this) ?? 0
      },
      set(this: HTMLElement, v: number) {
        tops.set(this, Math.max(0, Math.min(v, this.scrollHeight - this.clientHeight)))
      },
    },
  }
  Object.defineProperties(HTMLElement.prototype, metrics)
  return () => {
    for (const key of Object.keys(metrics)) Reflect.deleteProperty(HTMLElement.prototype, key)
  }
}
