import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useBrowserAgentStore } from '@/features/browser/lib/agent-activity-store'
import { attachBrowserAgentPeer } from '@/features/browser/lib/agent-peer'
import { registerWebview } from '@/features/browser/lib/webview-registry'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'

const ws = vi.hoisted(() => ({
  subscribe: vi.fn(),
  send: vi.fn(),
}))
vi.mock('@/lib/ws/manager', () => ({ wsManager: ws }))
const api = vi.hoisted(() => ({ evaluate: vi.fn(), navigate: vi.fn(), close: vi.fn() }))
vi.mock('@/lib/browser-bridge', () => ({ browserApi: api }))

let deliver: (frame: unknown) => void
let detach: () => void
const sent = () => ws.send.mock.calls.map(([, frame]) => frame as Record<string, unknown>)
const state = () => windowPaneStore.getState()

beforeEach(() => {
  vi.clearAllMocks()
  ws.subscribe.mockImplementation((_endpoint: string, cb: (frame: unknown) => void) => {
    deliver = cb
    return vi.fn()
  })
  resetWindowPaneStoreForTests()
  state().paneActions.setActiveProject('p1')
  state().paneActions.openChat('chat-a', { workspaceId: 'ws-a' })
  state().paneActions.openChat('chat-b', { workspaceId: 'ws-b' })
  detach = attachBrowserAgentPeer()
})

afterEach(() => detach())

function mountTab(chatId: string, url: string): string {
  const pane = state()
    .paneActions.getAllPaneGroups()
    .find((p) => p.chatId === chatId)
  const id = state().bufferActions.openContent(
    { type: 'browser', url, workspaceId: `ws-${chatId.slice(-1)}` },
    { paneId: pane?.id },
  )
  registerWebview(id, Promise.resolve())
  return id
}

describe('browser peer', () => {
  it('attaches to the daemon route and reports every tab with its chat when greeted', () => {
    expect(ws.subscribe).toHaveBeenCalledWith('/v0/browser/peer', expect.any(Function))
    const tab = mountTab('chat-a', 'https://a.test/')
    ws.send.mockClear()

    deliver({ type: 'hello' })

    expect(sent()).toEqual([
      {
        type: 'tabs',
        tabs: [{ id: tab, chatId: 'chat-a', url: 'https://a.test/', title: '', loading: false }],
      },
    ])
  })

  it('reports again when a tab changes, and stays quiet when nothing did', () => {
    const tab = mountTab('chat-a', 'https://a.test/')
    ws.send.mockClear()
    state().bufferActions.setBrowserLocation(tab, { title: 'Hello' })
    state().bufferActions.setBrowserLocation(tab, { title: 'Hello' })
    const tabsFrames = sent().filter((f) => f.type === 'tabs')
    expect(tabsFrames.at(-1)).toMatchObject({ tabs: [{ id: tab, title: 'Hello' }] })
    const before = ws.send.mock.calls.length
    state().bufferActions.setBrowserLocation(tab, { title: 'Hello' })
    expect(ws.send.mock.calls.length).toBe(before)
  })

  it('runs a request and answers it by id', async () => {
    const tab = mountTab('chat-a', 'https://a.test/')
    api.evaluate.mockResolvedValue('{"n":1}')
    ws.send.mockClear()

    deliver({
      type: 'request',
      id: '7',
      op: 'eval',
      chatId: 'chat-a',
      tab,
      args: { js: 'return 1' },
    })

    await vi.waitFor(() =>
      expect(sent()).toContainEqual({ type: 'response', id: '7', ok: true, result: { n: 1 } }),
    )
  })

  it('answers a forged tab id with an error, never a result', async () => {
    const theirs = mountTab('chat-b', 'https://b.test/')
    ws.send.mockClear()

    deliver({
      type: 'request',
      id: '8',
      op: 'eval',
      chatId: 'chat-a',
      tab: theirs,
      args: { js: '1' },
    })

    await vi.waitFor(() =>
      expect(sent()).toContainEqual(
        expect.objectContaining({ type: 'response', id: '8', ok: false }),
      ),
    )
    expect(api.evaluate).not.toHaveBeenCalled()
  })

  it('marks the tab as agent-controlled only while a command is in flight', async () => {
    const tab = mountTab('chat-a', 'https://a.test/')
    let finish: (v: string) => void = () => {}
    api.evaluate.mockReturnValue(new Promise<string>((resolve) => (finish = resolve)))

    deliver({ type: 'request', id: '9', op: 'eval', chatId: 'chat-a', tab, args: { js: '1' } })
    await vi.waitFor(() => expect(useBrowserAgentStore.getState().active[tab]).toBe(1))
    finish('1')
    await vi.waitFor(() => expect(useBrowserAgentStore.getState().active[tab]).toBeUndefined())
  })

  it('ignores frames that are not well-formed requests', () => {
    ws.send.mockClear()
    deliver({ type: 'request', id: 1 })
    deliver({ type: 'request', id: 'x', op: 'open', chatId: '' })
    deliver(null)
    deliver('nonsense')
    expect(ws.send).not.toHaveBeenCalled()
  })
})
