/**
 * One generic Document Range Semantic Tokens provider, registered for all
 * languages ('*'). Two backends, in priority order:
 *
 *   1. Tree-sitter — precise tokens from the existing wasm worker, when a
 *      grammar is provisioned for the language. Missing-grammar languages are
 *      cached so we don't refetch a 404 on every viewport request.
 *   2. Heuristic fallback — grammar-free, language-agnostic. Reuses Monaco's own
 *      per-language tokenization to find plain-identifier regions and colors
 *      function calls / type-like / ALL_CAPS names. Renders without any grammar.
 *
 * Either way the tokens flow through `encodeTokens` + the theme's semantic rules.
 */
import { editor as monacoEditor, languages } from 'monaco-editor'
import { getLanguageAssetConfig } from '@/features/editor/lib/wasm-parser/extension-assets'
import { tokenizerWorkerClient } from '@/features/editor/lib/wasm-parser/tokenizer-worker-client'
import { getLanguageIdFromPath } from '@/features/editor/utils/language-id'
import { heuristicTokensFromLineTokens, type LineToken } from './heuristic-tokens'
import { SEMANTIC_TOKEN_LEGEND, encodeTokens } from './semantic-tokens-encode'

// Shared, reused across calls — safe only because it carries no mutable data.
const EMPTY: languages.SemanticTokens = { data: new Uint32Array(0) }
const unsupportedLanguages = new Set<string>()

/** Grammar-free fallback: classify identifiers using Monaco's own tokenization. */
function heuristicTokens(
  model: monacoEditor.ITextModel,
  range: { startLineNumber: number; endLineNumber: number },
): languages.SemanticTokens {
  const text = model.getValue()
  const lineTokens = monacoEditor.tokenize(text, model.getLanguageId()) as LineToken[][]
  const tokens = heuristicTokensFromLineTokens(text, lineTokens, {
    emitStartRow: range.startLineNumber - 1,
    emitEndRow: range.endLineNumber - 1,
  })
  return { data: encodeTokens(tokens, (row) => model.getLineLength(row + 1)) }
}

export const treeSitterSemanticTokensProvider: languages.DocumentRangeSemanticTokensProvider = {
  getLegend: () => SEMANTIC_TOKEN_LEGEND,

  async provideDocumentRangeSemanticTokens(model, range, token) {
    const languageId = getLanguageIdFromPath(model.uri.path)

    // 1. Precise Tree-sitter tokens, when a grammar is available.
    if (languageId && !unsupportedLanguages.has(languageId)) {
      try {
        const assets = getLanguageAssetConfig(languageId)
        const result = await tokenizerWorkerClient.tokenize({
          bufferId: model.uri.toString(),
          content: model.getValue(),
          languageId,
          wasmPath: assets.wasmPath,
          highlightQueryUrl: assets.highlightQueryUrl,
          mode: 'range',
          viewportRange: {
            startLine: range.startLineNumber - 1,
            endLine: range.endLineNumber - 1,
          },
        })
        if (token.isCancellationRequested) return EMPTY
        const data = encodeTokens(result.tokens, (row) => model.getLineLength(row + 1))
        if (data.length > 0) return { data }
        // Grammar yielded nothing → fall through to the heuristic.
      } catch {
        // Almost always a missing grammar — cache so we don't refetch the 404
        // every scroll. Don't poison the cache on a cancelled (superseded) request.
        if (!token.isCancellationRequested) unsupportedLanguages.add(languageId)
      }
    }

    // 2. Grammar-free heuristic — works for every language.
    if (token.isCancellationRequested) return EMPTY
    return heuristicTokens(model, range)
  },
}

let registered = false
export function registerTreeSitterSemanticTokens(): void {
  if (registered) return
  registered = true
  languages.registerDocumentRangeSemanticTokensProvider('*', treeSitterSemanticTokensProvider)
}
