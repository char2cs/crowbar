// The shiki grammar tier: VSCode's TextMate grammars color identifiers over Monarch for every file.
// Synchronous (see semantic-tokens-provider.ts); rule stacks are kept per row so scroll and typing
// stay O(viewport), and a far jump yields nothing while stacks advance on a background timer.
import type { StateStack } from 'shiki/textmate'
import { grammarFor, type ShikiGrammar } from '@/features/editor/lib/shiki-grammars'
import { toMonacoLanguageId } from './language'

/** The subset of Monaco's ITextModel this needs. */
export interface TokenizerModel {
  uri: { toString(): string }
  getLineCount(): number
  getLineContent(lineNumber: number): string
  getVersionId?(): number
  onDidChangeContent?(
    listener: (e: { changes: readonly { range: { startLineNumber: number } }[] }) => void,
  ): { dispose(): void }
  onWillDispose?(listener: () => void): { dispose(): void }
}

/** Categories this emits; `semantic-tokens-legend.ts` maps each onto the legend. */
type ShikiCategory =
  | 'function'
  | 'type'
  | 'constant'
  | 'variable'
  | 'property'
  | 'tag'
  | 'attribute'
  | StructuralCategory

/** What Monarch colors — emitted only where Monaco has no grammar at all. */
type StructuralCategory = 'keyword' | 'string' | 'comment' | 'number' | 'operator'

/** One categorized span on one row (0-based row, [start, end) columns). */
export interface ShikiToken {
  category: ShikiCategory
  row: number
  start: number
  end: number
}

// ── Scope → category ──────────────────────────────────────────────────────────

// TextMate scope → category, `null` meaning "leave it to Monaco": only what Monarch can't know
// (function, type, constant, property). Matched exact, then dropping trailing `.segments` (and
// the language suffix), so longer keys win. Entries are scopes the installed grammars really emit.
const SCOPE_CATEGORY = new Map<string, ShikiCategory | null>([
  // function
  ['entity.name.function', 'function'],
  ['entity.name.method', 'function'],
  ['support.function', 'function'],
  ['variable.function', 'function'],
  ['meta.function-call.generic', 'function'],
  // type
  ['entity.name.type', 'type'],
  ['entity.name.class', 'type'],
  ['entity.name.struct', 'type'],
  ['entity.name.enum', 'type'],
  ['entity.name.interface', 'type'],
  ['entity.name.namespace', 'type'],
  ['entity.other.inherited-class', 'type'],
  ['support.class', 'type'],
  ['support.type', 'type'],
  // property (before their `support.type` / `variable.other` prefixes)
  ['support.type.property-name', 'property'],
  ['variable.other.property', 'property'],
  ['variable.other.member', 'property'],
  ['support.variable.property', 'property'],
  ['meta.object-literal.key', 'property'],
  ['meta.attribute', 'property'],
  // constant
  ['variable.other.constant', 'constant'],
  ['variable.other.enummember', 'constant'],
  ['constant.language', 'constant'],
  ['constant.other', 'constant'],
  ['support.constant', 'constant'],
  // markup
  ['entity.name.tag', 'tag'],
  ['entity.other.attribute-name', 'attribute'],
  // every remaining `variable.*` — parameters, locals, members
  ['variable', 'variable'],
  // `this` / `self` read as keywords; Monarch already colors them
  ['variable.language', null],
  // left to Monarch
  ['comment', null],
  ['string', null],
  ['keyword', null],
  ['storage', null],
  ['constant', null],
  ['punctuation', null],
  ['operator', null],
  ['markup', null],
  ['invalid', null],
  ['meta', null],
  ['source', null],
  ['text', null],
  ['entity', null],
  ['support', null],
])

/** For a language Monaco has no grammar for (plaintext, e.g. nix, dotenv), nothing else colors
 *  keywords, strings, comments or numbers, so this tier does. Checked before SCOPE_CATEGORY. */
const STRUCTURAL_CATEGORY = new Map<string, StructuralCategory>([
  ['keyword.operator', 'operator'],
  ['keyword', 'keyword'],
  ['storage', 'keyword'],
  ['string', 'string'],
  ['constant.numeric', 'number'],
])

/** Classify a token by its scope stack (outermost → innermost); innermost wins. Anything inside a
 *  comment is never re-colored, since doc-comment grammars emit code scopes in prose.
 *  `structural` adds STRUCTURAL_CATEGORY. */
