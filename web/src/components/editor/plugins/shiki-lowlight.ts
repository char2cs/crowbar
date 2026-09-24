/**
 * A lowlight-shaped highlighter backed by shiki, for `@platejs/code-block`.
 *
 * Plate's code block decorates through a lowlight instance: a SYNCHRONOUS
 * `highlight(lang, code)` returning a hast tree whose span classes become
 * decoration class names. The app already ships shiki (Monaco's grammar
 * fallback, fenced code in markdown previews) with the same TextMate grammars
 * VSCode uses, so a second highlighter — lowlight with every highlight.js
 * grammar — only duplicated it.
 *
 * Shiki is async only while loading a grammar; once loaded, a grammar
 * tokenizes synchronously. So `highlight` returns an empty tree for a language
 * that is not loaded yet and starts loading it; `onShikiLanguageReady` tells
 * the code block to re-decorate when it lands. Scopes map onto the
 * `hljs-*` class names the code block's stylesheet already colors.
 */
import type { StateStack } from 'shiki/textmate'

type ShikiBundle = typeof import('shiki/bundle/full')
type Highlighter = Awaited<ReturnType<ShikiBundle['getSingletonHighlighter']>>

interface Grammar {
  tokenizeLine(
    line: string,
    prev: StateStack | null,
  ): {
    tokens: readonly { startIndex: number; endIndex: number; scopes: string[] }[]
    ruleStack: StateStack
  }
}

interface HastText {
  type: 'text'
  value: string
}
interface HastElement {
  type: 'element'
  tagName: 'span'
  properties: { className: string[] }
  children: HastText[]
}
export interface HastRoot {
  type: 'root'
  children: (HastElement | HastText)[]
}

/** Most specific first: the first prefix a token's innermost scope matches wins. */
const SCOPE_CLASSES: [prefix: string, classes: string[]][] = [
  ['comment', ['hljs-comment']],
  ['string.regexp', ['hljs-regexp']],
  ['string', ['hljs-string']],
  ['constant.numeric', ['hljs-number']],
  ['constant.language', ['hljs-literal']],
  ['constant.character', ['hljs-string']],
  ['constant', ['hljs-variable']],
  ['variable.language', ['hljs-variable', 'language_']],
  ['variable.other.constant', ['hljs-variable']],
  ['keyword', ['hljs-keyword']],
  ['storage', ['hljs-keyword']],
  ['support.function', ['hljs-built_in']],
  ['support.type', ['hljs-type']],
  ['support.class', ['hljs-title', 'class_']],
  ['support.constant', ['hljs-built_in']],
  ['entity.name.function', ['hljs-title', 'function_']],
  ['entity.name.type', ['hljs-title', 'class_']],
  ['entity.name.class', ['hljs-title', 'class_']],
  ['entity.other.inherited-class', ['hljs-title', 'class_', 'inherited__']],
  ['entity.name.tag', ['hljs-name']],
  ['entity.other.attribute-name', ['hljs-attr']],
  ['entity.name.section', ['hljs-section']],
  ['markup.heading', ['hljs-section']],
  ['markup.bold', ['hljs-strong']],
  ['markup.italic', ['hljs-emphasis']],
  ['markup.inserted', ['hljs-addition']],
  ['markup.deleted', ['hljs-deletion']],
  ['markup.quote', ['hljs-quote']],
  ['markup.list', ['hljs-bullet']],
  ['meta.preprocessor', ['hljs-meta']],
  ['punctuation.definition.comment', ['hljs-comment']],
  ['punctuation.definition.string', ['hljs-string']],
]

/** The hljs classes for a token's scope stack (innermost scope decides). */
export function classesForScopes(scopes: readonly string[]): string[] {
  for (let i = scopes.length - 1; i >= 0; i -= 1) {
    const scope = scopes[i]!
    for (const [prefix, classes] of SCOPE_CLASSES) {
      if (scope === prefix || scope.startsWith(`${prefix}.`)) return classes
    }
  }
  return []
}

/** Tokenizes `code` line by line into the hast shape Plate's decorator reads. */
export function tokenizeToHast(grammar: Grammar, code: string): HastRoot {
  const children: HastRoot['children'] = []
  let stack: StateStack | null = null
  code.split('\n').forEach((line, index) => {
    if (index > 0) children.push({ type: 'text', value: '\n' })
    const { tokens, ruleStack } = grammar.tokenizeLine(line, stack)
    stack = ruleStack
    for (const token of tokens) {
      const value = line.slice(token.startIndex, token.endIndex)
      if (!value) continue
      const className = classesForScopes(token.scopes)
      children.push(
        className.length > 0
          ? {
              type: 'element',
              tagName: 'span',
              properties: { className },
              children: [{ type: 'text', value }],
            }
          : { type: 'text', value },
      )
    }
  })
  return { type: 'root', children }
}

type LanguageState = { status: 'loading' | 'failed' } | { status: 'ready'; grammar: Grammar }

const languages = new Map<string, LanguageState>()
const readyListeners = new Set<(lang: string) => void>()
let highlighter: Promise<Highlighter> | null = null

function loadLanguage(lang: string) {
  languages.set(lang, { status: 'loading' })
  // The same `shiki/bundle/full` singleton Monaco and markdown previews use:
  // one engine, one grammar cache.
  void import('shiki/bundle/full')
    .then(async ({ bundledLanguages, getSingletonHighlighter }) => {
      if (!(lang in bundledLanguages)) throw new Error(`no shiki grammar: ${lang}`)
      highlighter ??= getSingletonHighlighter({ themes: ['github-dark'], langs: [] })
      const hl = await highlighter
      await hl.loadLanguage(lang as Parameters<Highlighter['loadLanguage']>[0])
      const grammar = hl.getInternalContext().getLanguage(lang) as unknown as Grammar
      languages.set(lang, { status: 'ready', grammar })
      for (const listener of readyListeners) listener(lang)
    })
    .catch(() => {
      languages.set(lang, { status: 'failed' })
    })
}

/** Called once per language when its grammar becomes ready. */
export function onShikiLanguageReady(listener: (lang: string) => void): () => void {
  readyListeners.add(listener)
  return () => {
    readyListeners.delete(listener)
  }
}

const EMPTY: HastRoot = { type: 'root', children: [] }

/** The lowlight surface `@platejs/code-block` calls. */
export const shikiLowlight = {
  highlight(lang: string, code: string): HastRoot {
    const state = languages.get(lang)
    if (!state) loadLanguage(lang)
    return state?.status === 'ready' ? tokenizeToHast(state.grammar, code) : EMPTY
  },
  // Auto-detection was a highlight.js feature; an untagged block stays plain.
  highlightAuto(_code: string): HastRoot {
    return EMPTY
  },
  listLanguages(): string[] {
    return [...languages].flatMap(([lang, state]) => (state.status === 'ready' ? [lang] : []))
  },
}
