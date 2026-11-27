import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { isTauri } = vi.hoisted(() => ({ isTauri: vi.fn(() => false) }))
vi.mock('@/lib/crowbar-bridge', () => ({ isTauri }))

import { apiBaseFor } from '@/lib/api/fetch'
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