export function scopeCategory(scopes: readonly string[], structural = false): ShikiCategory | null {
  for (const scope of scopes) {
    if (scope === 'comment' || scope.startsWith('comment.')) return structural ? 'comment' : null
  }

  for (let i = scopes.length - 1; i >= 0; i--) {
    let probe = scopes[i]
    for (;;) {
      const structuralHit = structural ? STRUCTURAL_CATEGORY.get(probe) : undefined
      if (structuralHit) return structuralHit
      const hit = SCOPE_CATEGORY.get(probe)
      if (hit !== undefined) return hit
      const dot = probe.lastIndexOf('.')
      if (dot <= 0) break
      probe = probe.slice(0, dot)
    }
  }

  return null
}

// ── Per-line tokenization ─────────────────────────────────────────────────────

/** vscode-textmate can stall on pathological lines; Monaco gives up on them too. */
const MAX_LINE_LENGTH = 10_000
/** Per-line budget handed to the grammar, in ms. */
const TOKENIZE_TIME_LIMIT = 100
/** Rows we are willing to walk in one pass to reach the requested viewport. */
const MAX_ADVANCE_LINES = 1_000

const WHITESPACE = /\s/

/** Tokenize rows [startRow, endRow], appending categorized tokens to `out`. `stacks[row]` is the
 *  rule stack entering `row`, extended as rows are consumed. Exported for fake-grammar tests. */
export function tokenizeRows(
  grammar: ShikiGrammar,
  getLine: (row: number) => string,
  stacks: (StateStack | null)[],
  startRow: number,
  endRow: number,
  out: ShikiToken[],
  structural = false,
): void {
  for (let row = startRow; row <= endRow; row++) {
    const line = getLine(row)
    const prev = stacks[row] ?? null

    if (line.length > MAX_LINE_LENGTH) {
      stacks[row + 1] = prev
      continue
    }

    const result = grammar.tokenizeLine(line, prev, TOKENIZE_TIME_LIMIT)
    stacks[row + 1] = result.ruleStack

    for (const token of result.tokens) {
      const category = scopeCategory(token.scopes, structural)
      if (category === null) continue

      // Grammars routinely pad a token with the whitespace around it; emitting
      // that inflates the encoded array with spans that color nothing.
      let start = token.startIndex
      let end = Math.min(token.endIndex, line.length)
      while (start < end && WHITESPACE.test(line[start])) start++
      while (end > start && WHITESPACE.test(line[end - 1])) end--
      if (start >= end) continue

      out.push({ category, row, start, end })
    }
  }
}

/** Walk the rule stacks forward to `targetRow` without emitting tokens, at most `budget` rows;
 *  true once `stacks` covers `targetRow`. */
function advanceStacks(
  grammar: ShikiGrammar,
  getLine: (row: number) => string,
  stacks: (StateStack | null)[],
  targetRow: number,
  budget: number,
): boolean {
  let row = stacks.length - 1
  let spent = 0

  while (row < targetRow && spent < budget) {
    const line = getLine(row)
    if (line.length > MAX_LINE_LENGTH) {
      stacks.push(stacks[row] ?? null)
    } else {
      stacks.push(grammar.tokenizeLine(line, stacks[row] ?? null, TOKENIZE_TIME_LIMIT).ruleStack)
    }
    row++
    spent++
  }

  return stacks.length - 1 >= targetRow
}

/** Languages whose grammar has already been run over real code at least once. */
const compiledLanguages = new Set<string>()

// ── Per-model rule-stack cache ────────────────────────────────────────────────

interface ModelState {
  languageId: string
  grammar: ShikiGrammar
  /** `stacks[row]` = rule stack entering `row`; `stacks[0]` is always INITIAL (null). */
  stacks: (StateStack | null)[]
  disposables: { dispose(): void }[]
  catchUpTimer: ReturnType<typeof setTimeout> | null
  disposed: boolean
  /** Only consulted when the model exposes no content-change event to listen to. */
  versionId: number | null
  watchesContent: boolean
}

const modelStates = new Map<string, ModelState>()
const MAX_MODEL_STATES = 8

function disposeModelState(key: string): void {
  const state = modelStates.get(key)
  if (!state) return
  state.disposed = true
  if (state.catchUpTimer !== null) clearTimeout(state.catchUpTimer)
  for (const d of state.disposables) d.dispose()
  modelStates.delete(key)
}

