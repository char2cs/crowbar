import { describe, expect, it, vi } from 'vitest'
import {
  getOrCreateHostRuntime,
  getHostRuntime,
  destroyHostRuntime,
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
