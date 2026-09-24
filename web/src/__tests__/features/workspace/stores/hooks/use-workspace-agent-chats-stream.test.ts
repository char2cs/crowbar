import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook } from '@testing-library/react'

import type { AgentChat } from '@/features/agent/api/agent-api'
import {
  applySnapshot,
  reduceChatFrame,
  type ChatFrame,
} from '@/features/agent/lib/reduce-chat-frame'

// Hoisted fakes — must be declared before any vi.mock calls.
const {
  subscribe,
  chatBaseFn,
  listChatsFn,
  getChatFn,
  listProvidersFn,
  listChatFoldersFn,
  seedAgentChats,
  applyAgentChat,
  applyAgentChatFrame,
  notifyAgentChatMessages,
  seedAgentChatFolders,
  removeAgentChat,
  setAgentChatCompacting,
  setAgentChatTelemetry,
  setAgentChatPromptSettled,
  setAgentChatPromptAbandoned,
  setAgentChatStreamingMessage,
  setAgentChatStreamingReasoning,
  setAgentChatStreamingToolOutput,
  setAgentChatStreamingPlan,
  setAgentProviders,
  retargetPane,
  setPaneRunner,
  adoptBackgroundChat,
  setActivePane,
  forgetChat,
  toastInfo,
  toastError,
  resolveOwnerFn,
} = vi.hoisted(() => ({
  subscribe: vi.fn(() => () => {}),
  chatBaseFn: vi.fn(),
  listChatsFn: vi.fn(),
  getChatFn: vi.fn(),
  listProvidersFn: vi.fn(),
  listChatFoldersFn: vi.fn(),
  seedAgentChats: vi.fn(),
  applyAgentChat: vi.fn(),
  applyAgentChatFrame: vi.fn(),
  notifyAgentChatMessages: vi.fn(),
  seedAgentChatFolders: vi.fn(),
  removeAgentChat: vi.fn(),
  setAgentChatCompacting: vi.fn(),
  setAgentChatTelemetry: vi.fn(),
  setAgentChatPromptSettled: vi.fn(),
  setAgentChatPromptAbandoned: vi.fn(),
  setAgentChatStreamingMessage: vi.fn(),
  setAgentChatStreamingReasoning: vi.fn(),
  setAgentChatStreamingToolOutput: vi.fn(),
  setAgentChatStreamingPlan: vi.fn(),
  setAgentProviders: vi.fn(),
  retargetPane: vi.fn(),
  setPaneRunner: vi.fn(),
  adoptBackgroundChat: vi.fn(),
  setActivePane: vi.fn(),
  forgetChat: vi.fn(),
  toastInfo: vi.fn(),
  toastError: vi.fn(),
  resolveOwnerFn: vi.fn((): string | null => null),
}))

// Mutable fixtures the mocked stores' getState() reads from. The chat writes
// run the REAL reducer (reduce-chat-frame.ts) over `storeChats`, so the hook's
// own before/after reads — the edge an applied snapshot implies, the provider
// a move closed — see exactly what the slice would hold.
type FakePane = { id: string; chatId: string | null; runnerId: string | null }
let storeChats: AgentChat[] = []
let storeListSeeded = false
let storeProviders: Array<{ id: string; displayName: string; icon: string }> = []
// Mutated by the setAgentChatCompacting mock below — the hook's own self-heal
// check READS this back, so a write-only spy would not exercise that path.
let storeCompacting: Record<string, boolean> = {}
let panes: Record<string, FakePane> = {}

const openPane = (id: string, chatId: string, runnerId: string): FakePane => ({
  id,
  chatId,
  runnerId: runnerId || null,
})
const setPanes = (...list: FakePane[]) => {
  panes = Object.fromEntries(list.map((pane) => [pane.id, pane]))
}

vi.mock('@/lib/ws/manager', () => ({
  wsManager: { subscribe, send: vi.fn() },
}))

// chatBase is the real repo-scoped/home URL builder — the hook must go through
// it, not re-derive the shape itself. Faked here so the rest of this file stays
// agnostic of the URL shape; the first tests below assert the composition.
vi.mock('@/features/agent/api/agent-api', () => ({
  chatBase: (...a: unknown[]) => chatBaseFn(...a),
  listChats: (...a: unknown[]) => listChatsFn(...a),
  getChat: (...a: unknown[]) => getChatFn(...a),
  listProviders: (...a: unknown[]) => listProvidersFn(...a),
  listChatFolders: (...a: unknown[]) => listChatFoldersFn(...a),
  mapChat: (c: unknown) => c,
}))

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: {
    info: (...a: unknown[]) => toastInfo(...a),
    error: (...a: unknown[]) => toastError(...a),
  },
}))

vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  resolveChatOwnerWorkspaceId: (...a: unknown[]) => resolveOwnerFn(...(a as [])),
  getOrCreateWorkspaceStore: () => ({
    getState: () => ({
      agentChats: {
        chats: storeChats,
        listSeeded: storeListSeeded,
        providers: storeProviders,
        compacting: storeCompacting,
      },
      seedAgentChats,
      applyAgentChat,
      applyAgentChatFrame,
      notifyAgentChatMessages,
      seedAgentChatFolders,
      removeAgentChat,
      setAgentChatCompacting,
      setAgentChatTelemetry,
      setAgentChatPromptSettled,
      setAgentChatPromptAbandoned,
      setAgentChatStreamingMessage,
      setAgentChatStreamingReasoning,
      setAgentChatStreamingToolOutput,
      setAgentChatStreamingPlan,
      setAgentProviders,
    }),
  }),
}))

vi.mock('@/features/panes/stores/window-pane-store', () => ({
  windowPaneStore: {
    getState: () => ({
      panes,
      paneActions: { retargetPane, setPaneRunner, adoptBackgroundChat, setActivePane, forgetChat },
    }),
  },
}))

import {
  useWorkspaceAgentChatsStream,
  _resetProviderToastForTests,
} from '@/features/workspace/stores/hooks/use-workspace-agent-chats-stream'
import {
  beginProviderWrite,
  useAgentProvidersStore,
} from '@/features/settings/stores/agent-providers-store'
import { setWorkspaceScope, __resetWorkspaceScopesForTest } from '@/lib/workspace-scope'
import { ApiError } from '@/lib/api'
import { useFolderSignalStore } from '@/lib/store/folder-signal'

