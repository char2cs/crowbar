import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// The Monaco providers talk to the daemon through LspClient.request: one
// POST per feature to the owning chat's /lsp route, addressed by the MODEL's
// workspace (not whichever is active), aborted when Monaco cancels, and — for
// position-addressed features — preceded by the document's pending didChange
// so the server answers against the text on screen.

const { apiFetch } = vi.hoisted(() => ({
  apiFetch: vi.fn(async (..._args: unknown[]): Promise<unknown> => null),
}))
const { subscribe } = vi.hoisted(() => ({ subscribe: vi.fn(() => () => {}) }))

vi.mock('@/lib/api', () => ({ apiFetch }))
vi.mock('@/lib/ws/manager', () => ({ wsManager: { subscribe } }))
vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getActiveWorkspaceId: () => 'ws-active',
}))

import { LspClient } from '@/features/editor/lsp/lsp-client'
import { recordWorkspaceScope, setWorkspaceScope } from '@/lib/workspace-scope'

function call(index: number) {
  const c = apiFetch.mock.calls[index]
  if (!c) throw new Error(`no apiFetch call #${index}`)
  const init = c[1] as RequestInit | undefined
  return { url: String(c[0]), init, body: init?.body ? JSON.parse(String(init.body)) : undefined }
}

describe('LspClient.request', () => {
  beforeEach(() => {
    apiFetch.mockReset()
    apiFetch.mockResolvedValue(null)
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-active', owningChatId: 'chat-a' })
    recordWorkspaceScope({
      projectId: 'p1',
      repoId: 'r1',
      wsId: 'ws-other',
      owningChatId: 'chat-o',
    })
  })

  it("POSTs to the given workspace's owning chat, not the active one", async () => {
    const range = { start: { line: 3, character: 2 }, end: { line: 3, character: 9 } }
    apiFetch.mockResolvedValue([{ filePath: 'src/lib/target.ts', range }])

    const client = new LspClient()
    const result = await client.request('ws-other', 'definition', {
      path: 'src/app.ts',
      position: { line: 12, character: 4 },
    })

    expect(result).toEqual([{ filePath: 'src/lib/target.ts', range }])
    const { url, init, body } = call(0)
    expect(url).toBe('/v0/chats/chat-o/lsp/definition')
    expect(init?.method).toBe('POST')
    expect(body).toEqual({ path: 'src/app.ts', position: { line: 12, character: 4 } })
  })

  it('passes the abort signal through to the fetch', async () => {
    const controller = new AbortController()
    await new LspClient().request('ws-active', 'hover', { path: 'a.ts' }, controller.signal)
    expect(call(0).init?.signal).toBe(controller.signal)
  })

  it('resolves null without a request when the workspace has no owning chat yet', async () => {
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-cold' })
    await expect(new LspClient().request('ws-cold', 'hover', {})).resolves.toBeNull()
    expect(apiFetch).not.toHaveBeenCalled()
  })

  it('propagates a daemon failure so the caller can surface it', async () => {
    apiFetch.mockRejectedValue(new Error('lsp: textDocument/definition: timeout'))
    await expect(new LspClient().request('ws-active', 'definition', {})).rejects.toThrow('timeout')
  })

  it('reads the server status and restarts through the daemon', async () => {
    apiFetch.mockResolvedValue({ state: 'running', command: 'gopls', languageId: 'go' })
    const client = new LspClient()

    expect(await client.status('ws-active', 'src/main go.go')).toEqual({
      state: 'running',
      command: 'gopls',
      languageId: 'go',
    })
    expect(call(0).url).toBe('/v0/chats/chat-a/lsp/status?path=src%2Fmain%20go.go')

    await client.restart('ws-active', 'main.go')
    expect(call(1).url).toBe('/v0/chats/chat-a/lsp/restart')
    expect(call(1).body).toEqual({ path: 'main.go' })
  })
})

describe('LspClient didChange debounce and flush', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    apiFetch.mockReset()
    apiFetch.mockResolvedValue(null)
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-active', owningChatId: 'chat-a' })
  })
  afterEach(() => vi.useRealTimers())

  const changes = () => apiFetch.mock.calls.filter((c) => String(c[0]).endsWith('/didChange'))

  it('coalesces a burst of edits into one didChange carrying the latest text', async () => {
    const client = new LspClient()
    let text = 'a'
    client.scheduleChange('a.ts', () => text)
    text = 'ab'
    client.scheduleChange('a.ts', () => text)
    text = 'abc'
    await vi.advanceTimersByTimeAsync(1000)

    expect(changes()).toHaveLength(1)
    expect(JSON.parse(String((changes()[0]?.[1] as RequestInit).body)).text).toBe('abc')
  })

  it('flushChange sends the pending text immediately (before a completion request)', async () => {
    const client = new LspClient()
    client.scheduleChange('a.ts', () => 'foo.')
    await client.flushChange('a.ts')
    expect(changes()).toHaveLength(1)

    await vi.advanceTimersByTimeAsync(1000)
    expect(changes()).toHaveLength(1)
  })

  it('sends nothing when the text is gone by flush time', async () => {
    const client = new LspClient()
    client.scheduleChange('a.ts', () => null)
    await client.flushChange('a.ts')
    expect(changes()).toHaveLength(0)
  })

  it('didSave goes out only for a document the server has open', async () => {
    const client = new LspClient()
    await client.documentSave('a.ts')
    expect(apiFetch).not.toHaveBeenCalled()

    await client.documentOpen('a.ts', 'x', 'typescript')
    await client.documentSave('a.ts')
    expect(apiFetch.mock.calls.map((c) => String(c[0]))).toContain('/v0/chats/chat-a/lsp/didSave')
  })
})
