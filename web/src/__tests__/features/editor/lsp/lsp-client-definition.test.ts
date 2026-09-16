import { describe, it, expect, vi, beforeEach } from 'vitest'

// Cmd+Click go-to-definition never left the browser: `LspClient.getDefinition`
// was a stub that returned `null` unconditionally, so the daemon's
// POST /v0/.../lsp/definition route — which has existed all along — was never
// called and the editor's only feedback was a `logger.debug('No definition
// found')`. These lock the wire contract: workspace-relative path + 0-based
// position out, the daemon's workspace-relative `{filePath, range}` locations
// back.

const { apiFetch } = vi.hoisted(() => ({
  apiFetch: vi.fn(async (..._args: unknown[]): Promise<unknown> => []),
}))
const { subscribe } = vi.hoisted(() => ({ subscribe: vi.fn(() => () => {}) }))

vi.mock('@/lib/api', () => ({ apiFetch }))
vi.mock('@/lib/ws/manager', () => ({ wsManager: { subscribe } }))
vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getActiveWorkspaceId: () => 'ws-1',
}))

import { LspClient } from '@/features/editor/lsp/lsp-client'
import { setWorkspaceScope } from '@/lib/workspace-scope'

// chat-scoped API spec §4.2's owned bucket (§8 step 5): editor/LSP is
// addressed by the chat that owns the worktree, not the workspace.
const DEFINITION_URL = '/v0/chats/chat-1/lsp/definition'

function lastCall() {
  const call = apiFetch.mock.calls.at(-1)
  if (!call) throw new Error('apiFetch was never called')
  return { url: String(call[0]), init: call[1] as RequestInit }
}

describe('LspClient.getDefinition', () => {
  beforeEach(() => {
    apiFetch.mockClear()
    apiFetch.mockResolvedValue([])
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-1', owningChatId: 'chat-1' })
  })

  it('POSTs the workspace-relative path and 0-based position to the definition route', async () => {
    const client = new LspClient()
    await client.getDefinition('src/app.ts', 12, 4)

    const { url, init } = lastCall()
    expect(url).toBe(DEFINITION_URL)
    expect(init.method).toBe('POST')
    expect(JSON.parse(String(init.body))).toEqual({
      path: 'src/app.ts',
      position: { line: 12, character: 4 },
    })
  })

  it('returns the daemon locations with their workspace-relative file paths', async () => {
    const range = { start: { line: 3, character: 2 }, end: { line: 3, character: 9 } }
    apiFetch.mockResolvedValue([{ filePath: 'src/lib/target.ts', range }])

    const client = new LspClient()
    const definitions = await client.getDefinition('src/app.ts', 12, 4)

    expect(definitions).toEqual([{ filePath: 'src/lib/target.ts', range }])
  })

  it('returns null when no server resolves the symbol', async () => {
    apiFetch.mockResolvedValue([])

    const client = new LspClient()
    expect(await client.getDefinition('src/app.ts', 12, 4)).toBeNull()
  })

  it('propagates a daemon failure so the caller can surface it', async () => {
    apiFetch.mockRejectedValue(new Error('lsp: textDocument/definition: timeout'))

    const client = new LspClient()
    await expect(client.getDefinition('src/app.ts', 12, 4)).rejects.toThrow('timeout')
  })
})