type Frame = {
  chatId?: string
  workspaceId?: string
  kind?: string
  runnerId?: string
  folderId?: string
  reconnected?: boolean
  /** The chat's whole snapshot, and the version that orders it. */
  chat?: AgentChat
  version?: number
  /** An assistant message still being produced. Present on `message_delta` only. */
  message?: { id: string; text: string; kind?: string }
  plan?: { text: string; status: string }[]
  telemetry?: { observedAt: string; source: string }
  clientRequestId?: string
  promptConsumed?: boolean
}

/** A chat row as a LIST read returns it: version 1, older than any frame below. */
const chat = (id: string, over: Partial<AgentChat> = {}): AgentChat => ({
  id,
  workspaceId: 'w1',
  title: id,
  liveRunnerId: `${id}-r`,
  terminalSessionId: `${id}-pty`,
  activeProviderId: 'claude',
  working: false,
  createdAt: '2026-01-01T00:00:00Z',
  order: 0,
  phase: 'live',
  version: 1,
  ...over,
})

// Every frame the daemon's snapshot owner sends is newer than the last.
let clock = 100
/** A frame carrying a fresh snapshot of `id` — the shape of every lifecycle frame. */
const live = (
  kind: string,
  id: string,
  over: Partial<AgentChat> = {},
  extra: Partial<Frame> = {},
): Frame => {
  const snap = chat(id, { version: ++clock, ...over })
  return { chatId: id, workspaceId: 'w1', kind, chat: snap, version: snap.version, ...extra }
}
const held = (id: string) => storeChats.find((c) => c.id === id)

const FOLDER = { id: 'f1', workspaceId: 'w1', parentId: '', name: 'Spikes', order: 0 }

// Drain the microtask queue. This is NOT a clock: every promise in the hook's
// chains resolves from an already-settled mock, so a fixed number of ticks
// settles them deterministically.
const flush = async () => {
  for (let i = 0; i < 8; i++) await Promise.resolve()
}

function captureCb(callIndex = 0): (frame: Frame) => void {
  const call = subscribe.mock.calls[callIndex] as unknown as [string, (frame: Frame) => void]
  return call[1]
}

// Mirrors agent-api.ts's own chatBase branching: 'ws-home' is a project-home
// workspace (repoId ''), everything else carries a real repo.
const fakeChatBase = (id: string) =>
  id === 'ws-home' ? '/v0/projects/p1/home/chats' : '/v0/projects/p1/repos/r1/chats'

beforeEach(() => {
  vi.clearAllMocks()
  _resetProviderToastForTests()
  subscribe.mockReturnValue(() => {})
  chatBaseFn.mockImplementation(fakeChatBase)
  panes = {}
  storeChats = []
  storeListSeeded = false
  storeProviders = []
  storeCompacting = {}
  setAgentChatCompacting.mockImplementation((chatId: string, active: boolean) => {
    if (active) storeCompacting[chatId] = true
    else delete storeCompacting[chatId]
  })
  // The slice's chat writes, over the real reducer.
  applyAgentChat.mockImplementation((c: AgentChat) => {
    const next = applySnapshot(storeChats, c)
    storeChats = [...next.chats]
    return next.outcome
  })
  applyAgentChatFrame.mockImplementation((f: ChatFrame) => {
    const next = reduceChatFrame(storeChats, f)
    storeChats = [...next.chats]
    return next.outcome
  })
  seedAgentChats.mockImplementation((chats: AgentChat[]) => {
    const listed = new Set(chats.map((c) => c.id))
    const vanished = storeChats.filter((c) => !listed.has(c.id)).map((c) => c.id)
    for (const c of chats) storeChats = [...applySnapshot(storeChats, c).chats]
    storeListSeeded = true
    return vanished
  })
  removeAgentChat.mockImplementation((chatId: string) => {
    storeChats = storeChats.filter((c) => c.id !== chatId)
  })
  setAgentProviders.mockImplementation((p: typeof storeProviders) => {
    storeProviders = p
  })
  // The real slice mutates the pane in place; model that, so a second frame
  // (the idempotence cases) sees the state the first one left behind.
  retargetPane.mockImplementation((paneId: string, chatId: string, runnerId: string | null) => {
    for (const other of Object.values(panes)) {
      if (other.id !== paneId && other.chatId === chatId) delete panes[other.id]
    }
    const pane = panes[paneId]
    if (!pane) return
    pane.chatId = chatId
    pane.runnerId = runnerId
  })
  setPaneRunner.mockImplementation((paneId: string, runnerId: string | null) => {
    const pane = panes[paneId]
    if (pane) pane.runnerId = runnerId
  })
  forgetChat.mockImplementation((chatId: string) => {
    for (const pane of Object.values(panes)) {
      if (pane.chatId === chatId) delete panes[pane.id]
    }
  })
  listChatsFn.mockResolvedValue([chat('c1')])
  getChatFn.mockImplementation((_wsId: string, id: string) =>
    Promise.resolve(chat(id, { version: ++clock })),
  )
  listProvidersFn.mockResolvedValue([{ id: 'claude', displayName: 'Claude', icon: '<svg/>' }])
  listChatFoldersFn.mockResolvedValue([FOLDER])
  __resetWorkspaceScopesForTest()
  setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'w1' })
  useFolderSignalStore.setState({ generations: {} })
})

