import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { ChatDTO } from '@/lib/types'

// REGRESSION (restyle v2 follow-up): `subscribeHomeTree`'s reseed reads its
// two halves with one `Promise.all`, so a failed GET /home/chats/folders
// discards the chats that came back fine (and vice versa), and `setTree` is
// never called — `space-scroller.tsx`'s `homeSeeded` then draws NO home rows
// at all until some later structural frame or a socket reconnect happens to
// reseed. The repo tree's `openRepoTreeSubscription` isolates its halves for
// exactly this reason (app-sync-engine.ts `reseedHalf`); home does not.
const subscribers = new Set<(data: unknown) => void>()

vi.mock('@/lib/ws/manager', () => ({
  wsManager: {
    subscribe: (_endpoint: string, cb: (data: unknown) => void) => {
      subscribers.add(cb)
      return () => subscribers.delete(cb)
    },
    send: vi.fn(),
  },
}))

const fetchHomeChats = vi.fn()
const fetchHomeFolders = vi.fn()
vi.mock('@/lib/api', () => ({
  fetchHomeChats: (projectId: string) => fetchHomeChats(projectId) as unknown,
  fetchHomeFolders: (projectId: string) => fetchHomeFolders(projectId) as unknown,
  fetchRepos: vi.fn().mockResolvedValue([]),
  fetchHomeWorkspace: vi.fn().mockResolvedValue({ id: 'ws-home', owningChatId: 'c-home' }),
  assetURL: (path: string) => path,
}))

const { useHomeTreeStore, subscribeHomeTree } = await import('@/lib/store/home-tree')

const chat: ChatDTO = {
  id: 'c1',
  repoId: '',
  projectId: 'p1',
  workspaceId: '',
  ownsWorktree: false,
  order: 0,
  title: 'Fix the thing',
}

/** Resolves once both reads have settled — a real signal off the mocks. */
function whenBothSettled(): Promise<void> {
  return Promise.allSettled([
    fetchHomeChats.mock.results[0]?.value,
    fetchHomeFolders.mock.results[0]?.value,
  ]).then(() => undefined)
}

beforeEach(() => {
  subscribers.clear()
  fetchHomeChats.mockReset()
  fetchHomeFolders.mockReset()
  useHomeTreeStore.setState({ trees: {} })
  vi.spyOn(console, 'error').mockImplementation(() => {})
})

describe('subscribeHomeTree with one half failing', () => {
  it('still seeds the chats when only the folders read fails', async () => {
    fetchHomeChats.mockResolvedValue([chat])
    fetchHomeFolders.mockRejectedValue(new Error('500 folders'))
    const dispose = subscribeHomeTree('p1')
    await whenBothSettled()
    // One more turn for the store write that follows the reads.
    await Promise.resolve()

    expect(useHomeTreeStore.getState().trees['p1']?.chats.map((c) => c.id)).toEqual(['c1'])
    dispose()
  })
})
