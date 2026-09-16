import { describe, it, expect, vi, beforeEach } from 'vitest'

// LspClient is a single global singleton subscribed to ONE workspace's
// diagnostics topic at a time, but `handlers` accumulates one per mounted
// pane — including a pane showing a DIFFERENT (non-active) workspace's file,
// which stays registered the whole time. A bare filePath match (the only
// thing the diagnostics handler used to check) hands a background pane
// another workspace's diagnostics for a file that merely happens to share its
// relative path — two worktrees of the same repo, say. Same bleed shape as
// the Monaco model URI collision this session already root-caused, one layer
// up: `dispatch`/`onDiagnosticsUpdate` now pass the batch's own `wsId` to
// every handler so a pane can ignore a batch that isn't for its own
// workspace, instead of only checking the path.

const { apiFetch } = vi.hoisted(() => ({
  apiFetch: vi.fn(async (..._args: unknown[]): Promise<unknown> => []),
}))
const { subscribe } = vi.hoisted(() => ({ subscribe: vi.fn(() => () => {}) }))
const { wsIdHolder } = vi.hoisted(() => ({ wsIdHolder: { current: 'ws-a' as string | null } }))

vi.mock('@/lib/api', () => ({ apiFetch }))
vi.mock('@/lib/ws/manager', () => ({ wsManager: { subscribe } }))
vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getActiveWorkspaceId: () => wsIdHolder.current,
}))

import { LspClient } from '@/features/editor/lsp/lsp-client'
import { setWorkspaceScope, __resetWorkspaceScopesForTest } from '@/lib/workspace-scope'

// Topics are keyed by owning chat id, not wsId directly, so this grabs the
// LAST subscribed handler — always the one for whichever workspace
// ensureSubscribed() most recently (re)subscribed to.
function latestFrame(): (frame: unknown) => void {
  const calls = subscribe.mock.calls as unknown as [string, (frame: unknown) => void][]
  return calls.at(-1)![1]
}

const diag = (filePath: string) => ({
  filePath,
  range: { start: { line: 0, character: 0 }, end: { line: 0, character: 1 } },
  severity: 'error' as const,
  message: 'boom',
})

describe('LspClient — diagnostics batches carry their own wsId', () => {
  beforeEach(() => {
    apiFetch.mockClear()
    subscribe.mockClear()
    __resetWorkspaceScopesForTest()
    wsIdHolder.current = 'ws-a'
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-a', owningChatId: 'chat-a' })
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-b', owningChatId: 'chat-b' })
  })

  it('passes the dispatched wsId as the handler’s third argument', () => {
    const client = new LspClient()
    const handler = vi.fn()
    client.onDiagnosticsUpdate(handler)

    latestFrame()({ wsId: 'ws-a', diagnostics: [diag('/src/shared.ts')] })

    expect(handler).toHaveBeenCalledWith('/src/shared.ts', [diag('/src/shared.ts')], 'ws-a')
  })

  // The decisive regression: two workspaces sharing a relative path must be
  // distinguishable by a handler even though the SAME singleton, and the SAME
  // filePath, is involved in both dispatches.
  it('a handler can tell apart two different workspaces dispatching diagnostics for the SAME path', () => {
    const client = new LspClient()
    const seen: Array<{ wsId: string; count: number }> = []
    client.onDiagnosticsUpdate((_fp, diagnostics, wsId) => {
      seen.push({ wsId, count: diagnostics.length })
    })

    latestFrame()({ wsId: 'ws-a', diagnostics: [diag('/src/shared.ts')] })

    // The pane switches to a different workspace; the singleton resubscribes.
    wsIdHolder.current = 'ws-b'
    client.onDiagnosticsUpdate(() => {})
    latestFrame()({ wsId: 'ws-b', diagnostics: [diag('/src/shared.ts'), diag('/src/other.ts')] })

    expect(seen.some((s) => s.wsId === 'ws-a')).toBe(true)
    expect(seen.some((s) => s.wsId === 'ws-b')).toBe(true)
    // A consumer filtering on `wsId !== myOwnWorkspaceId` (use-pane-editor-
    // satellites.ts's applyMarkers) can now reject the wrong-workspace batch
    // instead of matching on path alone and painting it anyway.
  })

  it('replays the last batch to a late subscriber tagged with the workspace it actually belongs to', () => {
    const client = new LspClient()
    client.onDiagnosticsUpdate(() => {})
    latestFrame()({ wsId: 'ws-a', diagnostics: [diag('/src/shared.ts')] })

    const lateHandler = vi.fn()
    client.onDiagnosticsUpdate(lateHandler)

    expect(lateHandler).toHaveBeenCalledWith('/src/shared.ts', [diag('/src/shared.ts')], 'ws-a')
  })
})
