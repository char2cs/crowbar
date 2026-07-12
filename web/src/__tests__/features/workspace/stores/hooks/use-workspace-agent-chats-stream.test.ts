import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'

// Hoisted fakes — must be declared before any vi.mock calls.
const {
  subscribe,
  listChatsFn,
  getChatFn,
  listProvidersFn,
  seedAgentChats,
  upsertAgentChat,
  removeAgentChat,
  setAgentChatWorking,
  setAgentProviders,
  hydrateAgentChatOrder,
  closeBuffer,
  repointAgentChatBuffer,
  toastInfo,
} = vi.hoisted(() => ({
  subscribe: vi.fn(() => () => {}),
  listChatsFn: vi.fn(),
  getChatFn: vi.fn(),
  listProvidersFn: vi.fn(),
  seedAgentChats: vi.fn(),
  upsertAgentChat: vi.fn(),
  removeAgentChat: vi.fn(),
  setAgentChatWorking: vi.fn(),
  setAgentProviders: vi.fn(),
  hydrateAgentChatOrder: vi.fn(),
  closeBuffer: vi.fn(),
  repointAgentChatBuffer: vi.fn(),
  toastInfo: vi.fn(),
}))

// Mutable fixtures the mocked store's getState() reads from — tests set them
// directly to control the "close the deleted chat's pane tab" branch, the reseed's
// reconcile (which diffs the store's current chats against the GET), and the runner
// frames, which are resolved against the OPEN TABS (buffers) and the chat list.
type Buf = { id: string; type: string; chatId?: string; runnerId?: string }
type Chat = {
  id: string
  liveRunnerId?: string
  terminalSessionId?: string
  activeProviderId?: string
}
let buffers: Buf[] = []
let storeChats: Chat[] = []
let storeProviders: Array<{ id: string; displayName: string; icon: string }> = []
// The pane holding those buffers. Closing a chat tab must go through the pane
// (removeBufferFromPane) before the buffer is dropped, or the pane is left with a
// dangling activeBufferId and renders its EMPTY state — a live bug: deleting the
// chat you were looking at blanked the pane even though another tab was open.
let panes: Record<string, { id: string; bufferIds: string[] }> = {}

const removeBufferFromPane = vi.fn()
const activatePaneBuffer = vi.fn()

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

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { info: (...a: unknown[]) => toastInfo(...a) },
}))

vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getOrCreateWorkspaceStore: () => ({
    getState: () => ({
      agentChats: { chats: storeChats, providers: storeProviders },
      seedAgentChats,
      upsertAgentChat,
      removeAgentChat,
      setAgentChatWorking,
      setAgentProviders,
      hydrateAgentChatOrder,
      buffers,
      panes,
      bufferActions: { closeBuffer, repointAgentChatBuffer },
      paneActions: { removeBufferFromPane, activatePaneBuffer },
    }),
  }),
}))

import { useWorkspaceAgentChatsStream } from '@/features/workspace/stores/hooks/use-workspace-agent-chats-stream'

type Frame = {
  chatId?: string
  workspaceId?: string
  kind?: string
  runnerId?: string
  reconnected?: boolean
}

const chat = (id: string) => ({
  id,
  workspaceId: 'w1',
  title: id,
  liveRunnerId: `${id}-r`,
  terminalSessionId: `${id}-pty`,
  activeProviderId: 'claude',
  createdAt: '2026-01-01T00:00:00Z',
})

// Drain the microtask queue. This is NOT a clock: every promise in the hook's
// chains resolves from an already-settled mock, so a fixed number of ticks settles
// them deterministically (the deepest chain — refetch the chat entered, then act,
// then refetch the chat left — is four).
const flush = async () => {
  for (let i = 0; i < 8; i++) await Promise.resolve()
}

function captureCb(callIndex = 0): (frame: Frame) => void {
  const call = subscribe.mock.calls[callIndex] as unknown as [string, (frame: Frame) => void]
  return call[1]
}

