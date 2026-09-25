import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// LspClient keeps one session per workspace, addressed by the id the caller
// passes (the buffer's own workspace), never the active one. A session lives
// while it has an open document or a diagnostics handler; its diagnostics
// socket is open exactly while it lives AND the workspace's owning chat is
// known.

const { apiFetch } = vi.hoisted(() => ({
  apiFetch: vi.fn(async (..._args: unknown[]): Promise<unknown> => null),
}))
const { subscribe, unsubscribes } = vi.hoisted(() => {
  const unsubscribes = new Map<string, ReturnType<typeof vi.fn>>()
  const subscribe = vi.fn((topic: string, _cb: (frame: unknown) => void) => {
    const off = vi.fn()
    unsubscribes.set(topic, off)
    return off
  })
  return { subscribe, unsubscribes }
})

vi.mock('@/lib/api', () => ({ apiFetch }))
vi.mock('@/lib/ws/manager', () => ({ wsManager: { subscribe } }))

import { LspClient } from '@/features/editor/lsp/lsp-client'
import {
  __resetWorkspaceScopesForTest,
  forgetWorkspaceScope,
  recordWorkspaceScope,
  setWorkspaceScope,
} from '@/lib/workspace-scope'

const TOPIC_1 = '/v0/chats/chat-1/lsp/ws'
const TOPIC_2 = '/v0/chats/chat-2/lsp/ws'

const posts = () => apiFetch.mock.calls.map((c) => String(c[0]))
const postsTo = (suffix: string) => posts().filter((p) => p.endsWith(suffix))
const bodyOf = (index: number) =>
  JSON.parse(String((apiFetch.mock.calls[index]?.[1] as RequestInit).body)) as unknown

function frameHandler(topic: string): (frame: unknown) => void {
  const calls = subscribe.mock.calls as unknown as [string, (frame: unknown) => void][]
  const call = calls.filter(([t]) => t === topic).at(-1)
  if (!call) throw new Error(`never subscribed to ${topic}`)
  return call[1]
}

const diag = (filePath: string) => ({
  filePath,
  range: { start: { line: 0, character: 0 }, end: { line: 0, character: 1 } },
  severity: 'error',
  message: 'boom',
})

beforeEach(() => {
  apiFetch.mockReset()
  apiFetch.mockResolvedValue(null)
  subscribe.mockClear()
  unsubscribes.clear()
  __resetWorkspaceScopesForTest()
  setWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'w1', owningChatId: 'chat-1' })
  recordWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'w2', owningChatId: 'chat-2' })
})

describe('LspClient document sync', () => {
  it("addresses each document to its own workspace's chat", async () => {
    const client = new LspClient()
    await client.documentOpen('w1', 'src/a.ts', 'one', 'typescript')
    await client.documentOpen('w2', 'src/a.ts', 'two', 'typescript')
    await client.documentSave('w2', 'src/a.ts')
    await client.documentClose('w1', 'src/a.ts')

    expect(posts()).toEqual([
      '/v0/chats/chat-1/lsp/didOpen',
      '/v0/chats/chat-2/lsp/didOpen',
      '/v0/chats/chat-2/lsp/didSave',
      '/v0/chats/chat-1/lsp/didClose',
    ])
    expect(bodyOf(1)).toEqual({ path: 'src/a.ts', languageId: 'typescript', text: 'two' })
  })

  it('opens a document once however many panes hold it, and closes it with the last', async () => {
    const client = new LspClient()
    await client.documentOpen('w1', 'a.ts', 'x', 'typescript')
    await client.documentOpen('w1', 'a.ts', 'x', 'typescript')
    expect(postsTo('/didOpen')).toHaveLength(1)

    await client.documentClose('w1', 'a.ts')
    expect(postsTo('/didClose')).toHaveLength(0)
    await client.documentClose('w1', 'a.ts')
    expect(postsTo('/didClose')).toHaveLength(1)

    await client.documentOpen('w1', 'a.ts', 'x', 'typescript')
    expect(postsTo('/didOpen')).toHaveLength(2)
  })

  it('a close or save for a document that is not open sends nothing', async () => {
    const client = new LspClient()
    await client.documentClose('w1', 'never.ts')
    await client.documentSave('w1', 'never.ts')
    expect(apiFetch).not.toHaveBeenCalled()
  })

  it('defers didOpen until the owning chat is recorded, with the latest text', async () => {
    setWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'w3' })
    const client = new LspClient()
    await client.documentOpen('w3', 'a.ts', 'old', 'typescript')
    client.scheduleChange('w3', 'a.ts', () => 'new')
    await client.flushChange('w3', 'a.ts')
    expect(apiFetch).not.toHaveBeenCalled()
    expect(subscribe).not.toHaveBeenCalled()

    recordWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'w3', owningChatId: 'chat-3' })

    expect(posts()).toEqual(['/v0/chats/chat-3/lsp/didOpen'])
    expect(bodyOf(0)).toEqual({ path: 'a.ts', languageId: 'typescript', text: 'new' })
    expect(subscribe).toHaveBeenCalledWith('/v0/chats/chat-3/lsp/ws', expect.any(Function))
  })

  it('a document closed before its deferred didOpen went out is never sent', async () => {
    setWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'w3' })
    const client = new LspClient()
    await client.documentOpen('w3', 'a.ts', 'code', 'typescript')
    await client.documentClose('w3', 'a.ts')
    recordWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'w3', owningChatId: 'chat-3' })
    expect(apiFetch).not.toHaveBeenCalled()
    expect(subscribe).not.toHaveBeenCalled()
  })

  it('the home workspace has no LSP surface', async () => {
    setWorkspaceScope({ projectId: 'p', repoId: '', wsId: 'home-ws' })
    const client = new LspClient()
    client.onDiagnosticsUpdate('home-ws', () => {})
    await client.documentOpen('home-ws', 'a.md', 'x', 'markdown')
    expect(apiFetch).not.toHaveBeenCalled()
    expect(subscribe).not.toHaveBeenCalled()
  })

  it('reopen re-sends didOpen only for an open document', async () => {
    const client = new LspClient()
    await client.reopen('w1', 'a.ts', 'x', 'typescript')
    expect(apiFetch).not.toHaveBeenCalled()
    await client.documentOpen('w1', 'a.ts', 'x', 'typescript')
    await client.reopen('w1', 'a.ts', 'y', 'typescript')
    expect(postsTo('/didOpen')).toHaveLength(2)
  })

  it('tells listeners which workspace a document opened in', async () => {
    const client = new LspClient()
    const opened = vi.fn()
    client.onDocumentOpened(opened)
    await client.documentOpen('w2', 'a.ts', 'x', 'typescript')
    expect(opened).toHaveBeenCalledWith('w2', 'a.ts')
  })
})

