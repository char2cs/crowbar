/**
 * useAgentChatFolders: one GET per mounted workspace, seeding the store, and
 * NOTHING after it — no timer, no poll (see the hook's own doc comment). Two
 * failure shapes matter more than the happy path:
 *
 *  - a rejected fetch must be swallowed, not thrown into React — a workspace
 *    with no folders is exactly "every chat at the root", which is how the
 *    panel drew before folders existed;
 *  - an answer that lands AFTER the component unmounted must never write —
 *    a workspace switch that happens mid-flight must not seed the NEXT
 *    workspace's leftovers into whichever store happens to still be live.
 */
import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { listChatFolders } = vi.hoisted(() => ({ listChatFolders: vi.fn() }))

vi.mock('@/features/agent/api/agent-api', () => ({ listChatFolders }))

// The registry's own persistence subscriptions, stubbed the same way
// workspace-store-registry.test.ts does — nothing here exercises layout or
// session persistence, only the folders seed.
vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))

import { useAgentChatFolders } from '@/features/agent/components/use-agent-chat-folders'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import type { AgentChatFolder } from '@/features/agent/api/agent-api'

const folder = (id: string, order: number): AgentChatFolder => ({
  id,
  workspaceId: 'w1',
  parentId: '',
  name: id,
  order,
})

/** A promise this test settles by hand, to control exactly when the GET answers. */
function deferred<T>() {
  let resolve!: (v: T) => void
  let reject!: (e: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

beforeEach(() => {
  listChatFolders.mockReset()
})

afterEach(() => {
  destroyWorkspaceStore('w1')
  destroyWorkspaceStore('w2')
  vi.restoreAllMocks()
})

describe('useAgentChatFolders', () => {
  it('fetches once on mount, then seeds the store once the GET answers', async () => {
    const d = deferred<AgentChatFolder[]>()
    listChatFolders.mockReturnValueOnce(d.promise)

    renderHook(() => useAgentChatFolders('w1'))

    expect(listChatFolders).toHaveBeenCalledTimes(1)
    expect(listChatFolders).toHaveBeenCalledWith('w1')
    // Nothing written yet — the GET is still in flight.
    expect(getOrCreateWorkspaceStore('w1').getState().agentChats.folders).toEqual([])

    await act(async () => {
      d.resolve([folder('f1', 0)])
      await d.promise
    })

    expect(getOrCreateWorkspaceStore('w1').getState().agentChats.folders).toEqual([folder('f1', 0)])
  })

  it('a rejected GET is swallowed — the tree just draws every chat at the root, same as before folders existed', async () => {
    const d = deferred<AgentChatFolder[]>()
    listChatFolders.mockReturnValueOnce(d.promise)

    renderHook(() => useAgentChatFolders('w1'))

    // The rejection must not escape the hook as an unhandled rejection or a
    // thrown render error.
    await act(async () => {
      d.reject(new Error('network down'))
      await d.promise.catch(() => {})
    })

    expect(getOrCreateWorkspaceStore('w1').getState().agentChats.folders).toEqual([])
  })

  it('an answer that lands AFTER unmount is not written into the store', async () => {
    const d = deferred<AgentChatFolder[]>()
    listChatFolders.mockReturnValueOnce(d.promise)

    const { unmount } = renderHook(() => useAgentChatFolders('w1'))
    unmount() // the cleanup flips `cancelled` before the GET has answered

    await act(async () => {
      d.resolve([folder('f1', 0)])
      await d.promise
      // Let the hook's own `.then` (attached before unmount) run its course.
      await Promise.resolve()
    })

    expect(getOrCreateWorkspaceStore('w1').getState().agentChats.folders).toEqual([])
  })

  it('does not poll — a resolved GET never triggers a second call while the workspace stays mounted', async () => {
    listChatFolders.mockResolvedValueOnce([folder('f1', 0)])

    renderHook(() => useAgentChatFolders('w1'))
    await act(async () => {
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(listChatFolders).toHaveBeenCalledTimes(1)
  })

  it("switching workspace id issues its own GET and seeds only that workspace's store", async () => {
    listChatFolders.mockResolvedValueOnce([folder('f1', 0)]) // w1's answer
    listChatFolders.mockResolvedValueOnce([folder('f2', 0)]) // w2's answer

    const { rerender } = renderHook(({ wsId }) => useAgentChatFolders(wsId), {
      initialProps: { wsId: 'w1' },
    })
    await act(async () => {
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(getOrCreateWorkspaceStore('w1').getState().agentChats.folders).toEqual([folder('f1', 0)])

    rerender({ wsId: 'w2' })
    await act(async () => {
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(listChatFolders).toHaveBeenCalledTimes(2)
    expect(listChatFolders).toHaveBeenNthCalledWith(2, 'w2')
    expect(getOrCreateWorkspaceStore('w2').getState().agentChats.folders).toEqual([folder('f2', 0)])
    // w1's own store keeps what it already had — the switch never touches it.
    expect(getOrCreateWorkspaceStore('w1').getState().agentChats.folders).toEqual([folder('f1', 0)])
  })
})
