import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { scheduleIdleTask } from '@/features/editor/lib/idle-task'

describe('scheduleIdleTask', () => {
  beforeEach(() => {
    vi.useFakeTimers()
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
    const opens: string[] = []
    const open = (file: string) => scheduleIdleTask(() => opens.push(file))

    let pending = open('a.ts')
    pending.cancel()
    pending = open('b.ts')
    pending.cancel()
    open('c.ts')

    vi.runAllTimers()
    expect(opens).toEqual(['c.ts'])
  })
})
