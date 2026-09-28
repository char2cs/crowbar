import { describe, expect, it } from 'vitest'
import {
  getOrCreateHostRuntime,
  getHostRuntime,
  destroyHostRuntime,
  subscribeHostRuntimeRegistry,
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

  it('destroyHostRuntime removes it and notifies subscribers', () => {
    getOrCreateHostRuntime('h1')
    let notified: string | undefined
    const unsubscribe = subscribeHostRuntimeRegistry((change) => {
      notified = change
    })
    destroyHostRuntime('h1')
    expect(notified).toBe('destroyed')
    expect(getHostRuntime('h1')).toBeUndefined()
    unsubscribe()
  })
})
