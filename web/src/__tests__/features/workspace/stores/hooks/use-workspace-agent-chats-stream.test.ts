import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'

// Hoisted fakes — must be declared before any vi.mock calls.
const {
  subscribe,
  listChatsFn,
  getChatFn,
  listProvidersFn,
  listChatFoldersFn,
  seedAgentChats,
  notifyAgentChatMessages,
  seedAgentChatFolders,
  upsertAgentChat,
  removeAgentChat,
  setAgentChatWorking,
  setAgentChatTerminalWait,
  setAgentProviders,
  hydrateAgentChatOrder,
  closeBuffer,
  repointAgentChatBuffer,
  toastInfo,
  toastError,
} = vi.hoisted(() => ({
  subscribe: vi.fn(() => () => {}),
  listChatsFn: vi.fn(),
  getChatFn: vi.fn(),
  listProvidersFn: vi.fn(),
  listChatFoldersFn: vi.fn(),
  seedAgentChats: vi.fn(),
  notifyAgentChatMessages: vi.fn(),
  seedAgentChatFolders: vi.fn(),
  upsertAgentChat: vi.fn(),
  removeAgentChat: vi.fn(),
  setAgentChatWorking: vi.fn(),
  setAgentChatTerminalWait: vi.fn(),
  setAgentProviders: vi.fn(),
  hydrateAgentChatOrder: vi.fn(),
  closeBuffer: vi.fn(),
  repointAgentChatBuffer: vi.fn(),
  toastInfo: vi.fn(),
  toastError: vi.fn(),
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
  listChatFolders: (...a: unknown[]) => listChatFoldersFn(...a),
}))

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: {
    info: (...a: unknown[]) => toastInfo(...a),
    error: (...a: unknown[]) => toastError(...a),
  },
}))

vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getOrCreateWorkspaceStore: () => ({
    getState: () => ({
      agentChats: { chats: storeChats, providers: storeProviders },
      seedAgentChats,
      notifyAgentChatMessages,
      seedAgentChatFolders,
      upsertAgentChat,
      removeAgentChat,
      setAgentChatWorking,
      setAgentChatTerminalWait,
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
import {
  beginProviderWrite,
  useAgentProvidersStore,
} from '@/features/settings/stores/agent-providers-store'

type Frame = {
  chatId?: string
  workspaceId?: string
  kind?: string
  runnerId?: string
  folderId?: string
  reconnected?: boolean
  /** The server's folded busy state, carried on the chat kinds. Optional on the wire. */
  working?: boolean
  /** What the chat's CLI is blocked on that Crowbar cannot answer. Present on the
   *  `terminal_wait` kind only, and its ABSENCE there is the clearing edge. */
  terminalWait?: { kind: string }
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

const FOLDER = { id: 'f1', workspaceId: 'w1', parentId: '', name: 'Spikes', order: 0 }

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
  listChatFoldersFn.mockResolvedValue([FOLDER])
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
    listChatsFn.mockRejectedValue(new Error('boom'))
    listProvidersFn.mockRejectedValue(new Error('boom'))
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    expect(seedAgentChats).not.toHaveBeenCalled()
    expect(setAgentProviders).not.toHaveBeenCalled()
  })

  // ── The provider seed must be RECOVERABLE ──────────────────────────
  // agentChats.providers starts EMPTY, so one lost provider fetch — a daemon
  // restarting, a hot-reload remount, a transient socket error — used to leave it
  // empty for the whole life of the workspace, with nothing retrying and nothing
  // said. Every provider-dependent surface died at once: Settings → Providers
  // read "No providers available.", the sidebar's New chat row vanished, and both
  // the New Tab action and ⌘N silently did nothing. This is the reported live
  // failure: healthy daemon, both providers enabled in sqlite, empty UI.
  describe('provider seed recovery', () => {
    const providers = [{ id: 'claude', displayName: 'Claude', icon: '<svg/>' }]

    it('retries a failed provider fetch instead of giving up on first error', async () => {
      listProvidersFn.mockRejectedValueOnce(new Error('daemon restarting'))
      listProvidersFn.mockResolvedValue(providers)

      renderHook(() => useWorkspaceAgentChatsStream('w1'))
      await flush()

      expect(setAgentProviders).toHaveBeenCalledWith(providers)
    })

    it('says so when every attempt fails, instead of an empty UI with no explanation', async () => {
      listProvidersFn.mockRejectedValue(new Error('daemon is down'))

      renderHook(() => useWorkspaceAgentChatsStream('w1'))
      await flush()

      expect(setAgentProviders).not.toHaveBeenCalled()
      expect(toastError).toHaveBeenCalled()
    })

    it('re-seeds providers when the socket reconnects', async () => {
      // A daemon restart is exactly the case that empties them, and the socket
      // coming back is the app's own signal that it is answering again.
      listProvidersFn.mockRejectedValue(new Error('daemon is down'))
      renderHook(() => useWorkspaceAgentChatsStream('w1'))
      await flush()
      expect(setAgentProviders).not.toHaveBeenCalled()

      listProvidersFn.mockResolvedValue(providers)
      captureCb()({ reconnected: true })
      await flush()

      expect(setAgentProviders).toHaveBeenCalledWith(providers)
    })

    it('publishes the seeded list to the GLOBAL provider store as well', async () => {
      // Providers are machine-level and the Settings dialog is global, so the
      // per-workspace copy cannot be the only one: opening Settings with no
      // workspace in view read an empty list and said the daemon had none.
      listProvidersFn.mockResolvedValue(providers)

      renderHook(() => useWorkspaceAgentChatsStream('w1'))
      await flush()

      expect(useAgentProvidersStore.getState().providers).toEqual(providers)
      expect(useAgentProvidersStore.getState().status).toBe('ready')
    })

    it('lets only the LATEST provider read write — a stale one cannot overwrite it', async () => {
      let landOlder: (p: unknown[]) => void = () => {}
      listProvidersFn.mockReturnValueOnce(
        new Promise((resolve) => {
          landOlder = resolve as (p: unknown[]) => void
        }),
      )
      renderHook(() => useWorkspaceAgentChatsStream('w1'))
      await flush()

      // The reconnect's read is issued second and lands first.
      listProvidersFn.mockResolvedValue(providers)
      captureCb()({ reconnected: true })
      await flush()
      expect(setAgentProviders).toHaveBeenLastCalledWith(providers)

      landOlder([{ id: 'stale', displayName: 'Stale', icon: '' }])
      await flush()

      expect(setAgentProviders).toHaveBeenLastCalledWith(providers)
    })

    // A reseed is a GET, so it is a snapshot of the server BEFORE any
    // preferences PUT it overlaps. Sequencing reads against reads (the test
    // above) cannot see that: this read IS the latest read, and publishing it
    // would still undo the write — in the workspace copy the chat surfaces read
    // and in the global one the Settings tab renders, so the user watches their
    // Tools switch flip back on. See the write generation in
    // agent-providers-store.
    it('does not publish a reseed that a preferences write overtook', async () => {
      // The global store is module state a previous test in this file has
      // already written; start from a known empty list so "unchanged" is a fact
      // this test can assert exactly.
      useAgentProvidersStore.setState({ providers: [], status: 'idle' })
      let landReseed: (p: unknown[]) => void = () => {}
      listProvidersFn.mockReturnValueOnce(
        new Promise((resolve) => {
          landReseed = resolve as (p: unknown[]) => void
        }),
      )
      renderHook(() => useWorkspaceAgentChatsStream('w1'))
      await flush()
      expect(setAgentProviders).not.toHaveBeenCalled()

      // The user toggles a provider's Tools switch: a write is issued while this
      // reseed is still in flight.
      beginProviderWrite()

      landReseed([{ id: 'claude', displayName: 'Claude', icon: '<svg/>', mcpEnabled: true }])
      await flush()

      expect(setAgentProviders).not.toHaveBeenCalled()
      expect(useAgentProvidersStore.getState().providers).toEqual([])
    })
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

  it('turn_started/turn_stopped write the frame working state without a refetch', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    const onFrame = captureCb()

    onFrame({ chatId: 'c1', workspaceId: 'w1', kind: 'turn_started', working: true })
    expect(setAgentChatWorking).toHaveBeenCalledWith('c1', true)

    onFrame({ chatId: 'c1', workspaceId: 'w1', kind: 'turn_stopped', working: false })
    expect(setAgentChatWorking).toHaveBeenCalledWith('c1', false)

    // No round trip: these are the hottest frames on the feed and the spinner has to be
    // right the instant they land.
    expect(getChatFn).not.toHaveBeenCalled()
  })

  // THE BUG, at the FE seam. The backend fold alone did not fix the spinner: this hook
  // used to hardcode `turn_stopped -> false`, so the chat row went dark the moment claude
  // ended its turn to wait on a background subagent, no matter what the server said.
  //
  // `turn_stopped` is not "idle" — it is just the moment the answer can change, and the
  // answer is on the frame.
  it('keeps the spinner on for a turn_stopped that reports the chat still working', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    const onFrame = captureCb()

    // claude handed work to a background subagent and ended its turn — still working.
    onFrame({ chatId: 'c1', workspaceId: 'w1', kind: 'turn_stopped', working: true })

    expect(setAgentChatWorking).toHaveBeenCalledWith('c1', true)
    expect(setAgentChatWorking).not.toHaveBeenCalledWith('c1', false)
    expect(getChatFn).not.toHaveBeenCalled()
  })

  // The spinner must never be STUCK ON, which is worse than the bug it fixes. A frame
  // that omits `working` (an older daemon) reads as idle, never as spinning-forever.
  it('treats a frame with no working field as idle, never as stuck on', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    const onFrame = captureCb()

    onFrame({ chatId: 'c1', workspaceId: 'w1', kind: 'turn_stopped' })

    expect(setAgentChatWorking).toHaveBeenCalledWith('c1', false)
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

  it('created reseeds with keepWorking:true — a live-socket new chat must not blank other chats spinners', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    const onFrame = captureCb()
    seedAgentChats.mockClear()

    onFrame({ chatId: 'c2', workspaceId: 'w1', kind: 'created' })
    await flush()

    // A `created` reseed rides a LIVE socket, so it must KEEP newer frame state —
    // else a slightly stale list response can blank another mid-turn chat.
    expect(seedAgentChats).toHaveBeenCalledWith([chat('c1')], { keepWorking: true })
  })

  // TWO SEEDS RACING EACH OTHER. seedChats already refuses to be overtaken by a
  // per-chat READ (chatWrites), but nothing stopped it being overtaken by another
  // SEED — and a seed is a full REPLACE ("chats absent from the response are
  // DROPPED"). Two ⌘N presses issue two list reads; if the older one resolves
  // last it reinstates the list as it was before the second chat existed, and the
  // brand-new chat vanishes from the sidebar with nothing left to refetch it.
  it('a stale LIST seed must not drop a chat a newer seed already saw', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    // The two reads the two `created` frames issue, both held in flight.
    let landOlder: (chats: unknown[]) => void = () => {}
    let landNewer: (chats: unknown[]) => void = () => {}
    listChatsFn.mockReturnValueOnce(
      new Promise((resolve) => {
        landOlder = resolve as (chats: unknown[]) => void
      }),
    )
    listChatsFn.mockReturnValueOnce(
      new Promise((resolve) => {
        landNewer = resolve as (chats: unknown[]) => void
      }),
    )

    const onFrame = captureCb()
    onFrame({ chatId: 'c2', workspaceId: 'w1', kind: 'created' })
    onFrame({ chatId: 'c3', workspaceId: 'w1', kind: 'created' })
    await flush()

    // The NEWER read lands first and sees all three chats…
    landNewer([chat('c1'), chat('c2'), chat('c3')])
    await flush()
    // …then the OLDER one lands, taken before c3 was minted.
    landOlder([chat('c1'), chat('c2')])
    await flush()

    expect(storeChats.map((c) => c.id)).toEqual(['c1', 'c2', 'c3'])
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

  // THE /resume-INTO-AN-UNSEEN-CONVERSATION BUG.
  //
  // The CLI joins a conversation Crowbar has never seen, so the backend MINTS a chat and
  // MOVES the runner into it — two writes, on two aggregates, and therefore TWO FRAMES:
  // `created` (the chat) and, a beat later, `moved` (the runner that walked into it).
  //
  // `created` reseeds the whole list, so the LIST request goes out FIRST — and the daemon
  // can serve it in the window before the move is projected. Its payload is then already
  // stale: it shows the new chat with NO runner. The `moved` frame's single-chat read is
  // issued second, is fresh, and lands first. The stale list then arrives and REPLACES the
  // list wholesale (seedAgentChats assigns `chats`), silently overwriting the live chat
  // with the dormant snapshot of itself.
  //
  // Nothing refetches after that. The tab HAS followed the runner, so the pane is pointed
  // at a chat the store now says has no runner and no PTY — and it renders "This agent has
  // exited. Resume it…" over a CLI that is alive and typing. Which is the reported bug,
  // and it is intermittent for exactly the reason races are.
  //
  // A snapshot read BEFORE a single-chat read must never be applied AFTER it.
  it('a STALE list seed must not clobber the chat the runner just moved into', async () => {
    listChatsFn.mockResolvedValueOnce([chat('old')]) // the mount seed: old holds runner old-r
    buffers = [openTab('buf1', 'old', 'old-r')]
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    // Fresh single-chat reads: `new` has taken the runner, `old` is now dormant.
    getChatFn.mockImplementation((_ws: string, id: string) =>
      Promise.resolve(
        id === 'new'
          ? { ...chat('new'), liveRunnerId: 'old-r', terminalSessionId: 'old-pty' }
          : { ...chat('old'), liveRunnerId: '', terminalSessionId: '' },
      ),
    )
    // The list request `created` triggers: issued FIRST, served by the daemon BEFORE the
    // move was projected (so `new` still looks runnerless), and resolved LAST.
    let landStaleList: (chats: unknown[]) => void = () => {}
    listChatsFn.mockReturnValueOnce(
      new Promise((resolve) => {
        landStaleList = resolve as (chats: unknown[]) => void
      }),
    )
    // Any LATER list read is taken after those writes, so the daemon answers with the
    // truth — which is what makes discarding the overtaken snapshot and asking again a
    // real repair rather than a way to drop the reconcile.
    listChatsFn.mockResolvedValue([
      { ...chat('old'), liveRunnerId: '', terminalSessionId: '' },
      { ...chat('new'), liveRunnerId: 'old-r', terminalSessionId: 'old-pty' },
    ])

    const onFrame = captureCb()
    onFrame({ chatId: 'new', workspaceId: 'w1', kind: 'created' })
    onFrame({ chatId: 'new', workspaceId: 'w1', kind: 'moved', runnerId: 'old-r' })
    await flush()

    // The fresh reads have landed and the tab has followed the runner into `new`.
    expect(repointAgentChatBuffer).toHaveBeenCalledWith('buf1', {
      chatId: 'new',
      runnerId: 'old-r',
    })
    expect(storeChats.find((c) => c.id === 'new')?.liveRunnerId).toBe('old-r')

    landStaleList([
      { ...chat('old'), liveRunnerId: 'old-r', terminalSessionId: 'old-pty' }, // stale: still holds it
      { ...chat('new'), liveRunnerId: '', terminalSessionId: '' }, // stale: not yet moved into
    ])
    await flush()

    // The chat the pane is now showing must STILL have its runner and its PTY. Lose these
    // and the pane offers to Resume an agent that never left.
    expect(storeChats.find((c) => c.id === 'new')?.liveRunnerId).toBe('old-r')
    expect(storeChats.find((c) => c.id === 'new')?.terminalSessionId).toBe('old-pty')
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
  // replaces + reseeds working) rather than a merge of upserts, and it must take the
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

    // The store is told the authoritative list; the slice drops c2 and rebuilds
    // working from the server fold, so either missed turn edge is repaired.
    expect(notifyAgentChatMessages).toHaveBeenCalledTimes(1)
    expect(seedAgentChats).toHaveBeenCalledWith([chat('c1')])
  })

  it('reconnect invalidates current transcripts even when its repair GET fails', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    notifyAgentChatMessages.mockClear()
    seedAgentChats.mockClear()
    listChatsFn.mockRejectedValueOnce(new Error('daemon restarted again'))

    captureCb()({ reconnected: true })
    await flush()

    expect(notifyAgentChatMessages).toHaveBeenCalledTimes(1)
    expect(seedAgentChats).not.toHaveBeenCalled()
  })

  it('a created reseed that supersedes reconnect still performs authoritative working repair', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    seedAgentChats.mockClear()

    let resolveReconnect: (chats: ReturnType<typeof chat>[]) => void = () => {}
    let resolveCreated: (chats: ReturnType<typeof chat>[]) => void = () => {}
    listChatsFn
      .mockImplementationOnce(
        () =>
          new Promise<ReturnType<typeof chat>[]>((resolve) => {
            resolveReconnect = resolve
          }),
      )
      .mockImplementationOnce(
        () =>
          new Promise<ReturnType<typeof chat>[]>((resolve) => {
            resolveCreated = resolve
          }),
      )

    const onFrame = captureCb()
    onFrame({ reconnected: true })
    onFrame({ chatId: 'new', workspaceId: 'w1', kind: 'created' })

    resolveCreated([chat('c1'), chat('new')])
    await flush()
    expect(seedAgentChats).toHaveBeenCalledTimes(1)
    expect(seedAgentChats).toHaveBeenCalledWith([chat('c1'), chat('new')])

    resolveReconnect([chat('c1')])
    await flush()
    expect(seedAgentChats).toHaveBeenCalledTimes(1)
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

  it('closes a chat tab that no pane is holding', async () => {
    // A buffer can outlive its pane placement — the tab was closed out of the
    // pane but the buffer is still in the list. The delete has to drop it
    // anyway, and must not ask a pane that does not hold it to remove it.
    listChatsFn.mockResolvedValue([chat('c1')])
    buffers = [openTab('buf1', 'c1', 'c1-r')]
    panes = {}
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    captureCb()({ chatId: 'c1', workspaceId: 'w1', kind: 'deleted' })
    await flush()

    expect(removeAgentChat).toHaveBeenCalledWith('c1')
    expect(closeBuffer).toHaveBeenCalledWith('buf1')
    expect(removeBufferFromPane).not.toHaveBeenCalled()
  })

  it('re-points a taker whose own tab sits in no pane, without activating one', async () => {
    // The eviction path, with the surviving tab held by nothing: there is no
    // pane to bring forward, and asking a pane record that does not contain it
    // would activate some other pane's tab.
    listChatsFn.mockResolvedValue([chat('c1'), chat('c2')])
    buffers = [openTab('taker', 'c1', 'r1'), openTab('sitting', 'c2', 'r2')]
    panes = { p1: { id: 'p1', bufferIds: ['sitting'] } }
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    // r1 walks into c2, which `sitting` is already showing — so `sitting` is
    // evicted and `taker` follows the runner into it.
    captureCb()({ runnerId: 'r1', chatId: 'c2', workspaceId: 'w1', kind: 'moved' })
    await flush()

    expect(repointAgentChatBuffer).toHaveBeenCalledWith('taker', { chatId: 'c2', runnerId: 'r1' })
    expect(closeBuffer).toHaveBeenCalledWith('sitting')
    expect(activatePaneBuffer).not.toHaveBeenCalled()
  })

  it('does not re-read the chat a runner LEFT when it is the one it entered', async () => {
    // /resume back into the same conversation: the frame is a real move, and the
    // chat it names is both halves of it. One read, not two.
    listChatsFn.mockResolvedValue([chat('c1')])
    storeChats = [{ id: 'c1', liveRunnerId: 'r1' }]
    buffers = [openTab('buf1', 'c1', 'r1')]
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    getChatFn.mockClear()

    captureCb()({ runnerId: 'r1', chatId: 'c1', workspaceId: 'w1', kind: 'moved' })
    await flush()

    expect(getChatFn).toHaveBeenCalledTimes(1)
    expect(getChatFn).toHaveBeenCalledWith('w1', 'c1')
  })

  // ── `moved`, in the two shapes that name nothing to act on ─────────
  it('ignores a move that names no destination', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    getChatFn.mockClear()

    captureCb()({ runnerId: 'r1', chatId: '', workspaceId: 'w1', kind: 'moved' })
    await flush()

    // A move always names the chat it walked INTO; one that does not is a frame
    // about nothing, and refetching '' would ask the daemon for a chat with no id.
    expect(getChatFn).not.toHaveBeenCalled()
  })

  it('writes nothing for a move whose refetch lands after the workspace is gone', async () => {
    let release: (v: unknown) => void = () => {}
    getChatFn.mockReturnValueOnce(
      new Promise((resolve) => {
        release = resolve
      }),
    )
    buffers = [openTab('buf1', 'c1', 'r1')]
    const { unmount } = renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    repointAgentChatBuffer.mockClear()

    captureCb()({ runnerId: 'r1', chatId: 'c2', workspaceId: 'w1', kind: 'moved' })
    unmount()
    release({ ...chat('c2'), conversations: [] })
    await flush()

    // The tab belongs to a workspace nobody is looking at any more; re-pointing it
    // now would move a tab in a store the user has left.
    expect(repointAgentChatBuffer).not.toHaveBeenCalled()
  })

  // ── Folder frames: the tree is a SECOND aggregate on this one socket ────
  //
  // The daemon broadcasts folder mutations on the chats socket, carrying a
  // folderId and no row. The hook used to start its chat routing with
  // `if (!ev.chatId) return`, which dropped every one of them — so a folder made
  // in one window never appeared in another, and none of it survived a reconnect.
  describe('folder frames', () => {
    const folderFrame = (kind: string): Frame => ({
      folderId: 'f1',
      chatId: '',
      workspaceId: 'w1',
      kind,
    })

    it.each(['folder_created', 'folder_updated', 'folder_deleted'])(
      're-reads the folder list on %s',
      async (kind) => {
        renderHook(() => useWorkspaceAgentChatsStream('w1'))
        await flush()
        listChatFoldersFn.mockClear()
        seedAgentChatFolders.mockClear()

        captureCb()(folderFrame(kind))
        await flush()

        // The frame carries no row on purpose — this stream has no snapshot, and a
        // placement travelling on it would be a second truth that drifts from the
        // REST list. It says only that the tree moved.
        expect(listChatFoldersFn).toHaveBeenCalledWith('w1')
        expect(seedAgentChatFolders).toHaveBeenCalledWith([FOLDER])
      },
    )

    it('leaves the chat list alone — the tree moved, the conversations did not', async () => {
      renderHook(() => useWorkspaceAgentChatsStream('w1'))
      await flush()
      seedAgentChats.mockClear()
      getChatFn.mockClear()

      captureCb()(folderFrame('folder_created'))
      await flush()

      expect(seedAgentChats).not.toHaveBeenCalled()
      expect(getChatFn).not.toHaveBeenCalled()
    })

    it('re-reads folders on the reconnect sentinel, beside the chats', async () => {
      renderHook(() => useWorkspaceAgentChatsStream('w1'))
      await flush()
      listChatFoldersFn.mockClear()

      captureCb()({ reconnected: true })
      await flush()

      // Every folder frame dropped during the outage is a rearrangement this
      // client never heard about, and nothing else would ever ask again.
      expect(listChatFoldersFn).toHaveBeenCalledWith('w1')
      expect(seedAgentChatFolders).toHaveBeenCalledWith([FOLDER])
    })

    it('a failed folder read is non-fatal — the tree keeps the arrangement it has', async () => {
      renderHook(() => useWorkspaceAgentChatsStream('w1'))
      await flush()
      seedAgentChatFolders.mockClear()
      listChatFoldersFn.mockRejectedValue(new Error('boom'))

      captureCb()(folderFrame('folder_updated'))
      await flush()

      expect(seedAgentChatFolders).not.toHaveBeenCalled()
    })

    it('discards an older read that lands after a newer one', async () => {
      renderHook(() => useWorkspaceAgentChatsStream('w1'))
      await flush()
      seedAgentChatFolders.mockClear()

      // Two mutations in quick succession are two reads in flight, and resolution
      // order is not issue order: an older answer landing last would put the
      // folder that was just deleted back on screen.
      let releaseFirst: (v: unknown) => void = () => {}
      const first = new Promise((resolve) => {
        releaseFirst = resolve
      })
      listChatFoldersFn.mockReturnValueOnce(first).mockResolvedValueOnce([])

      const onFrame = captureCb()
      onFrame(folderFrame('folder_created'))
      onFrame(folderFrame('folder_deleted'))
      await flush()
      expect(seedAgentChatFolders).toHaveBeenCalledWith([])

      releaseFirst([FOLDER])
      await flush()

      // The stale answer is dropped, not applied on top of the fresh one.
      expect(seedAgentChatFolders).toHaveBeenCalledTimes(1)
    })

    it('writes nothing once the workspace has been unmounted', async () => {
      let release: (v: unknown) => void = () => {}
      const pending = new Promise((resolve) => {
        release = resolve
      })
      const { unmount } = renderHook(() => useWorkspaceAgentChatsStream('w1'))
      await flush()
      seedAgentChatFolders.mockClear()
      listChatFoldersFn.mockReturnValueOnce(pending)

      captureCb()(folderFrame('folder_created'))
      unmount()
      release([FOLDER])
      await flush()

      // A workspace switch is not a reason to write one workspace's tree into
      // another's store.
      expect(seedAgentChatFolders).not.toHaveBeenCalled()
    })
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

// ── terminal_wait: the modals no hook reports ─────────────────────────────
//
// A CLI parked on a workspace-trust dialog reports nothing through any hook, so
// this frame is the ONLY thing that tells a client its chat is blocked. Both
// edges ride the one kind, and the payload's absence is the clearing edge.

it('writes the wait through on a terminal_wait frame', () => {
  renderHook(() => useWorkspaceAgentChatsStream('w1'))

  captureCb()({
    chatId: 'c1',
    workspaceId: 'w1',
    kind: 'terminal_wait',
    terminalWait: { kind: 'workspace_trust' },
  })

  expect(setAgentChatTerminalWait).toHaveBeenCalledWith('c1', { kind: 'workspace_trust' })
})

it('clears the wait when the frame carries no payload', () => {
  renderHook(() => useWorkspaceAgentChatsStream('w1'))

  captureCb()({ chatId: 'c1', workspaceId: 'w1', kind: 'terminal_wait' })

  expect(setAgentChatTerminalWait).toHaveBeenCalledWith('c1', null)
})

// The frame carries the whole answer, so it must not cost a round trip: the user
// is looking at a pane that explains nothing, and a banner one request later is a
// banner after they gave up.
it('does not refetch the chat for a terminal_wait frame', () => {
  renderHook(() => useWorkspaceAgentChatsStream('w1'))

  captureCb()({
    chatId: 'c1',
    workspaceId: 'w1',
    kind: 'terminal_wait',
    terminalWait: { kind: '' },
  })

  expect(getChatFn).not.toHaveBeenCalled()
})
