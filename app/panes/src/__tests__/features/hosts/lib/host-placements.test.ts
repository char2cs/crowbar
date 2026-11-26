import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { IDBFactory } from 'fake-indexeddb'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { pairRemoteHost } from '@/__tests__/__fixtures__/remote-host'
import { getDB } from '@/lib/persistence/idb'
import { abortDuring, watchUnhandled } from '@/__tests__/__fixtures__/aborted-write'
import {
  hydrateHostPlacements,
  persistedHostOf,
  startHostPlacementCache,
} from '@/features/hosts/lib/host-placements'
import {
  chatEntity,
  markProjectsSynced,
  putEntities,
  resetEntities,
  workspaceDTO,
} from '@/__tests__/__fixtures__/sidebar-entities'

let stop: () => void

beforeEach(async () => {
  vi.stubGlobal('indexedDB', new IDBFactory())
  resetEntities()
  await hydrateHostPlacements()
  stop = startHostPlacementCache()
})
afterEach(() => stop())

const stored = async () => (await (await getDB()).getAll('host-placements')).length
/** A fresh app start: nothing held in memory, the store read back. */
async function restart() {
  stop()
  resetEntities()
  await hydrateHostPlacements()
  stop = startHostPlacementCache()
}

describe('host placements', () => {
  it('survives a restart: a listed workspace and chat name their host', async () => {
    putEntities('workspace', [workspaceDTO({ id: 'w1' })], 'host-1')
    putEntities('chat', [chatEntity({ id: 'c1', workspaceId: 'w1' })], 'local')
    await vi.waitFor(async () => expect(await stored()).toBe(2))

    await restart()

    expect(persistedHostOf('workspace', 'w1')).toBe('host-1')
    expect(persistedHostOf('chat', 'c1')).toBe('local')
    expect(persistedHostOf('workspace', 'nope')).toBeNull()
  })

  it('names no host for an id two hosts both held', async () => {
    putEntities('workspace', [workspaceDTO({ id: 'w1' })], 'host-1')
    putEntities('workspace', [workspaceDTO({ id: 'w1' })], 'host-2')
    await vi.waitFor(async () => expect(await stored()).toBe(2))

    await restart()

    expect(persistedHostOf('workspace', 'w1')).toBeNull()
  })

  it('drops a row the daemon removes', async () => {
    putEntities('chat', [chatEntity({ id: 'c1' })], 'host-1')
    await vi.waitFor(async () => expect(await stored()).toBe(1))

    const { entityStore } = await import('@/core/entities/entity-store')
    entityStore.setState((s) => ({ byKind: { ...s.byKind, chat: {} } }))

    await vi.waitFor(async () => expect(await stored()).toBe(0))
  })

  it('replaces what a host once held when its list lands without it', async () => {
    putEntities('workspace', [workspaceDTO({ id: 'w1' }), workspaceDTO({ id: 'w2' })], 'host-1')
    await vi.waitFor(async () => expect(await stored()).toBe(2))
    await restart()
    expect(persistedHostOf('workspace', 'w2')).toBe('host-1')

    putEntities('workspace', [workspaceDTO({ id: 'w1' })], 'host-1')
    markProjectsSynced('host-1')

    await vi.waitFor(() => expect(persistedHostOf('workspace', 'w2')).toBeNull())
    expect(persistedHostOf('workspace', 'w1')).toBe('host-1')
    await vi.waitFor(async () => expect(await stored()).toBe(1))
  })

  it('forgets a removed host with its rows only', async () => {
    putEntities('workspace', [workspaceDTO({ id: 'w1' })], 'host-1')
    putEntities('workspace', [workspaceDTO({ id: 'w2' })], 'host-2')
    await vi.waitFor(async () => expect(await stored()).toBe(2))
    pairRemoteHost('host-1')
    await restart()

    useHostConfigStore.getState().removeHost('host-1')
    await vi.waitFor(async () => expect(await stored()).toBe(1))

    expect(persistedHostOf('workspace', 'w1')).toBeNull()
    expect(persistedHostOf('workspace', 'w2')).toBe('host-2')
    expect(await stored()).toBe(1)
  })

  it.each([
    {
      write: 'a placement write',
      method: 'put' as const,
      before: async () => {},
      act: () => putEntities('workspace', [workspaceDTO({ id: 'w1' })], 'host-1'),
    },
    {
      write: "forgetting a removed host's placements",
      method: 'delete' as const,
      before: async () => {
        putEntities('workspace', [workspaceDTO({ id: 'w1' })], 'host-1')
        await vi.waitFor(async () => expect(await stored()).toBe(1))
        pairRemoteHost('host-1')
      },
      act: () => useHostConfigStore.getState().removeHost('host-1'),
    },
  ])(
    'leaves no unhandled rejection when the database aborts $write',
    async ({ method, before, act }) => {
      await before()
      const watch = watchUnhandled()
      try {
        const aborted = abortDuring(method)
        act()
        await vi.waitFor(() => expect(aborted).toHaveBeenCalled())
        await watch.flush()
        expect(watch.seen).toEqual([])
      } finally {
        watch.stop()
      }
    },
  )

  it('reads as empty when IndexedDB is unavailable', async () => {
    vi.stubGlobal('indexedDB', undefined)

    await expect(hydrateHostPlacements()).resolves.toBeUndefined()

    expect(persistedHostOf('workspace', 'w1')).toBeNull()
  })
})
