import { beforeEach, describe, expect, it } from 'vitest'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST } from '@/features/hosts/types'

describe('useHostConfigStore', () => {
  beforeEach(() => {
    useHostConfigStore.setState({ hosts: [] })
  })

  it('starts with no saved hosts', () => {
    expect(useHostConfigStore.getState().hosts).toEqual([])
  })

  it('addHost appends a host', () => {
    useHostConfigStore.getState().addHost({
      id: 'h1',
      name: 'homelab',
      endpoint: '1.2.3.4:3737',
      pairedFingerprint: 'SHA256:aaaa',
      clientId: 'client-1',
    })
    expect(useHostConfigStore.getState().hosts).toHaveLength(1)
    expect(useHostConfigStore.getState().hosts[0].id).toBe('h1')
  })

  it('removeHost drops it', () => {
    useHostConfigStore.getState().addHost({
      id: 'h1',
      name: 'homelab',
      endpoint: 'x',
      pairedFingerprint: 'x',
      clientId: 'client-1',
    })
    useHostConfigStore.getState().removeHost('h1')
    expect(useHostConfigStore.getState().hosts).toEqual([])
  })

  it('addHost never lets id "local" be added — it is implicit, not stored', () => {
    useHostConfigStore.getState().addHost({
      id: 'local',
      name: 'sneaky',
      endpoint: 'x',
      pairedFingerprint: 'x',
      clientId: 'client-1',
    })
    expect(useHostConfigStore.getState().hosts.find((h) => h.id === 'local')).toBeUndefined()
  })

  it('LOCAL_HOST has the correct shape and default values', () => {
    expect(LOCAL_HOST).toEqual({
      id: 'local',
      name: 'Local',
      endpoint: '',
      pairedFingerprint: '',
      clientId: '',
    })
  })
})
