import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import {
  markStart,
  markEnd,
  installPerfObserver,
  perfEnabled,
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