describe('LspClient didChange debounce and flush', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  const changes = () => apiFetch.mock.calls.filter((c) => String(c[0]).endsWith('/didChange'))

  it('coalesces a burst of edits into one didChange carrying the latest text', async () => {
    const client = new LspClient()
    await client.documentOpen('w2', 'a.ts', 'a', 'typescript')
    let text = 'a'
    client.scheduleChange('w2', 'a.ts', () => text)
    text = 'ab'
    client.scheduleChange('w2', 'a.ts', () => text)
    text = 'abc'
    await vi.advanceTimersByTimeAsync(1000)

    expect(changes()).toHaveLength(1)
    expect(String(changes()[0]?.[0])).toBe('/v0/chats/chat-2/lsp/didChange')
    expect(JSON.parse(String((changes()[0]?.[1] as RequestInit).body)).text).toBe('abc')
  })

  it('flushChange sends the pending text immediately, once', async () => {
    const client = new LspClient()
    await client.documentOpen('w1', 'a.ts', 'foo', 'typescript')
    client.scheduleChange('w1', 'a.ts', () => 'foo.')
    await client.flushChange('w1', 'a.ts')
    expect(changes()).toHaveLength(1)
    await vi.advanceTimersByTimeAsync(1000)
    expect(changes()).toHaveLength(1)
  })

  it('sends nothing when the text is gone by flush time, or the document is not open', async () => {
    const client = new LspClient()
    client.scheduleChange('w1', 'closed.ts', () => 'x')
    await client.documentOpen('w1', 'a.ts', 'x', 'typescript')
    client.scheduleChange('w1', 'a.ts', () => null)
    await vi.advanceTimersByTimeAsync(1000)
    expect(changes()).toHaveLength(0)
  })

  it('closing a document drops its pending change', async () => {
    const client = new LspClient()
    await client.documentOpen('w1', 'a.ts', 'x', 'typescript')
    client.scheduleChange('w1', 'a.ts', () => 'y')
    await client.documentClose('w1', 'a.ts')
    await vi.advanceTimersByTimeAsync(1000)
    expect(changes()).toHaveLength(0)
  })
})

