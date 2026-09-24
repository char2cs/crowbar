/**
 * Tests for the Shiki-backed semantic token fallback.
 *
 * Two contracts matter here and are asserted directly:
 *   - the scope→category map (fed the scope stacks the real grammars emit), and
 *   - the per-row rule-stack cache that lets a stateful TextMate grammar answer
 *     a viewport synchronously without re-tokenizing the file above it.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { StateStack } from 'shiki/textmate'

const mocks = vi.hoisted(() => {
  const grammars: Record<string, unknown> = {}
  return {
    grammars,
    bundledLanguages: {} as Record<string, unknown>,
    loadLanguage: vi.fn(async () => {}),
  }
})

vi.mock('shiki/bundle/full', () => ({
  bundledLanguages: mocks.bundledLanguages,
  getSingletonHighlighter: vi.fn(async () => ({
    loadLanguage: mocks.loadLanguage,
    getInternalContext: () => ({ getLanguage: (name: string) => mocks.grammars[name] }),
  })),
}))

import {
  __resetShikiTokensForTests,
  scopeCategory,
  shikiTokensInRange,
  tokenizeRows,
  type ShikiToken,
  type TokenizerGrammar,
  type TokenizerModel,
} from '@/features/editor/monaco/shiki-tokens'

// ── A stateful stand-in for a TextMate grammar ────────────────────────────────

interface FakeStack {
  inComment: boolean
}

/**
 * Splits a line into whitespace-padded tokens (real grammars do the same) and
 * threads a block-comment state across lines, so a viewport tokenized from the
 * wrong rule stack produces visibly wrong categories.
 */
function fakeGrammar(): TokenizerGrammar {
  return {
    tokenizeLine(line: string, prevState: StateStack | null) {
      let inComment = (prevState as unknown as FakeStack | null)?.inComment ?? false
      const tokens: { startIndex: number; endIndex: number; scopes: string[] }[] = []
      let cursor = 0

      for (const match of line.matchAll(/\S+/g)) {
        const word = match[0]
        const end = match.index + word.length
        if (word === '/*') inComment = true

        const scopes = inComment
          ? ['source.fake', 'comment.block.fake', 'entity.name.type.fake']
          : ['source.fake', scopeForWord(word)]
        tokens.push({ startIndex: cursor, endIndex: end, scopes })
        cursor = end

        if (word === '*/') inComment = false
      }

      if (cursor < line.length) {
        tokens.push({ startIndex: cursor, endIndex: line.length, scopes: ['source.fake'] })
      }
      return { tokens, ruleStack: { inComment } as unknown as StateStack }
    },
  }
}

function scopeForWord(word: string): string {
  if (word.startsWith('fn')) return 'entity.name.function.fake'
  if (word.startsWith('T')) return 'entity.name.type.fake'
  if (word.startsWith('K')) return 'keyword.control.fake'
  return 'variable.other.fake'
}

// ── A stand-in for Monaco's ITextModel ────────────────────────────────────────

function fakeModel(uri: string, lines: string[]) {
  let changeListener: ((e: { changes: { range: { startLineNumber: number } }[] }) => void) | null =
    null
  const model: TokenizerModel = {
    uri: { toString: () => uri },
    getLineCount: () => lines.length,
    getLineContent: (lineNumber: number) => lines[lineNumber - 1] ?? '',
    onDidChangeContent(listener) {
      changeListener = listener
      return { dispose: () => {} }
    },
    onWillDispose() {
      return { dispose: () => {} }
    },
  }
  return {
    model,
    edit(lineNumber: number) {
      changeListener?.({ changes: [{ range: { startLineNumber: lineNumber } }] })
    },
  }
}

beforeEach(() => {
  __resetShikiTokensForTests()
  for (const key of Object.keys(mocks.bundledLanguages)) delete mocks.bundledLanguages[key]
  for (const key of Object.keys(mocks.grammars)) delete mocks.grammars[key]
  mocks.loadLanguage.mockClear()
})

afterEach(() => {
  vi.useRealTimers()
})

/** Drain the dynamic import + promise chain warmLanguage builds. */
async function flush(): Promise<void> {
  for (let i = 0; i < 5; i++) await new Promise((resolve) => setTimeout(resolve, 0))
}

function register(languageId: string): void {
  mocks.bundledLanguages[languageId] = () => {}
  mocks.grammars[languageId] = fakeGrammar()
}

