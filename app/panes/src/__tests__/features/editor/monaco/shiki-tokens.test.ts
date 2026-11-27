import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { StateStack } from 'shiki/textmate'
import type { ShikiGrammar } from '@/features/editor/lib/shiki-grammars'

const grammars = vi.hoisted(() => new Map<string, ShikiGrammar>())

vi.mock('@/features/editor/lib/shiki-grammars', () => ({
  grammarFor: (languageId: string) => grammars.get(languageId) ?? null,
}))

import type {
  shikiTokensInRange as shikiTokensInRangeFn,
  TokenizerModel,
} from '@/features/editor/monaco/shiki-tokens'

let shikiTokensInRange: typeof shikiTokensInRangeFn

interface FakeStack {
  inComment: boolean
}

function fakeGrammar(): ShikiGrammar {
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

beforeEach(async () => {
  vi.resetModules()
  ;({ shikiTokensInRange } = await import('@/features/editor/monaco/shiki-tokens'))
  grammars.clear()
})

afterEach(() => {
  vi.useRealTimers()
})

function register(languageId: string): void {
  grammars.set(languageId, fakeGrammar())
}

async function categoryOf(scopes: readonly string[], languageId = 'fakelang') {
  grammars.set(languageId, {
    tokenizeLine: () => ({
      tokens: [{ startIndex: 0, endIndex: 1, scopes: [...scopes] }],
      ruleStack: null as unknown as StateStack,
    }),
  })
  const { model } = fakeModel('file:///scope.fake', ['x'])
  const onReady = vi.fn()
  shikiTokensInRange(model, languageId, 0, 0, onReady)
  await vi.waitFor(() => expect(onReady).toHaveBeenCalled())
  return shikiTokensInRange(model, languageId, 0, 0, () => {})?.[0]?.category ?? null
}

describe('scope classification', () => {
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
  ])('classifies %j as %s', async (scopes, expected) => {
    expect(await categoryOf(scopes)).toBe(expected)
  })

  it.each([
    [['source.ts', 'meta.var.expr.ts', 'storage.type.ts']],
    [['source.go', 'storage.type.string.go']],
    [['source.sql', 'keyword.other.DML.sql']],
    [['source.sql', 'string.quoted.single.sql']],
    [['source.go', 'constant.numeric.decimal.go']],
    [['source.ts', 'punctuation.definition.block.ts']],
    [['source.hcl', 'keyword.operator.accessor.hcl']],
    [['source.python', 'variable.language.special.self.python']],
    [['source.python', 'meta.function-call.python', 'meta.function-call.arguments.python']],
    [['text.html.erb', 'meta.embedded.line.erb', 'source.ruby']],
    [['source.dockerfile']],
  ])('leaves %j to Monaco', async (scopes) => {
    expect(await categoryOf(scopes)).toBeNull()
  })

  it('never re-colors inside a comment, even when a doc grammar names a type', async () => {
    expect(
      await categoryOf([
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
  ])('also classifies %j as %s where Monaco has no grammar', async (scopes, expected) => {
    expect(await categoryOf(scopes, 'plaintext')).toBe(expected)
  })

  it('takes the innermost scope over its container', async () => {
    expect(
      await categoryOf([
        'source.python',
        'meta.function-call.generic.python',
        'punctuation.definition.arguments.begin.python',
      ]),
    ).toBeNull()
  })
})

async function warm(model: TokenizerModel, languageId: string, endRow = 0): Promise<void> {
  const onReady = vi.fn()
  shikiTokensInRange(model, languageId, 0, endRow, onReady)
  await vi.waitFor(() => expect(onReady).toHaveBeenCalled())
}

describe('shikiTokensInRange', () => {
  it('stays null until the grammar is ready, then defers its first compile', async () => {
    const onReady = vi.fn()
    const { model } = fakeModel('file:///a.fake', ['Tfoo fnBar'])

    expect(shikiTokensInRange(model, 'fakelang', 0, 0, onReady)).toBeNull()

    register('fakelang')
    expect(shikiTokensInRange(model, 'fakelang', 0, 0, onReady)).toBeNull()
    expect(onReady).not.toHaveBeenCalled()
    await vi.waitFor(() => expect(onReady).toHaveBeenCalledOnce())

    const tokens = shikiTokensInRange(model, 'fakelang', 0, 0, onReady)
    expect(tokens?.map((t) => t.category)).toEqual(['type', 'function'])
  })

  it('emits categories and trims the whitespace grammars pad tokens with', async () => {
    register('fakelang')
    const { model } = fakeModel('file:///pad.fake', ['  fnRun  Tfoo  Kif  x'])
    await warm(model, 'fakelang')

    expect(shikiTokensInRange(model, 'fakelang', 0, 0, () => {})).toEqual([
      { category: 'function', row: 0, start: 2, end: 7 },
      { category: 'type', row: 0, start: 9, end: 13 },
      { category: 'variable', row: 0, start: 20, end: 21 },
    ])
  })

  it('resumes from the rule stack above the viewport instead of re-reading the file', async () => {
    register('fakelang')
    const lines = ['/*', 'Tinside', '*/', 'Tafter']
    const { model } = fakeModel('file:///block.fake', lines)
    await warm(model, 'fakelang')

    expect(shikiTokensInRange(model, 'fakelang', 1, 1, () => {})).toEqual([])
    expect(shikiTokensInRange(model, 'fakelang', 3, 3, () => {})?.[0]?.category).toBe('type')
  })

  it('drops only the rule stacks below an edit, keeping the ones above it', async () => {
    register('fakelang')
    const lines = ['Ta', 'Tb', '/*', 'Tc', '*/', 'Td']
    const { model, edit } = fakeModel('file:///edit.fake', lines)
    await warm(model, 'fakelang', 5)
    shikiTokensInRange(model, 'fakelang', 0, 5, () => {})

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
    expect(shikiTokensInRange(model, 'fakelang', 2400, 2450, onReady)).toBeNull()
    expect(onReady).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(10)
    expect(onReady).toHaveBeenCalled()
    expect(shikiTokensInRange(model, 'fakelang', 2400, 2450, onReady)).toHaveLength(51)
  })
})
