import type { HighlighterCore } from 'shiki/core'
import type {
  defaultJavaScriptRegexConstructor,
  JavaScriptRegexEngineOptions,
} from 'shiki/engine/javascript'
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

/** @internal */
export function regexEngineOptions(
  construct: typeof defaultJavaScriptRegexConstructor,
  target: 'ES2024' | 'ES2018' = supportsUnicodeSets() ? 'ES2024' : 'ES2018',
): JavaScriptRegexEngineOptions {
  return {
    forgiving: true,
    target,
    regexConstructor: (pattern) =>
      construct(pattern, {
        target,
        rules: {
          allowOrphanBackrefs: true,
          asciiWordBoundaries: true,
          captureGroup: true,
          recursionLimit: 5,
          singleline: false,
        },
      }),
  }
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
    const [{ createHighlighterCore }, javascript] = await Promise.all([
      import('shiki/core'),
      import('shiki/engine/javascript'),
    ])
    return await createHighlighterCore({
      langs: [],
      engine: javascript.createJavaScriptRegexEngine(
        regexEngineOptions(javascript.defaultJavaScriptRegexConstructor),
      ),
    })
  } catch (error) {
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
    () => languages.set(languageId, { status: 'failed' }),
  )
}

export function grammarFor(languageId: string): ShikiGrammar | null {
  const language = languages.get(languageId)
  if (!language) startLoad(languageId)
  return language?.status === 'ready' ? language.grammar : null
}

/** @internal */
export function grammarStateOf(languageId: string): GrammarState | undefined {
  return languages.get(languageId)?.status
}

export function readyLanguageIds(): string[] {
  return [...languages].flatMap(([id, language]) => (language.status === 'ready' ? [id] : []))
}

export function onGrammarReady(listener: (languageId: string) => void): () => void {
  readyListeners.add(listener)
  return () => {
    readyListeners.delete(listener)
  }
}
