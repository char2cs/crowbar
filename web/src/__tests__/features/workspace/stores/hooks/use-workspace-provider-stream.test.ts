import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { act, renderHook } from '@testing-library/react'

// The per-CHAT lifecycle WS subscription is load-bearing beyond data: the daemon
// starts the per-connection provider poll (PR status detection) ONLY when a
// client subscribes to a stream that resolves ONE worktree — `/v0/chats/:id/ws`,
// which the daemon resolves to that chat's worktree and refcounts the poll on.
// The repo-wide chat stream resolves no single workspace and never starts it, so
// a workspace whose branch has an open PR stayed 'new' (plain branch glyph)
// forever. This hook is what opens that stream while a workspace is viewed;
// these tests lock that wiring.
const { subscribeEntityStream, fetchWorkspace, syncSidebarFromCache } = vi.hoisted(() => ({
  subscribeEntityStream: vi.fn(),
  fetchWorkspace: vi.fn(),
  syncSidebarFromCache: vi.fn(),
}))

vi.mock('@/lib/ws/entity-stream', () => ({
  subscribeEntityStream: (...args: unknown[]) => subscribeEntityStream(...args),
}))

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  fetchWorkspace: (...args: unknown[]) => fetchWorkspace(...args),
}))

vi.mock('@/lib/store/sidebar-sync', () => ({
  syncSidebarFromCache: (...args: unknown[]) => syncSidebarFromCache(...args),
}))

import { useWorkspaceProviderStream } from '@/features/workspace/stores/hooks/use-workspace-provider-stream'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { WorkspaceDTO } from '@/lib/types'

/** A worktree_state frame as the chat feed sends it, chat `c1` owning `w1`. */
const worktreeFrame = (over: Record<string, unknown> = {}) => ({
  chatId: 'c1',
  workspaceId: 'w1',
  repoId: 'r1',
  kind: 'worktree_state',
  working: false,
  worktree: {
    branch: 'feature/x',
    status: 'pr-open',
    working: false,
    added: 0,
    deleted: 0,
    mergeStrategy: 'squash',
    canMergeLocally: true,
    mergeConflicts: false,
    owningChatId: 'c1',
  },
  ...over,
})

interface StreamOptions {
  endpoint: string
  store: string
  mapFrame?: (raw: unknown) => WorkspaceDTO | null
  pruneScope?: (ws: WorkspaceDTO) => boolean
}

const optionsOf = (call: number): StreamOptions =>
  subscribeEntityStream.mock.calls[call][0] as StreamOptions

beforeEach(() => {
  vi.clearAllMocks()
  __resetWorkspaceScopesForTest()
  subscribeEntityStream.mockReturnValue(() => {})
  fetchWorkspace.mockResolvedValue({ id: 'w1' })
  // The chat that owns the viewed workspace's worktree — the only id the
  // per-chat route can be addressed by. The sidebar records it from
  // WorkspaceDTO.owningChatId; nothing here can guess it.
  recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'w1', owningChatId: 'c1' })
  recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'w2', owningChatId: 'c2' })
})

afterEach(() => {
  __resetWorkspaceScopesForTest()
})

