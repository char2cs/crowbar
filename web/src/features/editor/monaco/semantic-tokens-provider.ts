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
 * Cancellation-safe design:
 *   Monaco wraps every provider call in createCancelablePromise. When _cancelAll()
 *   fires (triggered by scroll, resize, config change, etc.) the outer promise
 *   rejects immediately — any value the async provider later returns is silently
 *   dropped and setPartialSemanticTokens never runs.
 *
 *   The async tree-sitter path (100ms+ for the wasm fetch to fail in dev) was
 *   therefore never surviving cancellation. The fix: provideDocumentRangeSemanticTokens
 *   is now synchronous — it always returns heuristic tokens immediately so Monaco
 *   settles the promise before any macro-task can cancel it. Tree-sitter runs in
 *   the background; when it succeeds it populates the cache and fires providerChange,
 *   and the next Monaco request hits the cache synchronously.
 *
 *   Monaco's adaptive debounce (min 100ms, max 500ms) rewards fast providers by
 *   keeping the delay at the minimum — so resize/scroll performance is better than
 *   with the async path.
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

// Model URIs currently being parsed in the background. Prevents duplicate
// parses when Monaco fires multiple rapid requests for the same document.
const pendingParse = new Set<string>()

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
  return {
    dispose: () => providerChangeListeners.splice(providerChangeListeners.indexOf(listener), 1),
  }
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

  const tokens = heuristicTokensInRange(lineTokens, start, end, (row) =>
    row < lineCount ? model.getLineContent(row + 1) : '',
  )

  if (tokens.length === 0) return EMPTY
  const data = encodeTokens(tokens, (row) => model.getLineLength(row + 1))
  return data.length > 0 ? { data } : EMPTY
}

export const treeSitterSemanticTokensProvider: languages.DocumentRangeSemanticTokensProvider = {
  getLegend: () => SEMANTIC_TOKEN_LEGEND,
  onDidChange: providerOnDidChange,

  // Synchronous — always returns immediately so Monaco's createCancelablePromise
  // settles before any _cancelAll() macro-task can drop our tokens.
  provideDocumentRangeSemanticTokens(model, range) {
    const languageId = getLanguageIdFromPath(model.uri.path)
    if (!languageId) return EMPTY

    const key = model.uri.toString()
    const versionId = model.getVersionId()
    const startLine = range.startLineNumber - 1
    const endLine = range.endLineNumber - 1

    // Tree-sitter cache hit: filter to viewport — O(viewport tokens), no worker.
    const cached = fullTokenCache.get(key)
    if (cached && cached.versionId === versionId) {
      if (cached.tokens.length === 0) return EMPTY
      const filtered = cached.tokens.filter(
        (t) => t.endPosition.row >= startLine && t.startPosition.row <= endLine,
      )
      const data = encodeTokens(filtered, (row) => model.getLineLength(row + 1))
      return data.length > 0 ? { data } : EMPTY
    }

    // Tree-sitter grammar not available — heuristic only, no background work.
    if (unsupportedLanguages.has(languageId)) {
      return heuristicForRange(model, startLine, endLine)
    }

    // Cache miss: return heuristic immediately and kick off a background parse.
    // When tree-sitter finishes it populates the cache and fires providerChange,
    // which triggers a second Monaco request that hits the cache synchronously.
    void parseInBackground(model, languageId, key, versionId)
    return heuristicForRange(model, startLine, endLine)
  },
}

async function parseInBackground(
  model: editor.ITextModel,
  languageId: string,
  key: string,
  versionId: number,
): Promise<void> {
  if (pendingParse.has(key)) return
  pendingParse.add(key)
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

    // Discard if the document was edited while we were parsing.
    if (model.isDisposed() || model.getVersionId() !== versionId) return

    fullTokenCache.set(key, { versionId, tokens: result.tokens })
    if (fullTokenCache.size > MAX_FULL_TOKEN_CACHE) {
      const oldest = fullTokenCache.keys().next().value
      if (oldest !== undefined) fullTokenCache.delete(oldest)
    }

    // Trigger Monaco to re-request; next call is a synchronous cache hit.
    fireProviderChange()
  } catch {
    unsupportedLanguages.add(languageId)
    // No fireProviderChange — heuristic is already applied.
  } finally {
    pendingParse.delete(key)
  }
}

let registered = false
export function registerTreeSitterSemanticTokens(): void {
  if (registered) return
  registered = true
  languages.registerDocumentRangeSemanticTokensProvider('*', treeSitterSemanticTokensProvider)
}
