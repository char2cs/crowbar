import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/features/editor/lib/wasm-parser/tokenizer-worker-client', () => ({
  tokenizerWorkerClient: { tokenize: vi.fn() },
}))
vi.mock('@/features/editor/utils/language-id', () => ({
  getLanguageIdFromPath: vi.fn(),
}))

import { tokenizerWorkerClient } from '@/features/editor/lib/wasm-parser/tokenizer-worker-client'
import { getLanguageIdFromPath } from '@/features/editor/utils/language-id'
import { treeSitterSemanticTokensProvider } from '@/features/editor/monaco/semantic-tokens-provider'

const cancel = { isCancellationRequested: false } as any
function model(path: string, value = 'x') {
  return {
    uri: { path, toString: () => `file://${path}` },
    getValue: () => value,
    getLineLength: () => 100,
  } as any
}
const range = { startLineNumber: 1, endLineNumber: 10 } as any

describe('treeSitterSemanticTokensProvider', () => {
  beforeEach(() => vi.clearAllMocks())
  afterEach(() => vi.restoreAllMocks())

  it('returns empty for unknown languages (no tokenize call)', async () => {
    ;(getLanguageIdFromPath as any).mockReturnValue(null)
    const r = (await treeSitterSemanticTokensProvider.provideDocumentRangeSemanticTokens(
      model('/x.unknown'),
      range,
      cancel,
    ))!
    expect(r.data.length).toBe(0)
    expect(tokenizerWorkerClient.tokenize).not.toHaveBeenCalled()
  })

  it('encodes worker tokens for a known language', async () => {
    ;(getLanguageIdFromPath as any).mockReturnValue('go')
    ;(tokenizerWorkerClient.tokenize as any).mockResolvedValue({
      tokens: [
        {
          type: 'function.call',
          startIndex: 0,
          endIndex: 0,
          startPosition: { row: 0, column: 0 },
          endPosition: { row: 0, column: 5 },
        },
      ],
      normalizedText: 'x',
    })
    const r = (await treeSitterSemanticTokensProvider.provideDocumentRangeSemanticTokens(
      model('/main.go'),
      range,
      cancel,
    ))!
    expect(r.data.length).toBe(5)
    expect(tokenizerWorkerClient.tokenize).toHaveBeenCalledOnce()
  })

  it('returns empty and caches the language on worker failure', async () => {
    ;(getLanguageIdFromPath as any).mockReturnValue('zonk')
    ;(tokenizerWorkerClient.tokenize as any).mockRejectedValue(new Error('no wasm'))
    const r1 = (await treeSitterSemanticTokensProvider.provideDocumentRangeSemanticTokens(
      model('/a.zonk'),
      range,
      cancel,
    ))!
    expect(r1.data.length).toBe(0)
    // second call must NOT retry the worker (cached as unsupported)
    const r2 = (await treeSitterSemanticTokensProvider.provideDocumentRangeSemanticTokens(
      model('/b.zonk'),
      range,
      cancel,
    ))!
    expect(r2.data.length).toBe(0)
    expect(tokenizerWorkerClient.tokenize).toHaveBeenCalledOnce()
  })
})
