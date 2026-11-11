// Shared perf instrumentation, zero-cost unless armed (dev, or `window.__CROWBAR_PERF__` from the
// console / webview_execute_js). Entries land in the `window.__perfLog` ring, so Chrome traces and
// WKWebView peek-back read the same data (spec §3.1).
interface PerfLogEntry {
  name: string
  startTime: number
  duration: number
  entryType: string
}

declare global {
  interface Window {
    __CROWBAR_PERF__?: boolean
    __perfLog?: PerfLogEntry[]
    __measures?: PerfLogEntry[]
  }
}

const RING_CAP = 500
// A measure-only ring: Event Timing entries evict measures from __perfLog within seconds of real
// interaction, so external readers (perf capture runs) read this one, sized for a full scenario.
const MEASURE_RING_CAP = 2000
let observer: PerformanceObserver | null = null
const openMarks = new Set<string>()

/** `localStorage['crowbar:perf'] = '1'` arms a packaged build across reloads,
 *  which a launch span needs: the console flag comes too late for it. */
function armedByStorage(): boolean {
  try {
    return localStorage.getItem('crowbar:perf') === '1'
  } catch {
    return false
  }
}

export function perfEnabled(): boolean {
  return Boolean(import.meta.env.DEV || window.__CROWBAR_PERF__ || armedByStorage())
}

// Push into the shared perf ring, honouring the cap; exported so out-of-module producers (main.tsx's
// INP reporter) can't grow the ring without bound.
export function pushPerfEntry(entry: PerfLogEntry): void {
  if (!perfEnabled()) return
  window.__perfLog ??= []
  const log = window.__perfLog
  log.push(entry)
  if (log.length > RING_CAP) log.shift()
}

export function markStart(name: string): void {
  if (!perfEnabled()) return
  openMarks.add(name)
  performance.mark(`${name}:start`)
}

export function markEnd(name: string): void {
  if (!perfEnabled()) return
  if (!openMarks.has(name)) return
  openMarks.delete(name)
  performance.mark(`${name}:end`)
  // A missing `:start` mark must be a no-op, never a throw: performance.measure throws without it,
  // and this runs inside xterm's write() callback, where a throw breaks the write pipeline.
  try {
    performance.measure(name, `${name}:start`, `${name}:end`)
  } catch {
    // Start mark gone (raced away, or markEnd called without a live markStart) —
    // nothing to measure. Drop the orphaned end mark so it doesn't accumulate.
    performance.clearMarks(`${name}:end`)
  }
}

export function installPerfObserver(): void {
  if (!perfEnabled() || observer) return
  window.__perfLog ??= []
  window.__measures ??= []
  observer = new PerformanceObserver((list) => {
    const log = window.__perfLog!
    for (const e of list.getEntries()) {
      log.push({
        name: e.name,
        startTime: e.startTime,
        duration: e.duration,
        entryType: e.entryType,
      })
      if (log.length > RING_CAP) log.shift()
      if (e.entryType === 'measure') {
        // Second, measure-only ring (`__measures`): Event Timing noise never reaches it.
        const measures = window.__measures!
        measures.push({
          name: e.name,
          startTime: e.startTime,
          duration: e.duration,
          entryType: e.entryType,
        })
        if (measures.length > MEASURE_RING_CAP) measures.shift()
        // Mirrored into __perfLog — drop the native copies so the browser's
        // own performance timeline doesn't grow unbounded for the page
        // lifetime. Per-name only: other code's un-mirrored entries survive.
        performance.clearMeasures(e.name)
        performance.clearMarks(`${e.name}:end`)
        // clearMarks removes every mark of a name, and a new span of that name can open before this
        // async callback (coalesced keystroke echoes do). Clear `:start` only when no span of the
        // name is open; an open span drains its own `:start` when it closes.
        if (!openMarks.has(e.name)) performance.clearMarks(`${e.name}:start`)
      }
    }
  })
  // 'event' feeds Event Timing (input latency attribution); 'measure' feeds
  // our own spans. Both fail soft on runtimes lacking a type.
  try {
    observer.observe({ entryTypes: ['measure', 'event'] })
  } catch {
    observer.observe({ entryTypes: ['measure'] })
  }
}

/** Test-only: clears module state and performance entries. */
export function __resetPerfForTests(): void {
  observer?.disconnect()
  observer = null
  openMarks.clear()
  delete window.__perfLog
  delete window.__measures
  performance.clearMarks()
  performance.clearMeasures()
}
