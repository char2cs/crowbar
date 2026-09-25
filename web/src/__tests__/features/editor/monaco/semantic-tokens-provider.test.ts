/**
 * The viewport provider answers synchronously (Monaco drops async answers on
 * every scroll): the language server's range tokens while its full request
 * is in flight, else shiki, else nothing.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { editor, languages, CancellationToken, Range } from 'monaco-editor'
import type { StateStack } from 'shiki/textmate'

// `semantic-tokens-provider.ts` imports the light `editor.api.js` entry, not
// the bare 'monaco-editor' specifier (see the comment there) — mock that.
vi.mock('monaco-editor/esm/vs/editor/editor.api.js', () => ({
  editor: {},
  languages: { registerDocumentRangeSemanticTokensProvider: vi.fn() },
}))
vi.mock('@/features/editor/lsp/semantic-tokens', () => ({ lspViewportTokens: vi.fn(() => null) }))

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

import { lspViewportTokens } from '@/features/editor/lsp/semantic-tokens'
import { viewportSemanticTokensProvider } from '@/features/editor/monaco/semantic-tokens-provider'
import { SEMANTIC_TOKEN_LEGEND } from '@/features/editor/monaco/semantic-tokens-legend'
import { __resetShikiTokensForTests } from '@/features/editor/monaco/shiki-tokens'

const typeIndex = (type: string) => SEMANTIC_TOKEN_LEGEND.tokenTypes.indexOf(type)

/** Capitalized words are types, `#` a comment, everything else plain source. */
function fakeGrammar(line: string) {
  const tokens: { startIndex: number; endIndex: number; scopes: string[] }[] = []
  for (const match of line.matchAll(/\S+/g)) {
    const word = match[0]
    const scope = word.startsWith('#')
      ? 'comment.line.fake'
      : /^[A-Z]/.test(word)
        ? 'entity.name.type.fake'
        : 'source.fake'
    tokens.push({
      startIndex: match.index,
      endIndex: match.index + word.length,
      scopes: ['source.fake', scope],
    })
  }
  return { tokens, ruleStack: null as unknown as StateStack }
}

/** Drain the dynamic import + promise chain the grammar warm-up builds. */
async function flush(): Promise<void> {
  for (let i = 0; i < 5; i++) await new Promise((resolve) => setTimeout(resolve, 0))
}

function fakeModel(path: string, value: string): editor.ITextModel {
  const lines = value.split('\n')
  return {
    uri: { path, toString: () => `mock://${path}` },
    getVersionId: () => 1,
    getLineCount: () => lines.length,
    getLineContent: (n: number) => lines[n - 1] ?? '',
  } as unknown as editor.ITextModel
}

const range = { startLineNumber: 1, endLineNumber: 10 } as unknown as Range
const cancelToken = { isCancellationRequested: false } as unknown as CancellationToken

function provide(model: editor.ITextModel): languages.SemanticTokens {
  return viewportSemanticTokensProvider.provideDocumentRangeSemanticTokens(
    model,
    range,
    cancelToken,
  ) as languages.SemanticTokens
}

/** Shiki warms in two async steps: the grammar loads, then compiles on a timer. */
async function warm(model: editor.ITextModel): Promise<void> {
  provide(model)
  await flush()
  provide(model)
  await flush()
}

describe('viewportSemanticTokensProvider', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    __resetShikiTokensForTests()
    for (const key of Object.keys(shiki.bundledLanguages)) delete shiki.bundledLanguages[key]
    shiki.tokenizeLine.mockImplementation(fakeGrammar)
  })

  it('answers synchronously before the grammar is warm', () => {
    shiki.bundledLanguages.go = () => {}
    const result = viewportSemanticTokensProvider.provideDocumentRangeSemanticTokens(
      fakeModel('/cold.go', 'Foo()\n'),
      range,
      cancelToken,
    )
    expect(result).not.toBeInstanceOf(Promise)
    expect((result as languages.SemanticTokens).data).toHaveLength(0)
  })

  it('colors identifiers from shiki once the grammar is warm', async () => {
    shiki.bundledLanguages.go = () => {}
    const model = fakeModel('/warm.go', 'Widget thing # note\n')
    await warm(model)

    const data = provide(model).data
    // Only `Widget`: Monarch colors go's comments, so shiki leaves them alone.
    expect(Array.from(data)).toEqual([0, 0, 6, typeIndex('type'), 0])
  })

  it('also colors comments where Monaco has no grammar at all', async () => {
    shiki.bundledLanguages.nix = () => {}
    const model = fakeModel('/flake.nix', 'Widget # note\n')
    await warm(model)

    const types = Array.from(provide(model).data).filter((_, i) => i % 5 === 3)
    expect(types).toEqual([typeIndex('type'), typeIndex('comment')])
  })

  it("prefers the language server's tokens when it has them", () => {
    const fromServer = { data: Uint32Array.from([0, 0, 3, 12, 0]) }
    vi.mocked(lspViewportTokens).mockReturnValueOnce(fromServer)

    expect(provide(fakeModel('/served.go', 'Foo()\n'))).toBe(fromServer)
    expect(shiki.tokenizeLine).not.toHaveBeenCalled()
  })

  it('returns empty for an unknown file type', () => {
    expect(provide(fakeModel('/x.unknownext', 'Foo()\n')).data).toHaveLength(0)
  })
})
