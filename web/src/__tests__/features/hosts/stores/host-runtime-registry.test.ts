import { describe, expect, it, vi } from 'vitest'
import {
  getOrCreateHostRuntime,
  getHostRuntime,
  destroyHostRuntime,
  subscribeHostStatus,
  whileHostUp,
} from '@/features/hosts/stores/host-runtime-registry'

describe('host runtime registry', () => {
  it('getOrCreateHostRuntime mints a store with status "unknown"', () => {
    const store = getOrCreateHostRuntime('h1')
    expect(store.getState().status).toBe('unknown')
    destroyHostRuntime('h1')
  })

  it('getOrCreateHostRuntime returns the SAME store on a second call', () => {
    const a = getOrCreateHostRuntime('h1')
    const b = getOrCreateHostRuntime('h1')
    expect(a).toBe(b)
    destroyHostRuntime('h1')
  })

  it('getHostRuntime returns undefined for an unregistered id, without minting one', () => {
    expect(getHostRuntime('never-created')).toBeUndefined()
  })

  it('destroyHostRuntime removes it', () => {
    getOrCreateHostRuntime('h1')
    destroyHostRuntime('h1')
    expect(getHostRuntime('h1')).toBeUndefined()
  })
})

describe('whileHostUp', () => {
  /** What `run` was asked to do, as `start h` / `stop h`. */
  function recorder() {
    const log: string[] = []
    const run = (hostId: string) => {
      log.push(`start ${hostId}`)
      return () => log.push(`stop ${hostId}`)
    }
    return { log, run }
  }

  it('runs for a host each time it becomes up, and stops it each time it leaves up', () => {
    const { log, run } = recorder()
    getOrCreateHostRuntime('h1').getState().setStatus('down')
    const stop = whileHostUp(run)
    const h1 = getHostRuntime('h1')!.getState()

    h1.setStatus('up', 12)
    h1.setStatus('up', 9)
    h1.setStatus('needs-repair')
    h1.setStatus('up', 5)

    expect(log).toEqual(['start h1', 'stop h1', 'start h1'])
    stop()
    expect(log).toEqual(['start h1', 'stop h1', 'start h1', 'stop h1'])
    destroyHostRuntime('h1')
  })

  it('starts at once for a host already up', () => {
    const { log, run } = recorder()
    getOrCreateHostRuntime('h1').getState().setStatus('up')

    const stop = whileHostUp(run)

    expect(log).toEqual(['start h1'])
    stop()
    destroyHostRuntime('h1')
  })

  it('follows hosts that appear after it starts, and stops those destroyed', () => {
    const { log, run } = recorder()
    const stop = whileHostUp(run)

    const h2 = getOrCreateHostRuntime('h2')
    h2.getState().setStatus('up')
    destroyHostRuntime('h2')
    h2.getState().setStatus('down')
    h2.getState().setStatus('up')

    expect(log).toEqual(['start h2', 'stop h2'])
    stop()
  })

  it('runs nothing once stopped', () => {
    const { log, run } = recorder()
    getOrCreateHostRuntime('h1')
    whileHostUp(run)()

    getHostRuntime('h1')!.getState().setStatus('up')

    expect(log).toEqual([])
    destroyHostRuntime('h1')
  })
})

describe('subscribeHostStatus', () => {
  it('calls back on the host’s status changes, including a runtime minted after it subscribed', () => {
    const changed = vi.fn()
    const stop = subscribeHostStatus('h3', changed)

    const h3 = getOrCreateHostRuntime('h3')
    const afterMint = changed.mock.calls.length
    h3.getState().setStatus('up')

    expect(afterMint).toBeGreaterThan(0)
    expect(changed.mock.calls.length).toBeGreaterThan(afterMint)
    stop()
    destroyHostRuntime('h3')
  })
})