describe('useWorkspaceAgentChatsStream', () => {
  it('subscribes to chatBase(wsId)/ws — the same repo-scoped base agent-api.ts REST calls use', () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    expect(chatBaseFn).toHaveBeenCalledWith('w1')
    expect(subscribe).toHaveBeenCalledWith(
      '/v0/projects/p1/repos/r1/chats/ws',
      expect.any(Function),
    )
  })

  // The gap this closes: the hook used to build `${workspaceBase(wsId)}/chats/ws`
  // itself, independently of agent-api.ts's chatBase — so Task 17's rescope left
  // this one WS subscription still dialing the removed `/workspaces/:wsId/chats/ws`
  // route for every non-home workspace even after agent-api.ts's own REST calls
  // were fixed. Going through chatBase means this can never drift from the REST
  // routes again; chatBase's own branching is unit-tested in agent-api.test.ts.
  it('builds the repo-scoped WS URL for a non-home workspace, and the unchanged home shape for a home one', () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    expect(subscribe).toHaveBeenCalledWith(
      '/v0/projects/p1/repos/r1/chats/ws',
      expect.any(Function),
    )
    expect((subscribe.mock.calls[0] as unknown as [string])[0]).not.toContain('/workspaces/')

    // Its own scope, not 'w1''s: the hook now waits on `useWorkspaceScopeReady`
    // (workspace-scope.ts), which is keyed per wsId.
    setWorkspaceScope({ projectId: 'p1', repoId: '', wsId: 'ws-home' })
    renderHook(() => useWorkspaceAgentChatsStream('ws-home'))
    expect(subscribe).toHaveBeenLastCalledWith(
      '/v0/projects/p1/home/chats/ws',
      expect.any(Function),
    )
  })

  // Live-reported: WorkspaceHost force-mounts a workspace's effects the
  // instant pane/Recents state names it — regardless of `active`, and
  // regardless of whether the sidebar's own repo fetch has recorded that
  // workspace's scope yet (a genuine race, most reproducible right after a
  // cold boot). This hook used to call `chatBase(wsId)` — agent-api.ts's
  // `repoChatsBaseForWorkspace`, which falls through to `workspaceBase` and
  // throws for an entirely unrecorded scope — completely unguarded, tripping
  // the ErrorBoundary with "no project/repo scope recorded for workspace …"
  // for whichever background workspace lost that race. `chatBaseFn` stays
  // mocked (this file's own convention — see its own doc), so these assert
  // the hook's OWN gating rather than the real URL shape: with no scope
  // recorded, `subscribe`/`listChatsFn`/`listProvidersFn` must never fire.
  describe('scope readiness', () => {
    it('does not subscribe or fetch when the workspace scope is not yet recorded', async () => {
      __resetWorkspaceScopesForTest()

      expect(() => renderHook(() => useWorkspaceAgentChatsStream('ws-unrecorded'))).not.toThrow()
      await flush()

      expect(subscribe).not.toHaveBeenCalled()
      expect(listChatsFn).not.toHaveBeenCalled()
      expect(listProvidersFn).not.toHaveBeenCalled()

      // Restore for later tests in this file.
      setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'w1' })
    })

    it('subscribes and seeds once the scope arrives after mounting with none recorded', async () => {
      __resetWorkspaceScopesForTest()

      renderHook(() => useWorkspaceAgentChatsStream('ws-late'))
      await flush()
      expect(subscribe).not.toHaveBeenCalled()

      setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-late' })
      await flush()

      expect(listChatsFn).toHaveBeenCalledWith('ws-late')
      expect(subscribe).toHaveBeenCalledWith(
        '/v0/projects/p1/repos/r1/chats/ws',
        expect.any(Function),
      )

      // Restore for later tests in this file.
      setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'w1' })
    })
  })

  it('seeds chats + providers on mount and populates the slice', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    expect(listChatsFn).toHaveBeenCalledWith('w1')
    expect(seedAgentChats).toHaveBeenCalledWith([chat('c1')])
    expect(held('c1')).toEqual(chat('c1'))
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

    // The hook runs for every MOUNTED workspace now (WorkspaceView, up to
    // RETENTION_CAP = 6), and the daemon being unreachable fails all of them
    // at once — for the same machine-level list, with the same sentence. Its
    // only previous mount point was a single sidebar panel, so the plain toast
    // was correct then and would stack six identical copies now.
    it('says it ONCE per outage, not once per mounted workspace', async () => {
      listProvidersFn.mockRejectedValue(new Error('daemon is down'))

      renderHook(() => useWorkspaceAgentChatsStream('w1'))
      renderHook(() => useWorkspaceAgentChatsStream('w2'))
      renderHook(() => useWorkspaceAgentChatsStream('w3'))
      await flush()

      expect(toastError).toHaveBeenCalledTimes(1)
    })

    it('re-arms the announcement once the daemon answers again', async () => {
      // w2/w3 stand in for "some other workspace mount" here — the toast dedup
      // this test exercises is module-level, not scope-specific — so each just
      // needs ITS OWN recorded scope for `useWorkspaceScopeReady` to let the
      // effect run at all.
      setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'w2' })
      setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'w3' })

      listProvidersFn.mockRejectedValue(new Error('daemon is down'))
      const first = renderHook(() => useWorkspaceAgentChatsStream('w1'))
      await flush()
      expect(toastError).toHaveBeenCalledTimes(1)
      first.unmount()

      // Recovered...
      listProvidersFn.mockResolvedValue(providers)
      renderHook(() => useWorkspaceAgentChatsStream('w2'))
      await flush()

      // ...and down again: the user is told about the SECOND outage too.
      listProvidersFn.mockRejectedValue(new Error('daemon is down again'))
      renderHook(() => useWorkspaceAgentChatsStream('w3'))
      await flush()

      expect(toastError).toHaveBeenCalledTimes(2)
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

  // ── THE RULE: a snapshot applies only if its version is newer (A6) ──────────
  // Every lifecycle frame carries the chat's whole snapshot, and every read
  // answers one. Whichever lands last, the newer version is what the store holds
  // — so nothing here refetches, sequences reads, or guesses from the kind.

  it('turn frames apply the snapshot they carry, with no round trip', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    const onFrame = captureCb()

    onFrame(live('turn_started', 'c1', { working: true }))
    expect(held('c1')?.working).toBe(true)

    onFrame(live('turn_stopped', 'c1', { working: false }))
    expect(held('c1')?.working).toBe(false)

    expect(getChatFn).not.toHaveBeenCalled()
    expect(listChatsFn).toHaveBeenCalledTimes(1)
  })

  // `turn_stopped` is not "idle" — claude can end its turn to wait on a
  // background subagent. The answer is the snapshot's, never the kind's.
  it('keeps the spinner on for a turn_stopped whose snapshot is still working', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    captureCb()(live('turn_stopped', 'c1', { working: true }))

    expect(held('c1')?.working).toBe(true)
  })

  // THE /resume-INTO-AN-UNSEEN-CONVERSATION BUG, now structurally impossible:
  // a list served before the move was projected lands AFTER the move's frame.
  // Its row is an older version, so it is not applied.
  it('a list read older than a frame cannot clobber the chat the runner moved into', async () => {
    let land: (chats: AgentChat[]) => void = () => {}
    listChatsFn.mockImplementationOnce(
      () =>
        new Promise<AgentChat[]>((r) => {
          land = r
        }),
    )
    renderHook(() => useWorkspaceAgentChatsStream('w1'))

    captureCb()(live('moved', 'c2', { liveRunnerId: 'r9' }, { runnerId: 'r9' }))
    land([chat('c1'), chat('c2', { liveRunnerId: '', phase: 'dormant' })])
    await flush()

    expect(held('c2')?.liveRunnerId).toBe('r9')
    expect(held('c1')).toBeDefined()
  })

  it('an older frame delivered late is dropped', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    const onFrame = captureCb()
    const older = live('turn_started', 'c1', { working: true })
    const newer = live('turn_stopped', 'c1', { working: false })

    onFrame(newer)
    onFrame(older)

    expect(held('c1')?.working).toBe(false)
  })

  it('a snapshot carrying a terminal wait is applied as it lands', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    captureCb()(live('snapshot', 'c1', { terminalWait: { kind: 'workspace_trust' } }))

    expect(held('c1')?.terminalWait).toEqual({ kind: 'workspace_trust' })
    expect(getChatFn).not.toHaveBeenCalled()
  })

  // ── Background adoption: a LIVE rising edge only ────────────────────────────

  it('a chat that starts working with no pane gets a background row, once', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    const onFrame = captureCb()
    onFrame(live('turn_started', 'c1', { working: true }))
    onFrame(live('turn_stopped', 'c1', { working: true }))

    // Only the not-working → working edge adopts; a repeat mints nothing.
    expect(adoptBackgroundChat).toHaveBeenCalledTimes(1)
    expect(adoptBackgroundChat).toHaveBeenCalledWith('c1', 'p1')
  })

  it('a working chat that already has a pane adopts nothing', async () => {
    setPanes(openPane('p1', 'c1', ''))
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    captureCb()(live('turn_started', 'c1', { working: true }))

    expect(adoptBackgroundChat).not.toHaveBeenCalled()
  })

  it('a working frame that beats the boot list adopts nothing', async () => {
    let land: (chats: AgentChat[]) => void = () => {}
    listChatsFn.mockImplementationOnce(
      () =>
        new Promise<AgentChat[]>((r) => {
          land = r
        }),
    )
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    const onFrame = captureCb()
    onFrame(live('turn_started', 'c1', { working: true }))
    land([chat('c1')])
    await flush()
    onFrame(live('turn_stopped', 'c1', { working: true }))

    expect(adoptBackgroundChat).not.toHaveBeenCalled()
  })

  // A first sight through a read (boot, remount, reconnect) is not an edge.
  it('a list read that finds a chat already working adopts nothing', async () => {
    listChatsFn.mockResolvedValue([chat('c1', { working: true })])
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    listChatsFn.mockResolvedValue([chat('c1', { working: true, version: ++clock })])

    captureCb()({ reconnected: true })
    await flush()

    expect(adoptBackgroundChat).not.toHaveBeenCalled()
  })

  it('a chat born live on this stream adopts exactly once', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    const onFrame = captureCb()

    onFrame(live('created', 'new', { liveRunnerId: '' }))
    onFrame(live('started', 'new', {}, { runnerId: 'new-r' }))
    onFrame(live('turn_started', 'new', { working: true }))
    onFrame(live('turn_stopped', 'new', { working: true }))

    expect(adoptBackgroundChat.mock.calls).toEqual([['new', 'p1']])
    // Structural frames never cost a list read: the snapshot IS the row.
    expect(listChatsFn).toHaveBeenCalledTimes(1)
  })

  // Regression (shipped and reverted): clearing streamingMessages on a turn
  // edge threw away the still-alive output of an interrupted CLI that goes on to
  // complete its OWN turn. Only the ledger dedup retires those entries.
  it('neither turn_started nor turn_stopped touch streamingMessages — only the ledger dedup does', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    const onFrame = captureCb()

    onFrame(live('turn_started', 'c1', { working: true }))
    onFrame(live('turn_stopped', 'c1', { working: false }))

    expect(setAgentChatStreamingMessage).not.toHaveBeenCalled()
  })

  describe('message_delta batching', () => {
    const nextFrame = () => new Promise((resolve) => requestAnimationFrame(resolve))

    it('does not write to the store synchronously — it waits for the next frame', async () => {
      renderHook(() => useWorkspaceAgentChatsStream('w1'))
      await flush()
      const onFrame = captureCb()

      onFrame({
        chatId: 'c1',
        workspaceId: 'w1',
        kind: 'message_delta',
        message: { id: 'm1', text: 'Bui' },
      })

      expect(setAgentChatStreamingMessage).not.toHaveBeenCalled()
      await nextFrame()
      expect(setAgentChatStreamingMessage).toHaveBeenCalledWith('c1', { id: 'm1', text: 'Bui' })
    })

    // A THOUGHT is not an answer. It rides the same frame kind, tagged, and must
    // never reach streamingMessages: nothing in the ledger will ever match it, so
    // useChatMessages' prune-against-the-ledger pass could not retire it and it
    // would sit in the transcript as an assistant bubble forever.
    it('routes a reasoning delta to its own slot, never to the message stream', async () => {
      renderHook(() => useWorkspaceAgentChatsStream('w1'))
      await flush()
      const onFrame = captureCb()

      onFrame({
        chatId: 'c1',
        workspaceId: 'w1',
        kind: 'message_delta',
        message: { id: 'rs_1', text: '**Clarifying**', kind: 'reasoning' },
      })
      await nextFrame()

      expect(setAgentChatStreamingReasoning).toHaveBeenCalledWith('c1', {
        id: 'rs_1',
        text: '**Clarifying**',
      })
      expect(setAgentChatStreamingMessage).not.toHaveBeenCalled()
    })

    it("routes a tool's output delta to its own slot", async () => {
      renderHook(() => useWorkspaceAgentChatsStream('w1'))
      await flush()
      const onFrame = captureCb()

      onFrame({
        chatId: 'c1',
        workspaceId: 'w1',
        kind: 'message_delta',
        message: { id: 'call_1', text: 'line 1\n', kind: 'tool_output' },
      })
      await nextFrame()

      expect(setAgentChatStreamingToolOutput).toHaveBeenCalledWith('c1', {
        id: 'call_1',
        text: 'line 1\n',
      })
      expect(setAgentChatStreamingMessage).not.toHaveBeenCalled()
    })

    // The plan arrives WHOLESALE — the newest list is the entire truth, so this
    // is a replace and a missed frame costs nothing.
    it("replaces the agent's to-do list wholesale", async () => {
      renderHook(() => useWorkspaceAgentChatsStream('w1'))
      await flush()
      const onFrame = captureCb()

      onFrame({
        chatId: 'c1',
        workspaceId: 'w1',
        kind: 'plan',
        plan: [
          { text: 'Run the command', status: 'active' },
          { text: 'Summarise', status: 'pending' },
        ],
      })

      expect(setAgentChatStreamingPlan).toHaveBeenCalledWith('c1', [
        { text: 'Run the command', status: 'active' },
        { text: 'Summarise', status: 'pending' },
      ])
    })

    // The thought belongs to the turn that produced it — a stale one outliving
    // its turn would claim the agent is mid-thought when it has already answered.
    it('drops the thought at a turn edge', async () => {
      renderHook(() => useWorkspaceAgentChatsStream('w1'))
      await flush()
      const onFrame = captureCb()

      onFrame(live('turn_stopped', 'c1', { working: false }))

      expect(setAgentChatStreamingReasoning).toHaveBeenCalledWith('c1', null)
    })

    it('collapses several deltas arriving before the frame into one write, with the latest text', async () => {
      renderHook(() => useWorkspaceAgentChatsStream('w1'))
      await flush()
      const onFrame = captureCb()

      onFrame({
        chatId: 'c1',
        workspaceId: 'w1',
        kind: 'message_delta',
        message: { id: 'm1', text: 'B' },
      })
      onFrame({
        chatId: 'c1',
        workspaceId: 'w1',
        kind: 'message_delta',
        message: { id: 'm1', text: 'Bu' },
      })
      onFrame({
        chatId: 'c1',
        workspaceId: 'w1',
        kind: 'message_delta',
        message: { id: 'm1', text: 'Bui' },
      })
      await nextFrame()

      expect(setAgentChatStreamingMessage).toHaveBeenCalledTimes(1)
      expect(setAgentChatStreamingMessage).toHaveBeenCalledWith('c1', { id: 'm1', text: 'Bui' })
    })

    it('ignores a frame missing the message payload', async () => {
      renderHook(() => useWorkspaceAgentChatsStream('w1'))
      await flush()
      const onFrame = captureCb()

      onFrame({ chatId: 'c1', workspaceId: 'w1', kind: 'message_delta' })
      await nextFrame()

      expect(setAgentChatStreamingMessage).not.toHaveBeenCalled()
    })

    it('drops a still-pending delta on unmount rather than writing it late', async () => {
      const { unmount } = renderHook(() => useWorkspaceAgentChatsStream('w1'))
      await flush()
      const onFrame = captureCb()

      onFrame({
        chatId: 'c1',
        workspaceId: 'w1',
        kind: 'message_delta',
        message: { id: 'm1', text: 'Bui' },
      })
      unmount()
      await nextFrame()

      expect(setAgentChatStreamingMessage).not.toHaveBeenCalled()
    })
  })

  it('deleted removes the chat AND the pane holding it (spec §9)', async () => {
    setPanes(openPane('p1', 'c1', 'c1-r'), openPane('p2', 'other', 'other-r'))
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    captureCb()({ chatId: 'c1', workspaceId: 'w1', kind: 'deleted', version: ++clock })

    expect(held('c1')).toBeUndefined()
    // Deletion is the only act that removes a THING, so it is the only one that
    // can leave a name behind — `forgetChat` clears every pane holding it.
    expect(forgetChat).toHaveBeenCalledWith('c1')
    expect(panes.p1).toBeUndefined()
    expect(panes.p2.chatId).toBe('other')
  })

  it('a delete older than the snapshot held is dropped', async () => {
    setPanes(openPane('p1', 'c1', 'c1-r'))
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    const onFrame = captureCb()
    const staleVersion = ++clock

    onFrame(live('title_set', 'c1'))
    onFrame({ chatId: 'c1', workspaceId: 'w1', kind: 'deleted', version: staleVersion })

    expect(held('c1')).toBeDefined()
    expect(forgetChat).not.toHaveBeenCalled()
  })

  it('ignores a chat frame missing chatId', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    captureCb()({ workspaceId: 'w1', kind: 'turn_started' })

    expect(applyAgentChatFrame).not.toHaveBeenCalled()
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
  // The vendor CLI is a PROCESS, and it MOVES: /clear or /resume inside it lands
  // the runner on another chat. The daemon sends the snapshot of the chat it
  // entered (kind `moved`, runnerId set) and a `snapshot` of the chat it left.

  it('moved re-points the PANE following that runner at the chat it ENTERED', async () => {
    listChatsFn.mockResolvedValue([chat('c1'), chat('c2')])
    setPanes(openPane('p1', 'c1', 'c1-r'))
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    captureCb()(live('moved', 'c2', { liveRunnerId: 'c1-r' }, { runnerId: 'c1-r' }))

    expect(retargetPane).toHaveBeenCalledWith('p1', 'c2', 'c1-r')
    expect(getChatFn).not.toHaveBeenCalled()
  })

  it('the snapshot of the chat a runner LEFT does not pull the pane back', async () => {
    listChatsFn.mockResolvedValue([chat('c1'), chat('c2')])
    setPanes(openPane('p1', 'c1', 'c1-r'))
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    const onFrame = captureCb()

    onFrame(live('moved', 'c2', { liveRunnerId: 'c1-r' }, { runnerId: 'c1-r' }))
    onFrame(live('snapshot', 'c1', { liveRunnerId: '', phase: 'dormant' }, { runnerId: 'c1-r' }))

    expect(panes.p1).toEqual({ id: 'p1', chatId: 'c2', runnerId: 'c1-r' })
    expect(setPaneRunner).not.toHaveBeenCalled()
    expect(held('c1')?.liveRunnerId).toBe('')
  })

  it('a move older than the snapshot held re-points nothing', async () => {
    listChatsFn.mockResolvedValue([chat('c1'), chat('c2')])
    setPanes(openPane('p1', 'c1', 'c1-r'))
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    const onFrame = captureCb()
    const stale = live('moved', 'c2', { liveRunnerId: 'c1-r' }, { runnerId: 'c1-r' })

    onFrame(live('title_set', 'c2'))
    onFrame(stale)

    expect(retargetPane).not.toHaveBeenCalled()
  })

  it('moved onto a chat another pane already holds REMOVES that pane and focuses the taker', async () => {
    listChatsFn.mockResolvedValue([chat('c1'), chat('c2')])
    setPanes(openPane('taker', 'c1', 'c1-r'), openPane('evicted', 'c2', 'c2-r'))
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    captureCb()(live('moved', 'c2', { liveRunnerId: 'c1-r' }, { runnerId: 'c1-r' }))

    // One pane per live conversation: one retarget moves the taker onto c2 and
    // removes the pane that held it, and the taker is what you are left on.
    expect(retargetPane).toHaveBeenCalledWith('taker', 'c2', 'c1-r')
    expect(panes.evicted).toBeUndefined()
    expect(setActivePane).toHaveBeenCalledWith('taker')
  })

  it('the eviction toast names the provider that was closed', async () => {
    listChatsFn.mockResolvedValue([chat('c1'), chat('c2', { activeProviderId: 'codex' })])
    listProvidersFn.mockResolvedValue([
      { id: 'claude', displayName: 'Claude', icon: '<svg/>' },
      { id: 'codex', displayName: 'Codex', icon: '<svg/>' },
    ])
    setPanes(openPane('taker', 'c1', 'c1-r'), openPane('evicted', 'c2', 'c2-r'))
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    // The arriving snapshot names claude; the toast must name what was CLOSED.
    captureCb()(live('moved', 'c2', { liveRunnerId: 'c1-r' }, { runnerId: 'c1-r' }))

    expect(toastInfo).toHaveBeenCalledWith(
      'Conversation moved',
      'Codex was closed — that conversation is now in this pane.',
    )
  })

  it('a move with nothing to evict empties no pane and shows no toast', async () => {
    listChatsFn.mockResolvedValue([chat('c1'), chat('c2')])
    setPanes(openPane('p1', 'c1', 'c1-r'))
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    captureCb()(live('moved', 'c2', { liveRunnerId: 'c1-r' }, { runnerId: 'c1-r' }))

    expect(setActivePane).not.toHaveBeenCalled()
    expect(toastInfo).not.toHaveBeenCalled()
  })

  it('moved with no pane following that runner re-points nothing', async () => {
    listChatsFn.mockResolvedValue([chat('c1'), chat('c2')])
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    captureCb()(live('moved', 'c2', { liveRunnerId: 'c1-r' }, { runnerId: 'c1-r' }))

    expect(held('c2')?.liveRunnerId).toBe('c1-r')
    expect(retargetPane).not.toHaveBeenCalled()
    expect(setPaneRunner).not.toHaveBeenCalled()
  })

  // ── A runner taken off its chat ────────────────────────────────────────────
  // Displacement asserts nothing about liveness, and an `exited` may never come:
  // the snapshot that drops the runner is enough for the pane to let go.

  it('a snapshot that takes the runner off the chat makes its pane LET GO', async () => {
    setPanes(openPane('p1', 'c1', 'c1-r'))
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    captureCb()(
      live('snapshot', 'c1', { liveRunnerId: '', phase: 'dormant' }, { runnerId: 'c1-r' }),
    )

    // The pane KEEPS its chat and stops claiming the runner.
    expect(setPaneRunner).toHaveBeenCalledWith('p1', null)
    expect(panes.p1.chatId).toBe('c1')
    expect(getChatFn).not.toHaveBeenCalled()
  })

  it('letting go is idempotent — a dying CLI can be reported more than once', async () => {
    setPanes(openPane('p1', 'c1', 'c1-r'))
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    const onFrame = captureCb()

    onFrame(live('displaced', 'c1', { liveRunnerId: '' }, { runnerId: 'c1-r' }))
    setPaneRunner.mockClear()
    onFrame(live('exited', 'c1', { liveRunnerId: '' }, { runnerId: 'c1-r' }))

    expect(retargetPane).not.toHaveBeenCalled()
    expect(setPaneRunner).not.toHaveBeenCalled()
  })

  // The real shape: a prompt to a mixed-transport CLI displaces the outgoing
  // runner and spawns its replacement inside one backend call. A late,
  // OLDER snapshot of the displacement must not undo the replacement.
  it('a late displacement cannot clobber the replacement a newer frame confirmed', async () => {
    setPanes(openPane('p1', 'c1', 'c1-r'))
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    const onFrame = captureCb()
    const displaced = live('snapshot', 'c1', { liveRunnerId: '' }, { runnerId: 'c1-r' })
    const started = live('started', 'c1', { liveRunnerId: 'r2' }, { runnerId: 'r2' })

    onFrame(started)
    setPaneRunner.mockClear()
    onFrame(displaced)

    expect(held('c1')?.liveRunnerId).toBe('r2')
    expect(setPaneRunner).not.toHaveBeenCalled()
  })

  it('a chat-scoped session_bound applies its snapshot and touches no pane', async () => {
    setPanes(openPane('p1', 'c1', 'c1-r'))
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    captureCb()(live('session_bound', 'c1', { title: 'bound' }))

    expect(held('c1')?.title).toBe('bound')
    expect(getChatFn).not.toHaveBeenCalled()
    expect(retargetPane).not.toHaveBeenCalled()
    expect(setPaneRunner).not.toHaveBeenCalled()
  })

  // ── Reconnect reconcile ────────────────────────────────────────────────────
  // The reseed after an outage is the ONLY repair for frames the socket
  // dropped. Its rows apply under the same version rule; a chat it omits is a
  // SUSPECT, confirmed gone only by a definite not-found on the chat itself.

  it('reconnect applies the newer list and invalidates transcripts', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    listChatsFn.mockResolvedValue([chat('c1', { working: true, version: ++clock })])

    captureCb()({ reconnected: true })
    await flush()

    expect(notifyAgentChatMessages).toHaveBeenCalledTimes(1)
    expect(held('c1')?.working).toBe(true)
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

  it('reconnect forgets a chat deleted during the outage, and its pane', async () => {
    listChatsFn.mockResolvedValue([chat('c1'), chat('c2')])
    setPanes(openPane('p-c2', 'c2', ''))
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    listChatsFn.mockResolvedValue([chat('c1')]) // c2 gone
    getChatFn.mockImplementation((_wsId: string, id: string) =>
      id === 'c2'
        ? Promise.reject(new ApiError('not found', 404))
        : Promise.resolve(chat(id, { version: ++clock })),
    )
    captureCb()({ reconnected: true })
    await flush()

    expect(held('c2')).toBeUndefined()
    expect(forgetChat).toHaveBeenCalledWith('c2')
    expect(panes['p-c2']).toBeUndefined()
  })

  // A repo-scoped list can omit a project-home chat that still exists.
  it('reconnect keeps a chat the list omitted but the daemon still has', async () => {
    listChatsFn.mockResolvedValue([chat('c1'), chat('home-chat')])
    setPanes(openPane('p-home', 'home-chat', ''))
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    listChatsFn.mockResolvedValue([chat('c1')]) // the list omits it...
    captureCb()({ reconnected: true })
    await flush()

    expect(getChatFn).toHaveBeenCalledWith('w1', 'home-chat') // ...so it is confirmed
    expect(forgetChat).not.toHaveBeenCalled()
    expect(held('home-chat')).toBeDefined()
    expect(panes['p-home'].chatId).toBe('home-chat')
  })

  // The confirm must use the chat's OWN mount: this store's 404s a chat owned
  // by another workspace.
  it('reconnect never forgets a vanished chat another workspace owns', async () => {
    listChatsFn.mockResolvedValue([chat('c1'), chat('x', { workspaceId: 'w2' })])
    setPanes(openPane('p-x', 'x', ''))
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    listChatsFn.mockResolvedValue([chat('c1')])
    getChatFn.mockRejectedValue(new ApiError('not found', 404))
    captureCb()({ reconnected: true })
    await flush()

    expect(getChatFn).not.toHaveBeenCalledWith(expect.anything(), 'x')
    expect(forgetChat).not.toHaveBeenCalled()
    expect(panes['p-x'].chatId).toBe('x')
  })

  it('reconnect never forgets a vanished chat whose owner cannot be resolved', async () => {
    listChatsFn.mockResolvedValue([chat('c1'), chat('x', { workspaceId: '' })])
    setPanes(openPane('p-x', 'x', ''))
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    listChatsFn.mockResolvedValue([chat('c1')])
    getChatFn.mockRejectedValue(new ApiError('not found', 404))
    captureCb()({ reconnected: true })
    await flush()

    expect(resolveOwnerFn).toHaveBeenCalledWith('x')
    expect(forgetChat).not.toHaveBeenCalled()
  })

  it('reconnect never forgets on a transient read failure', async () => {
    listChatsFn.mockResolvedValue([chat('c1'), chat('c2')])
    setPanes(openPane('p-c2', 'c2', ''))
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    listChatsFn.mockResolvedValue([chat('c1')])
    getChatFn.mockRejectedValue(new Error('network down'))
    captureCb()({ reconnected: true })
    await flush()

    expect(forgetChat).not.toHaveBeenCalled()
    expect(held('c2')).toBeDefined()
  })

  it('reconnect leaves the panes of surviving chats alone', async () => {
    setPanes(openPane('p-c1', 'c1', ''))
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    captureCb()({ reconnected: true })
    await flush()

    expect(forgetChat).not.toHaveBeenCalled()
    expect(panes['p-c1'].chatId).toBe('c1')
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
    onFrame(live('turn_started', 'c1', { working: true }))

    expect(applyAgentChatFrame).not.toHaveBeenCalled()
  })

  it('ignores a runner frame delivered after teardown', async () => {
    setPanes(openPane('p1', 'c1', 'c1-r'))
    const { unmount } = renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()
    const onFrame = captureCb()
    unmount()

    onFrame(live('snapshot', 'c1', { liveRunnerId: '' }, { runnerId: 'c1-r' }))

    expect(retargetPane).not.toHaveBeenCalled()
    expect(setPaneRunner).not.toHaveBeenCalled()
  })

  it('ignores a move that names no destination', async () => {
    renderHook(() => useWorkspaceAgentChatsStream('w1'))
    await flush()

    captureCb()({ runnerId: 'r1', chatId: '', workspaceId: 'w1', kind: 'moved' })

    expect(applyAgentChatFrame).not.toHaveBeenCalled()
    expect(retargetPane).not.toHaveBeenCalled()
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

    // Task 34: the sidebar's own folders pipeline has no dedicated push
    // channel any more — it watches useFolderSignalStore's per-repo
    // generation, bumped here, instead of a WS frame of its own.
    it.each(['folder_created', 'folder_updated', 'folder_deleted'])(
      'bumps the repo-scoped folder signal on %s',
      async (kind) => {
        renderHook(() => useWorkspaceAgentChatsStream('w1'))
        await flush()
        const before = useFolderSignalStore.getState().generations.r1 ?? 0

        captureCb()(folderFrame(kind))
        await flush()

        expect(useFolderSignalStore.getState().generations.r1).toBe(before + 1)
      },
    )

    it('bumps the folder signal on the reconnect sentinel too', async () => {
      renderHook(() => useWorkspaceAgentChatsStream('w1'))
      await flush()
      const before = useFolderSignalStore.getState().generations.r1 ?? 0

      captureCb()({ reconnected: true })
      await flush()

      expect(useFolderSignalStore.getState().generations.r1).toBe(before + 1)
    })

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

  // Task D: a chat is a TREE row (design spec §3.1), so the sidebar's per-repo
  // tree subscription has to hear about a chat that appeared, was renamed, was
  // moved or is gone — through the same signal a folder frame bumps, because
  // both halves are one tree over one aggregate.
  describe('the repo tree signal on chat frames', () => {
    const chatFrame = (kind: string, over: Partial<Frame> = {}): Frame => ({
      chatId: 'c1',
      workspaceId: 'w1',
      kind,
      ...over,
    })

    it.each(['created', 'deleted', 'title_set', 'placement_set', 'order_set'])(
      'bumps the repo-scoped tree signal on %s',
      async (kind) => {
        renderHook(() => useWorkspaceAgentChatsStream('w1'))
        await flush()
        const before = useFolderSignalStore.getState().generations.r1 ?? 0

        captureCb()(chatFrame(kind))
        await flush()

        expect(useFolderSignalStore.getState().generations.r1).toBe(before + 1)
      },
    )

    // The hottest frames on the feed. Reseeding a whole repo's chat list on each
    // one is a request storm per agent turn — this is the guard against that.
    it.each([
      'turn_started',
      'turn_stopped',
      'message_delta',
      'session_bound',
      'snapshot',
      'started',
      'moved',
      'displaced',
      'exited',
      'plan',
      'telemetry',
    ])('does NOT bump on %s — it says nothing about the tree', async (kind) => {
      renderHook(() => useWorkspaceAgentChatsStream('w1'))
      await flush()
      const before = useFolderSignalStore.getState().generations.r1 ?? 0

      captureCb()(chatFrame(kind))
      await flush()

      expect(useFolderSignalStore.getState().generations.r1 ?? 0).toBe(before)
    })

    // A RUNNER frame is about a process, not a row: it is routed before the
    // chat branch is reached, so it must move no tree generation either.
    it('does not bump on a runner frame', async () => {
      renderHook(() => useWorkspaceAgentChatsStream('w1'))
      await flush()
      const before = useFolderSignalStore.getState().generations.r1 ?? 0

      captureCb()(chatFrame('started', { runnerId: 'r-1' }))
      await flush()

      expect(useFolderSignalStore.getState().generations.r1 ?? 0).toBe(before)
    })

    // THE CROSS-REPO GUARD. The repo id comes from this workspace's own
    // recorded scope, never from the frame or from a broad broadcast, so a chat
    // frame arriving on repo r1's feed can only ever move r1's generation —
    // which is what keeps repo r2's subscription from refetching.
    it('moves ONLY this workspace’s own repo generation', async () => {
      renderHook(() => useWorkspaceAgentChatsStream('w1'))
      await flush()

      captureCb()(chatFrame('created'))
      await flush()

      expect(useFolderSignalStore.getState().generations.r1).toBe(1)
      expect(useFolderSignalStore.getState().generations.r2).toBeUndefined()
    })

    // A workspace whose scope is genuinely unrecorded never gets this far any
    // more — `useWorkspaceScopeReady` (workspace-scope.ts) now defers the
    // whole effect, subscription included, until scope exists (see the
    // top-level 'does not subscribe...' tests below). The real case left for
    // `bumpTreeSignal`'s own optional-chaining to guard is a scope that DOES
    // exist but names no repo — a project-home workspace.
    it('bumps nothing for a home workspace, whose recorded scope has no repo to name', async () => {
      setWorkspaceScope({ projectId: 'p1', repoId: '', wsId: 'ws-home' })
      renderHook(() => useWorkspaceAgentChatsStream('ws-home'))
      await flush()

      captureCb()(chatFrame('created'))
      await flush()

      expect(useFolderSignalStore.getState().generations).toEqual({})
    })
  })

  it('tears down and re-subscribes when wsId changes', () => {
    const unsubW1 = vi.fn()
    const unsubW2 = vi.fn()
    subscribe.mockReturnValueOnce(unsubW1).mockReturnValueOnce(unsubW2)
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'w2' })

    const { rerender } = renderHook(({ w }: { w: string }) => useWorkspaceAgentChatsStream(w), {
      initialProps: { w: 'w1' },
    })
    expect(subscribe).toHaveBeenCalledTimes(1)

    rerender({ w: 'w2' })

    expect(unsubW1).toHaveBeenCalledTimes(1)
    expect(subscribe).toHaveBeenCalledTimes(2)
    expect((subscribe.mock.calls[1] as unknown as [string])[0]).toBe(
      '/v0/projects/p1/repos/r1/chats/ws',
    )
  })
})

