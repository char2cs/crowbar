/**
 * The one shiki highlighter Monaco and Plate share, on the JavaScript regex engine
 * so a first open skips the oniguruma wasm. Grammars load once, then tokenize synchronously.
 */
import type { HighlighterCore } from 'shiki/core'
import type { JavaScriptRegexEngineOptions } from 'shiki/engine/javascript'
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

/**
 * Never `auto`: it picks ES2025 on new V8, which mis-tokenizes uppercase SQL and
 * PowerShell. ES2024 matches oniguruma but needs the `v` flag (Safari 17+).
 * @internal Exported for unit tests.
 */
export function regexEngineOptions(
  target: 'ES2024' | 'ES2018' = supportsUnicodeSets() ? 'ES2024' : 'ES2018',
): JavaScriptRegexEngineOptions {
  return { forgiving: true, target }
}

function supportsUnicodeSets(): boolean {
  try {
    return new RegExp('', 'v') instanceof RegExp
  } catch {
    return false
  }
}

async function createHighlighter(): Promise<HighlighterCore> {
  try {
    const [{ createHighlighterCore }, { createJavaScriptRegexEngine }] = await Promise.all([
      import('shiki/core'),
      import('shiki/engine/javascript'),
    ])
    // No theme: nothing here reads colours, and scopes need none.
    return await createHighlighterCore({
      langs: [],
      engine: createJavaScriptRegexEngine(regexEngineOptions()),
    })
  } catch (error) {
    // A failed chunk load fails the languages waiting on it, not every later one.
    highlighter = null
    throw error
  }
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
