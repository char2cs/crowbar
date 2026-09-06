import { describe, it, expect, vi, beforeEach } from 'vitest'

// Same shape as the already-fixed use-workspace-effects.ts race: the owning
// chat id is recorded ASYNCHRONOUSLY by the sidebar's own chat-list fetch,
// independently of (and often slower than) a workspace's own hydration. A
// buffer becoming a pane's active model (including tab restoration on a cold
// activation) calls straight into `LspClient`'s diagnostics subscription and
// document-open path — both of which used to build their URL via
// `lspBaseForWorkspace`, which THROWS with no owning chat id recorded. That
// throw propagated straight out of the satellite hook's effect and crashed
// via the nearest error boundary. These lock the fix: wait for the id instead
// of throwing, and retry once it lands instead of losing diagnostics for that
// file forever.

const { apiFetch } = vi.hoisted(() => ({
  apiFetch: vi.fn(async (..._args: unknown[]): Promise<unknown> => []),
}))
const { subscribe } = vi.hoisted(() => ({ subscribe: vi.fn(() => () => {}) }))
const { wsIdHolder } = vi.hoisted(() => ({ wsIdHolder: { current: 'ws-race' } }))

vi.mock('@/lib/api', () => ({ apiFetch }))
vi.mock('@/lib/ws/manager', () => ({ wsManager: { subscribe } }))
vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getActiveWorkspaceId: () => wsIdHolder.current,
}))

import { LspClient } from '@/features/editor/lsp/lsp-client'
import {
  setWorkspaceScope,
  recordWorkspaceScope,
  __resetWorkspaceScopesForTest,
} from '@/lib/workspace-scope'

const LSP_WS_TOPIC = '/v0/chats/chat-race/lsp/ws'

function postPaths() {
  return apiFetch.mock.calls.map((c) => String(c[0]))
}

describe('LspClient owning-chat-id race (cold-boot "no owning chat recorded" crash)', () => {
  beforeEach(() => {
    apiFetch.mockClear()
    subscribe.mockClear()
    __resetWorkspaceScopesForTest()
    wsIdHolder.current = 'ws-race'
    // The route records a workspace's scope with NO chat id; only the
    // sidebar's separate async chat-list fetch attaches one later.
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-race' })
  })

  it('onDiagnosticsUpdate does not throw and does not subscribe before an owning chat id is recorded', () => {
    const client = new LspClient()
    expect(() => client.onDiagnosticsUpdate(() => {})).not.toThrow()
    expect(subscribe).not.toHaveBeenCalled()
  })

  it('subscribes to the diagnostics topic once the owning chat id arrives', () => {
    const client = new LspClient()
    client.onDiagnosticsUpdate(() => {})
    expect(subscribe).not.toHaveBeenCalled()

    recordWorkspaceScope({
      projectId: 'p1',
      repoId: 'r1',
      wsId: 'ws-race',
      owningChatId: 'chat-race',
    })

    expect(subscribe).toHaveBeenCalledWith(LSP_WS_TOPIC, expect.any(Function))
  })

  it('a diagnostics handler registered before the id arrives still receives diagnostics dispatched after', () => {
    const client = new LspClient()
    const handler = vi.fn()
    client.onDiagnosticsUpdate(handler)

    recordWorkspaceScope({
      projectId: 'p1',
      repoId: 'r1',
      wsId: 'ws-race',
      owningChatId: 'chat-race',
    })

    const calls = subscribe.mock.calls as unknown as [string, (frame: unknown) => void][]
    const [, onFrame] = calls.at(-1)!
    const diag = {
      filePath: '/src/a.ts',
      range: { start: { line: 0, character: 0 }, end: { line: 0, character: 1 } },
      severity: 'error',
      message: 'boom',
    }
    onFrame({ wsId: 'ws-race', diagnostics: [diag] })

    expect(handler).toHaveBeenCalledWith('/src/a.ts', [diag])
  })

  it('documentOpen defers the didOpen POST until the owning chat id arrives, then flushes it', async () => {
    const client = new LspClient()
    await client.documentOpen('/src/a.ts', 'code', 'typescript')
    expect(apiFetch).not.toHaveBeenCalled()

    recordWorkspaceScope({
      projectId: 'p1',
      repoId: 'r1',
      wsId: 'ws-race',
      owningChatId: 'chat-race',
    })

    expect(postPaths()).toContain('/v0/chats/chat-race/lsp/didOpen')
    const call = apiFetch.mock.calls.find(([url]) => String(url).endsWith('/didOpen'))!
    expect(JSON.parse(String((call[1] as RequestInit).body))).toEqual({
      path: '/src/a.ts',
      languageId: 'typescript',
      text: 'code',
    })
  })

  it('a document closed before its didOpen went out is never sent, and does not resurface later', () => {
    const client = new LspClient()
    void client.documentOpen('/src/a.ts', 'code', 'typescript')
    void client.documentClose('/src/a.ts')

    recordWorkspaceScope({
      projectId: 'p1',
      repoId: 'r1',
      wsId: 'ws-race',
      owningChatId: 'chat-race',
    })

    expect(apiFetch).not.toHaveBeenCalled()
  })

  it('getDefinition resolves to null instead of throwing before the owning chat id is recorded', async () => {
    const client = new LspClient()
    await expect(client.getDefinition('src/app.ts', 0, 0)).resolves.toBeNull()
    expect(apiFetch).not.toHaveBeenCalled()
  })

  it('documentChange no-ops instead of throwing before the owning chat id is recorded', async () => {
    const client = new LspClient()
    await expect(client.documentChange('/src/a.ts', 'code')).resolves.toBeUndefined()
    expect(apiFetch).not.toHaveBeenCalled()
  })

  it('a wait for a workspace the user has since navigated away from does not resubscribe to it', () => {
    const client = new LspClient()
    client.onDiagnosticsUpdate(() => {})

    // Navigate away before the sidebar's fetch resolves.
    wsIdHolder.current = 'ws-other'
    recordWorkspaceScope({
      projectId: 'p1',
      repoId: 'r1',
      wsId: 'ws-race',
      owningChatId: 'chat-race',
    })

    expect(subscribe).not.toHaveBeenCalled()
  })

  it('the home workspace never waits on an owning chat id (it has none)', () => {
    setWorkspaceScope({ projectId: 'p1', repoId: '', wsId: 'home-ws' })
    wsIdHolder.current = 'home-ws'
    const client = new LspClient()
    expect(() => client.onDiagnosticsUpdate(() => {})).not.toThrow()
    expect(subscribe).not.toHaveBeenCalled()

    // Recording a chat id for it later (there never is one) must not somehow
    // trigger a subscribe either.
    recordWorkspaceScope({ projectId: 'p1', repoId: '', wsId: 'home-ws' })
    expect(subscribe).not.toHaveBeenCalled()
  })
})
