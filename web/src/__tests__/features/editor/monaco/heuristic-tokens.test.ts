import { describe, expect, it } from 'vitest'
import {
  heuristicTokensFromLineTokens,
  type LineToken,
} from '@/features/editor/monaco/heuristic-tokens'

/** Tokenize a single line as "all plain code" (one default-scope token). */
function codeLine(): LineToken[] {
  return [{ offset: 0, type: '' }]
}

function run(text: string, lineTokens: LineToken[][]) {
  return heuristicTokensFromLineTokens(text, lineTokens).map((t) => ({
    type: t.type,
    word: text.split('\n')[t.startPosition.row].slice(t.startPosition.column, t.endPosition.column),
    row: t.startPosition.row,
  }))
}

describe('heuristicTokensFromLineTokens', () => {
  it('colors identifiers before "(" as functions', () => {
    const out = run('newRootCmd()\nSprintf(x)', [codeLine(), codeLine()])
    expect(out).toContainEqual({ type: 'function', word: 'newRootCmd', row: 0 })
    expect(out).toContainEqual({ type: 'function', word: 'Sprintf', row: 1 })
  })

  it('colors capitalized identifiers as types and ALL_CAPS as constants', () => {
    const out = run('var x cobra.Command', [codeLine()])
    expect(out).toContainEqual({ type: 'type', word: 'Command', row: 0 })
    const out2 = run('const MAX_RETRIES = 3', [codeLine()])
    expect(out2).toContainEqual({ type: 'constant', word: 'MAX_RETRIES', row: 0 })
  })

  it('leaves lowercase non-call identifiers uncolored', () => {
    const out = run('cmd := root', [codeLine()])
    expect(out).toEqual([])
  })

  it('skips identifiers inside Monaco-classified comment/string scopes', () => {
    // line: realCall()  // Command Foo()
    // Monaco tokens: code from col 0, comment scope starting at col 12.
    const line = 'realCall()  // Command Foo()'
    const tokens: LineToken[][] = [
      [
        { offset: 0, type: 'source' },
        { offset: 12, type: 'comment.go' },
      ],
    ]
    const out = run(line, tokens)
    expect(out).toEqual([{ type: 'function', word: 'realCall', row: 0 }])
  })

  it('skips a whole string-scoped line', () => {
    const line = '"Sprintf(NotAType)"'
    const tokens: LineToken[][] = [[{ offset: 0, type: 'string.go' }]]
    expect(run(line, tokens)).toEqual([])
  })

  it('does not re-color keyword-scoped identifiers', () => {
    // "Func" classified as keyword by some grammar must not become a type.
    const line = 'Func Bar'
    const tokens: LineToken[][] = [
      [
        { offset: 0, type: 'keyword.x' },
        { offset: 5, type: '' },
      ],
    ]
    expect(run(line, tokens)).toEqual([{ type: 'type', word: 'Bar', row: 0 }])
  })

  it('respects the emit row window', () => {
    const text = 'Foo()\nBar()\nBaz()'
    const out = heuristicTokensFromLineTokens(
      text,
      [codeLine(), codeLine(), codeLine()],
      { emitStartRow: 1, emitEndRow: 1 },
    )
    expect(out.map((t) => text.split('\n')[t.startPosition.row].slice(t.startPosition.column, t.endPosition.column))).toEqual(['Bar'])
  })
})