describe('LspClient diagnostics subscription', () => {
  it("subscribes to a workspace's topic with its first holder and delivers only its batches", () => {
    const client = new LspClient()
    const h1 = vi.fn()
    const h2 = vi.fn()
    client.onDiagnosticsUpdate('w1', h1)
    client.onDiagnosticsUpdate('w2', h2)
    expect(subscribe.mock.calls.map(([t]) => t)).toEqual([TOPIC_1, TOPIC_2])

    frameHandler(TOPIC_1)({ wsId: 'w1', diagnostics: [diag('src/shared.ts')] })
    expect(h1).toHaveBeenCalledWith('src/shared.ts', [diag('src/shared.ts')])
    expect(h2).not.toHaveBeenCalled()
  })

  it('replays the last batch to a late handler and clears files that lost their diagnostics', () => {
    const client = new LspClient()
    client.onDiagnosticsUpdate('w1', () => {})
    frameHandler(TOPIC_1)({ wsId: 'w1', diagnostics: [diag('a.ts')] })

    const late = vi.fn()
    client.onDiagnosticsUpdate('w1', late)
    expect(late).toHaveBeenCalledWith('a.ts', [diag('a.ts')])

    frameHandler(TOPIC_1)({ wsId: 'w1', diagnostics: [] })
    expect(late).toHaveBeenLastCalledWith('a.ts', [])
    expect(subscribe).toHaveBeenCalledTimes(1)
  })

  it('unsubscribes when the last handler and document of the workspace leave', async () => {
    const client = new LspClient()
    const off1 = client.onDiagnosticsUpdate('w1', () => {})
    const off2 = client.onDiagnosticsUpdate('w1', () => {})
    await client.documentOpen('w1', 'a.ts', 'x', 'typescript')
    client.onDiagnosticsUpdate('w2', () => {})

    off1()
    off2()
    expect(unsubscribes.get(TOPIC_1)).not.toHaveBeenCalled()
    await client.documentClose('w1', 'a.ts')
    expect(unsubscribes.get(TOPIC_1)).toHaveBeenCalledTimes(1)
    expect(unsubscribes.get(TOPIC_2)).not.toHaveBeenCalled()

    client.onDiagnosticsUpdate('w1', () => {})
    expect(subscribe.mock.calls.filter(([t]) => t === TOPIC_1)).toHaveLength(2)
  })

  it('unsubscribes immediately when the workspace is deleted, and never resubscribes', async () => {
    const client = new LspClient()
    const handler = vi.fn()
    client.onDiagnosticsUpdate('w2', handler)
    await client.documentOpen('w2', 'a.ts', 'x', 'typescript')
    frameHandler(TOPIC_2)({ wsId: 'w2', diagnostics: [diag('a.ts')] })

    forgetWorkspaceScope('w2')

    expect(unsubscribes.get(TOPIC_2)).toHaveBeenCalledTimes(1)
    expect(handler).toHaveBeenLastCalledWith('a.ts', [])
    apiFetch.mockClear()
    await client.documentSave('w2', 'a.ts')
    await client.documentClose('w2', 'a.ts')
    client.onDiagnosticsUpdate('w2', () => {})
    expect(apiFetch).not.toHaveBeenCalled()
    expect(subscribe.mock.calls.filter(([t]) => t === TOPIC_2)).toHaveLength(1)
  })
})

describe('LspClient.request', () => {
  it("POSTs to the given workspace's owning chat", async () => {
    const range = { start: { line: 3, character: 2 }, end: { line: 3, character: 9 } }
    apiFetch.mockResolvedValue([{ filePath: 'src/lib/target.ts', range }])

    const result = await new LspClient().request('w2', 'definition', {
      path: 'src/app.ts',
      position: { line: 12, character: 4 },
    })

    expect(result).toEqual([{ filePath: 'src/lib/target.ts', range }])
    expect(posts()).toEqual(['/v0/chats/chat-2/lsp/definition'])
    expect((apiFetch.mock.calls[0]?.[1] as RequestInit).method).toBe('POST')
    expect(bodyOf(0)).toEqual({ path: 'src/app.ts', position: { line: 12, character: 4 } })
  })

  it('passes the abort signal through to the fetch', async () => {
    const controller = new AbortController()
    await new LspClient().request('w1', 'hover', { path: 'a.ts' }, controller.signal)
    expect((apiFetch.mock.calls[0]?.[1] as RequestInit).signal).toBe(controller.signal)
  })

  it('resolves null without a request when the workspace has no owning chat yet', async () => {
    setWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'w3' })
    await expect(new LspClient().request('w3', 'hover', {})).resolves.toBeNull()
    expect(apiFetch).not.toHaveBeenCalled()
  })

  it('propagates a daemon failure so the caller can surface it', async () => {
    apiFetch.mockRejectedValue(new Error('lsp: textDocument/definition: timeout'))
    await expect(new LspClient().request('w1', 'definition', {})).rejects.toThrow('timeout')
  })

  it('reads the server status and restarts through the daemon', async () => {
    apiFetch.mockResolvedValue({ state: 'running', command: 'gopls', languageId: 'go' })
    const client = new LspClient()

    expect(await client.status('w1', 'src/main go.go')).toEqual({
      state: 'running',
      command: 'gopls',
      languageId: 'go',
    })
    expect(posts()[0]).toBe('/v0/chats/chat-1/lsp/status?path=src%2Fmain%20go.go')

    await client.restart('w1', 'main.go')
    expect(posts()[1]).toBe('/v0/chats/chat-1/lsp/restart')
    expect(bodyOf(1)).toEqual({ path: 'main.go' })
  })
})
