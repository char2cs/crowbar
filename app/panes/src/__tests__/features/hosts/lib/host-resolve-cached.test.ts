import { act, renderHook } from '@testing-library/react'
import { IDBFactory } from 'fake-indexeddb'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { getDB } from '@/lib/persistence/idb'
import {
  hydrateHostPlacements,
  startHostPlacementCache,
} from '@/features/hosts/lib/host-placements'
import { feedHostOf, isChatRecalled, useFeedHost } from '@/features/hosts/lib/host-resolve-cached'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'
import {
  chatEntity,
  markProjectsSynced,
  putEntities,
} from '@/__tests__/__fixtures__/sidebar-entities'

/** The last run listed chat c1 on `hostId`; this run starts cold, with `paired` hosts and no list. */
async function coldStartAfter(hostId: string, paired: string[]) {
  const stop = startHostPlacementCache()
  putEntities('chat', [chatEntity({ id: 'c1' })], hostId)
  await vi.waitFor(async () => expect(await (await getDB()).count('host-placements')).toBe(1))
  stop()
  resetHostWorld()
  paired.forEach((id) => pairRemoteHost(id))
  await hydrateHostPlacements()
}

beforeEach(async () => {
  vi.stubGlobal('indexedDB', new IDBFactory())
  resetHostWorld()
  await hydrateHostPlacements()
})

describe('the host a chat is recalled on before any list', () => {
  it.each([
    { held: 'host-1', paired: ['host-1'], listed: [], want: 'host-1' },
    { held: 'local', paired: [], listed: [], want: 'local' },
    { held: 'host-1', paired: ['host-2'], listed: [], want: null },
    { held: 'host-1', paired: ['host-1'], listed: ['host-1'], want: null },
    { held: 'host-1', paired: ['host-1'], listed: ['local'], want: 'host-1' },
  ])(
    'held on $held, paired $paired, listed $listed: $want',
    async ({ held, paired, listed, want }) => {
      await coldStartAfter(held, paired)
      listed.forEach((hostId) => markProjectsSynced(hostId))

      expect(feedHostOf('c1')).toBe(want)
      expect(isChatRecalled('c1')).toBe(want !== null)
    },
  )

  it('prefers the host the chat resolves to once its host lists it', async () => {
    await coldStartAfter('host-1', ['host-1'])
    putEntities('chat', [chatEntity({ id: 'c1' })], 'host-1')
    markProjectsSynced('host-1')

    expect(feedHostOf('c1')).toBe('host-1')
    expect(isChatRecalled('c1')).toBe(false)
  })

  it('re-renders the feed host when the recalled host lists without the chat', async () => {
    await coldStartAfter('host-1', ['host-1'])
    const { result } = renderHook(() => useFeedHost('c1'))
    expect(result.current).toBe('host-1')

    act(() => markProjectsSynced('host-1'))

    expect(result.current).toBeNull()
  })
})
