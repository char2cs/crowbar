/**
 * Tests for the two-tier, cancellation-safe semantic tokens provider.
 *
 * The provider is synchronous — it always returns immediately (tree-sitter
 * cache, shiki, or nothing) so Monaco's createCancelablePromise settles before
 * _cancelAll() can drop the result. Both engines warm in the background.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { editor, languages, CancellationToken, Range } from 'monaco-editor'
import type { StateStack } from 'shiki/textmate'

vi.mock('@/features/editor/lib/wasm-parser/tokenizer-worker-client', () => ({
  tokenizerWorkerClient: { tokenize: vi.fn() },
}))
vi.mock('@/features/editor/utils/language-id', () => ({
  getLanguageIdFromPath: vi.fn(),
}))
// `semantic-tokens-provider.ts` imports the light `editor.api.js` entry, not
// the bare 'monaco-editor' specifier (see the comment there) — mock that.
vi.mock('monaco-editor/esm/vs/editor/editor.api.js', () => ({
  editor: {},
  languages: { registerDocumentRangeSemanticTokensProvider: vi.fn() },
}))

const shiki = vi.hoisted(() => ({
  bundledLanguages: {} as Record<string, unknown>,
  tokenizeLine: vi.fn(),
}))

vi.mock('shiki/bundle/full', () => ({
  bundledLanguages: shiki.bundledLanguages,
  getSingletonHighlighter: vi.fn(async () => ({
    loadLanguage: async () => {},
    getInternalContext: () => ({ getLanguage: () => ({ tokenizeLine: shiki.tokenizeLine }) }),
  })),
}))

import { tokenizerWorkerClient } from '@/features/editor/lib/wasm-parser/tokenizer-worker-client'
import { getLanguageIdFromPath } from '@/features/editor/utils/language-id'
import { treeSitterSemanticTokensProvider } from '@/features/editor/monaco/semantic-tokens-provider'
import { __resetShikiTokensForTests } from '@/features/editor/monaco/shiki-tokens'

const mockGetLanguageIdFromPath = vi.mocked(getLanguageIdFromPath)
const mockTokenize = vi.mocked(tokenizerWorkerClient.tokenize)

/** Every capitalized word becomes a type; everything else is left alone. */
function capitalizedWordsAreTypes(line: string) {
  const tokens: { startIndex: number; endIndex: number; scopes: string[] }[] = []
  for (const match of line.matchAll(/\S+/g)) {
    tokens.push({
      startIndex: match.index,
      endIndex: match.index + match[0].length,
      scopes: ['source.fake', /^[A-Z]/.test(match[0]) ? 'entity.name.type.fake' : 'source.fake'],
    })
  }
  return { tokens, ruleStack: null as unknown as StateStack }
}

/** Drain the dynamic import + promise chain the engines build. */
async function flush(): Promise<void> {
  for (let i = 0; i < 5; i++) await new Promise((resolve) => setTimeout(resolve, 0))
}

function fakeModel(path: string, value = 'x', versionId = 1): editor.ITextModel {
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
  } as unknown as editor.ITextModel
}

const range = { startLineNumber: 1, endLineNumber: 10 } as unknown as Range
const cancelToken = { isCancellationRequested: false } as unknown as CancellationToken

function tokens(
  result: languages.ProviderResult<languages.SemanticTokens>,
): languages.SemanticTokens {
  return result as languages.SemanticTokens
}

function provide(model: editor.ITextModel): languages.SemanticTokens {
  return tokens(
    treeSitterSemanticTokensProvider.provideDocumentRangeSemanticTokens(model, range, cancelToken),
  )
}

