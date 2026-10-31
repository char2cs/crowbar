import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// LspClient keeps one session per workspace, addressed by the id the caller
// passes (the buffer's own workspace), never the active one. A session lives
// while it has an open document or a diagnostics handler; it holds the owning
// chat's lsp: scope exactly while it lives AND that chat is known.

const { apiFetch } = vi.hoisted(() => ({
  apiFetch: vi.fn(async (..._args: unknown[]): Promise<unknown> => null),
}))

vi.mock('@/lib/api/fetch', () => ({ apiFetch }))

import { LspClient } from '@/features/editor/lsp/lsp-client'
import { holding } from '@/core/stream/stream'
import { streamDaemon } from '@/__tests__/__fixtures__/stream-daemon'
import { toastManager } from '@/lib/toast-manager'
import {
  __resetWorkspaceScopesForTest,
  forgetWorkspaceScope,
  recordWorkspaceScope,
  setWorkspaceScope,
} from '@/lib/workspace-scope'

const SCOPE_1 = 'lsp:chat-1'
const SCOPE_2 = 'lsp:chat-2'

const posts = () => apiFetch.mock.calls.map((c) => String(c[0]))
const postsTo = (suffix: string) => posts().filter((p) => p.endsWith(suffix))
const bodyOf = (index: number) =>
  JSON.parse(String((apiFetch.mock.calls[index]?.[1] as RequestInit).body)) as unknown

const signal = (scope: string, data: unknown) =>
  streamDaemon.deliver({ t: 'signal', scope, kind: 'diagnostics', data })
const lspHeld = () =>
  ['lsp:chat-1', 'lsp:chat-2', 'lsp:chat-3', 'lsp:chat-9', 'lsp:home-ws'].filter((scope) =>
    holding(scope),
  )

const diag = (filePath: string) => ({
  filePath,
  range: { start: { line: 0, character: 0 }, end: { line: 0, character: 1 } },
  severity: 'error',
  message: 'boom',
})

// A client from an earlier test still holds its scopes; forgetting the workspaces lets them go.
afterEach(() => {
  for (const wsId of ['w1', 'w2', 'w3']) forgetWorkspaceScope(wsId)
})

