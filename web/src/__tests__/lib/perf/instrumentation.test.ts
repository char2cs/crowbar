import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import {
  markStart,
  markEnd,
  installPerfObserver,
  perfEnabled,
  pushPerfEntry,
  __resetPerfForTests,
} from '@/lib/perf/instrumentation'

describe('perf instrumentation', () => {
  beforeEach(() => {
    __resetPerfForTests()
  })

  // Runs even when a test body throws mid-way, so a failing test cannot
  // leak its DEV stub or arming flag into the tests after it.
  afterEach(() => {
    vi.unstubAllEnvs()
    delete (window as { __CROWBAR_PERF__?: boolean }).__CROWBAR_PERF__
  })

  it('is disabled in prod without the arming flag and no-ops', () => {
    vi.stubEnv('DEV', false)
    delete (window as { __CROWBAR_PERF__?: boolean }).__CROWBAR_PERF__
    expect(perfEnabled()).toBe(false)
    markStart('x')
    markEnd('x') // must not throw and must not create entries
    expect(performance.getEntriesByName('x', 'measure')).toHaveLength(0)
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
    // Real signal, no fixed-duration sleeps: a probe observer registered
    // AFTER the module's (observers fire in registration order within one
    // dispatch, so the module's mirror has already run) resolves once every
    // entry has been delivered.
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
    // Real entries through the real PerformanceObserver callback — the
    // module's own trim logic must do the capping, not the test.
    for (let i = 0; i < 510; i++) {
      performance.measure(`m${i}`)
    }
    await delivered
    const log = (window as unknown as { __perfLog: Array<{ name: string }> }).__perfLog
    expect(log).toHaveLength(500)
    expect(log[0]!.name).toBe('m10') // m0..m9 were dropped, oldest first
    expect(log[499]!.name).toBe('m509')
    // Mirrored measures are also cleared from the native timeline so it
    // doesn't grow unbounded for the page lifetime.
    expect(performance.getEntriesByType('measure')).toHaveLength(0)
  })

  it('markEnd without markStart is a safe no-op', () => {
    ;(window as { __CROWBAR_PERF__?: boolean }).__CROWBAR_PERF__ = true
    expect(() => markEnd('never-started')).not.toThrow()
  })

  // Regression: the observer's async mark-cleanup used to clear a still-OPEN
  // span's `:start` mark, then markEnd threw `No mark named 'x:start' exists`.
  // This is exactly the terminal.echo path — a second keystroke opens a new echo
  // span before the previous span's write callback (which calls markEnd) has run,
  // and the observer drains the first span in between — and the throw escaped
  // inside xterm's write() callback as an unhandled error. markEnd must be a real
  // no-op when its start mark was cleared out from under it, never a throw.
  it('markEnd does not throw when the observer cleared an in-flight span start mark', async () => {
    ;(window as { __CROWBAR_PERF__?: boolean }).__CROWBAR_PERF__ = true
    installPerfObserver()

    // Register a probe FIRST so it receives span A's measure. Observers fire in
    // registration order within one dispatch, so the module observer (registered
    // in installPerfObserver, before this probe) has already run its mark-cleanup
    // by the time the probe resolves — the exact moment the buggy code wiped the
    // open span's start mark.
    const drained = new Promise<void>((resolve) => {
      const probe = new PerformanceObserver((list) => {
        if (list.getEntries().some((e) => e.name === 'terminal.echo')) {
          probe.disconnect()
          resolve()
        }
      })
      probe.observe({ entryTypes: ['measure'] })
    })

    // Span A: opened and closed. Its measure is queued to the observers, whose
    // async cleanup will clear the `terminal.echo:*` marks.
    markStart('terminal.echo')
    markEnd('terminal.echo')

    // Span B: opened SYNCHRONOUSLY (before the async observer dispatch) — a second
    // keystroke's echo. Its `:start` mark shares the name the observer clears.
    markStart('terminal.echo')

    await drained

    // Closing span B must never throw: the observer must not have clawed back
    // its start mark, and markEnd must tolerate a missing one regardless.
    expect(() => markEnd('terminal.echo')).not.toThrow()
  })
})

// The 500-entry __perfLog ring floods with Event Timing entries within seconds
// of real interaction and evicts the `measure` spans an external reader (a perf
// capture run) actually wants. __measures is a second ring that only ever
// receives measures, sized for a whole scenario rather than a few seconds.
describe('measure-only ring', () => {
  beforeEach(() => {
    __resetPerfForTests()
    // pushPerfEntry and the observer both self-gate on perfEnabled(); arm
    // explicitly rather than relying on the runner's import.meta.env.DEV.
    ;(window as { __CROWBAR_PERF__?: boolean }).__CROWBAR_PERF__ = true
  })

  afterEach(() => {
    vi.unstubAllEnvs()
    delete (window as { __CROWBAR_PERF__?: boolean }).__CROWBAR_PERF__
  })

  // Resolves once the module's observer has mirrored `name`. Registered AFTER
  // installPerfObserver so it fires second within one dispatch — no polling.
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

    // The flood evicts the span from __perfLog — that is the bug this ring exists
    // to survive — while __measures still holds it.
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
