import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { scheduleIdleTask } from '@/features/editor/lib/idle-task'

/**
 * The deferred-LSP-open primitive. The load-bearing guarantee for LSP
 * `didOpen`/`didClose` lifecycle safety is that a CANCELLED task never runs —
 * so a rapid open→open→open swap sequence (each swap cancels the previous
 * pending open) ends with exactly one open document (the last).
 */
describe('scheduleIdleTask', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    // Force the setTimeout fallback path (deterministic under fake timers).
    vi.stubGlobal('requestIdleCallback', undefined)
    vi.stubGlobal('cancelIdleCallback', undefined)
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('runs the task on the next idle tick', () => {
    const task = vi.fn()
    scheduleIdleTask(task)
    expect(task).not.toHaveBeenCalled()
    vi.runAllTimers()
    expect(task).toHaveBeenCalledTimes(1)
  })

  it('does not run a cancelled task', () => {
    const task = vi.fn()
    const handle = scheduleIdleTask(task)
    handle.cancel()
    vi.runAllTimers()
    expect(task).not.toHaveBeenCalled()
  })

  it('cancel is idempotent and a no-op after the task fired', () => {
    const task = vi.fn()
    const handle = scheduleIdleTask(task)
    vi.runAllTimers()
    expect(task).toHaveBeenCalledTimes(1)
    handle.cancel()
    handle.cancel()
    expect(task).toHaveBeenCalledTimes(1)
  })

  it('rapid swap: cancelling pending opens leaves exactly one open (the last)', () => {
    // Simulate the LSP deferred-open lifecycle: each swap cancels the previous
    // pending open before scheduling its own. Only the final, un-cancelled open
    // should fire.
    const opens: string[] = []
    const open = (file: string) => scheduleIdleTask(() => opens.push(file))

    let pending = open('a.ts')
    pending.cancel() // swap to b
    pending = open('b.ts')
    pending.cancel() // swap to c
    pending = open('c.ts')
    // no further swap; c's open is allowed to fire

    vi.runAllTimers()
    expect(opens).toEqual(['c.ts'])
  })
})
