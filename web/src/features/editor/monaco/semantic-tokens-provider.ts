/**
 * One generic Document Range Semantic Tokens provider, registered for all
 * languages ('*'). Two-tier strategy:
 *
 *   1. Tree-sitter — precise tokens from the wasm worker, when a grammar is
 *      provisioned for the language. Missing-grammar languages are permanently
 *      cached in `unsupportedLanguages` after the first failed attempt.
 *
 *   2. Heuristic fallback — grammar-free, language-agnostic coloring via
 *      `heuristicTokensInRange`. Uses only `model.getLineContent()` (O(1)
 *      cache lookup per line) so it never blocks the main thread. Covers the
 *      high-signal cases:  name( → function,  Name → type,  ALL_CAPS → constant.
 *      False-positive rate inside string literals is low and acceptable.
 *
 * The previous heuristic path that called `monaco.editor.tokenize(fullText)`
 * on the main thread has been removed — it caused 60–120ms stalls every ~30s
 * and a self-sustaining fireProviderChange → re-request → tokenize loop.
 */
import { editor, languages } from 'monaco-editor'
import { getLanguageAssetConfig } from '@/features/editor/lib/wasm-parser/extension-assets'
import { tokenizerWorkerClient } from '@/features/editor/lib/wasm-parser/tokenizer-worker-client'
import type { HighlightToken } from '@/features/editor/lib/wasm-parser/types'
import { getLanguageIdFromPath } from '@/features/editor/utils/language-id'
import { heuristicTokensInRange } from './heuristic-tokens'
import type { LineToken } from './heuristic-tokens'
import { SEMANTIC_TOKEN_LEGEND, encodeTokens } from './semantic-tokens-encode'

const EMPTY: languages.SemanticTokens = { data: new Uint32Array(0) }

// Languages whose grammar wasm is not available. Populated on the first
// failed worker call and never cleared — a language is either supported or
// not for the lifetime of the page.
const unsupportedLanguages = new Set<string>()

// Full token set per {modelUri, versionId} — populated once per document
// version from a 'full' tree-sitter parse, then filtered to the viewport on
// every scroll without any worker call or getValue() allocation.
interface FullTokenCache {
  versionId: number
  tokens: HighlightToken[]
}
const fullTokenCache = new Map<string, FullTokenCache>()
const MAX_FULL_TOKEN_CACHE = 10

// Minimal event emitter for the provider's onDidChange signal.
// Only fired when tree-sitter successfully populates a new cache entry so
// Monaco re-requests tokens with real data. Never fired for fallback results.
type ListenerFn = () => void
const providerChangeListeners: ListenerFn[] = []
function fireProviderChange() {
  providerChangeListeners.slice().forEach((l) => l())
}
const providerOnDidChange = (listener: ListenerFn): { dispose(): void } => {
  providerChangeListeners.push(listener)
  return { dispose: () => providerChangeListeners.splice(providerChangeListeners.indexOf(listener), 1) }
}

/**
 * Heuristic fallback for a viewport range when tree-sitter is unavailable.
 *
 * Uses model.getLineContent() (O(1) per line from Monaco's model cache) and
 * heuristicTokensInRange() — no monaco.editor.tokenize() call, no worker
 * message, no main-thread blockage. Total cost is ~1–2ms for 80 visible lines.
 *
 * lineTokens is a sparse array indexed by absolute row: entries for rows
 * [startLine, endLine] are empty arrays (no Monarch scope data), which makes
 * the heuristic skip map all-false — every identifier is a candidate. The
 * classify() function's conservative rules limit false positives.
 */
function heuristicForRange(
  model: editor.ITextModel,
  startLine: number,
  endLine: number,
): languages.SemanticTokens {
  const lineCount = model.getLineCount()
  const start = Math.max(0, startLine)
  const end = Math.min(endLine, lineCount - 1)

  // Allocate a sparse lineTokens array indexed by absolute row number.
  // Rows [0, end] are all empty arrays; heuristicTokensInRange only accesses
  // rows in [start, end], so entries below start are never read.
  const lineTokens: LineToken[][] = Array.from({ length: end + 1 }, () => [])

  const tokens = heuristicTokensInRange(
    lineTokens,
    start,
    end,
    (row) => (row < lineCount ? model.getLineContent(row + 1) : ''),
  )

  if (tokens.length === 0) return EMPTY
  const data = encodeTokens(tokens, (row) => model.getLineLength(row + 1))
  return data.length > 0 ? { data } : EMPTY
}

export const treeSitterSemanticTokensProvider: languages.DocumentRangeSemanticTokensProvider = {
  getLegend: () => SEMANTIC_TOKEN_LEGEND,
  onDidChange: providerOnDidChange,

  async provideDocumentRangeSemanticTokens(model, range, token) {
    const languageId = getLanguageIdFromPath(model.uri.path)
    if (!languageId) return EMPTY

    const key = model.uri.toString()
    const versionId = model.getVersionId()
    const startLine = range.startLineNumber - 1
    const endLine = range.endLineNumber - 1

    // Tree-sitter cache hit: full token set already computed for this version.
    // Filter to viewport locally — O(viewport tokens), no getValue(), no worker.
    const cached = fullTokenCache.get(key)
    if (cached && cached.versionId === versionId) {
      if (cached.tokens.length === 0) return EMPTY
      const filtered = cached.tokens.filter(
        (t) => t.endPosition.row >= startLine && t.startPosition.row <= endLine,
      )
      const data = encodeTokens(filtered, (row) => model.getLineLength(row + 1))
      return data.length > 0 ? { data } : EMPTY
    }

    // Tree-sitter grammar not available for this language — use heuristic.
    if (unsupportedLanguages.has(languageId)) {
      return heuristicForRange(model, startLine, endLine)
    }

    // Cache miss: attempt a full tree-sitter parse, then cache for every
    // subsequent scroll / viewport request for this document version.
    try {
      const assets = getLanguageAssetConfig(languageId)
      const result = await tokenizerWorkerClient.tokenize({
        bufferId: key,
        content: model.getValue(),
        languageId,
        wasmPath: assets.wasmPath,
        highlightQueryUrl: assets.highlightQueryUrl,
        mode: 'full',
      })

      if (token.isCancellationRequested) return EMPTY

      fullTokenCache.set(key, { versionId, tokens: result.tokens })
      if (fullTokenCache.size > MAX_FULL_TOKEN_CACHE) {
        const oldest = fullTokenCache.keys().next().value
        if (oldest !== undefined) fullTokenCache.delete(oldest)
      }

      // Signal Monaco to re-apply tokens now that the cache is warm. Only
      // fired on tree-sitter success — firing for heuristic results would
      // create an infinite re-request loop.
      fireProviderChange()

      const filtered = result.tokens.filter(
        (t) => t.endPosition.row >= startLine && t.startPosition.row <= endLine,
      )
      const data = encodeTokens(filtered, (row) => model.getLineLength(row + 1))
      return data.length > 0 ? { data } : EMPTY
    } catch {
      // Mark the language permanently unsupported regardless of whether the
      // request was cancelled. A wasm-not-found error is a deployment-time
      // fact; cancellation doesn't change it.
      unsupportedLanguages.add(languageId)
      return heuristicForRange(model, startLine, endLine)
    }
  },
}

let registered = false
export function registerTreeSitterSemanticTokens(): void {
  if (registered) return
  registered = true
  languages.registerDocumentRangeSemanticTokensProvider('*', treeSitterSemanticTokensProvider)
}
