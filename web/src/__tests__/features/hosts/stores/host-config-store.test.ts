import { beforeEach, describe, expect, it, vi } from 'vitest'
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

  // Every window ("New Window") saves its hosts to the same key.
  describe('across windows', () => {
    const host = (id: string) => ({
      id,
      name: id,
      endpoint: `${id}.lan:3737`,
      pairedFingerprint: 'ff',
      clientId: `client-${id}`,
    })

    /** A window: its own store instance over the shared storage. */
    async function openWindow() {
      vi.resetModules()
      const { useHostConfigStore: store } =
        await import('@/features/hosts/stores/host-config-store')
      return store
    }

    /** What the browser tells every other window when one of them saves. */
    const announce = () =>
      window.dispatchEvent(
        new StorageEvent('storage', {
          key: 'crowbar.hosts',
          newValue: localStorage.getItem('crowbar.hosts'),
        }),
      )
    const ids = (store: typeof useHostConfigStore) => store.getState().hosts.map((h) => h.id)

    it('a host paired in one window appears in the other, which learns a remote was paired', async () => {
      const a = await openWindow()
      const b = await openWindow()

      a.getState().addHost(host('h1'))
      announce()

      expect(ids(b)).toEqual(['h1'])
      expect(b.getState().remoteEverPaired).toBe(true)
    })

    // Its client id is the only way to revoke that pairing server-side.
    it('pairing in a window that has not heard of the other’s host yet keeps both', async () => {
      const a = await openWindow()
      const b = await openWindow()

      a.getState().addHost(host('h1'))
      b.getState().addHost(host('h2'))

      expect(ids(b)).toEqual(['h1', 'h2'])
      expect(JSON.parse(localStorage.getItem('crowbar.hosts')!).state.hosts).toEqual([
        host('h1'),
        host('h2'),
      ])
    })

    it('a host removed in one window is gone from the other and stays gone', async () => {
      const a = await openWindow()
      a.getState().addHost(host('h1'))
      a.getState().addHost(host('h2'))
      const b = await openWindow()

      a.getState().removeHost('h1')
      announce()
      expect(ids(b)).toEqual(['h2'])

      b.getState().addHost(host('h3'))
      announce()
      expect(ids(a)).toEqual(['h2', 'h3'])
    })

    it('never forgets that a remote was paired, whatever another window saves', async () => {
      const a = await openWindow()
      a.getState().addHost(host('h1'))

      localStorage.setItem(
        'crowbar.hosts',
        JSON.stringify({ state: { hosts: [], remoteEverPaired: false }, version: 0 }),
      )
      announce()

      expect(ids(a)).toEqual([])
      expect(a.getState().remoteEverPaired).toBe(true)
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