beforeEach(() => {
  vi.clearAllMocks()
  subscribe.mockReturnValue(() => {})
  buffers = []
  panes = {}
  removeBufferFromPane.mockClear()
  activatePaneBuffer.mockClear()
  storeChats = []
  storeProviders = []
  // The real slices mutate; model that, so the hook's own reads (the vanished-chat
  // diff, the runner→chat lookup, the idempotence of a repeated frame) see a
  // faithful before/after rather than a frozen fixture.
  seedAgentChats.mockImplementation((chats: Chat[]) => {
    storeChats = chats
  })
  upsertAgentChat.mockImplementation((c: Chat) => {
    const i = storeChats.findIndex((x) => x.id === c.id)
    if (i === -1) storeChats.push(c)
    else storeChats[i] = c
  })
  setAgentProviders.mockImplementation((p: typeof storeProviders) => {
    storeProviders = p
  })
  repointAgentChatBuffer.mockImplementation(
    (id: string, to: { chatId: string; runnerId: string }) => {
      const b = buffers.find((x) => x.id === id)
      if (!b) return
      b.chatId = to.chatId
      b.runnerId = to.runnerId
    },
  )
  closeBuffer.mockImplementation((id: string) => {
    buffers = buffers.filter((b) => b.id !== id)
  })
  listChatsFn.mockResolvedValue([chat('c1')])
  getChatFn.mockImplementation((_wsId: string, id: string) =>
    Promise.resolve({ ...chat(id), conversations: [] }),
  )
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
    expect(seedAgentChats).toHaveBeenCalledWith([chat('c1')])
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
    expect(seedAgentChats).not.toHaveBeenCalled()
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

    // w2's own seed legitimately reconciles to an empty list; what must NOT happen
    // is w1's stale response landing in w2's store.
    expect(seedAgentChats).not.toHaveBeenCalledWith([chat('c1')])
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

  it.each(['title_set', 'session_bound'] as const)(
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
      expect(upsertAgentChat).toHaveBeenCalledWith({ ...chat('c1'), conversations: [] })
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

  it('cancels an in-flight refetchOne (title_set) when torn down before it resolves', async () => {
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

    resolveChat({ ...chat('c1'), conversations: [] })
    await flush()

    expect(upsertAgentChat).not.toHaveBeenCalled()
  })

  it('deleted removes the chat and closes its open pane tab THROUGH the pane', async () => {
    buffers = [{ id: 'buf1', type: 'agentChat', chatId: 'c1' }]
    panes = { p1: { id: 'p1', bufferIds: ['buf1', 'buf-other'] } }
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    const onFrame = captureCb()

    onFrame({ chatId: 'c1', workspaceId: 'w1', kind: 'deleted' })

    expect(removeAgentChat).toHaveBeenCalledWith('c1')
    // Removing it from the pane is what activates the adjacent tab. Dropping the
    // buffer alone leaves the pane's activeBufferId dangling and blanks it.
    expect(removeBufferFromPane).toHaveBeenCalledWith('p1', 'buf1')
    expect(closeBuffer).toHaveBeenCalledWith('buf1')
    expect(removeBufferFromPane.mock.invocationCallOrder[0]).toBeLessThan(
      closeBuffer.mock.invocationCallOrder[0],
    )
  })

  it('deleted with no open pane tab does not call closeBuffer', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    const onFrame = captureCb()

    onFrame({ chatId: 'c1', workspaceId: 'w1', kind: 'deleted' })

    expect(removeAgentChat).toHaveBeenCalledWith('c1')
    expect(closeBuffer).not.toHaveBeenCalled()
  })

  it('ignores a chat frame missing chatId', async () => {
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

  // ── Runner frames ──────────────────────────────────────────────────────────
  // The vendor CLI is a PROCESS, and it MOVES: the user types /clear or /resume
  // inside it and it switches conversation, so the runner lands on another chat.
  // These frames ride the same workspace-scoped feed as the chat frames and are
  // told apart by ONE thing — runnerId is present (`omitempty`; chat frames never
  // set it). Kind alone is ambiguous: `session_bound` exists in both vocabularies.

  const openTab = (id: string, chatId: string, runnerId: string): Buf => ({
    id,
    type: 'agentChat',
    chatId,
    runnerId,
  })

  it('moved re-points the tab following that runner at the chat it ENTERED', async () => {
    listChatsFn.mockResolvedValue([chat('c1'), chat('c2')])
    buffers = [openTab('buf1', 'c1', 'c1-r')]
    panes = { p1: { id: 'p1', bufferIds: ['buf1'] } }
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    captureCb()({ runnerId: 'c1-r', chatId: 'c2', workspaceId: 'w1', kind: 'moved' })
    await flush()

    expect(repointAgentChatBuffer).toHaveBeenCalledWith('buf1', { chatId: 'c2', runnerId: 'c1-r' })
  })

  it('moved ALSO invalidates the chat the runner LEFT — the frame names only the one it entered', async () => {
    listChatsFn.mockResolvedValue([chat('c1'), chat('c2')])
    buffers = [openTab('buf1', 'c1', 'c1-r')]
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    getChatFn.mockClear()

    captureCb()({ runnerId: 'c1-r', chatId: 'c2', workspaceId: 'w1', kind: 'moved' })
    await flush()

    // The destination — named by the frame.
    expect(getChatFn).toHaveBeenCalledWith('w1', 'c2')
    // The chat it VACATED — named by NOTHING. Miss this and c1 goes on advertising
    // a live runner that has gone: two chats claim one runner, the pane resolves to
    // the stale one, and the tab never follows.
    expect(getChatFn).toHaveBeenCalledWith('w1', 'c1')
  })

  it('moved does not refetch the vacated chat twice when the runner re-enters the chat it is already on', async () => {
    listChatsFn.mockResolvedValue([chat('c1')])
    buffers = [openTab('buf1', 'c1', 'c1-r')]
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    getChatFn.mockClear()

    captureCb()({ runnerId: 'c1-r', chatId: 'c1', workspaceId: 'w1', kind: 'moved' })
    await flush()

    expect(getChatFn).toHaveBeenCalledTimes(1)
  })

  it('moved onto a chat that already has a tab CLOSES the evicted tab and focuses the taker', async () => {
    listChatsFn.mockResolvedValue([chat('c1'), chat('c2')])
    buffers = [openTab('taker', 'c1', 'c1-r'), openTab('evicted', 'c2', 'c2-r')]
    panes = { p1: { id: 'p1', bufferIds: ['taker', 'evicted'] } }
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    captureCb()({ runnerId: 'c1-r', chatId: 'c2', workspaceId: 'w1', kind: 'moved' })
    await flush()

    // One tab per live conversation: the tab whose runner was pushed off c2 goes,
    // through the pane (so the pane never holds a dangling activeBufferId).
    expect(removeBufferFromPane).toHaveBeenCalledWith('p1', 'evicted')
    expect(closeBuffer).toHaveBeenCalledWith('evicted')
    expect(closeBuffer).not.toHaveBeenCalledWith('taker')
    // ...and the tab that took the conversation over is the one you are left looking at.
    expect(repointAgentChatBuffer).toHaveBeenCalledWith('taker', { chatId: 'c2', runnerId: 'c1-r' })
    expect(activatePaneBuffer).toHaveBeenCalledWith('p1', 'taker')
  })

  it('the eviction toast names the provider that was closed', async () => {
    listChatsFn.mockResolvedValue([chat('c1'), { ...chat('c2'), activeProviderId: 'codex' }])
    listProvidersFn.mockResolvedValue([
      { id: 'claude', displayName: 'Claude', icon: '<svg/>' },
      { id: 'codex', displayName: 'Codex', icon: '<svg/>' },
    ])
    buffers = [openTab('taker', 'c1', 'c1-r'), openTab('evicted', 'c2', 'c2-r')]
    panes = { p1: { id: 'p1', bufferIds: ['taker', 'evicted'] } }
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    captureCb()({ runnerId: 'c1-r', chatId: 'c2', workspaceId: 'w1', kind: 'moved' })
    await flush()

    expect(toastInfo).toHaveBeenCalledWith(
      'Conversation moved',
      'Codex was closed — that conversation is now in this terminal.',
    )
  })

  it('a move with nothing to evict closes no tab and shows no toast', async () => {
    listChatsFn.mockResolvedValue([chat('c1'), chat('c2')])
    buffers = [openTab('buf1', 'c1', 'c1-r')]
    panes = { p1: { id: 'p1', bufferIds: ['buf1'] } }
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    captureCb()({ runnerId: 'c1-r', chatId: 'c2', workspaceId: 'w1', kind: 'moved' })
    await flush()

    expect(closeBuffer).not.toHaveBeenCalled()
    expect(toastInfo).not.toHaveBeenCalled()
  })

  it('moved with no tab following that runner still invalidates both chats', async () => {
    listChatsFn.mockResolvedValue([chat('c1'), chat('c2')])
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    getChatFn.mockClear()

    captureCb()({ runnerId: 'c1-r', chatId: 'c2', workspaceId: 'w1', kind: 'moved' })
    await flush()

    expect(getChatFn).toHaveBeenCalledWith('w1', 'c2')
    expect(getChatFn).toHaveBeenCalledWith('w1', 'c1')
    expect(repointAgentChatBuffer).not.toHaveBeenCalled()
  })

  // ── displaced ──────────────────────────────────────────────────────────────
  // chatId is EMPTY on a displaced frame and that emptiness IS its meaning: Crowbar
  // has taken the CLI off its chat. The process may still be alive, so a client must
  // NOT wait for `exited` — if the kill failed, `exited` never comes.

  it('displaced (empty chatId) makes the tab following that runner LET GO — no exited required', async () => {
    listChatsFn.mockResolvedValue([chat('c1')])
    buffers = [openTab('buf1', 'c1', 'c1-r')]
    panes = { p1: { id: 'p1', bufferIds: ['buf1'] } }
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    getChatFn.mockClear()

    captureCb()({ runnerId: 'c1-r', chatId: '', workspaceId: 'w1', kind: 'displaced' })
    await flush()

    // The tab stops claiming the runner...
    expect(repointAgentChatBuffer).toHaveBeenCalledWith('buf1', { chatId: 'c1', runnerId: '' })
    // ...and the chat it held is re-read NOW (dormant, or whoever took it over) rather
    // than waiting for an `exited` that may never arrive.
    expect(getChatFn).toHaveBeenCalledWith('w1', 'c1')
  })

  it('displaced is idempotent — a dying CLI can emit it more than once', async () => {
    listChatsFn.mockResolvedValue([chat('c1')])
    buffers = [openTab('buf1', 'c1', 'c1-r')]
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    const onFrame = captureCb()
    onFrame({ runnerId: 'c1-r', chatId: '', workspaceId: 'w1', kind: 'displaced' })
    await flush()
    repointAgentChatBuffer.mockClear()

    onFrame({ runnerId: 'c1-r', chatId: '', workspaceId: 'w1', kind: 'displaced' })
    await flush()

    expect(repointAgentChatBuffer).not.toHaveBeenCalled()
  })

  it('displaced tolerates a runner the client never saw on any chat or tab', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    getChatFn.mockClear()

    captureCb()({ runnerId: 'ghost-r', chatId: '', workspaceId: 'w1', kind: 'displaced' })
    await flush()

    expect(repointAgentChatBuffer).not.toHaveBeenCalled()
    expect(getChatFn).not.toHaveBeenCalled()
  })

  // ── the other runner kinds ─────────────────────────────────────────────────

  it.each(['started', 'session_bound', 'exited'] as const)(
    'runner %s refetches the chat it names and touches no tab',
    async (kind) => {
      listChatsFn.mockResolvedValue([chat('c1')])
      buffers = [openTab('buf1', 'c1', 'c1-r')]
      renderHook(() => useWorkspaceAgentChatsStream('w1'))
      await flush()
      getChatFn.mockClear()

      captureCb()({ runnerId: 'c1-r', chatId: 'c1', workspaceId: 'w1', kind })
      await flush()

      expect(getChatFn).toHaveBeenCalledWith('w1', 'c1')
      expect(repointAgentChatBuffer).not.toHaveBeenCalled()
      expect(closeBuffer).not.toHaveBeenCalled()
    },
  )

  it('an exited that follows a displaced carries no chat and is dropped', async () => {
    listChatsFn.mockResolvedValue([chat('c1')])
    buffers = [openTab('buf1', 'c1', 'c1-r')]
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    getChatFn.mockClear()

    // The runner was displaced first, so its row already points at no chat. `displaced`
    // has already let go; there is nothing left to re-read.
    captureCb()({ runnerId: 'c1-r', chatId: '', workspaceId: 'w1', kind: 'exited' })
    await flush()

    expect(getChatFn).not.toHaveBeenCalled()
    expect(repointAgentChatBuffer).not.toHaveBeenCalled()
  })

  it('a CHAT frame is never routed as a runner frame (runnerId is the discriminator)', async () => {
    listChatsFn.mockResolvedValue([chat('c1')])
    buffers = [openTab('buf1', 'c1', 'c1-r')]
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    getChatFn.mockClear()

    // `session_bound` exists in BOTH vocabularies. This one carries no runnerId, so it
    // is the chat's — a refetch, and nothing done to any tab.
    captureCb()({ chatId: 'c1', workspaceId: 'w1', kind: 'session_bound' })
    await flush()

    expect(getChatFn).toHaveBeenCalledWith('w1', 'c1')
    expect(repointAgentChatBuffer).not.toHaveBeenCalled()
  })

  // ── Reconnect reconcile ────────────────────────────────────────────────────
  // The reseed after an outage is the ONLY repair for frames the socket dropped.
  // It must therefore hand the store an authoritative list (seedAgentChats
  // replaces + clears working) rather than a merge of upserts, and it must take the
  // pane tab of a chat deleted during the outage with it — exactly as the `deleted`
  // frame handler would have, had it arrived.

  it('reconnect reseed reconciles the list through seedAgentChats (drops the chat deleted during the outage)', async () => {
    listChatsFn.mockResolvedValue([chat('c1'), chat('c2')])
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    expect(seedAgentChats).toHaveBeenCalledWith([chat('c1'), chat('c2')])

    // c2 is deleted while the socket is down — its `deleted` frame is never seen.
    listChatsFn.mockResolvedValue([chat('c1')])
    seedAgentChats.mockClear()

    captureCb()({ reconnected: true })
    await flush()

    // The store is told the authoritative list; the slice drops c2 (and clears the
    // working map, so a dropped turn_stopped cannot strand c1's spinner).
    expect(seedAgentChats).toHaveBeenCalledWith([chat('c1')])
  })

  it('reconnect reseed closes the pane tab of a chat deleted during the outage', async () => {
    listChatsFn.mockResolvedValue([chat('c1'), chat('c2')])
    buffers = [{ id: 'buf-c2', type: 'agentChat', chatId: 'c2' }]
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    listChatsFn.mockResolvedValue([chat('c1')]) // c2 gone
    captureCb()({ reconnected: true })
    await flush()

    expect(closeBuffer).toHaveBeenCalledWith('buf-c2')
  })

  it('reconnect reseed leaves the pane tabs of surviving chats open', async () => {
    listChatsFn.mockResolvedValue([chat('c1')])
    buffers = [{ id: 'buf-c1', type: 'agentChat', chatId: 'c1' }]
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    captureCb()({ reconnected: true })
    await flush()

    expect(closeBuffer).not.toHaveBeenCalled()
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

  it('ignores a runner frame delivered after teardown', async () => {
    listChatsFn.mockResolvedValue([chat('c1')])
    buffers = [openTab('buf1', 'c1', 'c1-r')]
    const { unmount } = renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    const onFrame = captureCb()
    unmount()

    onFrame({ runnerId: 'c1-r', chatId: '', workspaceId: 'w1', kind: 'displaced' })
    await flush()

    expect(repointAgentChatBuffer).not.toHaveBeenCalled()
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
