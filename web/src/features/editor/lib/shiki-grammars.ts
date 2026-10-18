/**
 * The one shiki highlighter Monaco's semantic tokens and Plate's code blocks
 * share: `shiki/core`, the JavaScript regex engine and one dynamic import per
 * grammar, so a first open pays neither the oniguruma wasm nor the full bundle.
 *
 * A grammar is async only while it loads; once ready it tokenizes
 * synchronously. Each language moves `loading → ready | failed` exactly once.
 */
import type { HighlighterCore } from 'shiki/core'
import type { BundledLanguage } from 'shiki/langs'
import type { StateStack } from 'shiki/textmate'

export interface ShikiGrammar {
  tokenizeLine(
    line: string,
    prev: StateStack | null,
    timeLimit?: number,
  ): {
    readonly tokens: readonly {
      readonly startIndex: number
      readonly endIndex: number
      readonly scopes: string[]
    }[]
    readonly ruleStack: StateStack
  }
}

type GrammarState = 'loading' | 'ready' | 'failed'

/** Crowbar language id → shiki id, where the two disagree; shiki resolves its own aliases. */
const SHIKI_LANG_BY_LANGUAGE_ID: Record<string, string> = {
  javascriptreact: 'jsx',
  typescriptreact: 'tsx',
  embedded_template: 'erb',
  angular: 'angular-html',
  restructuredtext: 'rst',
}

const languages = new Map<
  string,
  { status: 'loading' | 'failed' } | { status: 'ready'; grammar: ShikiGrammar }
>()
const readyListeners = new Set<(languageId: string) => void>()
let highlighter: Promise<HighlighterCore> | null = null

async function createHighlighter(): Promise<HighlighterCore> {
  const [{ createHighlighterCore }, { createJavaScriptRegexEngine }] = await Promise.all([
    import('shiki/core'),
    import('shiki/engine/javascript'),
  ])
  // No theme: nothing here reads colours, and scopes need none.
  return createHighlighterCore({
    langs: [],
    engine: createJavaScriptRegexEngine({ forgiving: true }),
  })
}

async function loadGrammar(shikiLang: string): Promise<ShikiGrammar> {
  const [{ bundledLanguages }, shiki] = await Promise.all([
    import('shiki/langs'),
    (highlighter ??= createHighlighter()),
  ])
  if (!Object.hasOwn(bundledLanguages, shikiLang)) throw new Error(`no shiki grammar: ${shikiLang}`)
  await shiki.loadLanguage(bundledLanguages[shikiLang as BundledLanguage])
  return shiki.getLanguage(shikiLang)
}

function startLoad(languageId: string): void {
  languages.set(languageId, { status: 'loading' })
  void loadGrammar(SHIKI_LANG_BY_LANGUAGE_ID[languageId] ?? languageId).then(
    (grammar) => {
      languages.set(languageId, { status: 'ready', grammar })
      for (const listener of readyListeners) listener(languageId)
    },
    // No grammar for this language (gitignore, lockfiles, …) or shiki failed to
    // load: it stays plain text and is never retried.
    () => languages.set(languageId, { status: 'failed' }),
  )
}

/** The ready grammar for a Crowbar language id, or null; the first ask starts its load. */
export function grammarFor(languageId: string): ShikiGrammar | null {
  const language = languages.get(languageId)
  if (!language) startLoad(languageId)
  return language?.status === 'ready' ? language.grammar : null
}

/** @internal Exported for unit tests. */
export function grammarStateOf(languageId: string): GrammarState | undefined {
  return languages.get(languageId)?.status
}

/** Every language id asked for whose grammar is ready. */
export function readyLanguageIds(): string[] {
  return [...languages].flatMap(([id, language]) => (language.status === 'ready' ? [id] : []))
}

/** Called once per language id, with that id, when its grammar becomes ready. */
export function onGrammarReady(listener: (languageId: string) => void): () => void {
  readyListeners.add(listener)
  return () => {
    readyListeners.delete(listener)
  }
}