// ── compaction_started / compaction_stopped: the live "Compacting…" edge ──
//
// The ledger's own interruption record for a compaction is born already
// resolved (a bare /compact prompt never opens a tracked turn), so these two
// frames are the ONLY place "in progress" is ever observable — see
// AgentChatsState.compacting's doc comment. compact_post is not reliable, so
// ANY other chat frame arriving while marked compacting clears it, and so does
// an idle snapshot (the slice). No timer: a chat that goes silent is idle, and
// its next snapshot says so.

afterEach(() => {
  vi.useRealTimers()
})

it('marks the chat compacting on compaction_started', () => {
  renderHook(() => useWorkspaceAgentChatsStream('w1'))

  captureCb()({ chatId: 'c1', workspaceId: 'w1', kind: 'compaction_started' })

  expect(setAgentChatCompacting).toHaveBeenCalledWith('c1', true)
})

it('clears compacting on compaction_stopped', () => {
  renderHook(() => useWorkspaceAgentChatsStream('w1'))
  const onFrame = captureCb()

  onFrame({ chatId: 'c1', workspaceId: 'w1', kind: 'compaction_started' })
  onFrame({ chatId: 'c1', workspaceId: 'w1', kind: 'compaction_stopped' })

  expect(setAgentChatCompacting).toHaveBeenLastCalledWith('c1', false)
})