describe('useWorkspaceProviderStream', () => {
  it('subscribes to the OWNING CHAT stream when the full scope is present', () => {
    renderHook(() => useWorkspaceProviderStream('p1', 'r1', 'w1'))

    expect(subscribeEntityStream).toHaveBeenCalledTimes(1)
    expect(optionsOf(0).endpoint).toBe('/v0/chats/c1/ws')
    expect(optionsOf(0).store).toBe('crowbar_workspaces')
  })

  it('does NOT subscribe when any scope id is missing', () => {
    const { rerender } = renderHook<void, { p?: string; r?: string; w?: string }>(
      ({ p, r, w }) => useWorkspaceProviderStream(p, r, w),
      { initialProps: { p: 'p1', r: 'r1', w: undefined } },
    )
    expect(subscribeEntityStream).not.toHaveBeenCalled()

    rerender({ p: 'p1', r: undefined, w: 'w1' })
    expect(subscribeEntityStream).not.toHaveBeenCalled()

    rerender({ p: undefined, r: 'r1', w: 'w1' })
    expect(subscribeEntityStream).not.toHaveBeenCalled()
  })

  it('skips (never throws) a workspace whose owning chat is not recorded yet', () => {
    // This runs on every render of the IDE shell, including before the sidebar
    // has recorded which chat holds the route's workspace.
    __resetWorkspaceScopesForTest()
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'w1' })

    expect(() => renderHook(() => useWorkspaceProviderStream('p1', 'r1', 'w1'))).not.toThrow()
    expect(subscribeEntityStream).not.toHaveBeenCalled()
  })

  it('unsubscribes on unmount (mirrors the daemon refcount lifecycle)', () => {
    const unsub = vi.fn()
    subscribeEntityStream.mockReturnValue(unsub)

    const { unmount } = renderHook(() => useWorkspaceProviderStream('p1', 'r1', 'w1'))
    expect(subscribeEntityStream).toHaveBeenCalledTimes(1)

    unmount()
    expect(unsub).toHaveBeenCalledTimes(1)
  })

  it('tears down the old stream and opens a new one when the viewed workspace changes', () => {
    const unsubW1 = vi.fn()
    const unsubW2 = vi.fn()
    subscribeEntityStream.mockReturnValueOnce(unsubW1).mockReturnValueOnce(unsubW2)

    const { rerender } = renderHook(
      ({ w }: { w: string }) => useWorkspaceProviderStream('p1', 'r1', w),
      { initialProps: { w: 'w1' } },
    )
    expect(optionsOf(0)).toMatchObject({ endpoint: '/v0/chats/c1/ws' })

    rerender({ w: 'w2' })
    expect(unsubW1).toHaveBeenCalledTimes(1)
    expect(optionsOf(1)).toMatchObject({ endpoint: '/v0/chats/c2/ws' })
  })

  it('maps a worktree_state frame into the workspace DTO, and ignores every other kind', () => {
    renderHook(() => useWorkspaceProviderStream('p1', 'r1', 'w1'))
    const { mapFrame } = optionsOf(0)

    expect(mapFrame!(worktreeFrame())).toMatchObject({
      id: 'w1',
      repoId: 'r1',
      projectId: 'p1',
      branch: 'feature/x',
      status: 'pr-open',
      owningChatId: 'c1',
    })

    // The rest of the chat vocabulary rides the same socket and says nothing
    // about a worktree.
    expect(
      mapFrame!({ chatId: 'c1', workspaceId: 'w1', kind: 'turn_started', working: true }),
    ).toBeNull()
    expect(mapFrame!({ chatId: 'c1', workspaceId: 'w1', kind: 'deleted' })).toBeNull()
    expect(mapFrame!({ chatId: 'c1', kind: 'folder_created', folderId: 'f1' })).toBeNull()
    // A thread of c1 gets its parent's worktree state too; only the owning row
    // is this worktree's row.
    expect(mapFrame!(worktreeFrame({ chatId: 'c2' }))).toBeNull()
  })

  // REGRESSION. The owning chat cannot be read once and forgotten.
  //
  // The scope registry this hook resolves through is populated BY sidebar store
  // writes, so on a cold load the first render — the one the IDE shell mounts
  // this on — has no chat recorded for the route's workspace yet. Reading it
  // only inside the effect meant the early return was permanent: no dependency
  // would ever change again, the subscription never opened, and the daemon's
  // PR-status poll never started for the entire session, leaving every branch
  // glyph stuck on `new`. That is precisely the failure the per-chat mount was
  // added to prevent, reintroduced one layer up.
  it('subscribes as soon as the sidebar learns the owning chat (cold load)', () => {
    __resetWorkspaceScopesForTest()
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'w1' })

    renderHook(() => useWorkspaceProviderStream('p1', 'r1', 'w1'))
    expect(subscribeEntityStream).not.toHaveBeenCalled()

    // The sidebar lands its rows — in production the very same store write that
    // records the scope (recordRepoScopes / applyWorkspaceDTO).
    act(() => {
      recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'w1', owningChatId: 'c1' })
      useSidebarStore.setState((s) => ({ repos: [...s.repos] }))
    })

    expect(subscribeEntityStream).toHaveBeenCalledTimes(1)
    expect(optionsOf(0).endpoint).toBe('/v0/chats/c1/ws')
  })

  it('stays authoritative over the viewed workspace alone', () => {
    renderHook(() => useWorkspaceProviderStream('p1', 'r1', 'w1'))
    const { pruneScope } = optionsOf(0)
    expect(pruneScope!({ id: 'w1' } as WorkspaceDTO)).toBe(true)
    expect(pruneScope!({ id: 'w2' } as WorkspaceDTO)).toBe(false)
  })
})
