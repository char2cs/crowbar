/**
 * Semantic tokens from the language servers (daemon /lsp/semanticTokens*),
 * already rewritten into SEMANTIC_TOKEN_LEGEND by the daemon.
 *
 * The document provider owns the full/delta request per model. While one is
 * in flight, the viewport provider may ask `lspViewportTokens` for the visible
 * range, so a large file gets semantic colors before its full result lands;
 * once full tokens are applied Monaco stops asking for ranges at all.
 */
import { CancellationTokenSource } from 'monaco-editor/esm/vs/editor/editor.api.js'
import type * as Monaco from 'monaco-editor'
import { SEMANTIC_TOKEN_LEGEND } from '../monaco/semantic-tokens-legend'
import { query } from './lsp-query'
import { toLspRange } from './lsp-to-monaco'

interface WireTokens {
  resultId?: string
  data: number[]
}
interface WireDelta {
  resultId?: string
  edits: { start: number; deleteCount: number; data?: number[] }[]
}

/**
 * Monaco reads a document provider's null as "complete, and empty", which
 * would also silence the viewport (shiki) tier for the file. A cancellation
 * leaves the document's tokens as they are and is not reported as an error.
 */
function noServerTokens(): Error {
  const error = new Error('Canceled')
  error.name = 'Canceled'
  return error
}

function toMonacoTokens(
  result: WireTokens | WireDelta,
): Monaco.languages.SemanticTokens | Monaco.languages.SemanticTokensEdits {
  if ('edits' in result) {
    return {
      resultId: result.resultId,
      edits: result.edits.map((e) => ({
        start: e.start,
        deleteCount: e.deleteCount,
        data: e.data ? Uint32Array.from(e.data) : undefined,
      })),
    }
  }
  return { resultId: result.resultId, data: Uint32Array.from(result.data) }
}

interface ViewportFill {
  versionId: number
  range: string
  tokens: Monaco.languages.SemanticTokens | null
  cancel: CancellationTokenSource
}

/** Models whose full request is in flight, with their viewport fill (if any). */
const pendingFull = new Map<string, ViewportFill | null>()

function dropFill(uri: string): void {
  pendingFull.get(uri)?.cancel.dispose(true)
  pendingFull.delete(uri)
}

export const lspDocumentSemanticTokensProvider: Monaco.languages.DocumentSemanticTokensProvider = {
  getLegend: () => SEMANTIC_TOKEN_LEGEND,
  async provideDocumentSemanticTokens(model, lastResultId, token) {
    const uri = model.uri.toString()
    pendingFull.set(uri, null)
    try {
      const response = await query<WireTokens | WireDelta>(
        model,
        'semanticTokens',
        lastResultId ? { previousResultId: lastResultId } : {},
        token,
      )
      if (!response?.result) throw noServerTokens()
      return toMonacoTokens(response.result)
    } finally {
      dropFill(uri)
    }
  },
  // The daemon keeps no per-result state, so there is nothing to release.
  releaseDocumentSemanticTokens() {},
}

/**
 * Server tokens for a viewport while the model's full request is in flight;
 * null means "not (yet) available" and the caller colors from its own tier.
 * The first call for a range starts the request; `onReady` fires when it
 * lands so Monaco asks again.
 */
export function lspViewportTokens(
  model: Monaco.editor.ITextModel,
  range: Monaco.IRange,
  onReady: () => void,
): Monaco.languages.SemanticTokens | null {
  const uri = model.uri.toString()
  if (!pendingFull.has(uri)) return null
  const versionId = model.getVersionId()
  const key = `${range.startLineNumber}:${range.endLineNumber}`
  const fill = pendingFull.get(uri)
  if (fill && fill.versionId === versionId && fill.range === key) return fill.tokens

  fill?.cancel.dispose(true)
  const next: ViewportFill = {
    versionId,
    range: key,
    tokens: null,
    cancel: new CancellationTokenSource(),
  }
  pendingFull.set(uri, next)
  void query<WireTokens>(
    model,
    'semanticTokensRange',
    { range: toLspRange(range) },
    next.cancel.token,
  ).then((response) => {
    if (pendingFull.get(uri) !== next || !response?.result) return
    next.tokens = { data: Uint32Array.from(response.result.data) }
    onReady()
  })
  return null
}