// This is the PRIMARY self-heal path, and the one that matters most in
// practice: compact_post rarely arrives, but the next real thing that
// happens to the chat (a turn starting, in this test) always does.
it('self-heals off ANY other chat frame when compact_post never arrives', () => {
  renderHook(() => useWorkspaceAgentChatsStream('w1'))
  const onFrame = captureCb()

  onFrame({ chatId: 'c1', workspaceId: 'w1', kind: 'compaction_started' })
  expect(setAgentChatCompacting).toHaveBeenLastCalledWith('c1', true)

  // No compaction_stopped ever arrives — instead, ordinary chat life resumes.
  onFrame(live('turn_started', 'c1', { working: true }))

  expect(setAgentChatCompacting).toHaveBeenLastCalledWith('c1', false)
})

// A frame for a DIFFERENT chat must never clear c1's own compacting state —
// self-heal is scoped per chat, same as every other map in this store.
it('does not self-heal off a frame for a different chat', () => {
  renderHook(() => useWorkspaceAgentChatsStream('w1'))
  const onFrame = captureCb()

  onFrame({ chatId: 'c1', workspaceId: 'w1', kind: 'compaction_started' })
  onFrame(live('turn_started', 'c2', { working: true }))

  expect(setAgentChatCompacting).not.toHaveBeenCalledWith('c1', false)
})