describe('scopeCategory', () => {
  it.each([
    [['source.ts', 'meta.class.ts', 'entity.name.type.class.ts'], 'type'],
    [['source.python', 'meta.class.python', 'entity.other.inherited-class.python'], 'type'],
    [['source.ts', 'meta.type.annotation.ts', 'support.type.primitive.ts'], 'type'],
    [['source.go', 'entity.name.function.support.go'], 'function'],
    [['source.python', 'meta.function-call.python', 'support.function.builtin.python'], 'function'],
    [
      ['source.python', 'meta.member.access.python', 'meta.function-call.generic.python'],
      'function',
    ],
    [['source.go', 'variable.other.constant.go'], 'constant'],
    [['source.hcl.terraform', 'meta.block.hcl', 'variable.other.enummember.hcl'], 'constant'],
    [['source.go', 'constant.language.null.go'], 'constant'],
    [['source.go', 'variable.other.property.go'], 'property'],
    [['source.hcl.terraform', 'meta.block.hcl', 'variable.other.member.hcl'], 'property'],
    [['source.python', 'meta.member.access.python', 'meta.attribute.python'], 'property'],
    [['source.json', 'support.type.property-name.json'], 'property'],
    [['text.html.erb', 'meta.tag.structure.li.start.html', 'entity.name.tag.html'], 'tag'],
    [
      ['text.html.erb', 'meta.attribute.class.html', 'entity.other.attribute-name.html'],
      'attribute',
    ],
    [['source.go', 'variable.parameter.go'], 'variable'],
    [['source.go', 'variable.other.go'], 'variable'],
    [['source.ts', 'meta.class.ts', 'variable.other.readwrite.ts'], 'variable'],
  ])('classifies %j as %s', (scopes, expected) => {
    expect(scopeCategory(scopes)).toBe(expected)
  })

  it.each([
    [['source.ts', 'meta.var.expr.ts', 'storage.type.ts']], // `const`
    [['source.go', 'storage.type.string.go']], // `string`
    [['source.sql', 'keyword.other.DML.sql']], // `SELECT`
    [['source.sql', 'string.quoted.single.sql']],
    [['source.go', 'constant.numeric.decimal.go']],
    [['source.ts', 'punctuation.definition.block.ts']],
    [['source.hcl', 'keyword.operator.accessor.hcl']],
    [['source.python', 'variable.language.special.self.python']], // `self`
    [['source.python', 'meta.function-call.python', 'meta.function-call.arguments.python']],
    [['text.html.erb', 'meta.embedded.line.erb', 'source.ruby']],
    [['source.dockerfile']],
  ])('leaves %j to Monaco', (scopes) => {
    expect(scopeCategory(scopes)).toBeNull()
  })

  it('never re-colors inside a comment, even when a doc grammar names a type', () => {
    expect(
      scopeCategory([
        'source.js',
        'comment.block.documentation.js',
        'entity.name.type.instance.jsdoc',
      ]),
    ).toBeNull()
  })

  it.each([
    [['source.nix', 'keyword.other.nix'], 'keyword'],
    [['source.nix', 'storage.type.function.nix'], 'keyword'],
    [['source.nix', 'keyword.operator.nix'], 'operator'],
    [['source.nix', 'string.quoted.double.nix'], 'string'],
    [['source.nix', 'constant.numeric.nix'], 'number'],
    [['source.nix', 'comment.line.number-sign.nix'], 'comment'],
    [['source.nix', 'entity.name.function.nix'], 'function'],
  ])('also classifies %j as %s where Monaco has no grammar', (scopes, expected) => {
    expect(scopeCategory(scopes, true)).toBe(expected)
  })

  it('takes the innermost scope over its container', () => {
    // The `(` inside a python call: the call's meta scope would say "function".
    expect(
      scopeCategory([
        'source.python',
        'meta.function-call.generic.python',
        'punctuation.definition.arguments.begin.python',
      ]),
    ).toBeNull()
  })
})

describe('tokenizeRows', () => {
  it('emits categories and trims the whitespace grammars pad tokens with', () => {
    const lines = ['  fnRun  Tfoo  Kif  x']
    const tokens: ShikiToken[] = []
    tokenizeRows(fakeGrammar(), (row) => lines[row], [null], 0, 0, tokens)

    expect(tokens).toEqual([
      { category: 'function', row: 0, start: 2, end: 7 },
      { category: 'type', row: 0, start: 9, end: 13 },
      { category: 'variable', row: 0, start: 20, end: 21 },
    ])
  })

  it('records the rule stack entering the row after the last one tokenized', () => {
    const stacks: (StateStack | null)[] = [null]
    tokenizeRows(fakeGrammar(), () => '/* Tfoo', stacks, 0, 0, [])
    expect(stacks).toHaveLength(2)
    expect(stacks[1]).toEqual({ inComment: true })
  })
})

