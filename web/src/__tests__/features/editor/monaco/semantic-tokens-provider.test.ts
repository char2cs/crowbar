/**
 * Tests for the heuristic-first, cancellation-safe semantic tokens provider.
 *
 * The provider is synchronous — it always returns immediately (heuristic or
 * cache) so Monaco's createCancelablePromise settles before _cancelAll() can
 * drop the result. Tree-sitter runs in the background via parseInBackground.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/features/editor/lib/wasm-parser/tokenizer-worker-client', () => ({
  tokenizerWorkerClient: { tokenize: vi.fn() },
}))
vi.mock('@/features/editor/utils/language-id', () => ({
  getLanguageIdFromPath: vi.fn(),
}))
vi.mock('monaco-editor', () => ({
  editor: {},
  languages: { registerDocumentRangeSemanticTokensProvider: vi.fn() },
}))

import { tokenizerWorkerClient } from '@/features/editor/lib/wasm-parser/tokenizer-worker-client'
import { getLanguageIdFromPath } from '@/features/editor/utils/language-id'
import { treeSitterSemanticTokensProvider } from '@/features/editor/monaco/semantic-tokens-provider'

/** Drain pending microtasks so background promises settle. */
async function flush(): Promise<void> {
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
}

function fakeModel(path: string, value = 'x', versionId = 1) {
  const lines = value.split('\n')
  return {
    uri: { path, toString: () => `mock://${path}` },
    getValue: () => value,
    getVersionId: () => versionId,
    isDisposed: () => false,
    getLineCount: () => lines.length,
    getLineContent: (n: number) => lines[n - 1] ?? '',
    getLineLength: (n: number) => (lines[n - 1] ?? '').length,
    getLanguageId: () => 'go',
  } as any
}

const range = { startLineNumber: 1, endLineNumber: 10 } as any
const cancelToken = { isCancellationRequested: false } as any

describe('treeSitterSemanticTokensProvider', () => {
  beforeEach(() => vi.clearAllMocks())
  afterEach(() => vi.restoreAllMocks())

  it('returns heuristic tokens immediately on first call without awaiting tree-sitter', () => {
    ;(getLanguageIdFromPath as any).mockReturnValue('go')
    ;(tokenizerWorkerClient.tokenize as any).mockResolvedValue({ tokens: [], normalizedText: '' })

    const m = fakeModel('/first-call.go', 'Foo()\n')
    const result = treeSitterSemanticTokensProvider.provideDocumentRangeSemanticTokens(
      m, range, cancelToken,
    ) as any

    // Heuristic detects Foo() as a function call → non-empty token data
    expect(result.data.length).toBeGreaterThan(0)
    // Background parse was kicked off
    expect(tokenizerWorkerClient.tokenize).toHaveBeenCalledOnce()
  })

  it('returns tree-sitter tokens from cache on the second call after background parse succeeds', async () => {
    ;(getLanguageIdFromPath as any).mockReturnValue('go')
    ;(tokenizerWorkerClient.tokenize as any).mockResolvedValue({
      tokens: [{
        type: 'function.call',
        startIndex: 0, endIndex: 0,
        startPosition: { row: 0, column: 0 },
        endPosition: { row: 0, column: 5 },
      }],
      normalizedText: 'Foo()',
    })

    const m = fakeModel('/cache-upgrade.go', 'Foo()\n')

    // First call: heuristic + starts background parse
    treeSitterSemanticTokensProvider.provideDocumentRangeSemanticTokens(m, range, cancelToken)
    await flush()

    // Second call: cache hit → tree-sitter token (5 uint32s per token)
    const r2 = treeSitterSemanticTokensProvider.provideDocumentRangeSemanticTokens(
      m, range, cancelToken,
    ) as any
    expect(r2.data.length).toBe(5)
    // Worker called only once — second call was a cache read
    expect(tokenizerWorkerClient.tokenize).toHaveBeenCalledOnce()
  })

  it('returns empty for an unknown file extension without touching the worker', () => {
    ;(getLanguageIdFromPath as any).mockReturnValue(null)

    const r = treeSitterSemanticTokensProvider.provideDocumentRangeSemanticTokens(
      fakeModel('/x.unknown', 'Foo()\n'), range, cancelToken,
    ) as any

    expect(tokenizerWorkerClient.tokenize).not.toHaveBeenCalled()
    expect(r.data.length).toBe(0)
  })

  it('marks the language unsupported after a failed background parse and never retries the worker', async () => {
    ;(getLanguageIdFromPath as any).mockReturnValue('no-wasm-lang')
    ;(tokenizerWorkerClient.tokenize as any).mockRejectedValue(new Error('no wasm'))

    const m1 = fakeModel('/a.nwl', 'Foo()\n')
    const r1 = treeSitterSemanticTokensProvider.provideDocumentRangeSemanticTokens(
      m1, range, cancelToken,
    ) as any
    expect(r1.data.length).toBeGreaterThan(0) // heuristic returned immediately

    await flush() // background parse fails, marks 'no-wasm-lang' unsupported

    const m2 = fakeModel('/b.nwl', 'Bar()\n')
    treeSitterSemanticTokensProvider.provideDocumentRangeSemanticTokens(m2, range, cancelToken)
    expect(tokenizerWorkerClient.tokenize).toHaveBeenCalledOnce() // no retry
  })

  it('does not spawn duplicate background parses for the same model URI', () => {
    ;(getLanguageIdFromPath as any).mockReturnValue('go')
    ;(tokenizerWorkerClient.tokenize as any).mockResolvedValue({ tokens: [], normalizedText: '' })

    const m = fakeModel('/dup-guard.go', 'x')
    treeSitterSemanticTokensProvider.provideDocumentRangeSemanticTokens(m, range, cancelToken)
    treeSitterSemanticTokensProvider.provideDocumentRangeSemanticTokens(m, range, cancelToken)
    treeSitterSemanticTokensProvider.provideDocumentRangeSemanticTokens(m, range, cancelToken)

    expect(tokenizerWorkerClient.tokenize).toHaveBeenCalledOnce()
  })

  it('discards stale background parse results when the model version changed during parse', async () => {
    ;(getLanguageIdFromPath as any).mockReturnValue('go')
    ;(tokenizerWorkerClient.tokenize as any).mockResolvedValue({
      tokens: [{
        type: 'function.call',
        startIndex: 0, endIndex: 0,
        startPosition: { row: 0, column: 0 },
        endPosition: { row: 0, column: 5 },
      }],
      normalizedText: 'x',
    })

    const staleModel = fakeModel('/stale.go', 'x', 1)
    treeSitterSemanticTokensProvider.provideDocumentRangeSemanticTokens(staleModel, range, cancelToken)
    await flush() // cache populated with versionId=1

    // Same URI, newer version — simulates an edit while the parse was in flight
    const freshModel = fakeModel('/stale.go', 'y', 2)
    const r = treeSitterSemanticTokensProvider.provideDocumentRangeSemanticTokens(
      freshModel, range, cancelToken,
    ) as any

    // Cache miss (version mismatch) → heuristic for 'y' → no tokens
    expect(r.data.length).toBe(0)
  })
})