// ── telemetry: the usage gauge rides the feed; nothing polls for it ──

it('writes the pushed usage report through on a telemetry frame', () => {
  renderHook(() => useWorkspaceAgentChatsStream('w1'))

  captureCb()({
    chatId: 'c1',
    workspaceId: 'w1',
    kind: 'telemetry',
    telemetry: { observedAt: '2026-01-01T00:00:00Z', source: 'statusline' },
  })

  expect(setAgentChatTelemetry).toHaveBeenCalledWith('c1', {
    observedAt: '2026-01-01T00:00:00Z',
    source: 'statusline',
  })
  expect(getChatFn).not.toHaveBeenCalled()
})

// ── prompt_settled: which way a retired delivery is released ──
//
// REGRESSION, reported live against codex: "User's turns after some time of
// idle is lost, and does not record anywhere." A retired delivery used to be
// announced as a bare "this is over", and the composer's queue answered by
// deleting the item — along with the user's text, which at that moment exists
// nowhere else in the system (the daemon's journal keeps a hash of the prompt,
// never the text, and by definition nothing reached the ledger).
//
// `promptConsumed` is what separates a built-in the CLI demonstrably ran from
// the daemon's delivery timeout simply expiring. This is the frame-to-store
// mapping that has to carry it.

it('records a consumed prompt as settled, which lets the queue drop it', () => {
  renderHook(() => useWorkspaceAgentChatsStream('w1'))

  captureCb()({
    chatId: 'c1',
    workspaceId: 'w1',
    kind: 'prompt_settled',
    clientRequestId: 'req-1',
    promptConsumed: true,
  })

  expect(setAgentChatPromptSettled).toHaveBeenCalledWith('c1', 'req-1')
  expect(setAgentChatPromptAbandoned).not.toHaveBeenCalled()
})

it('records an unproven prompt as abandoned, so the queue keeps the text', () => {
  renderHook(() => useWorkspaceAgentChatsStream('w1'))

  captureCb()({
    chatId: 'c1',
    workspaceId: 'w1',
    kind: 'prompt_settled',
    clientRequestId: 'req-1',
  })

  expect(setAgentChatPromptAbandoned).toHaveBeenCalledWith('c1', 'req-1')
  expect(setAgentChatPromptSettled).not.toHaveBeenCalled()
})
