import { describe, it, expect } from 'vitest'
import { parseBlockInfo } from '@/features/markdown-chat/lib/parse-block-info'

describe('parseBlockInfo', () => {
  it('parses a bare language', () => {
    expect(parseBlockInfo('typescript')).toEqual({
      type: 'typescript',
      params: {},
      meta: '',
    })
  })

  it('parses widget-id params and keeps meta', () => {
    expect(parseBlockInfo('excalidraw widget-id:abc123')).toEqual({
      type: 'excalidraw',
      params: { 'widget-id': 'abc123' },
      meta: 'widget-id:abc123',
    })
  })

  it('trims surrounding whitespace', () => {
    expect(parseBlockInfo('  mermaid  ')).toEqual({
      type: 'mermaid',
      params: {},
      meta: '',
    })
  })

  it('returns an empty descriptor for a blank info string', () => {
    expect(parseBlockInfo('   ')).toEqual({ type: '', params: {}, meta: '' })
  })

  it('keeps non key:value tokens in meta but not in params', () => {
    expect(parseBlockInfo('ts {1,3-5}')).toEqual({
      type: 'ts',
      params: {},
      meta: '{1,3-5}',
    })
  })

  it('parses multiple key:value params', () => {
    expect(parseBlockInfo('chart widget-id:x kind:bar')).toEqual({
      type: 'chart',
      params: { 'widget-id': 'x', kind: 'bar' },
      meta: 'widget-id:x kind:bar',
    })
  })
})
