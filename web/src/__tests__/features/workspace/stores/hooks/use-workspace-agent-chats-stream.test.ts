import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'

// Hoisted fakes — must be declared before any vi.mock calls.
const {
  subscribe,
  listChatsFn,
  getChatFn,
  listProvidersFn,
  upsertAgentChat,
  removeAgentChat,
  setAgentChatWorking,
  setAgentProviders,
  hydrateAgentChatOrder,
  closeBuffer,
} = vi.hoisted(() => ({
  subscribe: vi.fn(() => () => {}),
  listChatsFn: vi.fn(),
  getChatFn: vi.fn(),
  listProvidersFn: vi.fn(),
  upsertAgentChat: vi.fn(),
  removeAgentChat: vi.fn(),
  setAgentChatWorking: vi.fn(),
  setAgentProviders: vi.fn(),
  hydrateAgentChatOrder: vi.fn(),
  closeBuffer: vi.fn(),
}))

// Mutable fixture the mocked store's getState() reads `buffers` from — tests
// set it directly to control the "close the deleted chat's pane tab" branch.
let buffers: Array<{ id: string; type: string; chatId?: string }> = []

vi.mock('@/lib/ws/manager', () => ({
  wsManager: { subscribe, send: vi.fn() },
}))

vi.mock('@/lib/workspace-scope-url', () => ({
  workspaceBase: (id: string) => `/v0/ws/${id}`,
}))

vi.mock('@/features/agent/api/agent-api', () => ({
  listChats: (...a: unknown[]) => listChatsFn(...a),
  getChat: (...a: unknown[]) => getChatFn(...a),
  listProviders: (...a: unknown[]) => listProvidersFn(...a),
}))

vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getOrCreateWorkspaceStore: () => ({
    getState: () => ({
      upsertAgentChat,
      removeAgentChat,
      setAgentChatWorking,
      setAgentProviders,
      hydrateAgentChatOrder,
      buffers,
      bufferActions: { closeBuffer },
    }),
  }),
}))

import { useWorkspaceAgentChatsStream } from '@/features/workspace/stores/hooks/use-workspace-agent-chats-stream'

type Frame = { chatId?: string; workspaceId?: string; kind?: string; reconnected?: boolean }

const chat = (id: string) => ({
  id,
  workspaceId: 'w1',
  title: id,
  activeSegmentId: `${id}-s`,
  activeProviderId: 'claude',
  createdAt: '2026-01-01T00:00:00Z',
})

const flush = () => Promise.resolve().then(() => Promise.resolve())

function captureCb(callIndex = 0): (frame: Frame) => void {
  const call = subscribe.mock.calls[callIndex] as unknown as [string, (frame: Frame) => void]
  return call[1]
}

beforeEach(() => {
  vi.clearAllMocks()
  subscribe.mockReturnValue(() => {})
  buffers = []
  listChatsFn.mockResolvedValue([chat('c1')])
  getChatFn.mockResolvedValue({ ...chat('c1'), segments: [] })
  listProvidersFn.mockResolvedValue([{ id: 'claude', displayName: 'Claude', icon: '<svg/>' }])
})

