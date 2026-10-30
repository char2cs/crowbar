import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import type { AgentChat } from '@/features/agent/api/agent-chat-reads'
import * as reads from '@/features/agent/api/agent-chat-reads'
import * as providersApi from '@/features/agent/api/agent-providers-api'
import { useChatScope } from '@/features/agent/hooks/use-chat-scope'
import {
  getScrollPosition,
  setScrollPosition,
} from '@/features/agent/hooks/lib/transcript-scroll-positions'
import {
  requestExcalidrawEdit,
  useExcalidrawEditRequests,
} from '@/features/agent/composer/lib/excalidraw-edit-requests'
import { attachScope, fetchChat, releaseChats } from '@/features/agent/stores/chat-scopes'
import { chatStore } from '@/features/agent/stores/chat-store'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { ApiError } from '@/lib/errors/api-error'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { __resetWorkspaceScopesForTest, setWorkspaceScope } from '@/lib/workspace-scope'

interface Sub {
  endpoint: string
  cb: (data: unknown) => void
  closed: boolean
}
const subs: Sub[] = []
vi.mock('@/lib/ws/manager', () => ({
  wsManager: {
    subscribe: (endpoint: string, cb: (data: unknown) => void) => {
      const sub: Sub = { endpoint, cb, closed: false }
      subs.push(sub)
      return () => {
        sub.closed = true
      }
    },
  },
}))

type FakePane = { id: string; chatId: string; runnerId: string | null }
let panes: Record<string, FakePane> = {}
const paneActions = {
  setPaneRunner: vi.fn((id: string, runnerId: string | null) => {
    if (panes[id]) panes[id].runnerId = runnerId
  }),
  adoptBackgroundChat: vi.fn(),
  forgetChat: vi.fn((chatId: string) => {
    for (const pane of Object.values(panes)) if (pane.chatId === chatId) delete panes[pane.id]
  }),
}
vi.mock('@/features/panes/stores/window-pane-store', () => ({
  windowPaneStore: { getState: () => ({ panes, paneActions }) },
}))

let clock = 100
const chat = (id: string, over: Partial<AgentChat> = {}): AgentChat => ({
  id,
  workspaceId: 'w1',
  title: id,
  liveRunnerId: '',
  terminalSessionId: '',
  activeProviderId: 'fake',
  working: false,
  createdAt: '2026-01-01T00:00:00Z',
  order: 0,
  phase: 'dormant',
  version: ++clock,
  ...over,
})
const frame = (kind: string, id: string, over: Partial<AgentChat> = {}, extra = {}) => {
  const snap = chat(id, over)
  return { chatId: id, kind, chat: snap, version: snap.version, ...extra }
}

const listChats = vi.spyOn(reads, 'listChats')
const getChat = vi.spyOn(reads, 'getChat')
const listProviders = vi.spyOn(providersApi, 'listProviders')
const open = () => subs.filter((s) => !s.closed)
const push = (data: unknown) => open().forEach((s) => s.cb(data))
const chats = () => chatStore.getState().chats
const generation = () => useFolderSignalStore.getState().generations.r1 ?? 0
// Every mocked read is already settled, so a fixed number of ticks drains them.
const flush = async () => {
  for (let i = 0; i < 8; i++) await Promise.resolve()
}
const held: (() => void)[] = []
const attach = (wsId: string) => {
  const detach = attachScope(wsId)
  held.push(detach)
  return detach
}

beforeEach(() => {
  subs.length = 0
  panes = {}
  vi.clearAllMocks()
  chatStore.setState({ chats: {}, byWorkspace: {}, byRunner: {}, scopes: {}, telemetry: {} })
  useAgentProvidersStore.setState({ status: 'ready', providers: [] })
  listChats.mockResolvedValue([chat('c1')])
  getChat.mockImplementation((_ws, id) => Promise.resolve({ ...chat(id), conversations: [] }))
  listProviders.mockResolvedValue([])
  __resetWorkspaceScopesForTest()
  setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'w1' })
  setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'w2' })
  setWorkspaceScope({ projectId: 'p1', repoId: 'r2', wsId: 'w3' })
  useFolderSignalStore.setState({ generations: {} })
})
afterEach(() => held.splice(0).forEach((detach) => detach()))

