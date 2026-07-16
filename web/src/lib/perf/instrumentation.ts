/**
 * Shared perf instrumentation. Zero-cost unless armed: enabled in dev, or in
 * any build when `window.__CROWBAR_PERF__` is truthy (set it from the console
 * or via webview_execute_js in the packaged app). All entries land in the
 * `window.__perfLog` ring buffer so Chrome traces (dev) and WKWebView
 * peek-back (packaged) read the same data. Spec §3.1.
 */
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
  }
}

const RING_CAP = 500
let observer: PerformanceObserver | null = null
const openMarks = new Set<string>()

export function perfEnabled(): boolean {
  return Boolean(import.meta.env.DEV || window.__CROWBAR_PERF__)
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
  // A missing `:start` mark must be a REAL no-op, never a throw. The observer
  // below clears a completed span's marks asynchronously, and clearMarks wipes
  // EVERY mark of a name at once — so a prior span's drain could clear the
  // `:start` of a span still open here (see the observer's guard, which now
  // prevents that). This try/catch is the belt to that braces: performance.measure
  // throws a SyntaxError when the start mark is absent, and this runs inside
  // xterm's write() parse-complete callback (use-terminal-connection), where an
  // uncaught throw escaped as an unhandled error and broke the write pipeline.
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
      // Mirrored into __perfLog — drop the native copies so the browser's
      // own performance timeline doesn't grow unbounded for the page
      // lifetime. Per-name only: other code's un-mirrored entries survive.
      if (e.entryType === 'measure') {
        performance.clearMeasures(e.name)
        performance.clearMarks(`${e.name}:end`)
        // clearMarks removes ALL marks of a name at once. A NEW span of the same
        // name can open (markStart) between a prior span's markEnd and this async
        // observer callback — coalesced keystroke echoes do exactly this. Clearing
        // `:start` unconditionally would then yank the live span's start mark out
        // from under it, and its markEnd would measure against a missing mark. Only
        // clear when no span of this name is currently open; an open span drains its
        // own `:start` when it closes (openMarks no longer has it by then).
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
  performance.clearMarks()
  performance.clearMeasures()
}
