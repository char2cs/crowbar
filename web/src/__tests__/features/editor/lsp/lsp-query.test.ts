import { describe, expect, it, vi } from 'vitest'
import type * as Monaco from 'monaco-editor'

const { request, flushChange } = vi.hoisted(() => ({
  request: vi.fn(async () => ({ ok: true })),
  flushChange: vi.fn(async () => {}),
}))
vi.mock('@/features/editor/lsp/lsp-client', () => ({
  LspClient: { getInstance: () => ({ request, flushChange }) },
}))

import { documentUri } from '@/features/editor/documents/document-ref'
import { query } from '@/features/editor/lsp/lsp-query'
import { workspaceRef, type DocumentRef } from '@/lib/api/documents'

const modelOf = (doc: DocumentRef) =>
  ({ uri: { toString: () => documentUri(doc) } }) as unknown as Monaco.editor.ITextModel
const token = {
  isCancellationRequested: false,
  onCancellationRequested: () => ({ dispose: () => {} }),
} as unknown as Monaco.CancellationToken

describe('query', () => {
  it('asks the language server of the model’s own workspace', async () => {
    const answer = await query(modelOf(workspaceRef('ws-1', 'src/a.ts')), 'hover', {}, token)

    expect(answer?.target).toEqual({ wsId: 'ws-1', path: 'src/a.ts' })
    expect(request).toHaveBeenCalledWith('ws-1', 'hover', { path: 'src/a.ts' }, expect.anything())
  })

  it('answers null for a document outside every workspace, asking nothing', async () => {
    request.mockClear()

    expect(
      await query(modelOf({ authority: 'host', path: '/tmp/a.ts' }), 'hover', {}, token),
    ).toBeNull()
    expect(request).not.toHaveBeenCalled()
  })
})
