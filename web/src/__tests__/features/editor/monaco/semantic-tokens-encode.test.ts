import { describe, expect, it } from 'vitest'
import {
  SEMANTIC_TOKEN_TYPES,
  SEMANTIC_TOKEN_LEGEND,
  captureToTypeIndex,
  encodeTokens,
} from '@/features/editor/monaco/semantic-tokens-encode'
import type { HighlightToken } from '@/features/editor/lib/wasm-parser/types'

function tok(
  type: string,
  sr: number,
  sc: number,
  er: number,
  ec: number,
): HighlightToken {
  return {
    type,
    startIndex: 0,
    endIndex: 0,
    startPosition: { row: sr, column: sc },
    endPosition: { row: er, column: ec },
  }
}

describe('semantic token legend', () => {
  it('legend types match the exported list with no modifiers', () => {
    expect(SEMANTIC_TOKEN_LEGEND.tokenTypes).toEqual([...SEMANTIC_TOKEN_TYPES])
    expect(SEMANTIC_TOKEN_LEGEND.tokenModifiers).toEqual([])
  })

  it('maps tree-sitter captures to the right legend index', () => {
    const idx = (t: string) => SEMANTIC_TOKEN_TYPES[captureToTypeIndex(t)]
    expect(idx('function.call')).toBe('function')
    expect(idx('function.method')).toBe('function')
    expect(idx('type.builtin')).toBe('type')
    expect(idx('variable.parameter')).toBe('variable')
    expect(idx('variable.member')).toBe('property')
    expect(idx('constant.numeric')).toBe('number')
    expect(idx('punctuation.bracket')).toBe('punctuation')
    expect(idx('keyword.return')).toBe('keyword')
  })

  it('accepts pre-mapped token-* class names stored by the tokenizer worker', () => {
    // The worker calls mapCaptureToClass before storing HighlightToken.type, so
    // captureToTypeIndex must handle 'token-function' directly (not re-map it).
    const idx = (t: string) => SEMANTIC_TOKEN_TYPES[captureToTypeIndex(t)]
    expect(idx('token-function')).toBe('function')
    expect(idx('token-type')).toBe('type')
    expect(idx('token-keyword')).toBe('keyword')
    expect(idx('token-variable')).toBe('variable')
    expect(idx('token-constant')).toBe('constant')
    expect(idx('token-comment')).toBe('comment')
    // 'token-text' is the fallback class for unstyled identifiers — should be skipped
    expect(captureToTypeIndex('token-text')).toBe(-1)
  })

  it('returns -1 for ignored / text captures', () => {
    expect(captureToTypeIndex('none')).toBe(-1)
    expect(captureToTypeIndex('spell')).toBe(-1)
    expect(captureToTypeIndex('_private')).toBe(-1)
    expect(captureToTypeIndex('totally-unknown-capture')).toBe(-1) // -> token-text -> skipped
  })
})

describe('encodeTokens', () => {
  const fIdx = SEMANTIC_TOKEN_TYPES.indexOf('function')
  const cIdx = SEMANTIC_TOKEN_TYPES.indexOf('comment')

  it('delta-encodes single-line tokens (5 ints each, relative)', () => {
    const data = encodeTokens(
      [tok('function.call', 0, 4, 0, 12), tok('function.call', 0, 20, 0, 23)],
      () => 100,
    )
    expect(Array.from(data)).toEqual([
      0, 4, 8, fIdx, 0, // first: line0 char4 len8
      0, 16, 3, fIdx, 0, // second: same line, deltaChar 20-4=16, len3
    ])
  })

  it('uses absolute char on a new line', () => {
    const data = encodeTokens(
      [tok('function.call', 0, 4, 0, 8), tok('function.call', 2, 2, 2, 5)],
      () => 100,
    )
    expect(Array.from(data)).toEqual([
      0, 4, 4, fIdx, 0,
      2, 2, 3, fIdx, 0, // deltaLine 2 -> deltaChar is absolute (2)
    ])
  })

  it('splits a multi-line token into per-line entries using line lengths', () => {
    // block comment from line 0 col 3 to line 2 col 4; line lengths: 0->10, 1->6, 2->20
    const lens = [10, 6, 20]
    const data = encodeTokens([tok('comment.block', 0, 3, 2, 4)], (row) => lens[row])
    expect(Array.from(data)).toEqual([
      0, 3, 7, cIdx, 0, // line0: col3..10 => len7
      1, 0, 6, cIdx, 0, // line1: full 6
      1, 0, 4, cIdx, 0, // line2: col0..4 => len4 (deltaLine 1)
    ])
  })

  it('skips ignored/text captures and zero-length segments', () => {
    const data = encodeTokens(
      [tok('none', 0, 0, 0, 5), tok('function.call', 1, 3, 1, 3)],
      () => 100,
    )
    expect(data.length).toBe(0)
  })

  it('deltas a following token off the last segment of a multi-line token', () => {
    const lens = [10, 6, 20]
    // block comment lines 0..2, then a function token on line 2 after col 4
    const data = encodeTokens(
      [tok('comment.block', 0, 3, 2, 4), tok('function.call', 2, 8, 2, 12)],
      (row) => lens[row],
    )
    // last comment segment emitted at (line2, char0); the function is line2 char8
    // => deltaLine 0, deltaChar 8-0=8, len 4
    expect(Array.from(data).slice(-5)).toEqual([0, 8, 4, fIdx, 0])
  })
})
