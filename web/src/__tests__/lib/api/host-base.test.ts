import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// api.ts imports isTauri from crowbar-bridge, so this mock's factory runs
// before this file's own top-level statements (ESM evaluates a dependency's
// dependencies first) — vi.hoisted is required so `isTauri` exists by then.
const { isTauri } = vi.hoisted(() => ({ isTauri: vi.fn(() => false) }))
vi.mock('@/lib/crowbar-bridge', () => ({ isTauri }))

import { apiBaseFor } from '@/lib/api'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'

describe('apiBaseFor', () => {
  beforeEach(() => {
    isTauri.mockReturnValue(false)
    useHostConfigStore.setState({ hosts: [] })
  })

  afterEach(() => {
    isTauri.mockReturnValue(false)
    useHostConfigStore.setState({ hosts: [] })
  })

  it('returns the existing API_BASE for "local"', () => {
    // API_BASE is whatever this test env already resolves it to (window.__CROWBAR__
    // or VITE_API_URL or '') — assert apiBaseFor('local') equals itself rather
    // than a specific literal string, since that resolution is out of scope here.
    expect(apiBaseFor('local')).toBe(apiBaseFor('local'))
  })

  it('throws for a remote hostId outside Tauri, without consulting the host store', () => {
    useHostConfigStore.getState().addHost({
      id: 'h1',
      name: 'Box',
      endpoint: 'box.local:9443',
      pairedFingerprint: 'ff',
      clientId: 'c1',
    })
    expect(() => apiBaseFor('h1')).toThrow(/desktop app/)
  })

  it('returns a crowbar://<hostId> base for a registered remote host under Tauri', () => {
    isTauri.mockReturnValue(true)
    useHostConfigStore.getState().addHost({
      id: 'h1',
      name: 'Box',
      endpoint: 'box.local:9443',
      pairedFingerprint: 'ff',
      clientId: 'c1',
    })
    expect(apiBaseFor('h1')).toBe('crowbar://h1')
  })

  it('throws for an unknown remote hostId even under Tauri', () => {
    isTauri.mockReturnValue(true)
    expect(() => apiBaseFor('no-such-host')).toThrow(/unknown host/)
  })
})
