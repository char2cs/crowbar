import { beforeEach, describe, expect, it, vi } from 'vitest'
import type * as Monaco from 'monaco-editor'

vi.mock('@/features/editor/lsp/lsp-query', () => ({ query: vi.fn() }))
vi.mock('monaco-editor/esm/vs/editor/editor.api.js', () => ({
  CancellationTokenSource: class {
    token = { isCancellationRequested: false, onCancellationRequested: () => ({ dispose() {} }) }
    dispose(cancel?: boolean) {
      if (cancel) this.token.isCancellationRequested = true
    }
  },
}))

import { query } from '@/features/editor/lsp/lsp-query'
import {
  lspDocumentSemanticTokensProvider,
  lspViewportTokens,
} from '@/features/editor/lsp/semantic-tokens'

const mockQuery = vi.mocked(query)
const target = { wsId: 'ws1', path: 'main.go' }
const none = {
  isCancellationRequested: false,
  onCancellationRequested: () => ({ dispose() {} }),
} as unknown as Monaco.CancellationToken

function fakeModel(uri: string, versionId = 1): Monaco.editor.ITextModel {
  return {
    uri: { toString: () => uri },
    getVersionId: () => versionId,
  } as unknown as Monaco.editor.ITextModel
}

const viewport = { startLineNumber: 1, startColumn: 1, endLineNumber: 40, endColumn: 1 }

function full(model: Monaco.editor.ITextModel, lastResultId: string | null = null) {
  return Promise.resolve(
    lspDocumentSemanticTokensProvider.provideDocumentSemanticTokens(model, lastResultId, none),
  )
}

describe('lspDocumentSemanticTokensProvider', () => {
  beforeEach(() => mockQuery.mockReset())

  it('turns a full result into Monaco tokens', async () => {
    mockQuery.mockResolvedValue({ target, result: { resultId: '4', data: [0, 1, 2, 12, 0] } })

    const tokens = (await full(fakeModel('m://full'))) as Monaco.languages.SemanticTokens

    expect(tokens.resultId).toBe('4')
    expect(tokens.data).toEqual(Uint32Array.from([0, 1, 2, 12, 0]))
    expect(mockQuery).toHaveBeenCalledWith(expect.anything(), 'semanticTokens', {}, none)
  })

  it('asks for a delta against the tokens Monaco holds', async () => {
    mockQuery.mockResolvedValue({
      target,
      result: { resultId: '5', edits: [{ start: 3, deleteCount: 1, data: [8] }] },
    })

    const delta = (await full(fakeModel('m://delta'), '4')) as Monaco.languages.SemanticTokensEdits

    expect(mockQuery).toHaveBeenCalledWith(
      expect.anything(),
      'semanticTokens',
      { previousResultId: '4' },
      none,
    )
    expect(delta.edits[0]).toEqual({ start: 3, deleteCount: 1, data: Uint32Array.from([8]) })
  })

  // Resolving null would mark the file's tokens complete-and-empty and stop
  // the grammar tier; a cancellation leaves it running.
  it('rejects as cancelled when no server offers tokens', async () => {
    mockQuery.mockResolvedValue({ target, result: null })

    await expect(full(fakeModel('m://none'))).rejects.toMatchObject({
      name: 'Canceled',
      message: 'Canceled',
    })
  })
})

describe('lspViewportTokens', () => {
  beforeEach(() => mockQuery.mockReset())

  it('only asks the server while the full request is in flight', async () => {
    const model = fakeModel('m://viewport')
    const onReady = vi.fn()
    expect(lspViewportTokens(model, viewport, onReady)).toBeNull()
    expect(mockQuery).not.toHaveBeenCalled()

    let finishFull!: (value: Awaited<ReturnType<typeof query>>) => void
    mockQuery.mockImplementation((_m, route) =>
      route === 'semanticTokens'
        ? new Promise((resolve) => (finishFull = resolve))
        : Promise.resolve({ target, result: { data: [0, 0, 3, 1, 0] } }),
    )
    const pending = full(model)
    await Promise.resolve()

    expect(lspViewportTokens(model, viewport, onReady)).toBeNull()
    await vi.waitFor(() => expect(onReady).toHaveBeenCalledOnce())
    expect(lspViewportTokens(model, viewport, onReady)?.data).toEqual(
      Uint32Array.from([0, 0, 3, 1, 0]),
    )
    expect(mockQuery).toHaveBeenCalledTimes(2)

    finishFull({ target, result: { data: [] } })
    await pending
    expect(lspViewportTokens(model, viewport, onReady)).toBeNull()
  })
})