beforeEach(() => {
  apiFetch.mockReset()
  apiFetch.mockResolvedValue(null)
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
    expect(lspHeld()).toEqual([])

    recordWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'w3', owningChatId: 'chat-3' })

    expect(posts()).toEqual(['/v0/chats/chat-3/lsp/didOpen'])
    expect(bodyOf(0)).toEqual({ path: 'a.ts', languageId: 'typescript', text: 'new' })
    expect(lspHeld()).toEqual(['lsp:chat-3'])
    await client.documentClose('w3', 'a.ts')
    expect(lspHeld()).toEqual([])
  })

  it('a document closed before its deferred didOpen went out is never sent', async () => {
    setWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'w3' })
    const client = new LspClient()
    await client.documentOpen('w3', 'a.ts', 'code', 'typescript')
    await client.documentClose('w3', 'a.ts')
    recordWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'w3', owningChatId: 'chat-3' })
    expect(apiFetch).not.toHaveBeenCalled()
    expect(lspHeld()).toEqual([])
  })

  it('the home workspace has no LSP surface', async () => {
    setWorkspaceScope({ projectId: 'p', repoId: '', wsId: 'home-ws' })
    const client = new LspClient()
    client.onDiagnosticsUpdate('home-ws', () => {})
    await client.documentOpen('home-ws', 'a.md', 'x', 'markdown')
    expect(apiFetch).not.toHaveBeenCalled()
    expect(lspHeld()).toEqual([])
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

  it('a failed file sync reports why', async () => {
    const toastAdd = vi.spyOn(toastManager, 'add')
    const client = new LspClient()
    apiFetch.mockRejectedValueOnce(new Error('lsp: didOpen: server exited'))
    await client.documentOpen('w1', 'a.ts', 'x', 'typescript')
    apiFetch.mockRejectedValueOnce(new Error('lsp: didSave: server exited'))
    await client.documentSave('w1', 'a.ts')
    apiFetch.mockRejectedValueOnce(new Error('lsp: didClose: server exited'))
    await client.documentClose('w1', 'a.ts')

    const titles = toastAdd.mock.calls.map(([toast]) => toast.title)
    expect(titles).toEqual([
      'Sync the file with its language server',
      'Sync the file with its language server',
      'Sync the file with its language server',
    ])
    toastAdd.mockRestore()
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
  it("holds a workspace's lsp scope with its first holder and delivers only its batches", () => {
    const client = new LspClient()
    const h1 = vi.fn()
    const h2 = vi.fn()
    client.onDiagnosticsUpdate('w1', h1)
    client.onDiagnosticsUpdate('w2', h2)
    expect(lspHeld()).toEqual([SCOPE_1, SCOPE_2])

    signal(SCOPE_1, { wsId: 'w1', diagnostics: [diag('src/shared.ts')] })
    expect(h1).toHaveBeenCalledWith('src/shared.ts', [diag('src/shared.ts')])
    expect(h2).not.toHaveBeenCalled()
  })

  it('replays the last batch to a late handler and clears files that lost their diagnostics', () => {
    const client = new LspClient()
    client.onDiagnosticsUpdate('w1', () => {})
    signal(SCOPE_1, { wsId: 'w1', diagnostics: [diag('a.ts')] })

    const late = vi.fn()
    client.onDiagnosticsUpdate('w1', late)
    expect(late).toHaveBeenCalledWith('a.ts', [diag('a.ts')])

    signal(SCOPE_1, { wsId: 'w1', diagnostics: [] })
    expect(late).toHaveBeenLastCalledWith('a.ts', [])
  })

  it('lets the scope go when the last handler and document of the workspace leave', async () => {
    const client = new LspClient()
    const off1 = client.onDiagnosticsUpdate('w1', () => {})
    const off2 = client.onDiagnosticsUpdate('w1', () => {})
    await client.documentOpen('w1', 'a.ts', 'x', 'typescript')
    client.onDiagnosticsUpdate('w2', () => {})

    off1()
    off2()
    expect(lspHeld()).toEqual([SCOPE_1, SCOPE_2])
    await client.documentClose('w1', 'a.ts')
    expect(lspHeld()).toEqual([SCOPE_2])

    client.onDiagnosticsUpdate('w1', () => {})
    expect(lspHeld()).toEqual([SCOPE_1, SCOPE_2])
  })

  it('moves to the new owning chat’s scope when the workspace changes hands', () => {
    const client = new LspClient()
    const handler = vi.fn()
    client.onDiagnosticsUpdate('w1', handler)
    recordWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'w1', owningChatId: 'chat-9' })

    expect(lspHeld()).toEqual(['lsp:chat-9'])
    signal('lsp:chat-9', { wsId: 'w1', diagnostics: [diag('a.ts')] })
    expect(handler).toHaveBeenCalledWith('a.ts', [diag('a.ts')])
  })

  it('lets the scope go when the workspace is deleted, and never holds it again', async () => {
    const client = new LspClient()
    const handler = vi.fn()
    client.onDiagnosticsUpdate('w2', handler)
    await client.documentOpen('w2', 'a.ts', 'x', 'typescript')
    signal(SCOPE_2, { wsId: 'w2', diagnostics: [diag('a.ts')] })

    forgetWorkspaceScope('w2')

    expect(lspHeld()).toEqual([])
    expect(handler).toHaveBeenLastCalledWith('a.ts', [])
    apiFetch.mockClear()
    await client.documentSave('w2', 'a.ts')
    await client.documentClose('w2', 'a.ts')
    client.onDiagnosticsUpdate('w2', () => {})
    expect(apiFetch).not.toHaveBeenCalled()
    expect(lspHeld()).toEqual([])
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