describe('attachScope', () => {
  it('subscribes once per lifecycle endpoint for two workspaces of one repo', async () => {
    attach('w1')
    attach('w2')
    attach('w3')
    await flush()
    expect(subs.map((s) => s.endpoint)).toEqual([
      '/v0/projects/p1/repos/r1/chats/ws',
      '/v0/projects/p1/repos/r2/chats/ws',
    ])
    expect(listChats).toHaveBeenCalledTimes(2)
    expect(chatStore.getState().scopes).toEqual({ w1: 'ready', w2: 'ready', w3: 'ready' })
  })

  it('applies a lifecycle frame once however many workspaces are mounted', async () => {
    attach('w1')
    attach('w2')
    attach('w1')
    await flush()
    const writes = vi.fn()
    const stop = chatStore.subscribe(writes)
    push(frame('title_set', 'c1', { title: 'Renamed' }))
    stop()
    expect(writes).toHaveBeenCalledTimes(1)
    expect(chats().c1?.title).toBe('Renamed')
    expect(generation()).toBe(1)
  })

  it('unsubscribes when the last workspace of the endpoint unmounts', async () => {
    const a = attach('w1')
    const b = attach('w2')
    await flush()
    a()
    a()
    expect(open()).toHaveLength(1)
    expect(chatStore.getState().scopes).toEqual({ w2: 'ready' })
    b()
    expect(open()).toHaveLength(0)
    expect(chats()).toEqual({})
  })

  it('reseeds an endpoint after the socket reports reconnected', async () => {
    attach('w1')
    await flush()
    listChats.mockResolvedValue([chat('c1', { title: 'After the outage' })])
    push({ reconnected: true })
    await flush()
    expect(listChats).toHaveBeenCalledTimes(2)
    expect(chats().c1?.title).toBe('After the outage')
    expect(listProviders).toHaveBeenCalledTimes(1)
    expect(generation()).toBe(1)
  })

  it('adopts a chat that starts working with no pane as a background chat', async () => {
    attach('w1')
    await flush()
    push(frame('turn_started', 'c1', { working: true }))
    push(frame('turn_stopped', 'c1', { working: true }))
    expect(paneActions.adoptBackgroundChat.mock.calls).toEqual([['c1', 'p1', 'w1']])
  })

  it('adopts nothing for a chat already working when the list finds it', async () => {
    listChats.mockResolvedValue([chat('c1', { working: true })])
    attach('w1')
    await flush()
    push({ reconnected: true })
    await flush()
    expect(paneActions.adoptBackgroundChat).not.toHaveBeenCalled()
  })

  it('adopts nothing for a working frame that beats the list', async () => {
    let land: (list: AgentChat[]) => void = () => {}
    listChats.mockImplementationOnce(() => new Promise((resolve) => (land = resolve)))
    attach('w1')
    push(frame('turn_started', 'c1', { working: true }))
    land([chat('c1', { version: 1 })])
    await flush()
    expect(paneActions.adoptBackgroundChat).not.toHaveBeenCalled()
  })

  it('lets go of a runner the chat no longer has', async () => {
    listChats.mockResolvedValue([chat('c1', { liveRunnerId: 'r-1' })])
    panes = { p: { id: 'p', chatId: 'c1', runnerId: 'r-1' } }
    attach('w1')
    await flush()
    push(frame('exited', 'c1', { liveRunnerId: '' }, { runnerId: 'r-1' }))
    expect(panes.p?.runnerId).toBeNull()
  })

  it('forgets a deleted chat and its pane, and drops an older delete', async () => {
    panes = { p: { id: 'p', chatId: 'c1', runnerId: null } }
    setScrollPosition('c1', { stuck: false, distanceFromBottom: 240 })
    requestExcalidrawEdit('c1', { elements: [], appState: {} })
    attach('w1')
    await flush()
    push({ chatId: 'c1', kind: 'deleted', version: 1 })
    expect(chats().c1).toBeDefined()
    push({ chatId: 'c1', kind: 'deleted', version: ++clock })
    expect(chats().c1).toBeUndefined()
    expect(paneActions.forgetChat).toHaveBeenCalledWith('c1')
    expect(panes.p).toBeUndefined()
    expect(getScrollPosition('c1')).toBeNull()
    expect(useExcalidrawEditRequests.getState().requests.c1).toBeUndefined()
  })

  it('forgets on reseed only a chat its own workspace confirms gone', async () => {
    listChats.mockResolvedValue([chat('c1'), chat('c2'), chat('elsewhere', { workspaceId: 'w9' })])
    attach('w1')
    await flush()
    listChats.mockResolvedValue([])
    getChat.mockImplementation((_ws, id) =>
      id === 'c1'
        ? Promise.reject(new ApiError('gone', 404))
        : Promise.resolve({ ...chat(id), conversations: [] }),
    )
    push({ reconnected: true })
    await flush()
    expect(Object.keys(chats()).sort()).toEqual(['c2', 'elsewhere'])
    expect(getChat.mock.calls.map(([ws, id]) => `${ws}:${id}`).sort()).toEqual(['w1:c1', 'w1:c2'])
  })

  it('keeps a chat a transient read failure could not confirm gone', async () => {
    attach('w1')
    await flush()
    listChats.mockResolvedValue([])
    getChat.mockRejectedValue(new Error('offline'))
    push({ reconnected: true })
    await flush()
    expect(chats().c1).toBeDefined()
  })

  it('writes a telemetry frame and leaves a turn view to the chat feed', async () => {
    attach('w1')
    await flush()
    const report = { observedAt: '2026-08-24T12:00:00Z', source: 'callback' as const }
    push({ chatId: 'c1', kind: 'telemetry', telemetry: report })
    push({ chatId: 'c1', kind: 'message_delta', chat: chat('c1', { title: 'ignored' }) })
    expect(chatStore.getState().telemetry.c1).toEqual(report)
    expect(chats().c1?.title).toBe('c1')
    expect(generation()).toBe(0)
  })

  it('bumps only its own repo tree on a folder frame', async () => {
    attach('w1')
    attach('w3')
    await flush()
    open()[0]?.cb({ folderId: 'f1', kind: 'folder_created' })
    expect(useFolderSignalStore.getState().generations).toEqual({ r1: 1 })
  })

  it('keeps a chat another endpoint still holds when one endpoint releases', async () => {
    const a = attach('w1')
    const b = attach('w3')
    await flush()
    open()[1]?.cb(frame('title_set', 'c1', { workspaceId: 'w3' }))
    a()
    expect(chats().c1).toBeDefined()
    b()
    expect(chats().c1).toBeUndefined()
  })

  it('writes nothing from a check that outlives its scope', async () => {
    const detach = attach('w1')
    await flush()
    let answer: (r: reads.AgentChatDetail) => void = () => {}
    let refuse: (e: unknown) => void = () => {}
    listChats.mockResolvedValue([])
    getChat.mockImplementationOnce(() => new Promise((res) => (answer = res)))
    push({ reconnected: true })
    await flush()
    detach()
    answer({ ...chat('c1'), conversations: [] })
    await flush()
    expect(chats()).toEqual({})

    listChats.mockResolvedValue([chat('c1')])
    const again = attach('w1')
    await flush()
    listChats.mockResolvedValue([])
    getChat.mockImplementationOnce(() => new Promise((_res, rej) => (refuse = rej)))
    push({ reconnected: true })
    await flush()
    again()
    listChats.mockResolvedValue([chat('c1')])
    attach('w1')
    await flush()
    refuse(new ApiError('gone', 404))
    await flush()
    expect(chats().c1).toBeDefined()
  })

  it('drops a seed that lands after its scope detached and a new one attached', async () => {
    let first: (list: AgentChat[]) => void = () => {}
    listChats.mockImplementationOnce(() => new Promise((resolve) => (first = resolve)))
    attach('w1')()
    listChats.mockResolvedValueOnce([chat('c1', { title: 'fresh', version: 5 })])
    attach('w1')
    await flush()
    first([chat('c1', { title: 'stale', version: 9 })])
    await flush()
    expect(chats().c1?.title).toBe('fresh')
  })

  it('reads a chat into the open scope, and releases a loose one only when asked', async () => {
    const detach = attach('w1')
    await flush()
    await fetchChat('w1', 'c2', () => false)
    expect(chats().c2).toBeDefined()
    detach()
    expect(chats().c2).toBeUndefined()

    await fetchChat('w1', 'c3', () => false)
    expect(chats().c3).toBeUndefined()
    await fetchChat('w1', 'c3', () => true)
    expect(chats().c3).toBeDefined()
    releaseChats(['c3'])
    expect(chats().c3).toBeUndefined()
  })

  it('records an endpoint whose first list failed', async () => {
    listChats.mockRejectedValue(new Error('offline'))
    attach('w1')
    await flush()
    expect(chatStore.getState().scopes.w1).toBe('error')
  })

  it('loads the providers once when nothing has asked yet, with no retry', async () => {
    useAgentProvidersStore.setState({ status: 'idle', providers: [] })
    listProviders.mockRejectedValue(new Error('offline'))
    attach('w1')
    attach('w2')
    await flush()
    expect(listProviders).toHaveBeenCalledTimes(1)
    expect(useAgentProvidersStore.getState().status).toBe('failed')
  })
})

describe('useChatScope', () => {
  it('waits for the workspace scope before attaching', async () => {
    const { unmount } = renderHook(() => useChatScope('w-late'))
    expect(subs).toHaveLength(0)
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'w-late' })
    await flush()
    expect(open().map((s) => s.endpoint)).toEqual(['/v0/projects/p1/repos/r1/chats/ws'])
    unmount()
    expect(open()).toHaveLength(0)
  })
})