function getModelState(
  model: TokenizerModel,
  languageId: string,
  grammar: ShikiGrammar,
): ModelState {
  const key = model.uri.toString()
  const versionId = model.getVersionId?.() ?? null
  const existing = modelStates.get(key)
  if (existing && existing.languageId === languageId && existing.grammar === grammar) {
    // A model that gives us no change event can only be invalidated wholesale.
    if (!existing.watchesContent && existing.versionId !== versionId) {
      existing.stacks.length = 1
      existing.versionId = versionId
    }
    return existing
  }
  if (existing) disposeModelState(key)

  // An edit only invalidates the stacks entering rows *after* its first changed
  // row, so typing keeps every checkpoint above the cursor and stays O(viewport).
  const onChange = model.onDidChangeContent?.((e) => {
    let firstRow = Number.MAX_SAFE_INTEGER
    for (const change of e.changes) {
      firstRow = Math.min(firstRow, change.range.startLineNumber - 1)
    }
    if (firstRow === Number.MAX_SAFE_INTEGER) return
    if (state.stacks.length > firstRow + 1) state.stacks.length = firstRow + 1
  })

  const state: ModelState = {
    languageId,
    grammar,
    stacks: [null],
    disposables: onChange ? [onChange] : [],
    catchUpTimer: null,
    disposed: false,
    versionId,
    watchesContent: Boolean(onChange),
  }

  const onDispose = model.onWillDispose?.(() => disposeModelState(key))
  if (onDispose) state.disposables.push(onDispose)

  modelStates.set(key, state)
  if (modelStates.size > MAX_MODEL_STATES) {
    const oldest = modelStates.keys().next().value
    if (oldest !== undefined && oldest !== key) disposeModelState(oldest)
  }
  return state
}

function scheduleCatchUp(
  state: ModelState,
  model: TokenizerModel,
  targetRow: number,
  onReady: () => void,
): void {
  if (state.catchUpTimer !== null) return

  const step = () => {
    state.catchUpTimer = null
    if (state.disposed) return
    try {
      const lineCount = model.getLineCount()
      const target = Math.min(targetRow, lineCount - 1)
      const reached = advanceStacks(
        state.grammar,
        (row) => model.getLineContent(row + 1),
        state.stacks,
        target,
        MAX_ADVANCE_LINES,
      )
      if (reached) onReady()
      else state.catchUpTimer = setTimeout(step, 0)
    } catch {
      // Model disposed mid-walk — drop the state rather than retry.
      disposeModelState(model.uri.toString())
    }
  }

  state.catchUpTimer = setTimeout(step, 0)
}

/** Grammar tokens for rows [startRow, endRow] (0-based, inclusive), or null to keep Monarch's
 *  colors: grammar loading or absent, rules not yet compiled, or viewport too far past the last
 *  stack. For the last two `onReady` fires once resolved; a grammar landing is onGrammarReady. */
export function shikiTokensInRange(
  model: TokenizerModel,
  languageId: string,
  startRow: number,
  endRow: number,
  onReady: () => void,
): ShikiToken[] | null {
  const grammar = grammarFor(languageId)
  if (!grammar) return null

  const lineCount = model.getLineCount()
  const start = Math.max(0, startRow)
  const end = Math.min(endRow, lineCount - 1)
  if (start > end) return null

  const state = getModelState(model, languageId, grammar)
  const getLine = (row: number) => model.getLineContent(row + 1)

  // A grammar compiles its regexes lazily as tokenizing reaches them, so a
  // language's first viewport costs far more than later ones. Walk it on a timer
  // so that one-off cost never lands inside a synchronous provider call.
  if (!compiledLanguages.has(languageId)) {
    compiledLanguages.add(languageId)
    scheduleCatchUp(state, model, end, onReady)
    return null
  }

  if (!advanceStacks(state.grammar, getLine, state.stacks, start, MAX_ADVANCE_LINES)) {
    scheduleCatchUp(state, model, start, onReady)
    return null
  }

  const tokens: ShikiToken[] = []
  const structural = toMonacoLanguageId(languageId) === 'plaintext'
  tokenizeRows(state.grammar, getLine, state.stacks, start, end, tokens, structural)
  return tokens
}

/** Test-only: drop every model rule stack and compiled-language mark. */
export function __resetShikiTokensForTests(): void {
  for (const key of [...modelStates.keys()]) disposeModelState(key)
  compiledLanguages.clear()
}
