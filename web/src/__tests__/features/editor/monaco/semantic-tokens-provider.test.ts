import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/features/editor/lib/wasm-parser/tokenizer-worker-client', () => ({
  tokenizerWorkerClient: { tokenize: vi.fn() },
}))
vi.mock('@/features/editor/utils/language-id', () => ({
  getLanguageIdFromPath: vi.fn(),
}))
// Mock Monaco: the provider only uses `editor.tokenize` (heuristic fallback) and
// `languages.register…` (registration, not exercised here). tokenize marks every
// line as plain code so the heuristic classifies identifiers.
vi.mock('monaco-editor', () => ({
  editor: {
    tokenize: vi.fn((text: string) => text.split('\n').map(() => [{ offset: 0, type: '' }])),
  },
  languages: { registerDocumentRangeSemanticTokensProvider: vi.fn() },
}))

import { tokenizerWorkerClient } from '@/features/editor/lib/wasm-parser/tokenizer-worker-client'
import { getLanguageIdFromPath } from '@/features/editor/utils/language-id'
import { treeSitterSemanticTokensProvider } from '@/features/editor/monaco/semantic-tokens-provider'

const cancel = { isCancellationRequested: false } as any
function model(path: string, value = 'x', lang = 'go') {
  const lines = value.split('\n')
  return {
    uri: { path, toString: () => `file://${path}` },
    getValue: () => value,
    getVersionId: () => 1,
    getLineCount: () => lines.length,
    getLineContent: (n: number) => lines[n - 1] ?? '',
    getLineLength: (n: number) => (lines[n - 1] ?? '').length,
    getLanguageId: () => lang,
  } as any
}
const range = { startLineNumber: 1, endLineNumber: 10 } as any

describe('treeSitterSemanticTokensProvider', () => {
  beforeEach(() => vi.clearAllMocks())
  afterEach(() => vi.restoreAllMocks())

  it('uses precise Tree-sitter tokens when the grammar yields them', async () => {
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
    const r = await treeSitterSemanticTokensProvider.provideDocumentRangeSemanticTokens(
      model('/main.go'),
      range,
      cancel,
    )
    expect(r!.data.length).toBe(5)
    expect(tokenizerWorkerClient.tokenize).toHaveBeenCalledOnce()
  })

  it('returns empty for an unknown extension (no language id → worker not called)', async () => {
    ;(getLanguageIdFromPath as any).mockReturnValue(null)
    const r = await treeSitterSemanticTokensProvider.provideDocumentRangeSemanticTokens(
      model('/x.unknown', 'newRootCmd()\n'),
      range,
      cancel,
    )
    // No path-derived language id → bail before any heuristic/worker work.
    expect(tokenizerWorkerClient.tokenize).not.toHaveBeenCalled()
    expect(r!.data.length).toBe(0)
  })

  it('falls back to the heuristic when the worker fails, and caches the language', async () => {
    ;(getLanguageIdFromPath as any).mockReturnValue('zonk')
    ;(tokenizerWorkerClient.tokenize as any).mockRejectedValue(new Error('no wasm'))
    const r1 = await treeSitterSemanticTokensProvider.provideDocumentRangeSemanticTokens(
      model('/a.zonk', 'Foo()\n'),
      range,
      cancel,
    )
    expect(r1!.data.length).toBe(5) // heuristic colored Foo()
    // second call: language cached unsupported → worker NOT retried, heuristic again
    const r2 = await treeSitterSemanticTokensProvider.provideDocumentRangeSemanticTokens(
      model('/b.zonk', 'Bar()\n'),
      range,
      cancel,
    )
    expect(r2!.data.length).toBe(5)
    expect(tokenizerWorkerClient.tokenize).toHaveBeenCalledOnce()
  })

  it('returns empty when cancelled (no heuristic work)', async () => {
    ;(getLanguageIdFromPath as any).mockReturnValue(null)
    const cancelled = { isCancellationRequested: true } as any
    const r = await treeSitterSemanticTokensProvider.provideDocumentRangeSemanticTokens(
      model('/x.unknown', 'Foo()\n'),
      range,
      cancelled,
    )
    expect(r!.data.length).toBe(0)
  })
})