/**
 * Drive the two asynchronous steps a language goes through — loading the
 * grammar, then compiling its rules against real code on a timer — so the next
 * call is the synchronous one under test.
 */
async function warm(model: TokenizerModel, languageId: string, endRow = 0): Promise<void> {
  shikiTokensInRange(model, languageId, 0, endRow, () => {})
  await flush()
  shikiTokensInRange(model, languageId, 0, endRow, () => {})
  await flush()
}

describe('shikiTokensInRange', () => {
  it('defers both the grammar load and its first compile off the synchronous path', async () => {
    register('fakelang')
    const onReady = vi.fn()
    const { model } = fakeModel('file:///a.fake', ['Tfoo fnBar'])

    // 1. grammar not loaded yet
    expect(shikiTokensInRange(model, 'fakelang', 0, 0, onReady)).toBeNull()
    expect(onReady).not.toHaveBeenCalled()

    await flush()
    expect(onReady).toHaveBeenCalledOnce()
    expect(mocks.loadLanguage).toHaveBeenCalledWith('fakelang')

    // 2. loaded, but its rules have never met real code — that pass runs on a timer
    expect(shikiTokensInRange(model, 'fakelang', 0, 0, onReady)).toBeNull()
    await flush()
    expect(onReady).toHaveBeenCalledTimes(2)

    // 3. synchronous from here on
    const tokens = shikiTokensInRange(model, 'fakelang', 0, 0, onReady)
    expect(tokens?.map((t) => t.category)).toEqual(['type', 'function'])
  })

  it('maps a crowbar language id onto shiki’s bundle id', async () => {
    register('tsx')
    const { model } = fakeModel('file:///a.tsx', ['Tfoo'])

    await warm(model, 'typescriptreact')

    expect(mocks.loadLanguage).toHaveBeenCalledWith('tsx')
    expect(shikiTokensInRange(model, 'typescriptreact', 0, 0, () => {})).toHaveLength(1)
  })

  it('stays null for a language shiki has no grammar for, and never retries', async () => {
    const onReady = vi.fn()
    const { model } = fakeModel('file:///.gitignore', ['Tfoo'])

    expect(shikiTokensInRange(model, 'gitignore', 0, 0, onReady)).toBeNull()
    await flush()
    expect(onReady).not.toHaveBeenCalled()
    expect(shikiTokensInRange(model, 'gitignore', 0, 0, onReady)).toBeNull()
    expect(mocks.loadLanguage).not.toHaveBeenCalled()
  })

  it('resumes from the rule stack above the viewport instead of re-reading the file', async () => {
    register('fakelang')
    const lines = ['/*', 'Tinside', '*/', 'Tafter']
    const { model } = fakeModel('file:///block.fake', lines)
    await warm(model, 'fakelang')

    // Row 1 sits inside the block comment opened on row 0 — nothing is re-colored.
    expect(shikiTokensInRange(model, 'fakelang', 1, 1, () => {})).toEqual([])
    // Row 3 is past the close and gets its real category.
    expect(shikiTokensInRange(model, 'fakelang', 3, 3, () => {})?.[0]?.category).toBe('type')
  })

  it('drops only the rule stacks below an edit, keeping the ones above it', async () => {
    register('fakelang')
    const lines = ['Ta', 'Tb', '/*', 'Tc', '*/', 'Td']
    const { model, edit } = fakeModel('file:///edit.fake', lines)
    await warm(model, 'fakelang', 5)
    shikiTokensInRange(model, 'fakelang', 0, 5, () => {})

    // Delete the comment opener, then re-ask for the rows that were inside it.
    lines[2] = 'Tnow'
    edit(3)

    const tokens = shikiTokensInRange(model, 'fakelang', 2, 3, () => {})
    expect(tokens?.map((t) => t.category)).toEqual(['type', 'type'])
  })

  it('defers a jump past the synchronous budget to a background walk', async () => {
    register('fakelang')
    const lines = Array.from({ length: 2600 }, () => 'Tx')
    const { model } = fakeModel('file:///big.fake', lines)

    await warm(model, 'fakelang')

    vi.useFakeTimers()
    const onReady = vi.fn()
    // Row 2400 is more than MAX_ADVANCE_LINES past the last known rule stack.
    expect(shikiTokensInRange(model, 'fakelang', 2400, 2450, onReady)).toBeNull()
    expect(onReady).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(10)
    expect(onReady).toHaveBeenCalled()
    expect(shikiTokensInRange(model, 'fakelang', 2400, 2450, onReady)).toHaveLength(51)
  })
})
