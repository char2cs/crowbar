import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { editor as monacoEditor, Uri } from 'monaco-editor/esm/vs/editor/editor.api.js'
import type * as Monaco from 'monaco-editor'

// A pane's document belongs to the pane's OWN workspace: switching which
// workspace is active must send nothing for a hidden pane, and never address
// another workspace's language server with this buffer's text.

const { apiFetch } = vi.hoisted(() => ({
  apiFetch: vi.fn(async (..._args: unknown[]): Promise<unknown> => ({ ok: true })),
}))
const { subscribe } = vi.hoisted(() => ({ subscribe: vi.fn(() => () => {}) }))

vi.mock('@/lib/api', () => ({ apiFetch }))
vi.mock('@/lib/ws/manager', () => ({ wsManager: { subscribe } }))

import { useLspDocumentSync } from '@/features/editor/hooks/use-lsp-document-sync'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import {
  __resetWorkspaceScopesForTest,
  recordWorkspaceScope,
  setWorkspaceScope,
} from '@/lib/workspace-scope'

const posts = () => apiFetch.mock.calls.map((c) => String(c[0]))

describe('useLspDocumentSync', () => {
  let model: Monaco.editor.ITextModel

  beforeEach(() => {
    vi.useFakeTimers()
    apiFetch.mockClear()
    subscribe.mockClear()
    __resetWorkspaceScopesForTest()
    model = monacoEditor.createModel('let a = 1', 'typescript', Uri.parse('file:///w1/src/a.ts'))
    setWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'w1', owningChatId: 'chat-1' })
    setActiveWorkspaceId('w1')
  })
  afterEach(() => {
    model.dispose()
    vi.useRealTimers()
  })

  it("opens and closes the document on its own workspace's chat", async () => {
    const { unmount } = renderHook(() => useLspDocumentSync(model, 'src/a.ts', 'typescript', 'w1'))
    await act(() => vi.runAllTimersAsync())
    expect(posts()).toEqual(['/v0/chats/chat-1/lsp/didOpen'])

    unmount()
    await act(() => vi.runAllTimersAsync())
    expect(posts()).toEqual(['/v0/chats/chat-1/lsp/didOpen', '/v0/chats/chat-1/lsp/didClose'])
  })

  it('sends nothing for a hidden pane when another workspace becomes active', async () => {
    const { rerender } = renderHook(() => useLspDocumentSync(model, 'src/a.ts', 'typescript', 'w1'))
    await act(() => vi.runAllTimersAsync())
    model.setValue('let a = 2 // unsaved')
    await act(() => vi.runAllTimersAsync())
    apiFetch.mockClear()

    // W2 is forked and becomes active: the route records it before the
    // sidebar knows its owning chat, then the chat lands.
    setActiveWorkspaceId('w2')
    setWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'w2' })
    rerender()
    act(() =>
      recordWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'w2', owningChatId: 'chat-2' }),
    )
    rerender()
    await act(() => vi.runAllTimersAsync())

    expect(posts()).toEqual([])
  })
})
