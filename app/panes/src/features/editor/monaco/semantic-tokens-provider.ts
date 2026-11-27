import { editor, languages } from 'monaco-editor/esm/vs/editor/editor.api.js'
import { onGrammarReady } from '@/features/editor/lib/shiki-grammars'
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

const viewportSemanticTokensProvider: languages.DocumentRangeSemanticTokensProvider = {
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
  onGrammarReady(fireProviderChange)
  languages.registerDocumentRangeSemanticTokensProvider('*', viewportSemanticTokensProvider)
}
