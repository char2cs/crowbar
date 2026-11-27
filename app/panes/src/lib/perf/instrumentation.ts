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
const MEASURE_RING_CAP = 2000
let observer: PerformanceObserver | null = null
const openMarks = new Set<string>()

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
  try {
    performance.measure(name, `${name}:start`, `${name}:end`)
  } catch {
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
        const measures = window.__measures!
        measures.push({
          name: e.name,
          startTime: e.startTime,
          duration: e.duration,
          entryType: e.entryType,
        })
        if (measures.length > MEASURE_RING_CAP) measures.shift()
        performance.clearMeasures(e.name)
        performance.clearMarks(`${e.name}:end`)
        if (!openMarks.has(e.name)) performance.clearMarks(`${e.name}:start`)
      }
    }
  })
  try {
    observer.observe({ entryTypes: ['measure', 'event'] })
  } catch {
    observer.observe({ entryTypes: ['measure'] })
  }
}
