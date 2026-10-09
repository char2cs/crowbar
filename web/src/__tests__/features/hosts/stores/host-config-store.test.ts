import { beforeEach, describe, expect, it } from 'vitest'
import { hasPairedRemote, useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST } from '@/features/hosts/types'

describe('useHostConfigStore', () => {
  beforeEach(() => {
    localStorage.clear()
    useHostConfigStore.setState({ hosts: [], remoteEverPaired: false })
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

  describe('remoteEverPaired', () => {
    const remote = {
      id: 'h1',
      name: 'homelab',
      endpoint: '1.2.3.4:3737',
      pairedFingerprint: 'SHA256:aaaa',
      clientId: 'client-1',
    }

    it('starts false and is set by addHost for a remote host', () => {
      expect(useHostConfigStore.getState().remoteEverPaired).toBe(false)
      useHostConfigStore.getState().addHost(remote)
      expect(useHostConfigStore.getState().remoteEverPaired).toBe(true)
    })

    it('is not set by an attempt to add the implicit local host', () => {
      useHostConfigStore.getState().addHost({ ...remote, id: 'local' })
      expect(useHostConfigStore.getState().remoteEverPaired).toBe(false)
    })

    it('is never reset, not even by removing the last host', () => {
      useHostConfigStore.getState().addHost(remote)
      useHostConfigStore.getState().removeHost('h1')
      expect(useHostConfigStore.getState().hosts).toEqual([])
      expect(useHostConfigStore.getState().remoteEverPaired).toBe(true)
    })

    it('is persisted with the hosts', () => {
      useHostConfigStore.getState().addHost(remote)
      const saved = JSON.parse(localStorage.getItem('crowbar.hosts')!)
      expect(saved.state).toEqual({ hosts: [remote], remoteEverPaired: true })
    })

    it('survives a reload after the last host was removed', async () => {
      useHostConfigStore.getState().addHost(remote)
      useHostConfigStore.getState().removeHost('h1')
      const saved = localStorage.getItem('crowbar.hosts')!
      // A fresh page load: memory is blank, storage is what the last session wrote.
      useHostConfigStore.setState({ hosts: [], remoteEverPaired: false })
      localStorage.setItem('crowbar.hosts', saved)
      await useHostConfigStore.persist.rehydrate()
      expect(useHostConfigStore.getState().hosts).toEqual([])
      expect(hasPairedRemote(useHostConfigStore.getState())).toBe(true)
    })

    it('counts a configured host as paired when the saved state predates the field', async () => {
      localStorage.setItem(
        'crowbar.hosts',
        JSON.stringify({ state: { hosts: [remote] }, version: 0 }),
      )
      await useHostConfigStore.persist.rehydrate()
      expect(useHostConfigStore.getState().remoteEverPaired).toBe(false)
      expect(hasPairedRemote(useHostConfigStore.getState())).toBe(true)
    })

    it('is false for an install that never paired a remote', async () => {
      localStorage.setItem('crowbar.hosts', JSON.stringify({ state: { hosts: [] }, version: 0 }))
      await useHostConfigStore.persist.rehydrate()
      expect(hasPairedRemote(useHostConfigStore.getState())).toBe(false)
    })
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
