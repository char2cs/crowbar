import { describe, expect, it } from 'vitest'
import {
  SEMANTIC_TOKEN_TYPES,
  SEMANTIC_TOKEN_LEGEND,
  captureToTypeIndex,
} from '@/features/editor/monaco/semantic-tokens-encode'

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

  it('returns -1 for ignored / text captures', () => {
    expect(captureToTypeIndex('none')).toBe(-1)
    expect(captureToTypeIndex('spell')).toBe(-1)
    expect(captureToTypeIndex('_private')).toBe(-1)
    expect(captureToTypeIndex('totally-unknown-capture')).toBe(-1) // -> token-text -> skipped
  })
})
