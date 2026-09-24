/**
 * The viewport semantic-tokens provider, registered for every language ('*').
 * Monaco consults it until a document provider (the language server's full
 * tokens, `lsp/semantic-tokens.ts`) has applied complete tokens, so it answers
 * from, in order:
 *
 *   1. the language server's range tokens, while its full request is in flight;
 *   2. shiki — the TextMate grammars VSCode ships — for every other file and
 *      the window before a server answers.
 *
 * SYNCHRONOUS by design: Monaco wraps each call in createCancelablePromise and
 * any scroll/resize cancels outstanding ones, so an async answer is routinely
 * dropped. Both tiers answer from what they already have and fire onDidChange
 * when they can say more, and Monaco re-requests.
 */
// See the comment in `monaco-diff-editor.tsx`: `editor.api` is the same real
// editor/languages singleton as the bare 'monaco-editor' specifier, without
// eagerly bundling all built-in language contributions.
import { editor, languages } from 'monaco-editor/esm/vs/editor/editor.api.js'
import { lspViewportTokens } from '@/features/editor/lsp/semantic-tokens'
import { getLanguageIdFromPath } from '@/features/editor/utils/language-id'
import { SEMANTIC_TOKEN_LEGEND, encodeShikiTokens } from './semantic-tokens-legend'
import { shikiTokensInRange } from './shiki-tokens'

const EMPTY: languages.SemanticTokens = { data: new Uint32Array(0) }

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

function shikiForRange(
  model: editor.ITextModel,
  languageId: string,
  startLine: number,
  endLine: number,
): languages.SemanticTokens {
  const tokens = shikiTokensInRange(model, languageId, startLine, endLine, fireProviderChange)
  if (!tokens || tokens.length === 0) return EMPTY
  return { data: encodeShikiTokens(tokens) }
}

export const viewportSemanticTokensProvider: languages.DocumentRangeSemanticTokensProvider = {
  getLegend: () => SEMANTIC_TOKEN_LEGEND,
  onDidChange: providerOnDidChange,
  provideDocumentRangeSemanticTokens(model, range) {
    const fromServer = lspViewportTokens(model, range, fireProviderChange)
    if (fromServer) return fromServer
    const languageId = getLanguageIdFromPath(model.uri.path)
    if (!languageId) return EMPTY
    return shikiForRange(model, languageId, range.startLineNumber - 1, range.endLineNumber - 1)
  },
}

let registered = false
export function registerViewportSemanticTokens(): void {
  if (registered) return
  registered = true
  languages.registerDocumentRangeSemanticTokensProvider('*', viewportSemanticTokensProvider)
}
