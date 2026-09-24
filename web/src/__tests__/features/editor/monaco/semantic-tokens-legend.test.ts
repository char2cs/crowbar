import { describe, expect, it } from 'vitest'
import {
  SEMANTIC_TOKEN_LEGEND,
  encodeShikiTokens,
} from '@/features/editor/monaco/semantic-tokens-legend'

describe('semantic token legend', () => {
  // The daemon rewrites every language server's tokens into this legend by
  // index (api/internal/engine/lsp/internal/semtok pins the same list): a
  // change here must land there in the same commit.
  it('pins the wire contract shared with the daemon', () => {
    expect(SEMANTIC_TOKEN_LEGEND.tokenTypes).toEqual([
      'namespace',
      'type',
      'class',
      'enum',
      'interface',
      'struct',
      'typeParameter',
      'parameter',
      'variable',
      'property',
      'enumMember',
      'event',
      'function',
      'method',
      'macro',
      'keyword',
      'modifier',
      'comment',
      'string',
      'number',
      'regexp',
      'operator',
      'decorator',
      'label',
      'tag',
      'attribute',
    ])
    expect(SEMANTIC_TOKEN_LEGEND.tokenModifiers).toEqual([
      'readonly',
      'declaration',
      'definition',
      'static',
      'deprecated',
      'abstract',
      'async',
      'modification',
      'documentation',
      'defaultLibrary',
    ])
  })
})

describe('encodeShikiTokens', () => {
  const idx = (t: string) => SEMANTIC_TOKEN_LEGEND.tokenTypes.indexOf(t)

  it('delta-encodes tokens relative to the previous one', () => {
    const data = encodeShikiTokens([
      { category: 'function', row: 0, start: 4, end: 12 },
      { category: 'function', row: 0, start: 20, end: 23 },
      { category: 'type', row: 2, start: 2, end: 5 },
    ])
    expect(Array.from(data)).toEqual([
      ...[0, 4, 8, idx('function'), 0],
      ...[0, 16, 3, idx('function'), 0],
      ...[2, 2, 3, idx('type'), 0], // a new line restarts the column
    ])
  })

  it('encodes a constant as a readonly variable', () => {
    const data = encodeShikiTokens([{ category: 'constant', row: 0, start: 0, end: 3 }])
    expect(Array.from(data)).toEqual([0, 0, 3, idx('variable'), 0b1])
  })
})
