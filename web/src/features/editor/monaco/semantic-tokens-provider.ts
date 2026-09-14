/**
 * One generic Document Range Semantic Tokens provider, registered for all
 * languages ('*'). Two-tier strategy:
 *
 *   1. Tree-sitter — precise tokens from the wasm worker, when a grammar is
 *      provisioned for the language. Missing-grammar languages are permanently
 *      cached in `unsupportedLanguages` after the first failed attempt.
 *
 *   2. Shiki fallback — the TextMate grammars VSCode ships, via
 *      `shikiTokensInRange`. Covers the languages with no tree-sitter parser and
 *      the window before one lands. Reads only `model.getLineContent()` and
 *      resumes from a cached per-row rule stack, so a viewport costs a viewport.
 *      See `shiki-tokens.ts` for why that stays synchronous.
 *
 * Cancellation-safe design:
 *   Monaco wraps every provider call in createCancelablePromise. When _cancelAll()
 *   fires (triggered by scroll, resize, config change, etc.) the outer promise
 *   rejects immediately — any value the async provider later returns is silently
 *   dropped and setPartialSemanticTokens never runs.
 *
 *   The async tree-sitter path (100ms+ for the wasm fetch to fail in dev) was
 *   therefore never surviving cancellation. The fix: provideDocumentRangeSemanticTokens
 *   is now synchronous — it always returns immediately so Monaco settles the
 *   promise before any macro-task can cancel it. Both engines load in the
 *   background; when either becomes ready it fires providerChange, and the next
 *   Monaco request is served synchronously.
 *
 *   Monaco's adaptive debounce (min 100ms, max 500ms) rewards fast providers by
 *   keeping the delay at the minimum — so resize/scroll performance is better than
 *   with the async path.
 */
// See the comment in `monaco-diff-editor.tsx`: `editor.api` is the same real
// editor/languages singleton as the bare 'monaco-editor' specifier, without
// eagerly bundling all built-in language contributions.
import { editor, languages } from 'monaco-editor/esm/vs/editor/editor.api.js'
import { getLanguageAssetConfig } from '@/features/editor/lib/wasm-parser/extension-assets'
import { tokenizerWorkerClient } from '@/features/editor/lib/wasm-parser/tokenizer-worker-client'
import type { HighlightToken } from '@/features/editor/lib/wasm-parser/types'
import { getLanguageIdFromPath } from '@/features/editor/utils/language-id'
import { SEMANTIC_TOKEN_LEGEND, encodeTokens } from './semantic-tokens-encode'
import { shikiTokensInRange } from './shiki-tokens'

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
// Fired when an engine becomes able to answer something it could not answer
// synchronously before — tree-sitter populating a cache entry, or shiki
// finishing a grammar load / rule-stack catch-up — so Monaco re-requests.
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
 * Shiki fallback for a viewport range when the tree-sitter cache cannot answer.
 *
 * Reads only model.getLineContent() (O(1) per line from Monaco's model cache)
 * and resumes TextMate tokenization from a cached rule stack, so an 80-line
 * viewport tokenizes 80 lines. Returns EMPTY — Monarch's own coloring stands —
 * whenever shiki has nothing to add yet; it calls back through fireProviderChange
 * once it does.
 */
function shikiForRange(
  model: editor.ITextModel,
  languageId: string,
  startLine: number,
  endLine: number,
): languages.SemanticTokens {
  const tokens = shikiTokensInRange(model, languageId, startLine, endLine, fireProviderChange)
  if (!tokens || tokens.length === 0) return EMPTY
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

    // Tree-sitter grammar not available — shiki only, no background parse.
    if (unsupportedLanguages.has(languageId)) {
      return shikiForRange(model, languageId, startLine, endLine)
    }

    // Cache miss: answer from shiki immediately and kick off a background parse.
    // When tree-sitter finishes it populates the cache and fires providerChange,
    // which triggers a second Monaco request that hits the cache synchronously.
    void parseInBackground(model, languageId, key, versionId)
    return shikiForRange(model, languageId, startLine, endLine)
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
    // No fireProviderChange — shiki already answered this range.
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
