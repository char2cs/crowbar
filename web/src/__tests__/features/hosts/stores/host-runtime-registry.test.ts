import { describe, expect, it, vi } from 'vitest'
import {
  getOrCreateHostRuntime,
  getHostRuntime,
  destroyHostRuntime,
  onHostUp,
  subscribeHostStatus,
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

describe('onHostUp', () => {
  it('names a host each time its status becomes up, and not while it stays up', () => {
    const up = vi.fn()
    getOrCreateHostRuntime('h1').getState().setStatus('down')
    const stop = onHostUp(up)
    const h1 = getHostRuntime('h1')!.getState()

    h1.setStatus('up', 12)
    h1.setStatus('up', 9)
    h1.setStatus('needs-repair')
    h1.setStatus('up', 5)

    expect(up.mock.calls).toEqual([['h1'], ['h1']])
    stop()
    destroyHostRuntime('h1')
  })

  it('follows hosts that appear after it starts, and lets go of those destroyed', () => {
    const up = vi.fn()
    const stop = onHostUp(up)

    const h2 = getOrCreateHostRuntime('h2')
    h2.getState().setStatus('up')
    destroyHostRuntime('h2')
    h2.getState().setStatus('down')
    h2.getState().setStatus('up')

    expect(up.mock.calls).toEqual([['h2']])
    stop()
  })

  it('names nothing once stopped', () => {
    const up = vi.fn()
    getOrCreateHostRuntime('h1')
    onHostUp(up)()

    getHostRuntime('h1')!.getState().setStatus('up')

    expect(up).not.toHaveBeenCalled()
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
