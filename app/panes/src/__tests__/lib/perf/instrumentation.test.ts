import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'

type Instrumentation = typeof import('@/lib/perf/instrumentation')

let markStart: Instrumentation['markStart']
let markEnd: Instrumentation['markEnd']
let installPerfObserver: Instrumentation['installPerfObserver']
let perfEnabled: Instrumentation['perfEnabled']
let pushPerfEntry: Instrumentation['pushPerfEntry']

const observers: PerformanceObserver[] = []

async function loadFreshInstrumentation(): Promise<void> {
  const RealObserver = PerformanceObserver
  vi.stubGlobal(
    'PerformanceObserver',
    class extends RealObserver {
      constructor(callback: PerformanceObserverCallback) {
        super(callback)
        observers.push(this)
      }
    },
  )
  vi.resetModules()
  ;({ markStart, markEnd, installPerfObserver, perfEnabled, pushPerfEntry } =
    await import('@/lib/perf/instrumentation'))
  delete window.__perfLog
  delete window.__measures
  performance.clearMarks()
  performance.clearMeasures()
}

function disconnectObservers(): void {
  for (const observer of observers.splice(0)) observer.disconnect()
  vi.unstubAllGlobals()
}

describe('perf instrumentation', () => {
  beforeEach(loadFreshInstrumentation)

  afterEach(() => {
    disconnectObservers()
    vi.unstubAllEnvs()
    delete (window as { __CROWBAR_PERF__?: boolean }).__CROWBAR_PERF__
  })

  it('is disabled in prod without the arming flag and no-ops', () => {
    vi.stubEnv('DEV', false)
    delete (window as { __CROWBAR_PERF__?: boolean }).__CROWBAR_PERF__
    expect(perfEnabled()).toBe(false)
    markStart('x')
    markEnd('x')
    expect(performance.getEntriesByName('x', 'measure')).toHaveLength(0)
    vi.unstubAllEnvs()
  })

  it("localStorage 'crowbar:perf' arms a packaged build, so a launch span is not missed", () => {
    vi.stubEnv('DEV', false)
    localStorage.setItem('crowbar:perf', '1')
    try {
      expect(perfEnabled()).toBe(true)
    } finally {
      localStorage.removeItem('crowbar:perf')
    }
    expect(perfEnabled()).toBe(false)
    vi.unstubAllEnvs()
  })

  it('__CROWBAR_PERF__ alone arms perf when DEV is false (packaged app)', () => {
    vi.stubEnv('DEV', false)
    ;(window as { __CROWBAR_PERF__?: boolean }).__CROWBAR_PERF__ = true
    expect(perfEnabled()).toBe(true)
    markStart('packaged.span')
    markEnd('packaged.span')
    expect(performance.getEntriesByName('packaged.span', 'measure')).toHaveLength(1)
    vi.unstubAllEnvs()
  })

  it('records a measure between markStart/markEnd when armed', () => {
    ;(window as { __CROWBAR_PERF__?: boolean }).__CROWBAR_PERF__ = true
    markStart('diff.open')
    markEnd('diff.open')
    const entries = performance.getEntriesByName('diff.open', 'measure')
    expect(entries).toHaveLength(1)
  })

  it('ring buffer caps at 500 entries, dropping oldest', async () => {
    ;(window as { __CROWBAR_PERF__?: boolean }).__CROWBAR_PERF__ = true
    installPerfObserver()
    const delivered = new Promise<void>((resolve) => {
      let seen = 0
      const probe = new PerformanceObserver((list) => {
        seen += list.getEntries().length
        if (seen >= 510) {
          probe.disconnect()
          resolve()
        }
      })
      probe.observe({ entryTypes: ['measure'] })
    })
    for (let i = 0; i < 510; i++) {
      performance.measure(`m${i}`)
    }
    await delivered
    const log = (window as unknown as { __perfLog: Array<{ name: string }> }).__perfLog
    expect(log).toHaveLength(500)
    expect(log[0]!.name).toBe('m10')
    expect(log[499]!.name).toBe('m509')
    expect(performance.getEntriesByType('measure')).toHaveLength(0)
  })

  it('markEnd without markStart is a safe no-op', () => {
    ;(window as { __CROWBAR_PERF__?: boolean }).__CROWBAR_PERF__ = true
    expect(() => markEnd('never-started')).not.toThrow()
  })

  it('markEnd does not throw when the observer cleared an in-flight span start mark', async () => {
    ;(window as { __CROWBAR_PERF__?: boolean }).__CROWBAR_PERF__ = true
    installPerfObserver()

    const drained = new Promise<void>((resolve) => {
      const probe = new PerformanceObserver((list) => {
        if (list.getEntries().some((e) => e.name === 'terminal.echo')) {
          probe.disconnect()
          resolve()
        }
      })
      probe.observe({ entryTypes: ['measure'] })
    })

    markStart('terminal.echo')
    markEnd('terminal.echo')

    markStart('terminal.echo')

    await drained

    expect(() => markEnd('terminal.echo')).not.toThrow()
  })
})

describe('measure-only ring', () => {
  beforeEach(async () => {
    await loadFreshInstrumentation()
    ;(window as { __CROWBAR_PERF__?: boolean }).__CROWBAR_PERF__ = true
  })

  afterEach(() => {
    disconnectObservers()
    vi.unstubAllEnvs()
    delete (window as { __CROWBAR_PERF__?: boolean }).__CROWBAR_PERF__
  })

  function drained(name: string): Promise<void> {
    return new Promise<void>((resolve) => {
      const probe = new PerformanceObserver((list) => {
        if (list.getEntries().some((e) => e.name === name)) {
          probe.disconnect()
          resolve()
        }
      })
      probe.observe({ entryTypes: ['measure'] })
    })
  }

  it('mirrors measures into window.__measures separately from __perfLog', async () => {
    installPerfObserver()
    const mirrored = drained('span.a')

    markStart('span.a')
    markEnd('span.a')
    await mirrored

    expect(window.__measures?.some((e) => e.name === 'span.a')).toBe(true)
    expect(window.__perfLog?.some((e) => e.name === 'span.a')).toBe(true)
  })

  it('keeps measures when __perfLog is flooded by non-measure entries', async () => {
    installPerfObserver()
    const mirrored = drained('span.keeper')

    markStart('span.keeper')
    markEnd('span.keeper')
    await mirrored

    for (let i = 0; i < 1000; i++) {
      pushPerfEntry({ name: `event:${i}`, startTime: i, duration: 1, entryType: 'event' })
    }

    expect(window.__perfLog?.some((e) => e.name === 'span.keeper')).toBe(false)
    expect(window.__measures?.some((e) => e.name === 'span.keeper')).toBe(true)
  })

  it('pushPerfEntry respects the __perfLog ring cap', () => {
    installPerfObserver()
    for (let i = 0; i < 2000; i++) {
      pushPerfEntry({ name: 'INP:good', startTime: i, duration: 1, entryType: 'event' })
    }
    expect(window.__perfLog!.length).toBeLessThanOrEqual(500)
  })
})