describe('treeSitterSemanticTokensProvider', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    __resetShikiTokensForTests()
    for (const key of Object.keys(shiki.bundledLanguages)) delete shiki.bundledLanguages[key]
    shiki.tokenizeLine.mockImplementation(capitalizedWordsAreTypes)
  })
  afterEach(() => vi.restoreAllMocks())

  it('answers synchronously on the first call, before either engine is warm', () => {
    mockGetLanguageIdFromPath.mockReturnValue('go')
    mockTokenize.mockResolvedValue({ tokens: [], normalizedText: '' })

    const result = treeSitterSemanticTokensProvider.provideDocumentRangeSemanticTokens(
      fakeModel('/first-call.go', 'Foo()\n'),
      range,
      cancelToken,
    )

    expect(result).not.toBeInstanceOf(Promise)
    expect(tokens(result).data).toBeInstanceOf(Uint32Array)
    expect(tokenizerWorkerClient.tokenize).toHaveBeenCalledOnce()
  })

  it('returns tree-sitter tokens from cache on the second call after background parse succeeds', async () => {
    mockGetLanguageIdFromPath.mockReturnValue('go')
    mockTokenize.mockResolvedValue({
      tokens: [
        {
          type: 'function.call',
          startIndex: 0,
          endIndex: 0,
          startPosition: { row: 0, column: 0 },
          endPosition: { row: 0, column: 5 },
        },
      ],
      normalizedText: 'Foo()',
    })

    const m = fakeModel('/cache-upgrade.go', 'Foo()\n')

    provide(m)
    await flush()

    // Second call: cache hit → tree-sitter token (5 uint32s per token)
    expect(provide(m).data.length).toBe(5)
    // Worker called only once — second call was a cache read
    expect(tokenizerWorkerClient.tokenize).toHaveBeenCalledOnce()
  })

  it('never consults shiki once the tree-sitter cache can answer', async () => {
    mockGetLanguageIdFromPath.mockReturnValue('shikilang')
    shiki.bundledLanguages.shikilang = () => {}
    mockTokenize.mockResolvedValue({
      tokens: [
        {
          type: 'function.call',
          startIndex: 0,
          endIndex: 0,
          startPosition: { row: 0, column: 0 },
          endPosition: { row: 0, column: 3 },
        },
      ],
      normalizedText: 'Foo',
    })

    const m = fakeModel('/tree-wins.sl', 'Foo\n')
    provide(m)
    await flush()

    shiki.tokenizeLine.mockClear()
    expect(provide(m).data.length).toBe(5)
    expect(shiki.tokenizeLine).not.toHaveBeenCalled()
  })

  it('colors a language tree-sitter has no parser for with shiki', async () => {
    mockGetLanguageIdFromPath.mockReturnValue('fallbacklang')
    shiki.bundledLanguages.fallbacklang = () => {}
    mockTokenize.mockRejectedValue(new Error('no wasm'))

    const m = fakeModel('/fallback.fl', 'Widget thing\n')

    // Nothing is warm yet, but every call still answers synchronously.
    expect(provide(m).data.length).toBe(0)
    await flush() // tree-sitter gives up; shiki's grammar loads
    expect(provide(m).data.length).toBe(0)
    await flush() // shiki compiles its rules against the first viewport

    const data = provide(m).data
    expect(data.length).toBe(5)
    expect(data[2]).toBe(6) // `Widget`
  })

  it('returns empty for an unknown file extension without touching the worker', () => {
    mockGetLanguageIdFromPath.mockReturnValue(null)

    const r = provide(fakeModel('/x.unknown', 'Foo()\n'))

    expect(tokenizerWorkerClient.tokenize).not.toHaveBeenCalled()
    expect(r.data.length).toBe(0)
  })

  it('marks the language unsupported after a failed background parse and never retries the worker', async () => {
    mockGetLanguageIdFromPath.mockReturnValue('no-wasm-lang')
    mockTokenize.mockRejectedValue(new Error('no wasm'))

    provide(fakeModel('/a.nwl', 'Foo()\n'))
    await flush() // background parse fails, marks 'no-wasm-lang' unsupported

    provide(fakeModel('/b.nwl', 'Bar()\n'))
    expect(tokenizerWorkerClient.tokenize).toHaveBeenCalledOnce() // no retry
  })

  it('does not spawn duplicate background parses for the same model URI', () => {
    mockGetLanguageIdFromPath.mockReturnValue('go')
    mockTokenize.mockResolvedValue({ tokens: [], normalizedText: '' })

    const m = fakeModel('/dup-guard.go', 'x')
    provide(m)
    provide(m)
    provide(m)

    expect(tokenizerWorkerClient.tokenize).toHaveBeenCalledOnce()
  })

  it('discards stale background parse results when the model version changed during parse', async () => {
    mockGetLanguageIdFromPath.mockReturnValue('stalelang')
    mockTokenize.mockResolvedValue({
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

    provide(fakeModel('/stale.sla', 'x', 1))
    await flush() // cache populated with versionId=1

    // Same URI, newer version — simulates an edit while the parse was in flight.
    // 'stalelang' has no shiki grammar either, so the miss yields nothing.
    expect(provide(fakeModel('/stale.sla', 'y', 2)).data.length).toBe(0)
  })
})
