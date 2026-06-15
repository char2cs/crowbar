/**
 * One generic Document Range Semantic Tokens provider that feeds the editor from
 * the existing Tree-sitter worker. Registered for all languages ('*'); gates
 * internally on whether a grammar exists, and caches languages with no grammar
 * so we don't refetch a missing parser on every viewport request.
 */
import { languages } from 'monaco-editor'
import { getLanguageAssetConfig } from '@/features/editor/lib/wasm-parser/extension-assets'
import { tokenizerWorkerClient } from '@/features/editor/lib/wasm-parser/tokenizer-worker-client'
import { getLanguageIdFromPath } from '@/features/editor/utils/language-id'
import { SEMANTIC_TOKEN_LEGEND, encodeTokens } from './semantic-tokens-encode'

const EMPTY: languages.SemanticTokens = { data: new Uint32Array(0) }
const unsupportedLanguages = new Set<string>()

export const treeSitterSemanticTokensProvider: languages.DocumentRangeSemanticTokensProvider = {
  getLegend: () => SEMANTIC_TOKEN_LEGEND,

  async provideDocumentRangeSemanticTokens(model, range, token) {
    const languageId = getLanguageIdFromPath(model.uri.path)
    if (!languageId || unsupportedLanguages.has(languageId)) return EMPTY

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
      return { data: encodeTokens(result.tokens, (row) => model.getLineLength(row + 1)) }
    } catch {
      unsupportedLanguages.add(languageId)
      return EMPTY
    }
  },
}

let registered = false
export function registerTreeSitterSemanticTokens(): void {
  if (registered) return
  registered = true
  languages.registerDocumentRangeSemanticTokensProvider('*', treeSitterSemanticTokensProvider)
}
