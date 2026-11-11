// A lowlight stand-in for `@platejs/code-block` over the shared shiki grammars, mapping scopes to
// the block stylesheet's `hljs-*` classes. `highlight` stays synchronous: an unready grammar yields
// an empty tree, and the block re-decorates on `onGrammarReady`.
import type { StateStack } from 'shiki/textmate'
import {
  grammarFor,
  readyLanguageIds,
  type ShikiGrammar,
} from '@/features/editor/lib/shiki-grammars'

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
/** @internal Exported for unit tests. */
export function classesForScopes(scopes: readonly string[]): string[] {
  for (let i = scopes.length - 1; i >= 0; i -= 1) {
    const scope = scopes[i]!
    for (const [prefix, classes] of SCOPE_CLASSES) {
      if (scope === prefix || scope.startsWith(`${prefix}.`)) return classes
    }
  }
  return []
}

type HastChild = HastRoot['children'][number]

function lineToHast(line: string, tokens: ReturnType<ShikiGrammar['tokenizeLine']>['tokens']) {
  const children: HastChild[] = []
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
  return children
}

interface TokenizedLine {
  line: string
  hast: HastChild[]
  /** The grammar's state after this line, where the next one starts. */
  stack: StateStack
}

// A streaming block grows at its end and is re-highlighted on every change: the lines
// before the first changed one keep their tokens, so each change costs its own lines.
let lastBlock: { grammar: ShikiGrammar; lines: TokenizedLine[] } | null = null

/** Tokenizes `code` line by line into the hast shape Plate's decorator reads. */
/** @internal Exported for unit tests. */
export function tokenizeToHast(grammar: ShikiGrammar, code: string): HastRoot {
  const previous = lastBlock?.grammar === grammar ? lastBlock.lines : []
  const lines: TokenizedLine[] = []
  let stack: StateStack | null = null
  // Only an unbroken prefix: a line after a changed one may start in another state.
  let reusing = true
  for (const [index, line] of code.split('\n').entries()) {
    const kept = previous[index]
    reusing &&= kept?.line === line
    if (reusing && kept) {
      lines.push(kept)
      stack = kept.stack
      continue
    }
    const { tokens, ruleStack } = grammar.tokenizeLine(line, stack)
    lines.push({ line, hast: lineToHast(line, tokens), stack: ruleStack })
    stack = ruleStack
  }
  lastBlock = { grammar, lines }
  const children: HastChild[] = []
  lines.forEach(({ hast }, index) => {
    if (index > 0) children.push({ type: 'text', value: '\n' })
    children.push(...hast)
  })
  return { type: 'root', children }
}

const EMPTY: HastRoot = { type: 'root', children: [] }

/** The lowlight surface `@platejs/code-block` calls. */
export const shikiLowlight = {
  highlight(lang: string, code: string): HastRoot {
    const grammar = grammarFor(lang)
    return grammar ? tokenizeToHast(grammar, code) : EMPTY
  },
  // Auto-detection was a highlight.js feature; an untagged block stays plain.
  highlightAuto(_code: string): HastRoot {
    return EMPTY
  },
  listLanguages: readyLanguageIds,
}