describe('useWorkspaceAgentChatsStream', () => {
  it('subscribes to the workspace-scoped /agent/ws/chats endpoint', () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    expect(subscribe).toHaveBeenCalledWith('/v0/ws/w1/agent/ws/chats', expect.any(Function))
  })

  it('seeds chats + providers on mount and populates the slice', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    expect(listChatsFn).toHaveBeenCalledWith('w1')
    expect(hydrateAgentChatOrder).toHaveBeenCalled()
    expect(upsertAgentChat).toHaveBeenCalledWith(chat('c1'))
    expect(listProvidersFn).toHaveBeenCalledWith('w1')
    expect(setAgentProviders).toHaveBeenCalledWith([
      { id: 'claude', displayName: 'Claude', icon: '<svg/>' },
    ])
  })

  it('seed failures (listChats/listProviders reject) are non-fatal', async () => {
    listChatsFn.mockRejectedValueOnce(new Error('boom'))
    listProvidersFn.mockRejectedValueOnce(new Error('boom'))
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    expect(upsertAgentChat).not.toHaveBeenCalled()
    expect(setAgentProviders).not.toHaveBeenCalled()
  })

  it('cancels the in-flight chats seed when wsId changes before it resolves', async () => {
    let resolveChats: (v: unknown[]) => void = () => {}
    listChatsFn.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveChats = resolve
        }),
    )
    listChatsFn.mockResolvedValue([]) // the w2 effect's own seed finds nothing

    const { rerender } = renderHook(({ w }: { w: string }) => useWorkspaceAgentChatsStream(w), {
      initialProps: { w: 'w1' },
    })
    rerender({ w: 'w2' }) // cleanup runs -> cancelled=true for the w1 effect

    resolveChats([chat('c1')]) // w1's stale seed resolves after teardown
    await flush()

    expect(upsertAgentChat).not.toHaveBeenCalled()
  })

  it('turn_started/turn_stopped toggle the working map without a refetch', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    const onFrame = captureCb()

    onFrame({ chatId: 'c1', workspaceId: 'w1', kind: 'turn_started' })
    expect(setAgentChatWorking).toHaveBeenCalledWith('c1', true)

    onFrame({ chatId: 'c1', workspaceId: 'w1', kind: 'turn_stopped' })
    expect(setAgentChatWorking).toHaveBeenCalledWith('c1', false)

    expect(getChatFn).not.toHaveBeenCalled()
  })

  it('created reseeds the whole list rather than refetching a single chat', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    const onFrame = captureCb()
    listChatsFn.mockClear()

    onFrame({ chatId: 'c2', workspaceId: 'w1', kind: 'created' })
    await flush()

    expect(listChatsFn).toHaveBeenCalledWith('w1')
    expect(getChatFn).not.toHaveBeenCalled()
  })

  it.each(['title_set', 'segment_opened', 'segment_ended', 'session_bound'] as const)(
    '%s refetches the single chat and upserts it',
    async (kind) => {
      renderHook(() => useWorkspaceAgentChatsStream('w1'))
      await flush()
      const onFrame = captureCb()
      getChatFn.mockClear()
      upsertAgentChat.mockClear()

      onFrame({ chatId: 'c1', workspaceId: 'w1', kind })
      await flush()

      expect(getChatFn).toHaveBeenCalledWith('w1', 'c1')
      expect(upsertAgentChat).toHaveBeenCalledWith({ ...chat('c1'), segments: [] })
    },
  )

  it('a refetchOne failure (e.g. chat already gone) is non-fatal', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    const onFrame = captureCb()
    getChatFn.mockRejectedValueOnce(new Error('404'))
    upsertAgentChat.mockClear()

    onFrame({ chatId: 'c1', workspaceId: 'w1', kind: 'title_set' })
    await flush()

    expect(upsertAgentChat).not.toHaveBeenCalled()
  })

  it('cancels an in-flight refetchOne (title_set/segment_*) when torn down before it resolves', async () => {
    let resolveChat: (v: unknown) => void = () => {}
    getChatFn.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveChat = resolve
        }),
    )

    const { unmount } = renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush() // let the (unrelated) initial seed settle
    const onFrame = captureCb()
    upsertAgentChat.mockClear()

    onFrame({ chatId: 'c1', workspaceId: 'w1', kind: 'title_set' }) // starts refetchOne
    unmount() // cancelled=true for this same effect instance before getChat resolves

    resolveChat({ ...chat('c1'), segments: [] })
    await flush()

    expect(upsertAgentChat).not.toHaveBeenCalled()
  })

  it('deleted removes the chat and closes its open pane tab', async () => {
    buffers = [{ id: 'buf1', type: 'agentChat', chatId: 'c1' }]
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    const onFrame = captureCb()

    onFrame({ chatId: 'c1', workspaceId: 'w1', kind: 'deleted' })

    expect(removeAgentChat).toHaveBeenCalledWith('c1')
    expect(closeBuffer).toHaveBeenCalledWith('buf1')
  })

  it('deleted with no open pane tab does not call closeBuffer', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    const onFrame = captureCb()

    onFrame({ chatId: 'c1', workspaceId: 'w1', kind: 'deleted' })

    expect(removeAgentChat).toHaveBeenCalledWith('c1')
    expect(closeBuffer).not.toHaveBeenCalled()
  })

  it('ignores a frame missing chatId', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    const onFrame = captureCb()

    onFrame({ workspaceId: 'w1', kind: 'turn_started' })

    expect(setAgentChatWorking).not.toHaveBeenCalled()
  })

  it('reconnect sentinel reseeds', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    const onFrame = captureCb()
    listChatsFn.mockClear()

    onFrame({ reconnected: true })
    await flush()

    expect(listChatsFn).toHaveBeenCalledWith('w1')
  })

  it('unsubscribes on unmount', () => {
    const unsub = vi.fn()
    subscribe.mockReturnValueOnce(unsub)

    const { unmount } = renderHook(() => useWorkspaceAgentChatsStream('w1'))
    unmount()

    expect(unsub).toHaveBeenCalledTimes(1)
  })

  it('ignores frames delivered to a callback after unmount/teardown', async () => {
    const { unmount } = renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    const onFrame = captureCb()
    unmount()

    // The mocked unsubscribe is a no-op, so this simulates a frame that was
    // already in flight when cleanup ran — the `cancelled` guard must catch it.
    onFrame({ chatId: 'c1', workspaceId: 'w1', kind: 'turn_started' })

    expect(setAgentChatWorking).not.toHaveBeenCalled()
  })

  it('tears down and re-subscribes when wsId changes', () => {
    const unsubW1 = vi.fn()
    const unsubW2 = vi.fn()
    subscribe.mockReturnValueOnce(unsubW1).mockReturnValueOnce(unsubW2)

    const { rerender } = renderHook(({ w }: { w: string }) => useWorkspaceAgentChatsStream(w), {
      initialProps: { w: 'w1' },
    })
    expect(subscribe).toHaveBeenCalledTimes(1)

    rerender({ w: 'w2' })

    expect(unsubW1).toHaveBeenCalledTimes(1)
    expect(subscribe).toHaveBeenCalledTimes(2)
    expect((subscribe.mock.calls[1] as unknown as [string])[0]).toBe('/v0/ws/w2/agent/ws/chats')
  })
})
